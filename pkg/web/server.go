package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/varuntirumala1/nbar-qos-classifier/internal/logger"
	"github.com/varuntirumala1/nbar-qos-classifier/pkg/config"
)

// Server represents the web server
type Server struct {
	config *config.WebConfig
	logger *logger.Logger
	router *mux.Router
	server *http.Server
}

// New creates a new web server
func New(cfg *config.WebConfig, logger *logger.Logger) *Server {
	s := &Server{
		config: cfg,
		logger: logger,
		router: mux.NewRouter(),
	}

	s.setupRoutes()

	return s
}

// setupRoutes sets up the HTTP routes
func (s *Server) setupRoutes() {
	// API routes
	api := s.router.PathPrefix("/api/v1").Subrouter()
	api.HandleFunc("/health", s.handleHealth).Methods("GET")
	api.HandleFunc("/ready", s.handleReady).Methods("GET")
	api.HandleFunc("/status", s.handleStatus).Methods("GET")
	api.HandleFunc("/protocols", s.handleProtocols).Methods("GET")
	api.HandleFunc("/classifications", s.handleClassifications).Methods("GET")
	api.HandleFunc("/cache/stats", s.handleCacheStats).Methods("GET")
	api.HandleFunc("/cache/clear", s.handleCacheClear).Methods("POST")

	// Static files (if enabled)
	if s.config.StaticDir != "" {
		s.router.PathPrefix("/static/").Handler(
			http.StripPrefix("/static/", http.FileServer(http.Dir(s.config.StaticDir))),
		)
	}

	// Default route
	s.router.HandleFunc("/", s.handleIndex).Methods("GET")

	// Add middleware
	s.router.Use(s.loggingMiddleware)
	s.router.Use(s.corsMiddleware)
}

// Start starts the web server
func (s *Server) Start() error {
	if !s.config.Enabled {
		s.logger.Info("Web server disabled")
		return nil
	}

	addr := fmt.Sprintf("%s:%d", s.config.Host, s.config.Port)

	s.server = &http.Server{
		Addr:         addr,
		Handler:      s.router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	s.logger.WithFields(logger.Fields{
		"address":     addr,
		"tls_enabled": s.config.TLSEnabled,
	}).Info("Starting web server")

	if s.config.TLSEnabled {
		return s.server.ListenAndServeTLS(s.config.CertFile, s.config.KeyFile)
	}

	return s.server.ListenAndServe()
}

// Stop stops the web server
func (s *Server) Stop() error {
	if s.server == nil {
		return nil
	}

	s.logger.Info("Stopping web server")
	return s.server.Close()
}

// Health check endpoint
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	response := map[string]interface{}{
		"status":    "healthy",
		"timestamp": time.Now().Unix(),
		"version":   "2.0.0", // TODO: Get from build info
	}

	s.writeJSON(w, http.StatusOK, response)
}

// Readiness check endpoint
func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	// TODO: Check if all dependencies are ready
	response := map[string]interface{}{
		"status":    "ready",
		"timestamp": time.Now().Unix(),
	}

	s.writeJSON(w, http.StatusOK, response)
}

// Status endpoint
func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	response := map[string]interface{}{
		"status":    "running",
		"timestamp": time.Now().Unix(),
		"uptime":    "unknown", // TODO: Calculate uptime
		"version":   "2.0.0",
		"components": map[string]string{
			"cache":   "healthy",
			"ssh":     "healthy",
			"ai":      "healthy",
			"metrics": "healthy",
		},
	}

	s.writeJSON(w, http.StatusOK, response)
}

// Protocols endpoint
func (s *Server) handleProtocols(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement protocol listing
	response := map[string]interface{}{
		"protocols": []string{},
		"count":     0,
		"timestamp": time.Now().Unix(),
	}

	s.writeJSON(w, http.StatusOK, response)
}

// Classifications endpoint
func (s *Server) handleClassifications(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement classification listing
	response := map[string]interface{}{
		"classifications": map[string]interface{}{},
		"count":           0,
		"timestamp":       time.Now().Unix(),
	}

	s.writeJSON(w, http.StatusOK, response)
}

// Cache stats endpoint
func (s *Server) handleCacheStats(w http.ResponseWriter, r *http.Request) {
	// TODO: Get actual cache stats
	response := map[string]interface{}{
		"size":      0,
		"hits":      0,
		"misses":    0,
		"hit_rate":  0.0,
		"timestamp": time.Now().Unix(),
	}

	s.writeJSON(w, http.StatusOK, response)
}

// Cache clear endpoint
func (s *Server) handleCacheClear(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement cache clearing
	response := map[string]interface{}{
		"status":    "cleared",
		"timestamp": time.Now().Unix(),
	}

	s.writeJSON(w, http.StatusOK, response)
}

// Index page
func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	html := `
<!DOCTYPE html>
<html>
<head>
    <title>NBAR QoS Classifier</title>
    <style>
        body { font-family: Arial, sans-serif; margin: 40px; }
        .header { background: #f4f4f4; padding: 20px; border-radius: 5px; }
        .section { margin: 20px 0; }
        .api-link { display: block; margin: 5px 0; color: #0066cc; }
    </style>
</head>
<body>
    <div class="header">
        <h1>NBAR QoS Classifier</h1>
        <p>Network protocol classification and QoS management</p>
    </div>

    <div class="section">
        <h2>API Endpoints</h2>
        <a href="/api/v1/health" class="api-link">Health Check</a>
        <a href="/api/v1/ready" class="api-link">Readiness Check</a>
        <a href="/api/v1/status" class="api-link">System Status</a>
        <a href="/api/v1/protocols" class="api-link">Protocols</a>
        <a href="/api/v1/classifications" class="api-link">Classifications</a>
        <a href="/api/v1/cache/stats" class="api-link">Cache Statistics</a>
    </div>

    <div class="section">
        <h2>Documentation</h2>
        <p>For detailed API documentation, please refer to the project README.</p>
    </div>
</body>
</html>
`

	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(html))
}

// writeJSON writes a JSON response
func (s *Server) writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(data); err != nil {
		s.logger.WithError(err).Error("Failed to encode JSON response")
	}
}

// Logging middleware
func (s *Server) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Create a response writer wrapper to capture status code
		wrapper := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		next.ServeHTTP(wrapper, r)

		duration := time.Since(start)

		s.logger.WithFields(logger.Fields{
			"method":      r.Method,
			"path":        r.URL.Path,
			"status_code": wrapper.statusCode,
			"duration":    duration,
			"remote_addr": r.RemoteAddr,
			"user_agent":  r.UserAgent(),
		}).Info("HTTP request")
	})
}

// CORS middleware
func (s *Server) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// responseWriter wraps http.ResponseWriter to capture status code
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// GetPort returns the configured port
func (s *Server) GetPort() int {
	return s.config.Port
}

// GetAddress returns the full server address
func (s *Server) GetAddress() string {
	return fmt.Sprintf("%s:%d", s.config.Host, s.config.Port)
}
