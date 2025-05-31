package ai

import (
	"context"
	"fmt"

	"github.com/varuntirumala1/nbar-qos-classifier/internal/logger"
	"github.com/varuntirumala1/nbar-qos-classifier/pkg/config"
	"github.com/varuntirumala1/nbar-qos-classifier/pkg/qos"
)

// OllamaProvider implements the Provider interface for Ollama
type OllamaProvider struct {
	config    *config.AIConfig
	logger    *logger.Logger
	apiKey    string
	model     string
	baseURL   string
	rateLimit *RateLimit
}

// NewOllamaProvider creates a new Ollama provider
func NewOllamaProvider(cfg *config.AIConfig, logger *logger.Logger) (*OllamaProvider, error) {
	provider := &OllamaProvider{
		config:  cfg,
		logger:  logger,
		apiKey:  cfg.APIKey,
		model:   cfg.Model,
		baseURL: "http://localhost:11434", // Default Ollama URL
		rateLimit: &RateLimit{
			RequestsPerMinute: cfg.RateLimit.RequestsPerMinute,
			BurstSize:         cfg.RateLimit.BurstSize,
			BackoffStrategy:   cfg.RateLimit.BackoffStrategy,
			MaxBackoff:        cfg.RateLimit.MaxBackoff,
		},
	}

	// Override with provider-specific config if available
	if providerCfg, exists := cfg.Providers["ollama"]; exists {
		if providerCfg.APIKey != "" {
			provider.apiKey = providerCfg.APIKey
		}
		if providerCfg.Model != "" {
			provider.model = providerCfg.Model
		}
		if providerCfg.BaseURL != "" {
			provider.baseURL = providerCfg.BaseURL
		}
	}

	return provider, nil
}

// Name returns the provider name
func (p *OllamaProvider) Name() string {
	return "ollama"
}

// IsAvailable checks if the provider is available
func (p *OllamaProvider) IsAvailable() bool {
	// Ollama doesn't require an API key, just check if model is set
	return p.model != ""
}

// GetRateLimit returns the rate limit configuration
func (p *OllamaProvider) GetRateLimit() *RateLimit {
	return p.rateLimit
}

// ClassifyProtocols classifies protocols using Ollama
func (p *OllamaProvider) ClassifyProtocols(ctx context.Context, protocols []string) (map[string]qos.Classification, error) {
	// TODO: Implement Ollama API integration
	p.logger.WithField("provider", p.Name()).Warn("Ollama provider not yet implemented")

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

	return results, fmt.Errorf("Ollama provider not yet implemented")
}
