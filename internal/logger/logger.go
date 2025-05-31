package logger

import (
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/varuntirumala1/nbar-qos-classifier/pkg/config"
)

// Logger wraps logrus.Logger with additional functionality
type Logger struct {
	*logrus.Logger
	config *config.LoggingConfig
}

// Fields type for structured logging
type Fields map[string]interface{}

// New creates a new logger instance
func New(cfg *config.LoggingConfig) (*Logger, error) {
	logger := logrus.New()

	// Set log level
	level, err := logrus.ParseLevel(cfg.Level)
	if err != nil {
		return nil, err
	}
	logger.SetLevel(level)

	// Set formatter
	switch strings.ToLower(cfg.Format) {
	case "json":
		logger.SetFormatter(&logrus.JSONFormatter{
			TimestampFormat: "2006-01-02T15:04:05.000Z07:00",
			FieldMap: logrus.FieldMap{
				logrus.FieldKeyTime:  "timestamp",
				logrus.FieldKeyLevel: "level",
				logrus.FieldKeyMsg:   "message",
				logrus.FieldKeyFunc:  "function",
				logrus.FieldKeyFile:  "file",
			},
		})
	case "text":
		logger.SetFormatter(&logrus.TextFormatter{
			FullTimestamp:   true,
			TimestampFormat: "2006-01-02T15:04:05.000Z07:00",
		})
	default:
		logger.SetFormatter(&logrus.JSONFormatter{})
	}

	// Set output
	output, err := getOutput(cfg)
	if err != nil {
		return nil, err
	}
	logger.SetOutput(output)

	// Enable caller reporting for better debugging
	logger.SetReportCaller(true)

	return &Logger{
		Logger: logger,
		config: cfg,
	}, nil
}

// getOutput determines the output destination for logs
func getOutput(cfg *config.LoggingConfig) (io.Writer, error) {
	switch strings.ToLower(cfg.Output) {
	case "stdout":
		return os.Stdout, nil
	case "stderr":
		return os.Stderr, nil
	case "file":
		if cfg.File == "" {
			return os.Stdout, nil
		}

		// Create directory if it doesn't exist
		dir := filepath.Dir(cfg.File)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, err
		}

		// Open or create log file
		file, err := os.OpenFile(cfg.File, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
		if err != nil {
			return nil, err
		}
		return file, nil
	default:
		return os.Stdout, nil
	}
}

// WithFields creates a new entry with the given fields
func (l *Logger) WithFields(fields Fields) *logrus.Entry {
	return l.Logger.WithFields(logrus.Fields(fields))
}

// WithField creates a new entry with a single field
func (l *Logger) WithField(key string, value interface{}) *logrus.Entry {
	return l.Logger.WithField(key, value)
}

// WithError creates a new entry with an error field
func (l *Logger) WithError(err error) *logrus.Entry {
	return l.Logger.WithError(err)
}

// WithComponent creates a new entry with a component field
func (l *Logger) WithComponent(component string) *logrus.Entry {
	return l.Logger.WithField("component", component)
}

// WithOperation creates a new entry with an operation field
func (l *Logger) WithOperation(operation string) *logrus.Entry {
	return l.Logger.WithField("operation", operation)
}

// WithProtocol creates a new entry with a protocol field
func (l *Logger) WithProtocol(protocol string) *logrus.Entry {
	return l.Logger.WithField("protocol", protocol)
}

// WithQoSClass creates a new entry with a QoS class field
func (l *Logger) WithQoSClass(class string) *logrus.Entry {
	return l.Logger.WithField("qos_class", class)
}

// WithDuration creates a new entry with a duration field
func (l *Logger) WithDuration(duration interface{}) *logrus.Entry {
	return l.Logger.WithField("duration", duration)
}

// WithCount creates a new entry with a count field
func (l *Logger) WithCount(count int) *logrus.Entry {
	return l.Logger.WithField("count", count)
}

// WithHost creates a new entry with a host field
func (l *Logger) WithHost(host string) *logrus.Entry {
	return l.Logger.WithField("host", host)
}

// WithUser creates a new entry with a user field
func (l *Logger) WithUser(user string) *logrus.Entry {
	return l.Logger.WithField("user", user)
}

// WithRequestID creates a new entry with a request ID field
func (l *Logger) WithRequestID(requestID string) *logrus.Entry {
	return l.Logger.WithField("request_id", requestID)
}

// Audit logs an audit event
func (l *Logger) Audit(action string, fields Fields) {
	auditFields := Fields{
		"audit":  true,
		"action": action,
	}

	// Merge with provided fields
	for k, v := range fields {
		auditFields[k] = v
	}

	l.WithFields(auditFields).Info("Audit event")
}

// Performance logs a performance metric
func (l *Logger) Performance(operation string, duration interface{}, fields Fields) {
	perfFields := Fields{
		"performance": true,
		"operation":   operation,
		"duration":    duration,
	}

	// Merge with provided fields
	for k, v := range fields {
		perfFields[k] = v
	}

	l.WithFields(perfFields).Info("Performance metric")
}

// Security logs a security event
func (l *Logger) Security(event string, fields Fields) {
	secFields := Fields{
		"security": true,
		"event":    event,
	}

	// Merge with provided fields
	for k, v := range fields {
		secFields[k] = v
	}

	l.WithFields(secFields).Warn("Security event")
}

// Classification logs a protocol classification event
func (l *Logger) Classification(protocol, class string, confidence float64, source string) {
	l.WithFields(Fields{
		"classification": true,
		"protocol":       protocol,
		"qos_class":      class,
		"confidence":     confidence,
		"source":         source,
	}).Info("Protocol classified")
}

// APICall logs an API call event
func (l *Logger) APICall(provider, model string, duration interface{}, success bool, fields Fields) {
	apiFields := Fields{
		"api_call": true,
		"provider": provider,
		"model":    model,
		"duration": duration,
		"success":  success,
	}

	// Merge with provided fields
	for k, v := range fields {
		apiFields[k] = v
	}

	if success {
		l.WithFields(apiFields).Info("API call completed")
	} else {
		l.WithFields(apiFields).Error("API call failed")
	}
}

// SSHConnection logs an SSH connection event
func (l *Logger) SSHConnection(host, user string, success bool, duration interface{}) {
	l.WithFields(Fields{
		"ssh_connection": true,
		"host":           host,
		"user":           user,
		"success":        success,
		"duration":       duration,
	}).Info("SSH connection attempt")
}

// ConfigChange logs a configuration change event
func (l *Logger) ConfigChange(changeType string, details Fields) {
	configFields := Fields{
		"config_change": true,
		"change_type":   changeType,
	}

	// Merge with provided fields
	for k, v := range details {
		configFields[k] = v
	}

	l.WithFields(configFields).Info("Configuration change")
}

// CacheOperation logs a cache operation
func (l *Logger) CacheOperation(operation string, hit bool, size int) {
	l.WithFields(Fields{
		"cache_operation": true,
		"operation":       operation,
		"hit":             hit,
		"size":            size,
	}).Debug("Cache operation")
}

// ProtocolDiscovery logs protocol discovery events
func (l *Logger) ProtocolDiscovery(host string, protocolCount int, duration interface{}) {
	l.WithFields(Fields{
		"protocol_discovery": true,
		"host":               host,
		"protocol_count":     protocolCount,
		"duration":           duration,
	}).Info("Protocol discovery completed")
}

// Close closes any open file handles
func (l *Logger) Close() error {
	// If output is a file, close it
	if file, ok := l.Logger.Out.(*os.File); ok && file != os.Stdout && file != os.Stderr {
		return file.Close()
	}
	return nil
}

// SetLevel dynamically changes the log level
func (l *Logger) SetLevel(level string) error {
	logLevel, err := logrus.ParseLevel(level)
	if err != nil {
		return err
	}
	l.Logger.SetLevel(logLevel)
	return nil
}

// GetLevel returns the current log level
func (l *Logger) GetLevel() string {
	return l.Logger.GetLevel().String()
}
