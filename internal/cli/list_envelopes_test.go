package cli

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	cligen "github.com/Yacobolo/leapview/internal/cli/gen"
	"github.com/spf13/cobra"
)

func TestAgentConversationsDecodesEnvelopePreservingJSONOutput(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/agent/conversations" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		writeCLIJSON(t, w, map[string]any{
			"items": []map[string]any{{
				"id":          "conv_1",
				"principalId": "prn_1",
				"title":       "Ask",
				"status":      "active",
				"createdAt":   "2026-01-02T15:04:05Z",
				"updatedAt":   "2026-01-02T15:05:05Z",
			}},
			"page": map[string]any{"nextCursor": "opaque"},
		})
	}))
	defer server.Close()

	output := captureStdout(t, func() {
		err := runAgentConversations(context.Background(), &rootOptions{target: server.URL, token: "token", jsonOutput: true})
		if err != nil {
			t.Fatalf("run conversations: %v", err)
		}
	})

	var rows []map[string]any
	if err := json.Unmarshal([]byte(output), &rows); err != nil {
		t.Fatalf("decode output: %v output=%s", err, output)
	}
	if len(rows) != 1 || rows[0]["id"] != "conv_1" || rows[0]["title"] != "Ask" {
		t.Fatalf("rows = %#v", rows)
	}
	if strings.Contains(output, "nextCursor") || strings.Contains(output, `"items"`) {
		t.Fatalf("output leaked envelope:\n%s", output)
	}
}

func TestFriendlyListCommandsPassPaginationQuery(t *testing.T) {
	for _, tc := range []struct {
		name    string
		command func(context.Context, *rootOptions) *cobra.Command
		args    []string
		path    string
	}{
		{
			name:    "workspaces",
			command: workspacesCommand,
			args:    []string{"list"},
			path:    "/api/v1/workspaces",
		},
		{
			name:    "dashboards",
			command: dashboardsCommand,
			args:    []string{"list"},
			path:    "/api/v1/workspaces/test/dashboards",
		},
		{
			name:    "semantic-models",
			command: semanticModelsCommand,
			args:    []string{"list"},
			path:    "/api/v1/workspaces/test/semantic-models",
		},
		{
			name:    "agent conversations",
			command: agentCommand,
			args:    []string{"conversations"},
			path:    "/api/v1/agent/conversations",
		},
		{
			name:    "search",
			command: searchCommand,
			args:    []string{"orders", "--workspace", "test", "--type", "visual"},
			path:    "/api/v1/search",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != tc.path {
					t.Fatalf("path = %s want %s", r.URL.Path, tc.path)
				}
				if got := r.URL.Query().Get("limit"); got != "7" {
					t.Fatalf("limit = %q", got)
				}
				if got := r.URL.Query().Get("pageToken"); got != "cursor" {
					t.Fatalf("pageToken = %q", got)
				}
				if tc.name == "search" {
					if got := r.URL.Query().Get("q"); got != "orders" {
						t.Fatalf("q=%q", got)
					}
					if got := r.URL.Query().Get("type"); got != "visual" {
						t.Fatalf("type=%q", got)
					}
					if got := r.URL.Query().Get("workspace"); got != "test" {
						t.Fatalf("workspace=%q", got)
					}
				}
				writeCLIJSON(t, w, map[string]any{
					"items": []map[string]any{},
					"page":  map[string]any{"nextCursor": ""},
				})
			}))
			defer server.Close()

			opts := &rootOptions{workspaceID: "test"}
			cmd := tc.command(context.Background(), opts)
			args := append([]string{}, tc.args...)
			args = append(args, "--target", server.URL, "--token", "token", "--limit", "7", "--page-token", "cursor")
			cmd.SetArgs(args)
			captureStdout(t, func() {
				if err := cmd.Execute(); err != nil {
					t.Fatalf("run command: %v", err)
				}
			})
		})
	}
}

func TestSearchCommandRendersConciseRows(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/search" {
			t.Fatalf("path=%s", r.URL.Path)
		}
		if got := r.URL.Query().Get("q"); got != "orders" {
			t.Fatalf("q=%q", got)
		}
		writeCLIJSON(t, w, map[string]any{
			"items": []map[string]any{{
				"reference":   map[string]any{"workspaceId": "test", "type": "visual", "id": "executive-sales.orders"},
				"name":        "Orders",
				"description": "Orders visual on Overview.",
			}},
			"page": map[string]any{"nextCursor": ""},
		})
	}))
	defer server.Close()

	output := captureStdout(t, func() {
		cmd := searchCommand(context.Background(), &rootOptions{workspaceID: "test"})
		cmd.SetArgs([]string{"orders", "--target", server.URL, "--token", "token"})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("run search: %v", err)
		}
	})
	for _, want := range []string{"WORKSPACE", "TYPE", "NAME", "DESCRIPTION", "ID", "test", "visual", "Orders", "Orders visual on Overview."} {
		if !strings.Contains(output, want) {
			t.Fatalf("search output missing %q:\n%s", want, output)
		}
	}
	if strings.Contains(output, "items") || strings.Contains(output, "nextCursor") {
		t.Fatalf("search output leaked envelope:\n%s", output)
	}
}

func TestSearchGeneratedCLIMetadataUsesQueryAsOnlyPositionalArg(t *testing.T) {
	var spec cligen.APIGenCommandSpec
	for _, candidate := range cligen.APIGeneratedCommandSpecs {
		if candidate.OperationID == "search" {
			spec = candidate
			break
		}
	}
	if spec.OperationID == "" {
		t.Fatal("search CLI metadata missing")
	}
	if len(spec.Args) != 1 || spec.Args[0].Name != "q" || spec.Args[0].Source != "query" {
		t.Fatalf("search CLI args = %#v, want one query arg q", spec.Args)
	}
}

func TestDashboardDataCommandsUseGeneratedURLsAndBodies(t *testing.T) {
	for _, tc := range []struct {
		name     string
		args     []string
		method   string
		path     string
		wantBody []string
		response any
	}{
		{
			name:     "page",
			args:     []string{"page", "executive-sales", "overview"},
			method:   http.MethodGet,
			path:     "/api/v1/workspaces/test/dashboards/executive-sales/pages/overview",
			response: map[string]any{"id": "overview", "title": "Overview", "components": []map[string]any{}},
		},
		{
			name:     "visual describe",
			args:     []string{"visual", "executive-sales", "overview", "orders"},
			method:   http.MethodGet,
			path:     "/api/v1/workspaces/test/dashboards/executive-sales/pages/overview/visuals/orders",
			response: map[string]any{"id": "orders", "title": "Orders"},
		},
		{
			name:     "filter describe",
			args:     []string{"filter", "executive-sales", "overview", "state"},
			method:   http.MethodGet,
			path:     "/api/v1/workspaces/test/dashboards/executive-sales/pages/overview/filters/state",
			response: map[string]any{"definition": map[string]any{"id": "state"}, "binding": map[string]any{"key": "fb_state"}},
		},
		{
			name:     "visual data",
			args:     []string{"visual-data", "executive-sales", "overview", "orders", "--count", "7", "--filter-state-json", `{"version":"typed_v1","controls":{"fb_state":{"kind":"set","operator":"in","values":[{"kind":"string","value":"SP"}]}}}`},
			method:   http.MethodPost,
			path:     "/api/v1/workspaces/test/dashboards/executive-sales/pages/overview/visuals/orders/query",
			wantBody: []string{`"filterState"`, `"typed_v1"`, `"fb_state"`, `"limit":7`},
			response: map[string]any{"id": "orders", "data": []map[string]any{}},
		},
		{
			name:     "filter options",
			args:     []string{"filter-options", "executive-sales", "overview", "state", "--limit", "7", "--page-token", "cursor"},
			method:   http.MethodPost,
			path:     "/api/v1/workspaces/test/dashboards/executive-sales/pages/overview/filters/state/values",
			wantBody: []string{},
			response: map[string]any{"items": []map[string]any{}, "page": map[string]any{"nextCursor": ""}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != tc.method {
					t.Fatalf("method=%s want=%s", r.Method, tc.method)
				}
				if r.URL.Path != tc.path {
					t.Fatalf("path=%s want=%s", r.URL.Path, tc.path)
				}
				if tc.name == "filter options" {
					if got := r.URL.Query().Get("limit"); got != "7" {
						t.Fatalf("limit=%q", got)
					}
					if got := r.URL.Query().Get("pageToken"); got != "cursor" {
						t.Fatalf("pageToken=%q", got)
					}
				}
				body, err := io.ReadAll(r.Body)
				if err != nil {
					t.Fatalf("read body: %v", err)
				}
				for _, want := range tc.wantBody {
					if !strings.Contains(string(body), want) {
						t.Fatalf("body missing %q: %s", want, body)
					}
				}
				writeCLIJSON(t, w, tc.response)
			}))
			defer server.Close()

			opts := &rootOptions{workspaceID: "test"}
			cmd := dashboardsCommand(context.Background(), opts)
			args := append([]string{}, tc.args...)
			args = append(args, "--target", server.URL, "--token", "token")
			cmd.SetArgs(args)
			captureStdout(t, func() {
				if err := cmd.Execute(); err != nil {
					t.Fatalf("run command: %v", err)
				}
			})
		})
	}
}

func TestSemanticModelDatasetCommandsUseGeneratedURLsAndBodies(t *testing.T) {
	for _, tc := range []struct {
		name     string
		args     []string
		method   string
		path     string
		wantBody []string
		response any
	}{
		{
			name:     "datasets",
			args:     []string{"datasets", "test", "--limit", "7", "--page-token", "cursor"},
			method:   http.MethodGet,
			path:     "/api/v1/workspaces/test/semantic-models/test/datasets",
			response: map[string]any{"items": []map[string]any{}, "page": map[string]any{"nextCursor": ""}},
		},
		{
			name:     "dataset",
			args:     []string{"dataset", "test", "orders"},
			method:   http.MethodGet,
			path:     "/api/v1/workspaces/test/semantic-models/test/datasets/orders",
			response: map[string]any{"id": "orders"},
		},
		{
			name:     "fields",
			args:     []string{"fields", "test", "orders", "--limit", "7", "--page-token", "cursor"},
			method:   http.MethodGet,
			path:     "/api/v1/workspaces/test/semantic-models/test/datasets/orders/fields",
			response: map[string]any{"items": []map[string]any{}, "page": map[string]any{"nextCursor": ""}},
		},
		{
			name:     "preview",
			args:     []string{"preview", "test", "orders", "--body-json", `{"dimensions":[{"field":"orders.order_id"}]}`},
			method:   http.MethodPost,
			path:     "/api/v1/workspaces/test/semantic-models/test/datasets/orders/preview",
			wantBody: []string{`"orders.order_id"`},
			response: map[string]any{"columns": []string{"order_id"}, "items": []map[string]any{}, "page": map[string]any{"nextCursor": ""}},
		},
		{
			name:     "explain preview",
			args:     []string{"explain-preview", "test", "orders", "--body-json", `{"dimensions":[{"field":"orders.order_id"}]}`},
			method:   http.MethodPost,
			path:     "/api/v1/workspaces/test/semantic-models/test/datasets/orders/preview/explain",
			wantBody: []string{`"orders.order_id"`},
			response: map[string]any{"mode": "preview", "sql": "SELECT 1", "args": []map[string]any{}, "columns": []string{"order_id"}, "warnings": []string{}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != tc.method {
					t.Fatalf("method=%s want=%s", r.Method, tc.method)
				}
				if r.URL.Path != tc.path {
					t.Fatalf("path=%s want=%s", r.URL.Path, tc.path)
				}
				if tc.name == "datasets" || tc.name == "fields" {
					if got := r.URL.Query().Get("limit"); got != "7" {
						t.Fatalf("limit=%q", got)
					}
					if got := r.URL.Query().Get("pageToken"); got != "cursor" {
						t.Fatalf("pageToken=%q", got)
					}
				}
				body, err := io.ReadAll(r.Body)
				if err != nil {
					t.Fatalf("read body: %v", err)
				}
				for _, want := range tc.wantBody {
					if !strings.Contains(string(body), want) {
						t.Fatalf("body missing %q: %s", want, body)
					}
				}
				writeCLIJSON(t, w, tc.response)
			}))
			defer server.Close()

			opts := &rootOptions{workspaceID: "test"}
			cmd := semanticModelsCommand(context.Background(), opts)
			args := append([]string{}, tc.args...)
			args = append(args, "--target", server.URL, "--token", "token")
			cmd.SetArgs(args)
			captureStdout(t, func() {
				if err := cmd.Execute(); err != nil {
					t.Fatalf("run command: %v", err)
				}
			})
		})
	}
}

func TestAgentToolsCommandListsGeneratedTools(t *testing.T) {
	output := captureStdout(t, func() {
		cmd := agentCommand(context.Background(), &rootOptions{})
		cmd.SetArgs([]string{"tools"})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("agent tools: %v", err)
		}
	})
	for _, want := range []string{"NAME", "PRIVILEGE", "list_dashboards", "VIEW_ITEM", "list_assets", "describe_asset", "asset_lineage", "search", "query_dashboard_visual", "query_semantic_model", "explain_semantic_model_query", "query_visual"} {
		if !strings.Contains(output, want) {
			t.Fatalf("agent tools output missing %q:\n%s", want, output)
		}
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	original := os.Stdout
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe stdout: %v", err)
	}
	os.Stdout = write
	defer func() {
		os.Stdout = original
	}()
	type readResult struct {
		bytes []byte
		err   error
	}
	readDone := make(chan readResult, 1)
	go func() {
		bytes, err := io.ReadAll(read)
		readDone <- readResult{bytes: bytes, err: err}
	}()
	fn()
	if err := write.Close(); err != nil {
		t.Fatalf("close stdout pipe: %v", err)
	}
	result := <-readDone
	if result.err != nil {
		t.Fatalf("read stdout: %v", result.err)
	}
	return string(result.bytes)
}

func writeCLIJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Fatalf("encode JSON: %v", err)
	}
}
