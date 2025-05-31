package ai

import (
	"context"
	"fmt"
	"time"

	"github.com/varuntirumala1/nbar-qos-classifier/internal/logger"
	"github.com/varuntirumala1/nbar-qos-classifier/pkg/config"
	"github.com/varuntirumala1/nbar-qos-classifier/pkg/qos"
)

// Provider represents an AI provider interface
type Provider interface {
	Name() string
	ClassifyProtocols(ctx context.Context, protocols []string) (map[string]qos.Classification, error)
	IsAvailable() bool
	GetRateLimit() *RateLimit
}

// RateLimit represents rate limiting configuration
type RateLimit struct {
	RequestsPerMinute int
	BurstSize         int
	BackoffStrategy   string
	MaxBackoff        time.Duration
}

// Manager manages multiple AI providers with fallback support
type Manager struct {
	providers       []Provider
	primaryProvider Provider
	config          *config.AIConfig
	logger          *logger.Logger
	rateLimiter     *RateLimiter
}

// NewManager creates a new AI provider manager
func NewManager(cfg *config.AIConfig, logger *logger.Logger) (*Manager, error) {
	manager := &Manager{
		config:    cfg,
		logger:    logger,
		providers: make([]Provider, 0),
	}

	// Initialize rate limiter
	manager.rateLimiter = NewRateLimiter(
		cfg.RateLimit.RequestsPerMinute,
		cfg.RateLimit.BurstSize,
		cfg.RateLimit.BackoffStrategy,
		cfg.RateLimit.MaxBackoff,
	)

	// Initialize providers based on configuration
	if err := manager.initializeProviders(); err != nil {
		return nil, fmt.Errorf("failed to initialize providers: %w", err)
	}

	return manager, nil
}

// initializeProviders initializes AI providers based on configuration
func (m *Manager) initializeProviders() error {
	// Initialize primary provider
	primary, err := m.createProvider(m.config.Provider, m.config)
	if err != nil {
		return fmt.Errorf("failed to create primary provider %s: %w", m.config.Provider, err)
	}
	m.primaryProvider = primary
	m.providers = append(m.providers, primary)

	// Initialize fallback providers
	for _, fallback := range m.config.Fallback {
		if !fallback.Enabled {
			continue
		}

		providerConfig, exists := m.config.Providers[fallback.Provider]
		if !exists {
			m.logger.WithField("provider", fallback.Provider).Warn("Fallback provider configuration not found")
			continue
		}

		// Create a temporary config for the fallback provider
		fallbackConfig := &config.AIConfig{
			Provider:    fallback.Provider,
			APIKey:      providerConfig.APIKey,
			Model:       providerConfig.Model,
			Temperature: providerConfig.Temperature,
			MaxTokens:   providerConfig.MaxTokens,
			Timeout:     m.config.Timeout,
		}

		provider, err := m.createProvider(fallback.Provider, fallbackConfig)
		if err != nil {
			m.logger.WithError(err).WithField("provider", fallback.Provider).Warn("Failed to create fallback provider")
			continue
		}

		m.providers = append(m.providers, provider)
	}

	if len(m.providers) == 0 {
		return fmt.Errorf("no AI providers available")
	}

	m.logger.WithField("provider_count", len(m.providers)).Info("Initialized AI providers")
	return nil
}

// createProvider creates a specific AI provider
func (m *Manager) createProvider(providerName string, cfg *config.AIConfig) (Provider, error) {
	switch providerName {
	case "deepseek":
		return NewDeepSeekProvider(cfg, m.logger)
	case "openai":
		return NewOpenAIProvider(cfg, m.logger)
	case "claude":
		return NewClaudeProvider(cfg, m.logger)
	case "ollama":
		return NewOllamaProvider(cfg, m.logger)
	default:
		return nil, fmt.Errorf("unsupported AI provider: %s", providerName)
	}
}

// ClassifyProtocols classifies protocols using the available providers
func (m *Manager) ClassifyProtocols(ctx context.Context, protocols []string, batchSize int) (map[string]qos.Classification, error) {
	if len(protocols) == 0 {
		return make(map[string]qos.Classification), nil
	}

	m.logger.WithFields(logger.Fields{
		"protocol_count": len(protocols),
		"batch_size":     batchSize,
	}).Info("Starting AI classification")

	// Process protocols in batches
	results := make(map[string]qos.Classification)

	for i := 0; i < len(protocols); i += batchSize {
		end := i + batchSize
		if end > len(protocols) {
			end = len(protocols)
		}
		batch := protocols[i:end]

		// Wait for rate limit
		if err := m.rateLimiter.Wait(ctx); err != nil {
			return nil, fmt.Errorf("rate limit error: %w", err)
		}

		// Try each provider until one succeeds
		batchResults, err := m.classifyBatch(ctx, batch)
		if err != nil {
			return nil, fmt.Errorf("failed to classify batch: %w", err)
		}

		// Merge results
		for protocol, classification := range batchResults {
			results[protocol] = classification
		}
	}

	m.logger.WithField("classified_count", len(results)).Info("AI classification completed")
	return results, nil
}

// classifyBatch classifies a batch of protocols using available providers
func (m *Manager) classifyBatch(ctx context.Context, protocols []string) (map[string]qos.Classification, error) {
	var lastErr error

	for i, provider := range m.providers {
		if !provider.IsAvailable() {
			m.logger.WithField("provider", provider.Name()).Debug("Provider not available, skipping")
			continue
		}

		start := time.Now()
		results, err := provider.ClassifyProtocols(ctx, protocols)
		duration := time.Since(start)

		m.logger.APICall(
			provider.Name(),
			"", // Model will be logged by the provider
			duration,
			err == nil,
			logger.Fields{
				"protocol_count": len(protocols),
				"is_fallback":    i > 0,
			},
		)

		if err == nil {
			// Success! Add source information to results
			for protocol, classification := range results {
				classification.Source = "ai"
				classification.Timestamp = time.Now().Unix()
				results[protocol] = classification
			}
			return results, nil
		}

		lastErr = err
		m.logger.WithError(err).WithField("provider", provider.Name()).Warn("Provider failed, trying next")
	}

	return nil, fmt.Errorf("all AI providers failed, last error: %w", lastErr)
}

// GetPrimaryProvider returns the primary AI provider
func (m *Manager) GetPrimaryProvider() Provider {
	return m.primaryProvider
}

// GetProviders returns all available providers
func (m *Manager) GetProviders() []Provider {
	return m.providers
}

// GetStats returns statistics about AI provider usage
func (m *Manager) GetStats() map[string]interface{} {
	stats := make(map[string]interface{})

	stats["rate_limiter"] = m.rateLimiter.GetStats()
	stats["provider_count"] = len(m.providers)
	stats["primary_provider"] = m.primaryProvider.Name()

	providerStats := make(map[string]interface{})
	for _, provider := range m.providers {
		providerStats[provider.Name()] = map[string]interface{}{
			"available": provider.IsAvailable(),
		}
	}
	stats["providers"] = providerStats

	return stats
}

// Close closes all providers and cleans up resources
func (m *Manager) Close() error {
	// Close rate limiter
	if m.rateLimiter != nil {
		m.rateLimiter.Close()
	}

	// Close providers if they implement io.Closer
	for _, provider := range m.providers {
		if closer, ok := provider.(interface{ Close() error }); ok {
			if err := closer.Close(); err != nil {
				m.logger.WithError(err).WithField("provider", provider.Name()).Warn("Failed to close provider")
			}
		}
	}

	return nil
}

// Response represents a generic AI response
type Response struct {
	Content  string                 `json:"content"`
	Model    string                 `json:"model"`
	Usage    Usage                  `json:"usage"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// Usage represents token usage information
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// Request represents a generic AI request
type Request struct {
	Model       string                 `json:"model"`
	Messages    []Message              `json:"messages"`
	Temperature float64                `json:"temperature"`
	MaxTokens   int                    `json:"max_tokens"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// Message represents a chat message
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// BuildClassificationPrompt builds a prompt for protocol classification
func BuildClassificationPrompt(protocols []string) string {
	protocolsText := ""
	for i, protocol := range protocols {
		protocolsText += fmt.Sprintf("%d. %s\n", i+1, protocol)
	}

	return fmt.Sprintf(`Classify these network protocols into QoS classes for a Cisco 9300 switch:

%s

QoS Classes:
- EF: Real-time (voice/video calls)
- AF41: Business-critical (interactive apps)
- AF21: Important (email, transfers)
- CS1: Background (updates, browsing)

Respond ONLY with JSON array:
[{"protocol":"name","class":"CLASS"}]`, protocolsText)
}

// ParseClassificationResponse parses the AI response into classifications
func ParseClassificationResponse(content string) ([]qos.Classification, error) {
	// This will be implemented to parse various response formats
	// For now, return empty slice
	return []qos.Classification{}, nil
}
