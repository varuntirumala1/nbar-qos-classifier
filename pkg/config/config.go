package config

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Config represents the application configuration
type Config struct {
	// Application settings
	App AppConfig `yaml:"app"`

	// SSH connection settings
	SSH SSHConfig `yaml:"ssh"`

	// AI provider settings
	AI AIConfig `yaml:"ai"`

	// QoS classification settings
	QoS QoSConfig `yaml:"qos"`

	// Cache settings
	Cache CacheConfig `yaml:"cache"`

	// Logging settings
	Logging LoggingConfig `yaml:"logging"`

	// Metrics settings
	Metrics MetricsConfig `yaml:"metrics"`

	// Web interface settings
	Web WebConfig `yaml:"web"`

	// Security settings
	Security SecurityConfig `yaml:"security"`
}

// AppConfig contains general application settings
type AppConfig struct {
	Name        string        `yaml:"name"`
	Version     string        `yaml:"version"`
	Environment string        `yaml:"environment"`
	BatchSize   int           `yaml:"batch_size"`
	Timeout     time.Duration `yaml:"timeout"`
	MaxRetries  int           `yaml:"max_retries"`
}

// SSHConfig contains SSH connection settings
type SSHConfig struct {
	Host               string        `yaml:"host"`
	Port               string        `yaml:"port"`
	User               string        `yaml:"user"`
	KeyFile            string        `yaml:"key_file"`
	Timeout            time.Duration `yaml:"timeout"`
	MaxConnections     int           `yaml:"max_connections"`
	ConnectionPoolSize int           `yaml:"connection_pool_size"`
	KeepAlive          time.Duration `yaml:"keep_alive"`
}

// AIConfig contains AI provider settings
type AIConfig struct {
	Provider    string                    `yaml:"provider"`
	APIKey      string                    `yaml:"api_key"`
	Model       string                    `yaml:"model"`
	Temperature float64                   `yaml:"temperature"`
	MaxTokens   int                       `yaml:"max_tokens"`
	Timeout     time.Duration             `yaml:"timeout"`
	RateLimit   RateLimitConfig           `yaml:"rate_limit"`
	Fallback    []FallbackConfig          `yaml:"fallback"`
	Providers   map[string]ProviderConfig `yaml:"providers"`
}

// RateLimitConfig contains rate limiting settings
type RateLimitConfig struct {
	RequestsPerMinute int           `yaml:"requests_per_minute"`
	BurstSize         int           `yaml:"burst_size"`
	BackoffStrategy   string        `yaml:"backoff_strategy"`
	MaxBackoff        time.Duration `yaml:"max_backoff"`
}

// FallbackConfig contains fallback provider settings
type FallbackConfig struct {
	Provider string `yaml:"provider"`
	Enabled  bool   `yaml:"enabled"`
}

// ProviderConfig contains provider-specific settings
type ProviderConfig struct {
	APIKey      string  `yaml:"api_key"`
	Model       string  `yaml:"model"`
	Temperature float64 `yaml:"temperature"`
	MaxTokens   int     `yaml:"max_tokens"`
	BaseURL     string  `yaml:"base_url"`
}

// QoSConfig contains QoS classification settings
type QoSConfig struct {
	Classes             map[string]QoSClassConfig `yaml:"classes"`
	DefaultClass        string                    `yaml:"default_class"`
	CustomRules         []CustomRuleConfig        `yaml:"custom_rules"`
	ProtocolFamilies    map[string][]string       `yaml:"protocol_families"`
	LearningEnabled     bool                      `yaml:"learning_enabled"`
	ConfidenceThreshold float64                   `yaml:"confidence_threshold"`
}

// QoSClassConfig contains settings for a specific QoS class
type QoSClassConfig struct {
	Name        string   `yaml:"name"`
	Description string   `yaml:"description"`
	DSCP        string   `yaml:"dscp"`
	Priority    int      `yaml:"priority"`
	Protocols   []string `yaml:"protocols"`
}

// CustomRuleConfig contains custom classification rules
type CustomRuleConfig struct {
	Name     string `yaml:"name"`
	Pattern  string `yaml:"pattern"`
	Class    string `yaml:"class"`
	Priority int    `yaml:"priority"`
	Enabled  bool   `yaml:"enabled"`
}

// CacheConfig contains cache settings
type CacheConfig struct {
	Enabled     bool          `yaml:"enabled"`
	TTL         time.Duration `yaml:"ttl"`
	MaxSize     int           `yaml:"max_size"`
	FilePath    string        `yaml:"file_path"`
	Compression bool          `yaml:"compression"`
	BackupPath  string        `yaml:"backup_path"`
}

// LoggingConfig contains logging settings
type LoggingConfig struct {
	Level      string `yaml:"level"`
	Format     string `yaml:"format"`
	Output     string `yaml:"output"`
	File       string `yaml:"file"`
	MaxSize    int    `yaml:"max_size"`
	MaxBackups int    `yaml:"max_backups"`
	MaxAge     int    `yaml:"max_age"`
	Compress   bool   `yaml:"compress"`
}

// MetricsConfig contains metrics settings
type MetricsConfig struct {
	Enabled   bool   `yaml:"enabled"`
	Port      int    `yaml:"port"`
	Path      string `yaml:"path"`
	Namespace string `yaml:"namespace"`
	Subsystem string `yaml:"subsystem"`
}

// WebConfig contains web interface settings
type WebConfig struct {
	Enabled     bool   `yaml:"enabled"`
	Host        string `yaml:"host"`
	Port        int    `yaml:"port"`
	TLSEnabled  bool   `yaml:"tls_enabled"`
	CertFile    string `yaml:"cert_file"`
	KeyFile     string `yaml:"key_file"`
	StaticDir   string `yaml:"static_dir"`
	TemplateDir string `yaml:"template_dir"`
}

// SecurityConfig contains security settings
type SecurityConfig struct {
	Use1Password       bool          `yaml:"use_1password"`
	CredentialRotation bool          `yaml:"credential_rotation"`
	AuditLogging       bool          `yaml:"audit_logging"`
	EncryptionKey      string        `yaml:"encryption_key"`
	JWTSecret          string        `yaml:"jwt_secret"`
	SessionTimeout     time.Duration `yaml:"session_timeout"`
}

// LoadConfig loads configuration from file
func LoadConfig(configPath string) (*Config, error) {
	// Set default config path if not provided
	if configPath == "" {
		configPath = getDefaultConfigPath()
	}

	// Read config file
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Parse YAML
	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Apply defaults
	applyDefaults(&config)

	// Resolve 1Password references if enabled
	if config.Security.Use1Password {
		if err := resolve1PasswordReferences(&config); err != nil {
			return nil, fmt.Errorf("failed to resolve 1Password references: %w", err)
		}
	}

	// Validate configuration
	if err := validateConfig(&config); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return &config, nil
}

// getDefaultConfigPath returns the default configuration file path
func getDefaultConfigPath() string {
	// Check for config file in various locations
	paths := []string{
		"./config.yaml",
		"./configs/config.yaml",
		"/etc/nbar-classifier/config.yaml",
		filepath.Join(os.Getenv("HOME"), ".nbar-classifier", "config.yaml"),
	}

	for _, path := range paths {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	return "./config.yaml"
}

// applyDefaults applies default values to the configuration
func applyDefaults(config *Config) {
	// App defaults
	if config.App.Name == "" {
		config.App.Name = "nbar-qos-classifier"
	}
	if config.App.Version == "" {
		config.App.Version = "1.0.0"
	}
	if config.App.Environment == "" {
		config.App.Environment = "production"
	}
	if config.App.BatchSize == 0 {
		config.App.BatchSize = 25
	}
	if config.App.Timeout == 0 {
		config.App.Timeout = 90 * time.Second
	}
	if config.App.MaxRetries == 0 {
		config.App.MaxRetries = 3
	}

	// SSH defaults
	if config.SSH.Port == "" {
		config.SSH.Port = "22"
	}
	if config.SSH.Timeout == 0 {
		config.SSH.Timeout = 10 * time.Second
	}
	if config.SSH.MaxConnections == 0 {
		config.SSH.MaxConnections = 5
	}
	if config.SSH.ConnectionPoolSize == 0 {
		config.SSH.ConnectionPoolSize = 3
	}
	if config.SSH.KeepAlive == 0 {
		config.SSH.KeepAlive = 30 * time.Second
	}

	// AI defaults
	if config.AI.Provider == "" {
		config.AI.Provider = "deepseek"
	}
	if config.AI.Model == "" {
		config.AI.Model = "deepseek-reasoner"
	}
	if config.AI.Temperature == 0 {
		config.AI.Temperature = 0.1
	}
	if config.AI.MaxTokens == 0 {
		config.AI.MaxTokens = 1000
	}
	if config.AI.Timeout == 0 {
		config.AI.Timeout = 90 * time.Second
	}

	// Rate limit defaults
	if config.AI.RateLimit.RequestsPerMinute == 0 {
		config.AI.RateLimit.RequestsPerMinute = 60
	}
	if config.AI.RateLimit.BurstSize == 0 {
		config.AI.RateLimit.BurstSize = 10
	}
	if config.AI.RateLimit.BackoffStrategy == "" {
		config.AI.RateLimit.BackoffStrategy = "exponential"
	}
	if config.AI.RateLimit.MaxBackoff == 0 {
		config.AI.RateLimit.MaxBackoff = 60 * time.Second
	}

	// QoS defaults
	if config.QoS.DefaultClass == "" {
		config.QoS.DefaultClass = "CS1"
	}
	if config.QoS.ConfidenceThreshold == 0 {
		config.QoS.ConfidenceThreshold = 0.8
	}

	// Cache defaults
	if config.Cache.TTL == 0 {
		config.Cache.TTL = 24 * time.Hour
	}
	if config.Cache.MaxSize == 0 {
		config.Cache.MaxSize = 10000
	}
	if config.Cache.FilePath == "" {
		config.Cache.FilePath = "protocol_classifications_cache.json"
	}

	// Logging defaults
	if config.Logging.Level == "" {
		config.Logging.Level = "info"
	}
	if config.Logging.Format == "" {
		config.Logging.Format = "json"
	}
	if config.Logging.Output == "" {
		config.Logging.Output = "stdout"
	}

	// Metrics defaults
	if config.Metrics.Port == 0 {
		config.Metrics.Port = 9090
	}
	if config.Metrics.Path == "" {
		config.Metrics.Path = "/metrics"
	}
	if config.Metrics.Namespace == "" {
		config.Metrics.Namespace = "nbar_classifier"
	}

	// Web defaults
	if config.Web.Host == "" {
		config.Web.Host = "0.0.0.0"
	}
	if config.Web.Port == 0 {
		config.Web.Port = 8080
	}

	// Security defaults
	if config.Security.SessionTimeout == 0 {
		config.Security.SessionTimeout = 24 * time.Hour
	}
}

// validateConfig validates the configuration
func validateConfig(config *Config) error {
	// Validate required fields
	if config.SSH.Host == "" {
		return fmt.Errorf("SSH host is required")
	}
	if config.SSH.User == "" {
		return fmt.Errorf("SSH user is required")
	}
	if config.AI.APIKey == "" {
		return fmt.Errorf("AI API key is required")
	}

	// Validate ranges
	if config.App.BatchSize < 1 || config.App.BatchSize > 100 {
		return fmt.Errorf("batch size must be between 1 and 100")
	}
	if config.AI.Temperature < 0 || config.AI.Temperature > 2 {
		return fmt.Errorf("AI temperature must be between 0 and 2")
	}

	return nil
}

// SaveConfig saves configuration to file
func SaveConfig(config *Config, configPath string) error {
	data, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// resolve1PasswordReferences resolves 1Password references in configuration
func resolve1PasswordReferences(config *Config) error {
	// Resolve SSH key file
	if strings.HasPrefix(config.SSH.KeyFile, "op://") {
		resolved, err := resolve1PasswordReference(config.SSH.KeyFile)
		if err != nil {
			return fmt.Errorf("failed to resolve SSH key: %w", err)
		}
		config.SSH.KeyFile = resolved
	}

	// Resolve AI API key
	if strings.HasPrefix(config.AI.APIKey, "op://") {
		resolved, err := resolve1PasswordReference(config.AI.APIKey)
		if err != nil {
			return fmt.Errorf("failed to resolve AI API key: %w", err)
		}
		config.AI.APIKey = resolved
	}

	// Resolve provider-specific API keys
	for providerName, providerConfig := range config.AI.Providers {
		if strings.HasPrefix(providerConfig.APIKey, "op://") {
			resolved, err := resolve1PasswordReference(providerConfig.APIKey)
			if err != nil {
				return fmt.Errorf("failed to resolve %s API key: %w", providerName, err)
			}
			providerConfig.APIKey = resolved
			config.AI.Providers[providerName] = providerConfig
		}
	}

	return nil
}

// resolve1PasswordReference resolves a single 1Password reference
func resolve1PasswordReference(reference string) (string, error) {
	if !strings.HasPrefix(reference, "op://") {
		return reference, nil
	}

	cmd := exec.Command("op", "read", reference)
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to read 1Password reference %s: %w", reference, err)
	}

	return strings.TrimSpace(string(output)), nil
}