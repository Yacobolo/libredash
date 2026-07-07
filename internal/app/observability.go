package app

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Yacobolo/libredash/internal/secret"
	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type httpTelemetry struct {
	registry    *prometheus.Registry
	requests    *prometheus.CounterVec
	duration    *prometheus.HistogramVec
	response    *prometheus.HistogramVec
	inFlight    prometheus.Gauge
	handlerOpts promhttp.HandlerOpts
}

func newHTTPTelemetry() *httpTelemetry {
	registry := prometheus.NewRegistry()
	telemetry := &httpTelemetry{
		registry: registry,
		requests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "libredash_http_requests_total",
			Help: "Total HTTP requests served by LibreDash.",
		}, []string{"method", "route", "status"}),
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "libredash_http_request_duration_seconds",
			Help:    "HTTP request duration in seconds.",
			Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30},
		}, []string{"method", "route", "status"}),
		response: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "libredash_http_response_size_bytes",
			Help:    "HTTP response size in bytes.",
			Buckets: prometheus.ExponentialBuckets(128, 2, 16),
		}, []string{"method", "route", "status"}),
		inFlight: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "libredash_http_requests_in_flight",
			Help: "HTTP requests currently being served by LibreDash.",
		}),
		handlerOpts: promhttp.HandlerOpts{EnableOpenMetrics: true},
	}
	registry.MustRegister(telemetry.requests, telemetry.duration, telemetry.response, telemetry.inFlight)
	return telemetry
}

func (t *httpTelemetry) middleware(next http.Handler) http.Handler {
	if t == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		t.inFlight.Inc()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		defer func() {
			t.inFlight.Dec()
			route := routePattern(r)
			status := strconv.Itoa(rec.status)
			t.requests.WithLabelValues(r.Method, route, status).Inc()
			t.duration.WithLabelValues(r.Method, route, status).Observe(time.Since(start).Seconds())
			t.response.WithLabelValues(r.Method, route, status).Observe(float64(rec.bytes))
		}()
		next.ServeHTTP(rec, r)
	})
}

func (t *httpTelemetry) handler() http.Handler {
	if t == nil || t.registry == nil {
		return http.NotFoundHandler()
	}
	return promhttp.HandlerFor(t.registry, t.handlerOpts)
}

func (s *Server) metricsHandler() http.Handler {
	handler := s.telemetry.handler()
	token := strings.TrimSpace(s.metricsBearerToken)
	if token == "" {
		return handler
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if provided := bearerToken(r); !secret.Equal(provided, token) {
			w.Header().Set("WWW-Authenticate", `Bearer realm="libredash-metrics"`)
			http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
			return
		}
		handler.ServeHTTP(w, r)
	})
}

func routePattern(r *http.Request) string {
	if r == nil {
		return "unknown"
	}
	if routeCtx := chi.RouteContext(r.Context()); routeCtx != nil {
		if pattern := routeCtx.RoutePattern(); pattern != "" {
			return pattern
		}
	}
	return "unmatched"
}
