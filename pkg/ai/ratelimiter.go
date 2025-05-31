package ai

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"
)

// RateLimiter implements rate limiting for AI API calls
type RateLimiter struct {
	requestsPerMinute int
	burstSize         int
	backoffStrategy   string
	maxBackoff        time.Duration

	tokens    chan struct{}
	lastReset time.Time
	mutex     sync.Mutex

	// Statistics
	totalRequests   int64
	blockedRequests int64
	totalWaitTime   time.Duration
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(requestsPerMinute, burstSize int, backoffStrategy string, maxBackoff time.Duration) *RateLimiter {
	rl := &RateLimiter{
		requestsPerMinute: requestsPerMinute,
		burstSize:         burstSize,
		backoffStrategy:   backoffStrategy,
		maxBackoff:        maxBackoff,
		tokens:            make(chan struct{}, burstSize),
		lastReset:         time.Now(),
	}

	// Fill the initial token bucket
	for i := 0; i < burstSize; i++ {
		rl.tokens <- struct{}{}
	}

	// Start the token refill routine
	go rl.refillTokens()

	return rl
}

// Wait waits for permission to make a request
func (rl *RateLimiter) Wait(ctx context.Context) error {
	start := time.Now()
	defer func() {
		rl.mutex.Lock()
		rl.totalRequests++
		rl.totalWaitTime += time.Since(start)
		rl.mutex.Unlock()
	}()

	select {
	case <-rl.tokens:
		return nil
	case <-ctx.Done():
		rl.mutex.Lock()
		rl.blockedRequests++
		rl.mutex.Unlock()
		return ctx.Err()
	}
}

// TryAcquire tries to acquire a token without blocking
func (rl *RateLimiter) TryAcquire() bool {
	select {
	case <-rl.tokens:
		rl.mutex.Lock()
		rl.totalRequests++
		rl.mutex.Unlock()
		return true
	default:
		rl.mutex.Lock()
		rl.blockedRequests++
		rl.mutex.Unlock()
		return false
	}
}

// refillTokens refills the token bucket at the specified rate
func (rl *RateLimiter) refillTokens() {
	// Calculate the interval between token additions
	interval := time.Minute / time.Duration(rl.requestsPerMinute)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for range ticker.C {
		select {
		case rl.tokens <- struct{}{}:
			// Token added successfully
		default:
			// Bucket is full, skip
		}
	}
}

// GetStats returns rate limiter statistics
func (rl *RateLimiter) GetStats() map[string]interface{} {
	rl.mutex.Lock()
	defer rl.mutex.Unlock()

	availableTokens := len(rl.tokens)

	return map[string]interface{}{
		"requests_per_minute": rl.requestsPerMinute,
		"burst_size":          rl.burstSize,
		"available_tokens":    availableTokens,
		"total_requests":      rl.totalRequests,
		"blocked_requests":    rl.blockedRequests,
		"total_wait_time":     rl.totalWaitTime.String(),
		"average_wait_time":   rl.getAverageWaitTime().String(),
		"last_reset":          rl.lastReset.Format(time.RFC3339),
	}
}

// getAverageWaitTime calculates the average wait time
func (rl *RateLimiter) getAverageWaitTime() time.Duration {
	if rl.totalRequests == 0 {
		return 0
	}
	return rl.totalWaitTime / time.Duration(rl.totalRequests)
}

// Reset resets the rate limiter statistics
func (rl *RateLimiter) Reset() {
	rl.mutex.Lock()
	defer rl.mutex.Unlock()

	rl.totalRequests = 0
	rl.blockedRequests = 0
	rl.totalWaitTime = 0
	rl.lastReset = time.Now()
}

// UpdateLimits updates the rate limiting parameters
func (rl *RateLimiter) UpdateLimits(requestsPerMinute, burstSize int) {
	rl.mutex.Lock()
	defer rl.mutex.Unlock()

	rl.requestsPerMinute = requestsPerMinute

	// If burst size changed, create a new token bucket
	if rl.burstSize != burstSize {
		rl.burstSize = burstSize

		// Create new token channel
		newTokens := make(chan struct{}, burstSize)

		// Transfer existing tokens (up to new burst size)
		existingTokens := len(rl.tokens)
		tokensToTransfer := existingTokens
		if tokensToTransfer > burstSize {
			tokensToTransfer = burstSize
		}

		// Drain old channel and fill new one
		for i := 0; i < existingTokens; i++ {
			<-rl.tokens
		}

		for i := 0; i < tokensToTransfer; i++ {
			newTokens <- struct{}{}
		}

		rl.tokens = newTokens
	}
}

// Close closes the rate limiter and stops the refill routine
func (rl *RateLimiter) Close() {
	// The refill routine will stop when the rate limiter is garbage collected
	// since the ticker will be stopped by the defer statement
}

// CircuitBreaker implements a circuit breaker pattern for AI providers
type CircuitBreaker struct {
	maxFailures     int
	resetTimeout    time.Duration
	state           CircuitState
	failures        int
	lastFailureTime time.Time
	mutex           sync.RWMutex
}

// CircuitState represents the state of the circuit breaker
type CircuitState int

const (
	CircuitClosed CircuitState = iota
	CircuitOpen
	CircuitHalfOpen
)

// String returns the string representation of the circuit state
func (s CircuitState) String() string {
	switch s {
	case CircuitClosed:
		return "closed"
	case CircuitOpen:
		return "open"
	case CircuitHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// NewCircuitBreaker creates a new circuit breaker
func NewCircuitBreaker(maxFailures int, resetTimeout time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		maxFailures:  maxFailures,
		resetTimeout: resetTimeout,
		state:        CircuitClosed,
	}
}

// Call executes a function with circuit breaker protection
func (cb *CircuitBreaker) Call(fn func() error) error {
	if !cb.allowRequest() {
		return ErrCircuitOpen
	}

	err := fn()
	cb.recordResult(err)
	return err
}

// allowRequest checks if a request should be allowed
func (cb *CircuitBreaker) allowRequest() bool {
	cb.mutex.RLock()
	defer cb.mutex.RUnlock()

	switch cb.state {
	case CircuitClosed:
		return true
	case CircuitOpen:
		return time.Since(cb.lastFailureTime) >= cb.resetTimeout
	case CircuitHalfOpen:
		return true
	default:
		return false
	}
}

// recordResult records the result of a request
func (cb *CircuitBreaker) recordResult(err error) {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	if err != nil {
		cb.failures++
		cb.lastFailureTime = time.Now()

		if cb.failures >= cb.maxFailures {
			cb.state = CircuitOpen
		}
	} else {
		// Success
		cb.failures = 0
		if cb.state == CircuitHalfOpen {
			cb.state = CircuitClosed
		}
	}

	// Transition from open to half-open if timeout has passed
	if cb.state == CircuitOpen && time.Since(cb.lastFailureTime) >= cb.resetTimeout {
		cb.state = CircuitHalfOpen
		cb.failures = 0
	}
}

// GetState returns the current state of the circuit breaker
func (cb *CircuitBreaker) GetState() CircuitState {
	cb.mutex.RLock()
	defer cb.mutex.RUnlock()
	return cb.state
}

// GetStats returns circuit breaker statistics
func (cb *CircuitBreaker) GetStats() map[string]interface{} {
	cb.mutex.RLock()
	defer cb.mutex.RUnlock()

	return map[string]interface{}{
		"state":             cb.state.String(),
		"failures":          cb.failures,
		"max_failures":      cb.maxFailures,
		"last_failure_time": cb.lastFailureTime.Format(time.RFC3339),
		"reset_timeout":     cb.resetTimeout.String(),
	}
}

// Reset resets the circuit breaker to closed state
func (cb *CircuitBreaker) Reset() {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	cb.state = CircuitClosed
	cb.failures = 0
	cb.lastFailureTime = time.Time{}
}

// Custom errors
var (
	ErrCircuitOpen = fmt.Errorf("circuit breaker is open")
)

// BackoffStrategy implements different backoff strategies
type BackoffStrategy interface {
	NextDelay(attempt int, baseDelay time.Duration) time.Duration
}

// ExponentialBackoff implements exponential backoff
type ExponentialBackoff struct {
	MaxDelay time.Duration
}

// NextDelay calculates the next delay using exponential backoff
func (eb *ExponentialBackoff) NextDelay(attempt int, baseDelay time.Duration) time.Duration {
	delay := time.Duration(float64(baseDelay) * math.Pow(2, float64(attempt)))
	if delay > eb.MaxDelay {
		delay = eb.MaxDelay
	}
	return delay
}

// LinearBackoff implements linear backoff
type LinearBackoff struct {
	MaxDelay time.Duration
}

// NextDelay calculates the next delay using linear backoff
func (lb *LinearBackoff) NextDelay(attempt int, baseDelay time.Duration) time.Duration {
	delay := baseDelay * time.Duration(attempt+1)
	if delay > lb.MaxDelay {
		delay = lb.MaxDelay
	}
	return delay
}
