package integration

import (
	"context"
	"encoding/json"
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
	"github.com/Yacobolo/libredash/internal/app"
	reportdef "github.com/Yacobolo/libredash/internal/dashboard/report"
	dashboardruntime "github.com/Yacobolo/libredash/internal/dashboard/runtime"
	"github.com/Yacobolo/libredash/internal/testutil/ssetest"
)

type harness struct {
	handler http.Handler
}

type harnessConfig struct {
	catalogPath string
	fixture     func(t *testing.T, dir string)
}

type harnessOption func(*harnessConfig)

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

	return &harness{
		handler: app.New(metrics).Routes(),
	}
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

	writeFixture(t, dir, "olist_orders_dataset.csv", `order_id,customer_id,order_status,order_purchase_timestamp,order_delivered_customer_date
o1,c1,delivered,2018-01-10 10:00:00,2018-01-14 10:00:00
o2,c2,shipped,2017-06-10 10:00:00,2017-06-20 10:00:00
`)
	writeFixture(t, dir, "olist_order_items_dataset.csv", `order_id,order_item_id,product_id,price,freight_value
o1,1,p1,100.00,10.00
o2,1,p2,50.00,5.00
`)
	writeFixture(t, dir, "olist_order_payments_dataset.csv", `order_id,payment_value
o1,110.00
o2,55.00
`)
	writeFixture(t, dir, "olist_products_dataset.csv", `product_id,product_category_name
p1,beleza_saude
p2,relogios_presentes
`)
	writeFixture(t, dir, "olist_customers_dataset.csv", `customer_id,customer_state
c1,SP
c2,RJ
`)
	writeFixture(t, dir, "olist_order_reviews_dataset.csv", `review_id,order_id,review_score
r1,o1,5
r2,o2,3
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
