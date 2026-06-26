package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Yacobolo/libredash/internal/dashboard"
)

func TestBIAPIListResponsesUseStandardEnvelope(t *testing.T) {
	server := NewWithOptions(fakeMetrics{}, Options{Store: testStore(t), DefaultWorkspaceID: "test"})

	for _, tc := range []struct {
		path string
		name string
	}{
		{path: "/api/v1/workspaces/test/dashboards?limit=1", name: "dashboards"},
		{path: "/api/v1/workspaces/test/semantic-models?limit=1", name: "semantic models"},
	} {
		req := httptest.NewRequest(http.MethodGet, tc.path, nil)
		req.Header.Set("Accept", "application/json")
		rec := httptest.NewRecorder()
		server.Routes().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status=%d body=%s", tc.path, rec.Code, rec.Body.String())
		}
		var response struct {
			Items []map[string]any `json:"items"`
			Page  struct {
				NextCursor string `json:"nextCursor"`
			} `json:"page"`
			Dashboards any `json:"dashboards"`
			Models     any `json:"models"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
			t.Fatalf("decode %s response: %v body=%s", tc.name, err, rec.Body.String())
		}
		if len(response.Items) != 1 {
			t.Fatalf("%s items = %#v", tc.name, response.Items)
		}
		if response.Dashboards != nil || response.Models != nil {
			t.Fatalf("%s response leaked legacy wrapper: %s", tc.name, rec.Body.String())
		}
	}

	for _, tc := range []struct {
		path string
		want string
	}{
		{path: "/api/v1/workspaces/test/dashboards/executive-sales", want: `"detail_tools"`},
		{path: "/api/v1/workspaces/test/semantic-models/test", want: `"model_tables"`},
	} {
		req := httptest.NewRequest(http.MethodGet, tc.path, nil)
		req.Header.Set("Accept", "application/json")
		rec := httptest.NewRecorder()
		server.Routes().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), tc.want) {
			t.Fatalf("%s status=%d body=%s", tc.path, rec.Code, rec.Body.String())
		}
	}
}

func TestBIAPIListPaginationRejectsMalformedLimit(t *testing.T) {
	server := NewWithOptions(fakeMetrics{}, Options{Store: testStore(t), DefaultWorkspaceID: "test"})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/workspaces/test/dashboards?limit=oops", nil)
	req.Header.Set("Accept", "application/json")
	rec := httptest.NewRecorder()
	server.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want=%d body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	assertAPIError(t, rec, http.StatusBadRequest, "limit")
}

func TestBIAPIQueriesBoundRowsAndPageData(t *testing.T) {
	server := NewWithOptions(manyRowsMetrics{}, Options{Store: testStore(t), DefaultWorkspaceID: "test"})

	pageReq := httptest.NewRequest(http.MethodPost, "/api/v1/workspaces/test/dashboards/executive-sales/pages/overview/query", strings.NewReader(`{"filters":{"controls":{"state":{"type":"multi_select","operator":"in","values":["SP"]}}}}`))
	pageReq.Header.Set("Accept", "application/json")
	pageReq.Header.Set("Content-Type", "application/json")
	pageRec := httptest.NewRecorder()
	server.Routes().ServeHTTP(pageRec, pageReq)
	if pageRec.Code != http.StatusOK || !strings.Contains(pageRec.Body.String(), `"visuals"`) {
		t.Fatalf("page query status=%d body=%s", pageRec.Code, pageRec.Body.String())
	}

	tableReq := httptest.NewRequest(http.MethodPost, "/api/v1/workspaces/test/dashboards/executive-sales/tables/orders/query", strings.NewReader(`{"pageId":"overview","count":500}`))
	tableReq.Header.Set("Accept", "application/json")
	tableReq.Header.Set("Content-Type", "application/json")
	tableRec := httptest.NewRecorder()
	server.Routes().ServeHTTP(tableRec, tableReq)
	if tableRec.Code != http.StatusOK {
		t.Fatalf("table query status=%d body=%s", tableRec.Code, tableRec.Body.String())
	}
	var table dashboard.Table
	if err := json.Unmarshal(tableRec.Body.Bytes(), &table); err != nil {
		t.Fatalf("decode table: %v body=%s", err, tableRec.Body.String())
	}
	if table.AvailableRows != 50 || len(table.Blocks["a"].Rows) != 50 {
		t.Fatalf("table not capped to 50 rows: %#v", table)
	}
}

func TestBIAPIDashboardVisualDataSurface(t *testing.T) {
	server := NewWithOptions(fakeMetrics{}, Options{Store: testStore(t), DefaultWorkspaceID: "test"})

	componentReq := httptest.NewRequest(http.MethodGet, "/api/v1/workspaces/test/dashboards/executive-sales/pages/overview/components?limit=2", nil)
	componentReq.Header.Set("Accept", "application/json")
	componentRec := httptest.NewRecorder()
	server.Routes().ServeHTTP(componentRec, componentReq)
	if componentRec.Code != http.StatusOK {
		t.Fatalf("components status=%d body=%s", componentRec.Code, componentRec.Body.String())
	}
	var components struct {
		Items []struct {
			ID        string  `json:"id"`
			Kind      string  `json:"kind"`
			Ref       string  `json:"ref"`
			Title     string  `json:"title"`
			Placement any     `json:"placement"`
			X         float64 `json:"x"`
		} `json:"items"`
		Page struct {
			NextCursor string `json:"nextCursor"`
		} `json:"page"`
	}
	if err := json.Unmarshal(componentRec.Body.Bytes(), &components); err != nil {
		t.Fatalf("decode components: %v body=%s", err, componentRec.Body.String())
	}
	if len(components.Items) != 2 || components.Items[1].ID != "state-filter" || components.Page.NextCursor == "" {
		t.Fatalf("components response = %#v", components)
	}

	visualReq := httptest.NewRequest(http.MethodGet, "/api/v1/workspaces/test/dashboards/executive-sales/pages/overview/visuals/orders", nil)
	visualReq.Header.Set("Accept", "application/json")
	visualRec := httptest.NewRecorder()
	server.Routes().ServeHTTP(visualRec, visualReq)
	if visualRec.Code != http.StatusOK || !strings.Contains(visualRec.Body.String(), `"title":"Orders"`) || !strings.Contains(visualRec.Body.String(), `"componentId":"orders-chart"`) {
		t.Fatalf("visual describe status=%d body=%s", visualRec.Code, visualRec.Body.String())
	}

	dataReq := httptest.NewRequest(http.MethodPost, "/api/v1/workspaces/test/dashboards/executive-sales/pages/overview/visuals/orders/data", strings.NewReader(`{"filters":{"controls":{"state":{"type":"multi_select","operator":"in","values":["SP"]}}}}`))
	dataReq.Header.Set("Accept", "application/json")
	dataReq.Header.Set("Content-Type", "application/json")
	dataRec := httptest.NewRecorder()
	server.Routes().ServeHTTP(dataRec, dataReq)
	if dataRec.Code != http.StatusOK || !strings.Contains(dataRec.Body.String(), `"data"`) || !strings.Contains(dataRec.Body.String(), `"delivered"`) {
		t.Fatalf("visual data status=%d body=%s", dataRec.Code, dataRec.Body.String())
	}

	tableReq := httptest.NewRequest(http.MethodPost, "/api/v1/workspaces/test/dashboards/executive-sales/pages/overview/tables/orders/data", strings.NewReader(`{"count":10}`))
	tableReq.Header.Set("Accept", "application/json")
	tableReq.Header.Set("Content-Type", "application/json")
	tableRec := httptest.NewRecorder()
	server.Routes().ServeHTTP(tableRec, tableReq)
	if tableRec.Code != http.StatusOK || !strings.Contains(tableRec.Body.String(), `"order_id":"o1"`) {
		t.Fatalf("table data status=%d body=%s", tableRec.Code, tableRec.Body.String())
	}

	filterReq := httptest.NewRequest(http.MethodPost, "/api/v1/workspaces/test/dashboards/executive-sales/pages/overview/filters/state/options?limit=1", strings.NewReader(`{}`))
	filterReq.Header.Set("Accept", "application/json")
	filterReq.Header.Set("Content-Type", "application/json")
	filterRec := httptest.NewRecorder()
	server.Routes().ServeHTTP(filterRec, filterReq)
	if filterRec.Code != http.StatusOK || !strings.Contains(filterRec.Body.String(), `"items"`) || !strings.Contains(filterRec.Body.String(), `"SP"`) {
		t.Fatalf("filter options status=%d body=%s", filterRec.Code, filterRec.Body.String())
	}
}

func TestBIAPIDashboardVisualDataSurfaceNotFoundAndMalformedBody(t *testing.T) {
	server := NewWithOptions(fakeMetrics{}, Options{Store: testStore(t), DefaultWorkspaceID: "test"})

	for _, tc := range []struct {
		method string
		path   string
	}{
		{method: http.MethodGet, path: "/api/v1/workspaces/test/dashboards/executive-sales/pages/overview/visuals/missing"},
		{method: http.MethodPost, path: "/api/v1/workspaces/test/dashboards/executive-sales/pages/overview/visuals/missing/data"},
		{method: http.MethodPost, path: "/api/v1/workspaces/test/dashboards/executive-sales/pages/overview/tables/missing/data"},
		{method: http.MethodPost, path: "/api/v1/workspaces/test/dashboards/executive-sales/pages/overview/filters/missing/options"},
	} {
		req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(`{}`))
		req.Header.Set("Accept", "application/json")
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		server.Routes().ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("%s %s status=%d body=%s", tc.method, tc.path, rec.Code, rec.Body.String())
		}
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/workspaces/test/dashboards/executive-sales/pages/overview/visuals/orders/data", strings.NewReader(`{"filters":`))
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("malformed status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestBIAPISemanticDatasetSurface(t *testing.T) {
	server := NewWithOptions(fakeMetrics{}, Options{Store: testStore(t), DefaultWorkspaceID: "test"})

	for _, tc := range []struct {
		method string
		path   string
		body   string
		want   []string
	}{
		{
			method: http.MethodGet,
			path:   "/api/v1/workspaces/test/semantic-models/test/datasets?limit=1",
			want:   []string{`"items"`, `"id":"orders"`, `"page"`},
		},
		{
			method: http.MethodGet,
			path:   "/api/v1/workspaces/test/semantic-models/test/datasets/orders",
			want:   []string{`"primaryKey":"order_id"`, `"grain":"order_id"`},
		},
		{
			method: http.MethodGet,
			path:   "/api/v1/workspaces/test/semantic-models/test/datasets/orders/fields?limit=3",
			want:   []string{`"kind":"dimension"`, `"kind":"measure"`, `"order_count"`},
		},
		{
			method: http.MethodPost,
			path:   "/api/v1/workspaces/test/semantic-models/test/datasets/orders/query",
			body:   `{"dimensions":[{"field":"orders.status","alias":"status"}],"measures":[{"field":"order_count"}],"sort":[{"field":"status","direction":"asc"}],"limit":1}`,
			want:   []string{`"columns"`, `"items"`, `"delivered"`, `"nextCursor"`},
		},
		{
			method: http.MethodPost,
			path:   "/api/v1/workspaces/test/semantic-models/test/datasets/orders/preview",
			body:   `{"dimensions":[{"field":"orders.order_id"},{"field":"orders.status"}],"sort":[{"field":"order_id","direction":"asc"}],"limit":1}`,
			want:   []string{`"order_id"`, `"o1"`, `"nextCursor"`},
		},
		{
			method: http.MethodPost,
			path:   "/api/v1/workspaces/test/semantic-models/test/datasets/orders/query/explain",
			body:   `{"dimensions":[{"field":"orders.status","alias":"status"}],"measures":[{"field":"order_count"}],"sort":[{"field":"status","direction":"asc"}]}`,
			want:   []string{`"mode":"query"`, `"sql"`, `"columns"`},
		},
		{
			method: http.MethodPost,
			path:   "/api/v1/workspaces/test/semantic-models/test/datasets/orders/preview/explain",
			body:   `{"dimensions":[{"field":"orders.order_id"}],"sort":[{"field":"order_id","direction":"asc"}]}`,
			want:   []string{`"mode":"preview"`, `"sql"`, `"columns"`},
		},
	} {
		t.Run(tc.path, func(t *testing.T) {
			body := strings.NewReader(tc.body)
			if tc.body == "" {
				body = strings.NewReader(`{}`)
			}
			req := httptest.NewRequest(tc.method, tc.path, body)
			req.Header.Set("Accept", "application/json")
			if tc.method == http.MethodPost {
				req.Header.Set("Content-Type", "application/json")
			}
			rec := httptest.NewRecorder()
			server.Routes().ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
			}
			for _, want := range tc.want {
				if !strings.Contains(rec.Body.String(), want) {
					t.Fatalf("body missing %q: %s", want, rec.Body.String())
				}
			}
		})
	}
}

func TestBIAPISemanticDatasetErrors(t *testing.T) {
	server := NewWithOptions(fakeMetrics{}, Options{Store: testStore(t), DefaultWorkspaceID: "test"})

	for _, tc := range []struct {
		method string
		path   string
		body   string
		status int
	}{
		{method: http.MethodGet, path: "/api/v1/workspaces/test/semantic-models/test/datasets/missing", status: http.StatusNotFound},
		{method: http.MethodPost, path: "/api/v1/workspaces/test/semantic-models/test/datasets/orders/query", body: `{"dimensions":[{"field":"missing.field"}]}`, status: http.StatusBadRequest},
		{method: http.MethodPost, path: "/api/v1/workspaces/test/semantic-models/test/datasets/orders/query", body: `{"dimensions":[{"field":"orders.status"}],"sort":[{"field":"missing"}]}`, status: http.StatusBadRequest},
		{method: http.MethodPost, path: "/api/v1/workspaces/test/semantic-models/test/datasets/orders/query", body: `{"dimensions":`, status: http.StatusBadRequest},
	} {
		req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
		req.Header.Set("Accept", "application/json")
		if tc.method == http.MethodPost {
			req.Header.Set("Content-Type", "application/json")
		}
		rec := httptest.NewRecorder()
		server.Routes().ServeHTTP(rec, req)
		if rec.Code != tc.status {
			t.Fatalf("%s %s status=%d want=%d body=%s", tc.method, tc.path, rec.Code, tc.status, rec.Body.String())
		}
	}
}

type manyRowsMetrics struct {
	fakeMetrics
}

func (manyRowsMetrics) QueryTablePage(_ context.Context, _ string, _ string, _ dashboard.Filters, request dashboard.TableRequest) (dashboard.Table, error) {
	rows := make([]map[string]any, 0, request.Count)
	for i := 0; i < request.Count; i++ {
		rows = append(rows, map[string]any{"order_id": i})
	}
	return dashboard.Table{
		Title:         "Orders",
		AvailableRows: len(rows),
		Blocks:        map[string]dashboard.TableBlock{"a": {Rows: rows}},
	}, nil
}
