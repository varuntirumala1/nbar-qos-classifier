package metrics

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/varuntirumala1/nbar-qos-classifier/internal/logger"
	"github.com/varuntirumala1/nbar-qos-classifier/pkg/config"
)

// Metrics holds all the Prometheus metrics
type Metrics struct {
	// Protocol classification metrics
	ProtocolsClassified    prometheus.CounterVec
	ClassificationDuration prometheus.HistogramVec
	ClassificationErrors   prometheus.CounterVec

	// AI provider metrics
	AIRequests        prometheus.CounterVec
	AIRequestDuration prometheus.HistogramVec
	AIRequestErrors   prometheus.CounterVec
	AITokensUsed      prometheus.CounterVec

	// SSH connection metrics
	SSHConnections        prometheus.CounterVec
	SSHConnectionDuration prometheus.HistogramVec
	SSHConnectionErrors   prometheus.CounterVec

	// Cache metrics
	CacheHits      prometheus.CounterVec
	CacheMisses    prometheus.CounterVec
	CacheSize      prometheus.GaugeVec
	CacheEvictions prometheus.CounterVec

	// QoS class distribution
	QoSClassDistribution prometheus.GaugeVec

	// System metrics
	ConfigChanges             prometheus.CounterVec
	ProtocolDiscoveryDuration prometheus.HistogramVec

	// Rate limiting metrics
	RateLimitedRequests prometheus.CounterVec
	RateLimitWaitTime   prometheus.HistogramVec

	config   *config.MetricsConfig
	logger   *logger.Logger
	registry *prometheus.Registry
}

// New creates a new metrics instance
func New(cfg *config.MetricsConfig, logger *logger.Logger) *Metrics {
	registry := prometheus.NewRegistry()

	m := &Metrics{
		config:   cfg,
		logger:   logger,
		registry: registry,
	}

	m.initializeMetrics()
	m.registerMetrics()

	return m
}

// initializeMetrics initializes all Prometheus metrics
func (m *Metrics) initializeMetrics() {
	namespace := m.config.Namespace
	subsystem := m.config.Subsystem

	// Protocol classification metrics
	m.ProtocolsClassified = *prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "protocols_classified_total",
			Help:      "Total number of protocols classified",
		},
		[]string{"qos_class", "source"},
	)

	m.ClassificationDuration = *prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "classification_duration_seconds",
			Help:      "Time spent classifying protocols",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{"source"},
	)

	m.ClassificationErrors = *prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "classification_errors_total",
			Help:      "Total number of classification errors",
		},
		[]string{"error_type", "source"},
	)

	// AI provider metrics
	m.AIRequests = *prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "ai_requests_total",
			Help:      "Total number of AI API requests",
		},
		[]string{"provider", "model", "status"},
	)

	m.AIRequestDuration = *prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "ai_request_duration_seconds",
			Help:      "Duration of AI API requests",
			Buckets:   []float64{0.1, 0.5, 1.0, 2.0, 5.0, 10.0, 30.0, 60.0},
		},
		[]string{"provider", "model"},
	)

	m.AIRequestErrors = *prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "ai_request_errors_total",
			Help:      "Total number of AI API request errors",
		},
		[]string{"provider", "model", "error_type"},
	)

	m.AITokensUsed = *prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "ai_tokens_used_total",
			Help:      "Total number of AI tokens used",
		},
		[]string{"provider", "model", "token_type"},
	)

	// SSH connection metrics
	m.SSHConnections = *prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "ssh_connections_total",
			Help:      "Total number of SSH connections",
		},
		[]string{"host", "status"},
	)

	m.SSHConnectionDuration = *prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "ssh_connection_duration_seconds",
			Help:      "Duration of SSH connections",
			Buckets:   []float64{0.1, 0.5, 1.0, 2.0, 5.0, 10.0},
		},
		[]string{"host"},
	)

	m.SSHConnectionErrors = *prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "ssh_connection_errors_total",
			Help:      "Total number of SSH connection errors",
		},
		[]string{"host", "error_type"},
	)

	// Cache metrics
	m.CacheHits = *prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "cache_hits_total",
			Help:      "Total number of cache hits",
		},
		[]string{"cache_type"},
	)

	m.CacheMisses = *prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "cache_misses_total",
			Help:      "Total number of cache misses",
		},
		[]string{"cache_type"},
	)

	m.CacheSize = *prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "cache_size",
			Help:      "Current size of the cache",
		},
		[]string{"cache_type"},
	)

	m.CacheEvictions = *prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "cache_evictions_total",
			Help:      "Total number of cache evictions",
		},
		[]string{"cache_type"},
	)

	// QoS class distribution
	m.QoSClassDistribution = *prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "qos_class_distribution",
			Help:      "Distribution of protocols by QoS class",
		},
		[]string{"qos_class"},
	)

	// System metrics
	m.ConfigChanges = *prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "config_changes_total",
			Help:      "Total number of configuration changes",
		},
		[]string{"change_type"},
	)

	m.ProtocolDiscoveryDuration = *prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "protocol_discovery_duration_seconds",
			Help:      "Duration of protocol discovery operations",
			Buckets:   []float64{1.0, 5.0, 10.0, 30.0, 60.0, 120.0},
		},
		[]string{"host"},
	)

	// Rate limiting metrics
	m.RateLimitedRequests = *prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "rate_limited_requests_total",
			Help:      "Total number of rate limited requests",
		},
		[]string{"provider"},
	)

	m.RateLimitWaitTime = *prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "rate_limit_wait_time_seconds",
			Help:      "Time spent waiting for rate limits",
			Buckets:   []float64{0.1, 0.5, 1.0, 2.0, 5.0, 10.0},
		},
		[]string{"provider"},
	)
}

// registerMetrics registers all metrics with the registry
func (m *Metrics) registerMetrics() {
	m.registry.MustRegister(
		m.ProtocolsClassified,
		m.ClassificationDuration,
		m.ClassificationErrors,
		m.AIRequests,
		m.AIRequestDuration,
		m.AIRequestErrors,
		m.AITokensUsed,
		m.SSHConnections,
		m.SSHConnectionDuration,
		m.SSHConnectionErrors,
		m.CacheHits,
		m.CacheMisses,
		m.CacheSize,
		m.CacheEvictions,
		m.QoSClassDistribution,
		m.ConfigChanges,
		m.ProtocolDiscoveryDuration,
		m.RateLimitedRequests,
		m.RateLimitWaitTime,
	)
}

// RecordProtocolClassification records a protocol classification
func (m *Metrics) RecordProtocolClassification(qosClass, source string, duration time.Duration) {
	m.ProtocolsClassified.WithLabelValues(qosClass, source).Inc()
	m.ClassificationDuration.WithLabelValues(source).Observe(duration.Seconds())
}

// RecordClassificationError records a classification error
func (m *Metrics) RecordClassificationError(errorType, source string) {
	m.ClassificationErrors.WithLabelValues(errorType, source).Inc()
}

// RecordAIRequest records an AI API request
func (m *Metrics) RecordAIRequest(provider, model, status string, duration time.Duration) {
	m.AIRequests.WithLabelValues(provider, model, status).Inc()
	m.AIRequestDuration.WithLabelValues(provider, model).Observe(duration.Seconds())
}

// RecordAIError records an AI API error
func (m *Metrics) RecordAIError(provider, model, errorType string) {
	m.AIRequestErrors.WithLabelValues(provider, model, errorType).Inc()
}

// RecordAITokenUsage records AI token usage
func (m *Metrics) RecordAITokenUsage(provider, model, tokenType string, count int) {
	m.AITokensUsed.WithLabelValues(provider, model, tokenType).Add(float64(count))
}

// RecordSSHConnection records an SSH connection
func (m *Metrics) RecordSSHConnection(host, status string, duration time.Duration) {
	m.SSHConnections.WithLabelValues(host, status).Inc()
	m.SSHConnectionDuration.WithLabelValues(host).Observe(duration.Seconds())
}

// RecordSSHError records an SSH error
func (m *Metrics) RecordSSHError(host, errorType string) {
	m.SSHConnectionErrors.WithLabelValues(host, errorType).Inc()
}

// RecordCacheHit records a cache hit
func (m *Metrics) RecordCacheHit(cacheType string) {
	m.CacheHits.WithLabelValues(cacheType).Inc()
}

// RecordCacheMiss records a cache miss
func (m *Metrics) RecordCacheMiss(cacheType string) {
	m.CacheMisses.WithLabelValues(cacheType).Inc()
}

// SetCacheSize sets the current cache size
func (m *Metrics) SetCacheSize(cacheType string, size int) {
	m.CacheSize.WithLabelValues(cacheType).Set(float64(size))
}

// RecordCacheEviction records a cache eviction
func (m *Metrics) RecordCacheEviction(cacheType string) {
	m.CacheEvictions.WithLabelValues(cacheType).Inc()
}

// SetQoSClassDistribution sets the QoS class distribution
func (m *Metrics) SetQoSClassDistribution(qosClass string, count int) {
	m.QoSClassDistribution.WithLabelValues(qosClass).Set(float64(count))
}

// RecordConfigChange records a configuration change
func (m *Metrics) RecordConfigChange(changeType string) {
	m.ConfigChanges.WithLabelValues(changeType).Inc()
}

// RecordProtocolDiscovery records a protocol discovery operation
func (m *Metrics) RecordProtocolDiscovery(host string, duration time.Duration) {
	m.ProtocolDiscoveryDuration.WithLabelValues(host).Observe(duration.Seconds())
}

// RecordRateLimitedRequest records a rate limited request
func (m *Metrics) RecordRateLimitedRequest(provider string, waitTime time.Duration) {
	m.RateLimitedRequests.WithLabelValues(provider).Inc()
	m.RateLimitWaitTime.WithLabelValues(provider).Observe(waitTime.Seconds())
}

// Handler returns the HTTP handler for metrics
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
}

// StartServer starts the metrics HTTP server
func (m *Metrics) StartServer() error {
	if !m.config.Enabled {
		m.logger.Info("Metrics server disabled")
		return nil
	}

	addr := ":" + strconv.Itoa(m.config.Port)

	mux := http.NewServeMux()
	mux.Handle(m.config.Path, m.Handler())

	m.logger.WithFields(logger.Fields{
		"address": addr,
		"path":    m.config.Path,
	}).Info("Starting metrics server")

	return http.ListenAndServe(addr, mux)
}
