package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/varuntirumala1/nbar-qos-classifier/internal/logger"
	"github.com/varuntirumala1/nbar-qos-classifier/pkg/config"
	"github.com/varuntirumala1/nbar-qos-classifier/pkg/qos"
)

// DeepSeekProvider implements the Provider interface for DeepSeek AI
type DeepSeekProvider struct {
	config     *config.AIConfig
	logger     *logger.Logger
	httpClient *http.Client
	apiKey     string
	baseURL    string
	model      string
	rateLimit  *RateLimit
}

// DeepSeekResponse represents the DeepSeek API response structure
type DeepSeekResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Index   int `json:"index"`
		Message struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

// DeepSeekRequest represents the DeepSeek API request structure
type DeepSeekRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Temperature float64   `json:"temperature"`
	MaxTokens   int       `json:"max_tokens"`
	Stream      bool      `json:"stream"`
}

// NewDeepSeekProvider creates a new DeepSeek provider
func NewDeepSeekProvider(cfg *config.AIConfig, logger *logger.Logger) (*DeepSeekProvider, error) {
	provider := &DeepSeekProvider{
		config: cfg,
		logger: logger,
		httpClient: &http.Client{
			Timeout: cfg.Timeout,
		},
		apiKey:  cfg.APIKey,
		baseURL: "https://api.deepseek.com/v1",
		model:   cfg.Model,
		rateLimit: &RateLimit{
			RequestsPerMinute: cfg.RateLimit.RequestsPerMinute,
			BurstSize:         cfg.RateLimit.BurstSize,
			BackoffStrategy:   cfg.RateLimit.BackoffStrategy,
			MaxBackoff:        cfg.RateLimit.MaxBackoff,
		},
	}

	// Override base URL if specified in provider config
	if providerCfg, exists := cfg.Providers["deepseek"]; exists && providerCfg.BaseURL != "" {
		provider.baseURL = providerCfg.BaseURL
	}

	return provider, nil
}

// Name returns the provider name
func (p *DeepSeekProvider) Name() string {
	return "deepseek"
}

// IsAvailable checks if the provider is available
func (p *DeepSeekProvider) IsAvailable() bool {
	return p.apiKey != ""
}

// GetRateLimit returns the rate limit configuration
func (p *DeepSeekProvider) GetRateLimit() *RateLimit {
	return p.rateLimit
}

// ClassifyProtocols classifies protocols using DeepSeek AI
func (p *DeepSeekProvider) ClassifyProtocols(ctx context.Context, protocols []string) (map[string]qos.Classification, error) {
	if len(protocols) == 0 {
		return make(map[string]qos.Classification), nil
	}

	p.logger.WithFields(logger.Fields{
		"provider":       p.Name(),
		"protocol_count": len(protocols),
		"model":          p.model,
	}).Debug("Starting DeepSeek classification")

	// Build the prompt
	prompt := BuildClassificationPrompt(protocols)

	// Create the request
	request := DeepSeekRequest{
		Model: p.model,
		Messages: []Message{
			{
				Role:    "system",
				Content: "You are an expert in network protocols and QoS classification.",
			},
			{
				Role:    "user",
				Content: prompt,
			},
		},
		Temperature: p.config.Temperature,
		MaxTokens:   p.config.MaxTokens,
		Stream:      false,
	}

	// Make the API call with retries
	response, err := p.makeAPICall(ctx, request)
	if err != nil {
		return nil, fmt.Errorf("API call failed: %w", err)
	}

	// Parse the response
	classifications, err := p.parseResponse(response, protocols)
	if err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	p.logger.WithFields(logger.Fields{
		"provider":         p.Name(),
		"classified_count": len(classifications),
		"tokens_used":      response.Usage.TotalTokens,
	}).Info("DeepSeek classification completed")

	return classifications, nil
}

// makeAPICall makes an API call to DeepSeek with retries
func (p *DeepSeekProvider) makeAPICall(ctx context.Context, request DeepSeekRequest) (*DeepSeekResponse, error) {
	maxRetries := 3
	baseDelay := 2 * time.Second

	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff
			delay := time.Duration(float64(baseDelay) * math.Pow(2, float64(attempt-1)))
			if delay > p.rateLimit.MaxBackoff {
				delay = p.rateLimit.MaxBackoff
			}

			p.logger.WithFields(logger.Fields{
				"attempt": attempt + 1,
				"delay":   delay,
			}).Debug("Retrying DeepSeek API call")

			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
		}

		response, err := p.singleAPICall(ctx, request)
		if err == nil {
			return response, nil
		}

		lastErr = err
		p.logger.WithError(err).WithField("attempt", attempt+1).Warn("DeepSeek API call failed")
	}

	return nil, fmt.Errorf("all retry attempts failed, last error: %w", lastErr)
}

// singleAPICall makes a single API call to DeepSeek
func (p *DeepSeekProvider) singleAPICall(ctx context.Context, request DeepSeekRequest) (*DeepSeekResponse, error) {
	// Marshal request
	jsonData, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	url := fmt.Sprintf("%s/chat/completions", p.baseURL)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)

	// Make the request
	start := time.Now()
	resp, err := p.httpClient.Do(httpReq)
	duration := time.Since(start)

	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Log the API call
	p.logger.WithFields(logger.Fields{
		"status_code": resp.StatusCode,
		"duration":    duration,
		"body_size":   len(body),
	}).Debug("DeepSeek API call completed")

	// Check for HTTP errors
	if resp.StatusCode != http.StatusOK {
		var errorResp struct {
			Error struct {
				Message string `json:"message"`
				Type    string `json:"type"`
				Code    string `json:"code"`
			} `json:"error"`
		}

		if err := json.Unmarshal(body, &errorResp); err == nil && errorResp.Error.Message != "" {
			return nil, fmt.Errorf("API error: %s (type: %s, code: %s)",
				errorResp.Error.Message, errorResp.Error.Type, errorResp.Error.Code)
		}

		return nil, fmt.Errorf("API error: status code %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var response DeepSeekResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("failed to parse response JSON: %w", err)
	}

	if len(response.Choices) == 0 {
		return nil, fmt.Errorf("no choices in response")
	}

	return &response, nil
}

// parseResponse parses the DeepSeek response into classifications
func (p *DeepSeekProvider) parseResponse(response *DeepSeekResponse, protocols []string) (map[string]qos.Classification, error) {
	if len(response.Choices) == 0 {
		return nil, fmt.Errorf("no choices in response")
	}

	content := response.Choices[0].Message.Content

	p.logger.WithField("response_content", content).Debug("Parsing DeepSeek response")

	// Try to parse as JSON first
	classifications, err := p.parseJSONResponse(content)
	if err == nil {
		return p.mapToClassifications(classifications), nil
	}

	p.logger.WithError(err).Debug("JSON parsing failed, trying regex extraction")

	// Fallback to regex extraction
	classifications, err = p.parseWithRegex(content)
	if err == nil {
		return p.mapToClassifications(classifications), nil
	}

	p.logger.WithError(err).Debug("Regex parsing failed, trying manual extraction")

	// Final fallback: manual extraction
	return p.parseManually(content, protocols), nil
}

// parseJSONResponse parses JSON response
func (p *DeepSeekProvider) parseJSONResponse(content string) ([]qos.Classification, error) {
	// Try to extract JSON from the content
	jsonRegex := regexp.MustCompile(`\[.*\]`)
	jsonMatch := jsonRegex.FindString(content)
	if jsonMatch == "" {
		return nil, fmt.Errorf("no JSON array found in response")
	}

	var classifications []qos.Classification
	if err := json.Unmarshal([]byte(jsonMatch), &classifications); err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON: %w", err)
	}

	return classifications, nil
}

// parseWithRegex parses response using regex patterns
func (p *DeepSeekProvider) parseWithRegex(content string) ([]qos.Classification, error) {
	// Look for math display blocks (sometimes AI wraps JSON in these)
	mathRegex := regexp.MustCompile(`\$begin:math:display\$(\s*\{.*?\}\s*,?)+\$end:math:display\$`)
	mathMatch := mathRegex.FindString(content)
	if mathMatch != "" {
		// Extract the JSON part
		jsonPart := strings.TrimPrefix(mathMatch, "$begin:math:display$")
		jsonPart = strings.TrimSuffix(jsonPart, "$end:math:display$")
		jsonPart = "[" + jsonPart + "]"

		var classifications []qos.Classification
		if err := json.Unmarshal([]byte(jsonPart), &classifications); err == nil {
			return classifications, nil
		}
	}

	return nil, fmt.Errorf("regex parsing failed")
}

// parseManually manually extracts classifications from text
func (p *DeepSeekProvider) parseManually(content string, protocols []string) map[string]qos.Classification {
	results := make(map[string]qos.Classification)
	lines := strings.Split(content, "\n")

	for _, line := range lines {
		for _, protocol := range protocols {
			if strings.Contains(strings.ToLower(line), strings.ToLower(protocol)) {
				class := qos.CS1 // Default
				upperLine := strings.ToUpper(line)

				if strings.Contains(upperLine, "EF") {
					class = qos.EF
				} else if strings.Contains(upperLine, "AF41") {
					class = qos.AF41
				} else if strings.Contains(upperLine, "AF21") {
					class = qos.AF21
				}

				results[protocol] = qos.Classification{
					Protocol:   protocol,
					Class:      class,
					Confidence: 0.7, // Medium confidence for manual extraction
					Source:     "ai",
				}
				break
			}
		}
	}

	return results
}

// mapToClassifications converts parsed classifications to map
func (p *DeepSeekProvider) mapToClassifications(classifications []qos.Classification) map[string]qos.Classification {
	results := make(map[string]qos.Classification)

	for _, classification := range classifications {
		// Validate the classification
		if !classification.Class.IsValid() {
			p.logger.WithFields(logger.Fields{
				"protocol": classification.Protocol,
				"class":    classification.Class,
			}).Warn("Invalid QoS class from AI, using default")
			classification.Class = qos.CS1
		}

		// Set confidence and source
		if classification.Confidence == 0 {
			classification.Confidence = 0.8 // High confidence for JSON responses
		}
		classification.Source = "ai"

		results[classification.Protocol] = classification
	}

	return results
}
