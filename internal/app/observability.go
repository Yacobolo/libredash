package app

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	dashboardstream "github.com/Yacobolo/leapview/internal/dashboard/stream"
	"github.com/Yacobolo/leapview/internal/secret"
	visualizationir "github.com/Yacobolo/leapview/internal/visualization/ir"
	"github.com/Yacobolo/leapview/internal/workload"
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
	visualizationFrameRows        *prometheus.HistogramVec
	visualizationFrameBytes       *prometheus.HistogramVec
	visualizationCardinality      *prometheus.HistogramVec
	workloadRunning               *prometheus.GaugeVec
	workloadQueued                *prometheus.GaugeVec
	workloadBorrowed              *prometheus.GaugeVec
	workloadAdmissions            *prometheus.CounterVec
	workloadQueueWait             *prometheus.HistogramVec
	workloadExecution             *prometheus.HistogramVec
	workloadMu                    sync.Mutex
	workloadLabels                map[string][2]string
	publicDashboardDocuments      *prometheus.CounterVec
	publicDashboardStreams        *prometheus.GaugeVec
	publicDashboardCommands       *prometheus.CounterVec
	publicDashboardRateLimits     *prometheus.CounterVec
	handlerOpts                   promhttp.HandlerOpts
}

func newHTTPTelemetry() *httpTelemetry {
	registry := prometheus.NewRegistry()
	telemetry := &httpTelemetry{
		registry: registry,
		requests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "leapview_http_requests_total",
			Help: "Total HTTP requests served by LeapView.",
		}, []string{"method", "route", "status"}),
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "leapview_http_request_duration_seconds",
			Help:    "HTTP request duration in seconds.",
			Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30},
		}, []string{"method", "route", "status"}),
		response: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "leapview_http_response_size_bytes",
			Help:    "HTTP response size in bytes.",
			Buckets: prometheus.ExponentialBuckets(128, 2, 16),
		}, []string{"method", "route", "status"}),
		inFlight: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "leapview_http_requests_in_flight",
			Help: "HTTP requests currently being served by LeapView.",
		}),
		dashboardRefreshDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "leapview_dashboard_refresh_duration_seconds",
			Help:    "End-to-end dashboard refresh duration in seconds.",
			Buckets: []float64{0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30},
		}, []string{"command", "outcome"}),
		dashboardStageDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "leapview_dashboard_refresh_stage_duration_seconds",
			Help:    "Dashboard refresh stage duration in seconds.",
			Buckets: []float64{0.001, 0.0025, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
		}, []string{"stage", "outcome"}),
		dashboardRefreshInFlight: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "leapview_dashboard_refreshes_in_flight",
			Help: "Dashboard refreshes currently in flight.",
		}, []string{"command"}),
		dashboardRefreshCancellations: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "leapview_dashboard_refresh_cancellations_total",
			Help: "Total dashboard refresh cancellations.",
		}, []string{"command"}),
		dashboardCacheOutcomes: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "leapview_dashboard_cache_outcomes_total",
			Help: "Dashboard query cache outcomes.",
		}, []string{"outcome"}),
		dashboardTargetOutcomes: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "leapview_dashboard_target_outcomes_total",
			Help: "Dashboard refresh target outcomes.",
		}, []string{"kind", "outcome"}),
		visualizationFrameRows: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name: "leapview_visualization_frame_rows", Help: "Rows shaped for a visualization envelope without recording governed values.",
			Buckets: prometheus.ExponentialBuckets(1, 4, 10),
		}, []string{"kind"}),
		visualizationFrameBytes: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name: "leapview_visualization_frame_size_bytes", Help: "Encoded visualization frame size without recording governed values.",
			Buckets: prometheus.ExponentialBuckets(256, 4, 10),
		}, []string{"kind"}),
		visualizationCardinality: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name: "leapview_visualization_cardinality", Help: "Known visualization result cardinality.",
			Buckets: prometheus.ExponentialBuckets(1, 4, 10),
		}, []string{"kind"}),
		workloadRunning:    prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "leapview_workload_running", Help: "Currently running workload operations."}, []string{"class", "workspace"}),
		workloadQueued:     prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "leapview_workload_queued", Help: "Currently queued workload operations."}, []string{"class", "workspace"}),
		workloadBorrowed:   prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "leapview_workload_borrowed", Help: "Capacity currently borrowed above each class reservation."}, []string{"class"}),
		workloadAdmissions: prometheus.NewCounterVec(prometheus.CounterOpts{Name: "leapview_workload_admissions_total", Help: "Workload admission outcomes."}, []string{"class", "outcome", "reason"}),
		workloadQueueWait:  prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: "leapview_workload_queue_wait_seconds", Help: "Time spent waiting for workload admission.", Buckets: prometheus.ExponentialBuckets(0.001, 2, 17)}, []string{"class"}),
		workloadExecution:  prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: "leapview_workload_execution_duration_seconds", Help: "Admitted workload execution duration.", Buckets: prometheus.ExponentialBuckets(0.005, 2, 18)}, []string{"class"}),
		workloadLabels:     map[string][2]string{},
		publicDashboardDocuments: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "leapview_public_dashboard_documents_total",
			Help: "Public dashboard document load outcomes.",
		}, []string{"presentation", "outcome"}),
		publicDashboardStreams: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "leapview_public_dashboard_streams_active",
			Help: "Active anonymous dashboard streams.",
		}, []string{"presentation"}),
		publicDashboardCommands: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "leapview_public_dashboard_commands_total",
			Help: "Anonymous dashboard command attempts.",
		}, []string{"command", "outcome"}),
		publicDashboardRateLimits: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "leapview_public_dashboard_rate_limit_rejections_total",
			Help: "Anonymous dashboard requests rejected by public traffic family.",
		}, []string{"family"}),
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
		telemetry.visualizationFrameRows,
		telemetry.visualizationFrameBytes,
		telemetry.visualizationCardinality,
		telemetry.workloadRunning,
		telemetry.workloadQueued,
		telemetry.workloadBorrowed,
		telemetry.workloadAdmissions,
		telemetry.workloadQueueWait,
		telemetry.workloadExecution,
		telemetry.publicDashboardDocuments,
		telemetry.publicDashboardStreams,
		telemetry.publicDashboardCommands,
		telemetry.publicDashboardRateLimits,
	)
	return telemetry
}

func (t *httpTelemetry) ObserveWorkload(stats workload.Stats) {
	if t == nil {
		return
	}
	t.workloadMu.Lock()
	defer t.workloadMu.Unlock()
	for _, labels := range t.workloadLabels {
		t.workloadRunning.WithLabelValues(labels[0], labels[1]).Set(0)
		t.workloadQueued.WithLabelValues(labels[0], labels[1]).Set(0)
	}
	t.workloadLabels = map[string][2]string{}
	for class, classStats := range stats.Classes {
		classLabel := string(class)
		t.workloadBorrowed.WithLabelValues(classLabel).Set(float64(classStats.Borrowed))
		for workspace, workspaceStats := range classStats.Workspaces {
			labels := [2]string{classLabel, workspace}
			t.workloadLabels[classLabel+"\x00"+workspace] = labels
			t.workloadRunning.WithLabelValues(classLabel, workspace).Set(float64(workspaceStats.Running))
			t.workloadQueued.WithLabelValues(classLabel, workspace).Set(float64(workspaceStats.Queued))
		}
	}
}

func (t *httpTelemetry) ObserveAdmission(event workload.AdmissionEvent) {
	if t == nil {
		return
	}
	outcome := event.Outcome
	switch outcome {
	case "admitted", "rejected", "completed", "timeout", "canceled":
	default:
		outcome = "other"
	}
	reason := string(event.Reason)
	if reason == "" {
		reason = "none"
	}
	t.workloadAdmissions.WithLabelValues(string(event.Class), outcome, reason).Inc()
	if outcome == "admitted" || outcome == "rejected" {
		t.workloadQueueWait.WithLabelValues(string(event.Class)).Observe(event.QueueWait.Seconds())
	}
	if outcome == "completed" || outcome == "timeout" || outcome == "canceled" {
		t.workloadExecution.WithLabelValues(string(event.Class)).Observe(event.Execution.Seconds())
	}
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
		t.observeVisualizationEnvelope(event.Value)
	case dashboardstream.RefreshEventVisualMetadata:
		t.observeVisualizationEnvelope(event.Value)
	case dashboardstream.RefreshEventTargetError:
		kind := event.Target
		if prefix, _, ok := strings.Cut(kind, ":"); ok {
			kind = prefix
		}
		t.dashboardTargetObserved(kind, "error")
	}
}

func (t *httpTelemetry) observeVisualizationEnvelope(value any) {
	envelope, ok := value.(visualizationir.VisualizationEnvelope)
	if !ok {
		return
	}
	switch state := envelope.DataState.Value.(type) {
	case *visualizationir.InlineVisualizationDataState:
		rows := 0
		for _, dataset := range state.Datasets {
			rows += len(dataset.Rows)
		}
		t.observeVisualizationFrame("inline", rows, rows, envelope)
	case *visualizationir.WindowedVisualizationDataState:
		rows := 0
		for _, block := range state.Blocks {
			rows += len(block.Rows)
		}
		t.observeVisualizationFrame("windowed", rows, visualizationCardinality(state.Cardinality, state.AvailableRows), envelope)
	case *visualizationir.SpatialWindowedVisualizationDataState:
		rows := 0
		if state.Window != nil {
			rows = len(state.Window.Rows)
		}
		t.observeVisualizationFrame("spatial_windowed", rows, visualizationCardinality(state.Cardinality, int64(rows)), envelope)
	}
}

func visualizationCardinality(cardinality visualizationir.VisualizationCardinality, fallback int64) int {
	if cardinality.Count != nil {
		return int(*cardinality.Count)
	}
	return int(fallback)
}

func (t *httpTelemetry) observeVisualizationFrame(kind string, rows, cardinality int, value any) {
	if t == nil {
		return
	}
	t.visualizationFrameRows.WithLabelValues(kind).Observe(float64(max(rows, 0)))
	t.visualizationCardinality.WithLabelValues(kind).Observe(float64(max(cardinality, 0)))
	if encoded, err := json.Marshal(value); err == nil {
		t.visualizationFrameBytes.WithLabelValues(kind).Observe(float64(len(encoded)))
	}
}

func (t *httpTelemetry) publicDocumentObserved(presentation, outcome string) {
	if t == nil {
		return
	}
	if presentation != "embed" {
		presentation = "public"
	}
	if outcome != "success" {
		outcome = "not_found"
	}
	t.publicDashboardDocuments.WithLabelValues(presentation, outcome).Inc()
}

func (t *httpTelemetry) publicStreamStarted(presentation string) func() {
	if t == nil {
		return func() {}
	}
	if presentation != "embed" {
		presentation = "public"
	}
	t.publicDashboardStreams.WithLabelValues(presentation).Inc()
	return func() { t.publicDashboardStreams.WithLabelValues(presentation).Dec() }
}

func (t *httpTelemetry) publicCommandObserved(command, outcome string) {
	if t == nil {
		return
	}
	command = dashboardCommandLabel(command)
	if outcome != "accepted" {
		outcome = "rejected"
	}
	t.publicDashboardCommands.WithLabelValues(command, outcome).Inc()
}

func (t *httpTelemetry) publicRateLimitObserved(family string) {
	if t == nil {
		return
	}
	switch family {
	case "page", "command", "stream":
	default:
		family = "unknown"
	}
	t.publicDashboardRateLimits.WithLabelValues(family).Inc()
}

func dashboardCommandLabel(value string) string {
	value = normalizedMetricLabel(value)
	switch value {
	case "initial", "reload", "reset_filters", "filter_change", "select", "clear_selection", "visual_window", "refresh_materializations":
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
	case "end_to_end", "target_work_sum", "target_critical_path", "admission_wait", "connection_wait", "planning", "database", "execution":
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
	case "filter_options", "visual", "visual_count", "refresh":
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
	case "targetworksum":
		return "target_work_sum"
	case "targetcriticalpath":
		return "target_critical_path"
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
			w.Header().Set("WWW-Authenticate", `Bearer realm="leapview-metrics"`)
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
