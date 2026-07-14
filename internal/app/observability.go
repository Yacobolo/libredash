package app

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	dashboardstream "github.com/Yacobolo/libredash/internal/dashboard/stream"
	"github.com/Yacobolo/libredash/internal/secret"
	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type httpTelemetry struct {
	registry                      *prometheus.Registry
	requests                      *prometheus.CounterVec
	duration                      *prometheus.HistogramVec
	response                      *prometheus.HistogramVec
	inFlight                      prometheus.Gauge
	dashboardRefreshDuration      *prometheus.HistogramVec
	dashboardStageDuration        *prometheus.HistogramVec
	dashboardRefreshInFlight      *prometheus.GaugeVec
	dashboardRefreshCancellations *prometheus.CounterVec
	dashboardCacheOutcomes        *prometheus.CounterVec
	dashboardTargetOutcomes       *prometheus.CounterVec
	handlerOpts                   promhttp.HandlerOpts
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
		dashboardRefreshDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "libredash_dashboard_refresh_duration_seconds",
			Help:    "End-to-end dashboard refresh duration in seconds.",
			Buckets: []float64{0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30},
		}, []string{"command", "outcome"}),
		dashboardStageDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "libredash_dashboard_refresh_stage_duration_seconds",
			Help:    "Dashboard refresh stage duration in seconds.",
			Buckets: []float64{0.001, 0.0025, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
		}, []string{"stage", "outcome"}),
		dashboardRefreshInFlight: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "libredash_dashboard_refreshes_in_flight",
			Help: "Dashboard refreshes currently in flight.",
		}, []string{"command"}),
		dashboardRefreshCancellations: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "libredash_dashboard_refresh_cancellations_total",
			Help: "Total dashboard refresh cancellations.",
		}, []string{"command"}),
		dashboardCacheOutcomes: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "libredash_dashboard_cache_outcomes_total",
			Help: "Dashboard query cache outcomes.",
		}, []string{"outcome"}),
		dashboardTargetOutcomes: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "libredash_dashboard_target_outcomes_total",
			Help: "Dashboard refresh target outcomes.",
		}, []string{"kind", "outcome"}),
		handlerOpts: promhttp.HandlerOpts{EnableOpenMetrics: true},
	}
	registry.MustRegister(
		telemetry.requests,
		telemetry.duration,
		telemetry.response,
		telemetry.inFlight,
		telemetry.dashboardRefreshDuration,
		telemetry.dashboardStageDuration,
		telemetry.dashboardRefreshInFlight,
		telemetry.dashboardRefreshCancellations,
		telemetry.dashboardCacheOutcomes,
		telemetry.dashboardTargetOutcomes,
	)
	return telemetry
}

func (t *httpTelemetry) dashboardRefreshStarted(command string) {
	if t == nil {
		return
	}
	t.dashboardRefreshInFlight.WithLabelValues(dashboardCommandLabel(command)).Inc()
}

func (t *httpTelemetry) dashboardRefreshFinished(summary dashboardstream.RefreshSummary) {
	if t == nil {
		return
	}
	command := dashboardCommandLabel(summary.Command)
	outcome := dashboardOutcomeLabel(summary.Outcome)
	t.dashboardRefreshInFlight.WithLabelValues(command).Dec()
	if summary.CancellationCount > 0 {
		t.dashboardRefreshCancellations.WithLabelValues(command).Add(float64(summary.CancellationCount))
	}
	for stage, milliseconds := range summary.StageTimingsMs {
		if milliseconds < 0 {
			continue
		}
		seconds := milliseconds / 1000
		stage = dashboardStageLabel(stage)
		t.dashboardStageDuration.WithLabelValues(stage, outcome).Observe(seconds)
		if stage == "end_to_end" {
			t.dashboardRefreshDuration.WithLabelValues(command, outcome).Observe(seconds)
		}
	}
}

func (t *httpTelemetry) dashboardCacheObserved(outcome string) {
	if t == nil {
		return
	}
	t.dashboardCacheOutcomes.WithLabelValues(dashboardCacheLabel(outcome)).Inc()
}

func (t *httpTelemetry) dashboardTargetObserved(kind, outcome string) {
	if t == nil {
		return
	}
	t.dashboardTargetOutcomes.WithLabelValues(dashboardTargetKindLabel(kind), dashboardTargetOutcomeLabel(outcome)).Inc()
}

func (t *httpTelemetry) dashboardRefreshEventObserved(event dashboardstream.RefreshEvent) {
	if t == nil {
		return
	}
	switch event.Type {
	case dashboardstream.RefreshEventFilterOptions:
		t.dashboardTargetObserved("filter_options", "success")
	case dashboardstream.RefreshEventVisual:
		t.dashboardTargetObserved("visual", "success")
	case dashboardstream.RefreshEventTable:
		t.dashboardTargetObserved("table", "success")
	case dashboardstream.RefreshEventTableCountErr:
		t.dashboardTargetObserved("table_count", "error")
	case dashboardstream.RefreshEventTargetError:
		kind := event.Target
		if prefix, _, ok := strings.Cut(kind, ":"); ok {
			kind = prefix
		}
		t.dashboardTargetObserved(kind, "error")
	}
}

func dashboardCommandLabel(value string) string {
	value = normalizedMetricLabel(value)
	switch value {
	case "initial", "reload", "reset_filters", "filter_change", "select", "clear_selection", "table_window", "refresh_materializations":
		return value
	default:
		return "other"
	}
}

func dashboardOutcomeLabel(value string) string {
	value = normalizedMetricLabel(value)
	switch value {
	case "complete", "partial", "error", "canceled":
		return value
	default:
		return "other"
	}
}

func dashboardStageLabel(value string) string {
	value = normalizedMetricLabel(value)
	switch value {
	case "end_to_end", "target_execution", "admission_wait", "connection_wait", "planning", "database", "execution":
		return value
	default:
		return "other"
	}
}

func dashboardCacheLabel(value string) string {
	value = normalizedMetricLabel(value)
	switch value {
	case "hit", "miss", "coalesced", "disabled", "error":
		return value
	default:
		return "other"
	}
}

func dashboardTargetKindLabel(value string) string {
	value = normalizedMetricLabel(value)
	switch value {
	case "filter_options", "visual", "table", "table_count", "refresh":
		return value
	default:
		return "other"
	}
}

func dashboardTargetOutcomeLabel(value string) string {
	value = normalizedMetricLabel(value)
	switch value {
	case "success", "error", "canceled":
		return value
	default:
		return "other"
	}
}

func normalizedMetricLabel(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, "-", "_")
	value = strings.ReplaceAll(value, " ", "_")
	switch value {
	case "endtoend":
		return "end_to_end"
	case "admissionwait":
		return "admission_wait"
	case "connectionwait":
		return "connection_wait"
	case "targetexecution":
		return "target_execution"
	default:
		return value
	}
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
