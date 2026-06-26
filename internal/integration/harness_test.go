package integration

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	analyticsduckdb "github.com/Yacobolo/libredash/internal/analytics/duckdb"
	materializeruntime "github.com/Yacobolo/libredash/internal/analytics/materialize"
	semanticmodel "github.com/Yacobolo/libredash/internal/analytics/model"
	"github.com/Yacobolo/libredash/internal/app"
	"github.com/Yacobolo/libredash/internal/dashboard"
	reportdef "github.com/Yacobolo/libredash/internal/dashboard/report"
	dashboardruntime "github.com/Yacobolo/libredash/internal/dashboard/runtime"
	"github.com/Yacobolo/libredash/internal/testutil/ssetest"
)

type harness struct {
	handler http.Handler
	server  *httptest.Server
}

type harnessConfig struct {
	catalogPath string
	fixture     func(t *testing.T, dir string)
	wrapMetrics func(*dashboardruntime.Service) integrationMetrics
}

type harnessOption func(*harnessConfig)

type integrationMetrics interface {
	Catalog() dashboard.Catalog
	DefaultDashboardID() string
	ModelIDForDashboard(dashboardID string) string
	Report(dashboardID string) (reportdef.Dashboard, *semanticmodel.Model, bool)
	SemanticModel(modelID string) (*semanticmodel.Model, bool)
	DefaultFilters(dashboardID string) dashboard.Filters
	NormalizeTableRequest(dashboardID string, request dashboard.TableRequest) dashboard.TableRequest
	QueryDashboard(ctx context.Context, dashboardID string, filters dashboard.Filters) (dashboard.Patch, error)
	QueryDashboardPage(ctx context.Context, dashboardID, pageID string, filters dashboard.Filters) (dashboard.Patch, error)
	QueryTable(ctx context.Context, dashboardID string, filters dashboard.Filters, request dashboard.TableRequest) (dashboard.Table, error)
	QueryTablePage(ctx context.Context, dashboardID, pageID string, filters dashboard.Filters, request dashboard.TableRequest) (dashboard.Table, error)
	QuerySemantic(ctx context.Context, modelID string, request reportdef.AggregateQuery) (reportdef.QueryRows, error)
	PreviewSemantic(ctx context.Context, modelID string, request reportdef.RowQuery) (reportdef.QueryRows, error)
	RefreshMaterializations(ctx context.Context, modelID string) error
	DataDir() string
	Pages(dashboardID string) []dashboard.Page
}

func withCatalog(path string) harnessOption {
	return func(config *harnessConfig) {
		config.catalogPath = path
	}
}

func withOlistFixture(fixture func(t *testing.T, dir string)) harnessOption {
	return func(config *harnessConfig) {
		config.fixture = fixture
	}
}

func withMetricsWrapper(wrapper func(*dashboardruntime.Service) integrationMetrics) harnessOption {
	return func(config *harnessConfig) {
		config.wrapMetrics = wrapper
	}
}

func newHarness(t *testing.T, opts ...harnessOption) *harness {
	t.Helper()

	config := harnessConfig{
		fixture: writeMinimalOlistFixture,
	}
	for _, opt := range opts {
		opt(&config)
	}
	if config.catalogPath == "" {
		config.catalogPath = discoverCatalogPath(t)
	}

	dataDir := t.TempDir()
	duckDBDir := t.TempDir()
	config.fixture(t, dataDir)

	metrics, err := dashboardruntime.NewFromCatalog(dataDir, config.catalogPath, duckDBDir, integrationDataRuntimeFactory{})
	if err != nil {
		t.Fatalf("create dashboard runtime: %v", err)
	}
	t.Cleanup(func() { _ = metrics.Close() })

	metricsForApp := integrationMetrics(metrics)
	if config.wrapMetrics != nil {
		metricsForApp = config.wrapMetrics(metrics)
	}

	h := &harness{
		handler: app.New(metricsForApp).Routes(),
	}
	h.server = httptest.NewServer(h.handler)
	t.Cleanup(h.server.Close)
	return h
}

func (h *harness) getUpdates(t *testing.T, dashboardID, pageID string, signals map[string]any) string {
	t.Helper()

	encodedSignals, err := json.Marshal(signals)
	if err != nil {
		t.Fatalf("marshal Datastar signals: %v", err)
	}
	values := url.Values{}
	values.Set("dashboard", dashboardID)
	values.Set("page", pageID)
	values.Set("datastar", string(encodedSignals))

	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()
	req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/updates?"+values.Encode(), nil)
	rec := httptest.NewRecorder()

	h.handler.ServeHTTP(rec, req)
	if got := rec.Code; got != http.StatusOK {
		t.Fatalf("GET /updates status = %d, body:\n%s", got, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/event-stream") {
		t.Fatalf("GET /updates content type = %q, want text/event-stream", got)
	}
	return rec.Body.String()
}

func (h *harness) getUpdatesSignals(t *testing.T, dashboardID, pageID string, signals map[string]any) []map[string]any {
	t.Helper()

	body := h.getUpdates(t, dashboardID, pageID, signals)
	patches := ssetest.PatchSignals(t, body)
	if len(patches) == 0 {
		t.Fatalf("GET /updates did not stream Datastar patch signals:\n%s", body)
	}
	return patches
}

func (h *harness) openUpdatesStream(t *testing.T, dashboardID, pageID string, signals map[string]any) *streamClient {
	t.Helper()

	serverURL := h.serverURL(t)
	encodedSignals, err := json.Marshal(signals)
	if err != nil {
		t.Fatalf("marshal Datastar signals: %v", err)
	}
	values := url.Values{}
	values.Set("dashboard", dashboardID)
	values.Set("page", pageID)
	values.Set("datastar", string(encodedSignals))

	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, serverURL+"/updates?"+values.Encode(), nil)
	if err != nil {
		cancel()
		t.Fatalf("create updates request: %v", err)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		cancel()
		t.Fatalf("open updates stream: %v", err)
	}
	if res.StatusCode != http.StatusOK {
		defer res.Body.Close()
		body, _ := io.ReadAll(res.Body)
		cancel()
		t.Fatalf("GET /updates status = %d, body:\n%s", res.StatusCode, string(body))
	}
	if got := res.Header.Get("Content-Type"); !strings.HasPrefix(got, "text/event-stream") {
		_ = res.Body.Close()
		cancel()
		t.Fatalf("GET /updates content type = %q, want text/event-stream", got)
	}

	client := &streamClient{
		cancel:  cancel,
		body:    res.Body,
		patches: make(chan map[string]any, 16),
		errs:    make(chan error, 1),
	}
	go client.read()
	t.Cleanup(client.close)
	return client
}

func (h *harness) postCommand(t *testing.T, path string, signals map[string]any) int {
	t.Helper()

	encodedSignals, err := json.Marshal(signals)
	if err != nil {
		t.Fatalf("marshal Datastar signals: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, h.serverURL(t)+path, bytes.NewReader(encodedSignals))
	if err != nil {
		t.Fatalf("create command request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", path, err)
	}
	defer res.Body.Close()
	if res.StatusCode >= 400 {
		body, _ := io.ReadAll(res.Body)
		t.Fatalf("POST %s status = %d, body:\n%s", path, res.StatusCode, string(body))
	}
	return res.StatusCode
}

func (h *harness) serverURL(t *testing.T) string {
	t.Helper()
	return h.server.URL
}

type streamClient struct {
	cancel  context.CancelFunc
	body    io.ReadCloser
	patches chan map[string]any
	errs    chan error
}

func (c *streamClient) nextPatch(t *testing.T) map[string]any {
	t.Helper()
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	select {
	case patch, ok := <-c.patches:
		if !ok {
			t.Fatal("updates stream closed before next patch")
		}
		return patch
	case err := <-c.errs:
		if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, http.ErrAbortHandler) {
			t.Fatal("updates stream closed before next patch")
		}
		t.Fatalf("read updates stream: %v", err)
	case <-timer.C:
		t.Fatal("timed out waiting for next updates patch")
	}
	return nil
}

func (c *streamClient) expectNoPatch(t *testing.T, duration time.Duration) {
	t.Helper()
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case patch, ok := <-c.patches:
		if !ok {
			return
		}
		t.Fatalf("unexpected updates patch: %#v", patch)
	case err := <-c.errs:
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Fatalf("read updates stream: %v", err)
		}
	case <-timer.C:
	}
}

func (c *streamClient) close() {
	c.cancel()
	_ = c.body.Close()
}

func (c *streamClient) read() {
	defer close(c.patches)

	reader := bufio.NewReader(c.body)
	var event strings.Builder
	for {
		line, err := reader.ReadString('\n')
		if len(line) > 0 {
			event.WriteString(line)
			if line == "\n" || line == "\r\n" {
				events := ssetest.ParseEvents(event.String())
				event.Reset()
				for _, evt := range events {
					patch, ok, err := ssetest.DecodePatchSignalEvent(evt)
					if err != nil {
						c.errs <- err
						return
					}
					if ok {
						c.patches <- patch
					}
				}
			}
		}
		if err == nil {
			continue
		}
		if errors.Is(err, io.EOF) {
			return
		}
		c.errs <- fmt.Errorf("read SSE event: %w", err)
		return
	}
}

func discoverCatalogPath(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	for {
		candidate := filepath.Join(dir, "dashboards", "catalog.yaml")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find dashboards/catalog.yaml")
		}
		dir = parent
	}
}

func writeMinimalOlistFixture(t *testing.T, dir string) {
	t.Helper()

	writeFixture(t, dir, "olist_orders_dataset.csv", `order_id,customer_id,order_status,order_purchase_timestamp,order_approved_at,order_delivered_carrier_date,order_delivered_customer_date,order_estimated_delivery_date
o1,c1,delivered,2018-01-10 10:00:00,2018-01-10 11:00:00,2018-01-11 10:00:00,2018-01-14 10:00:00,2018-01-20 10:00:00
o2,c2,shipped,2017-06-10 10:00:00,2017-06-10 11:00:00,2017-06-12 10:00:00,2017-06-20 10:00:00,2017-06-25 10:00:00
`)
	writeFixture(t, dir, "olist_order_items_dataset.csv", `order_id,order_item_id,product_id,seller_id,shipping_limit_date,price,freight_value
o1,1,p1,s1,2018-01-12 10:00:00,100.00,10.00
o2,1,p2,s2,2017-06-15 10:00:00,50.00,5.00
`)
	writeFixture(t, dir, "olist_order_payments_dataset.csv", `order_id,payment_sequential,payment_type,payment_installments,payment_value
o1,1,credit_card,1,110.00
o2,1,boleto,1,55.00
`)
	writeFixture(t, dir, "olist_products_dataset.csv", `product_id,product_category_name,product_name_lenght,product_description_lenght,product_photos_qty,product_weight_g,product_length_cm,product_height_cm,product_width_cm
p1,beleza_saude,10,20,1,500,20,10,15
p2,relogios_presentes,12,22,1,700,25,12,16
`)
	writeFixture(t, dir, "olist_customers_dataset.csv", `customer_id,customer_unique_id,customer_zip_code_prefix,customer_city,customer_state
c1,u1,01000,sao paulo,SP
c2,u2,20000,rio de janeiro,RJ
`)
	writeFixture(t, dir, "olist_order_reviews_dataset.csv", `review_id,order_id,review_score,review_comment_title,review_comment_message,review_creation_date,review_answer_timestamp
r1,o1,5,great,fast,2018-01-15,2018-01-16
r2,o2,3,ok,slow,2017-06-21,2017-06-22
`)
	writeFixture(t, dir, "product_category_name_translation.csv", `product_category_name,product_category_name_english
beleza_saude,health_beauty
relogios_presentes,watches_gifts
`)
}

func writeFixture(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("write fixture %s: %v", name, err)
	}
}

type integrationDataRuntimeFactory struct{}

func (integrationDataRuntimeFactory) OpenDashboardDataRuntime(ctx context.Context, config dashboardruntime.DataRuntimeConfig) (dashboardruntime.DataRuntime, error) {
	runtime, err := analyticsduckdb.OpenMaterializeRuntime(ctx, materializeruntime.RuntimeConfig{
		ModelID: config.ModelID,
		Model:   config.Model,
		DataDir: config.DataDir,
		DBDir:   config.DBDir,
	})
	if err != nil {
		return nil, err
	}
	return integrationDataRuntime{
		runtime: runtime,
		data:    reportdef.NewAnalyticsDataService(runtime.Queries()),
	}, nil
}

type integrationDataRuntime struct {
	runtime *materializeruntime.Runtime
	data    reportdef.DataService
}

func (r integrationDataRuntime) Query(ctx context.Context, request reportdef.AggregateQuery) (reportdef.QueryRows, error) {
	return r.data.Query(ctx, request)
}

func (r integrationDataRuntime) Rows(ctx context.Context, request reportdef.RowQuery) (reportdef.QueryRows, error) {
	return r.data.Rows(ctx, request)
}

func (r integrationDataRuntime) Count(ctx context.Context, request reportdef.CountQuery) (int, error) {
	return r.data.Count(ctx, request)
}

func (r integrationDataRuntime) Histogram(ctx context.Context, request reportdef.RawValueQuery, binCount int) ([]reportdef.HistogramBin, error) {
	return r.data.Histogram(ctx, request, binCount)
}

func (r integrationDataRuntime) Distribution(ctx context.Context, request reportdef.RawValueQuery, sort []reportdef.QuerySort, limit int) (reportdef.QueryRows, error) {
	return r.data.Distribution(ctx, request, sort, limit)
}

func (r integrationDataRuntime) Refresh(ctx context.Context) error {
	return r.runtime.Refresh(ctx)
}

func (r integrationDataRuntime) Close() error {
	return r.runtime.Close()
}

func (r integrationDataRuntime) LastRefresh() time.Time {
	return r.runtime.LastRefresh()
}
