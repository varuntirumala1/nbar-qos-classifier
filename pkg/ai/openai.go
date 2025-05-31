package ai

import (
	"context"
	"fmt"

	"github.com/varuntirumala1/nbar-qos-classifier/internal/logger"
	"github.com/varuntirumala1/nbar-qos-classifier/pkg/config"
	"github.com/varuntirumala1/nbar-qos-classifier/pkg/qos"
)

// OpenAIProvider implements the Provider interface for OpenAI
type OpenAIProvider struct {
	config    *config.AIConfig
	logger    *logger.Logger
	apiKey    string
	model     string
	rateLimit *RateLimit
}

// NewOpenAIProvider creates a new OpenAI provider
func NewOpenAIProvider(cfg *config.AIConfig, logger *logger.Logger) (*OpenAIProvider, error) {
	provider := &OpenAIProvider{
		config: cfg,
		logger: logger,
		apiKey: cfg.APIKey,
		model:  cfg.Model,
		rateLimit: &RateLimit{
			RequestsPerMinute: cfg.RateLimit.RequestsPerMinute,
			BurstSize:         cfg.RateLimit.BurstSize,
			BackoffStrategy:   cfg.RateLimit.BackoffStrategy,
			MaxBackoff:        cfg.RateLimit.MaxBackoff,
		},
	}

	// Override with provider-specific config if available
	if providerCfg, exists := cfg.Providers["openai"]; exists {
		if providerCfg.APIKey != "" {
			provider.apiKey = providerCfg.APIKey
		}
		if providerCfg.Model != "" {
			provider.model = providerCfg.Model
		}
	}

	return provider, nil
}

// Name returns the provider name
func (p *OpenAIProvider) Name() string {
	return "openai"
}

// IsAvailable checks if the provider is available
func (p *OpenAIProvider) IsAvailable() bool {
	return p.apiKey != ""
}

// GetRateLimit returns the rate limit configuration
func (p *OpenAIProvider) GetRateLimit() *RateLimit {
	return p.rateLimit
}

// ClassifyProtocols classifies protocols using OpenAI
func (p *OpenAIProvider) ClassifyProtocols(ctx context.Context, protocols []string) (map[string]qos.Classification, error) {
	// TODO: Implement OpenAI API integration
	p.logger.WithField("provider", p.Name()).Warn("OpenAI provider not yet implemented")

	// Return default classifications for now
	results := make(map[string]qos.Classification)
	for _, protocol := range protocols {
		results[protocol] = qos.Classification{
			Protocol:   protocol,
			Class:      qos.CS1,
			Confidence: 0.5,
			Source:     "ai",
		}
	}

	return results, fmt.Errorf("OpenAI provider not yet implemented")
}
