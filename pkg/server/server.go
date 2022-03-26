package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gofrs/uuid"
	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"github.com/jhaals/yopass/pkg/yopass"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
	"github.com/Microsoft/ApplicationInsights-Go/appinsights"
)

// Server struct holding database and settings.
// This should be created with server.New
type Server struct {
	db                  Database
	maxLength           int
	registry            *prometheus.Registry
	forceOneTimeSecrets bool
	logger              *zap.Logger
	appInsights		appinsights.TelemetryClient
}

// New is the main way of creating the server.
func New(db Database, maxLength int, r *prometheus.Registry, forceOneTimeSecrets bool, logger *zap.Logger, appInsights appinsights.TelemetryClient) Server {
	if logger == nil {
		logger = zap.NewNop()
	}
	return Server{
		db:                  db,
		maxLength:           maxLength,
		registry:            r,
		forceOneTimeSecrets: forceOneTimeSecrets,
		logger:              logger,
		appInsights:		appInsights,
	}
}

// createSecret creates secret
func (y *Server) createSecret(w http.ResponseWriter, request *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	//y.appInsights.TrackEvent("FAXXX")
	decoder := json.NewDecoder(request.Body)
	var s yopass.Secret
	if err := decoder.Decode(&s); err != nil {
		y.logger.Debug("Unable to decode request", zap.Error(err))
		http.Error(w, `{"message": "Unable to parse json"}`, http.StatusBadRequest)
		return
	}

	if !validExpiration(s.Expiration) {
		http.Error(w, `{"message": "Invalid expiration specified"}`, http.StatusBadRequest)
		return
	}

	if !s.OneTime && y.forceOneTimeSecrets {
		http.Error(w, `{"message": "Secret must be one time download"}`, http.StatusBadRequest)
		return
	}

	if len(s.Message) > y.maxLength {
		http.Error(w, `{"message": "The encrypted message is too long"}`, http.StatusBadRequest)
		return
	}

	// Generate new UUID
	uuidVal, err := uuid.NewV4()
	if err != nil {
		y.logger.Error("Unable to generate UUID", zap.Error(err))
		http.Error(w, `{"message": "Unable to generate UUID"}`, http.StatusInternalServerError)
		return
	}
	key := uuidVal.String()

	// store secret in memcache with specified expiration.
	if err := y.db.Put(key, s); err != nil {
		y.logger.Error("Unable to store secret", zap.Error(err))
		http.Error(w, `{"message": "Failed to store secret in database"}`, http.StatusInternalServerError)
		return
	}

	resp := map[string]string{"message": key}
	jsonData, err := json.Marshal(resp)
	if err != nil {
		y.logger.Error("Failed to marshal create secret response", zap.Error(err), zap.String("key", key))
	}

	if _, err = w.Write(jsonData); err != nil {
		y.logger.Error("Failed to write response", zap.Error(err), zap.String("key", key))
	}
}

// getSecret from database
func (y *Server) getSecret(w http.ResponseWriter, request *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Cache-Control", "private, no-cache")

	secretKey := mux.Vars(request)["key"]
	y.appInsights.TrackEvent("getSecret:" + secretKey)
	secret, err := y.db.Get(secretKey)
	if err != nil {
		y.logger.Debug("Secret not found", zap.Error(err), zap.String("key", secretKey))
		y.appInsights.TrackEvent("Error: Secret not found:" + secretKey)
		http.Error(w, `{"message": "Secret not found"}`, http.StatusNotFound)
		return
	}

	data, err := secret.ToJSON()
	if err != nil {
		y.logger.Error("Failed to encode request", zap.Error(err), zap.String("key", secretKey))
		y.appInsights.TrackEvent("Error: Failed to encode request:" + secretKey)
		http.Error(w, `{"message": "Failed to encode secret"}`, http.StatusInternalServerError)
		return
	}

	if _, err := w.Write(data); err != nil {
		y.logger.Error("Failed to write response", zap.Error(err), zap.String("key", secretKey))
		y.appInsights.TrackEvent("Error: Failed to write response:" + secretKey)
	}
}

// deleteSecret from database
func (y *Server) deleteSecret(w http.ResponseWriter, request *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")

	deleted, err := y.db.Delete(mux.Vars(request)["key"])
	if err != nil {
		http.Error(w, `{"message": "Failed to delete secret"}`, http.StatusInternalServerError)
		y.appInsights.TrackEvent("Error: Failed to delete secret:" + (mux.Vars(request)["key"]))
		return
	}

	if !deleted {
		http.Error(w, `{"message": "Secret not found"}`, http.StatusNotFound)
		return
	}

	w.WriteHeader(204)
}

// optionsSecret handle the Options http method by returning the correct CORS headers
func (y *Server) optionsSecret(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", strings.Join([]string{http.MethodGet, http.MethodDelete, http.MethodOptions}, ","))
}

// HTTPHandler containing all routes
func (y *Server) HTTPHandler() http.Handler {
	mx := mux.NewRouter()
	mx.Use(newMetricsMiddleware(y.registry))

	mx.HandleFunc("/secret", y.createSecret).Methods(http.MethodPost)
	mx.HandleFunc("/secret/"+keyParameter, y.getSecret).Methods(http.MethodGet)
	mx.HandleFunc("/secret/"+keyParameter, y.deleteSecret).Methods(http.MethodDelete)
	mx.HandleFunc("/secret/"+keyParameter, y.optionsSecret).Methods(http.MethodOptions)

	mx.HandleFunc("/file", y.createSecret).Methods(http.MethodPost)
	mx.HandleFunc("/file/"+keyParameter, y.getSecret).Methods(http.MethodGet)
	mx.HandleFunc("/file/"+keyParameter, y.deleteSecret).Methods(http.MethodDelete)
	mx.HandleFunc("/file/"+keyParameter, y.optionsSecret).Methods(http.MethodOptions)

	mx.PathPrefix("/").Handler(http.FileServer(http.Dir("public")))
	return handlers.CustomLoggingHandler(nil, SecurityHeadersHandler(mx), httpLogFormatter(y.logger))
}

const keyParameter = "{key:(?:[0-9a-f]{8}-(?:[0-9a-f]{4}-){3}[0-9a-f]{12})}"

// validExpiration validates that expiration is either
// 3600(1hour), 86400(1day) or 604800(1week)
func validExpiration(expiration int32) bool {
	for _, ttl := range []int32{3600, 28800, 86400, 259200, 432000, 604800} {
		if ttl == expiration {
			return true
		}
	}
	return false
}

// SecurityHeadersHandler returns a middleware which sets common security
// HTTP headers on the response to mitigate common web vulnerabilities.
func SecurityHeadersHandler(next http.Handler) http.Handler {
	csp := []string{
		"default-src 'self'",
		"font-src 'self'",
		"form-action 'self'",
		"frame-ancestors 'none'",
		"script-src 'self'",
		"style-src 'self' 'unsafe-inline'",
		"base-uri 'self'",
		"script-src 'self' https://storage.googleapis.com https://dc.services.visualstudio.com 'unsafe-inline'",
		"img-src 'self'",
		"upgrade-insecure-requests",
		"block-all-mixed-content",
		"media-src 'self'",
		"object-src 'none'",
		"font-src 'self' https://fonts.gstatic.com",
		"form-action 'self'",
		"frame-ancestors 'none'",
		"connect-src 'self' https://dc.services.visualstudio.com",
		//"script-src 'self' 'unsafe-inline' https://storage.googleapis.com",
		"style-src 'self' 'unsafe-inline' https://fonts.googleapis.com",
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-security-policy", strings.Join(csp, "; "))
		w.Header().Set("referrer-policy", "no-referrer")
		w.Header().Set("x-content-type-options", "nosniff")
		w.Header().Set("x-frame-options", "DENY")
		w.Header().Set("x-xss-protection", "1; mode=block")
		if r.URL.Scheme == "https" || r.Header.Get("X-Forwarded-Proto") == "https" {
			w.Header().Set("strict-transport-security", "max-age=31536000")
		}
		next.ServeHTTP(w, r)
	})
}

// newMetricsHandler creates a middleware handler recording all HTTP requests in
// the given Prometheus registry
func newMetricsMiddleware(reg prometheus.Registerer) func(http.Handler) http.Handler {
	requests := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "yopass_http_requests_total",
			Help: "Total number of requests served by HTTP method, path and response code.",
		},
		[]string{"method", "path", "code"},
	)
	reg.MustRegister(requests)

	duration := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "yopass_http_request_duration_seconds",
			Help:    "Histogram of HTTP request latencies by method and path.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "path"},
	)
	reg.MustRegister(duration)

	return func(handler http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := statusCodeRecorder{ResponseWriter: w, statusCode: http.StatusOK}
			handler.ServeHTTP(&rec, r)
			path := normalizedPath(r)
			requests.WithLabelValues(r.Method, path, strconv.Itoa(rec.statusCode)).Inc()
			duration.WithLabelValues(r.Method, path).Observe(time.Since(start).Seconds())
		})
	}
}

// normlizedPath returns a normalized mux path template representation
func normalizedPath(r *http.Request) string {
	if route := mux.CurrentRoute(r); route != nil {
		if tmpl, err := route.GetPathTemplate(); err == nil {
			return strings.ReplaceAll(tmpl, keyParameter, ":key")
		}
	}
	return "<other>"
}

// statusCodeRecorder is a HTTP ResponseWriter recording the response code
type statusCodeRecorder struct {
	http.ResponseWriter
	statusCode int
}

// WriteHeader implements http.ResponseWriter
func (rw *statusCodeRecorder) WriteHeader(code int) {
	rw.ResponseWriter.WriteHeader(code)
	rw.statusCode = code
}
