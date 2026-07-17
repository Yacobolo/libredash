package app

import (
	"context"
	"database/sql"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Yacobolo/libredash/internal/access"
	"github.com/Yacobolo/libredash/internal/agent"
	"github.com/Yacobolo/libredash/internal/platform"
	"github.com/Yacobolo/libredash/internal/queryaudit"
	"github.com/Yacobolo/libredash/internal/ui"
	uisignals "github.com/Yacobolo/libredash/internal/ui/signals"
	"github.com/Yacobolo/libredash/pkg/pagestream"
	_ "github.com/duckdb/duckdb-go/v2"
)

func TestAdminRouteRejectsViewer(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	viewer := testPrincipal(t, ctx, store, "viewer@example.com", "Viewer", access.RoleViewer)
	token := testAPIToken(t, ctx, store, viewer.ID, "test")
	auth := testAuth(store, "test", AuthConfig{APITokenOnly: true})
	server := NewWithOptions(fakeMetrics{}, Options{Store: store, Auth: auth, DefaultWorkspaceID: "test"})

	for _, tc := range []struct {
		method string
		path   string
		body   string
	}{
		{method: http.MethodGet, path: "/admin"},
		{method: http.MethodGet, path: "/admin/agent"},
		{method: http.MethodGet, path: "/admin/storage"},
		{method: http.MethodGet, path: "/updates?route=admin&section=storage"},
		{method: http.MethodPost, path: "/admin/storage/select-table", body: `{}`},
	} {
		req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		server.Routes().ServeHTTP(rec, req)

		if rec.Code != http.StatusForbidden {
			t.Fatalf("%s status = %d, want %d body=%s", tc.path, rec.Code, http.StatusForbidden, rec.Body.String())
		}
	}
}

func TestAdminPagesRenderReadOnlyAccessData(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	owner := testPlatformPrincipal(t, ctx, store, "owner@example.com", "Owner", access.RoleAdmin)
	analyst := testPrincipal(t, ctx, store, "analyst@example.com", "Analyst", access.RoleViewer)
	repo := testAccessRepository(store)
	group, err := repo.UpsertGroup(ctx, access.GroupInput{ID: "group_finance", WorkspaceID: "test", Provider: "local", ExternalID: "finance", Name: "Finance"})
	if err != nil {
		t.Fatalf("seed group: %v", err)
	}
	if err := repo.AddGroupMember(ctx, "test", group.ID, analyst.ID); err != nil {
		t.Fatalf("seed group member: %v", err)
	}
	if _, err := repo.CreateRoleBinding(ctx, access.RoleBindingInput{WorkspaceID: "test", SubjectType: access.SubjectGroup, SubjectID: group.ID, Role: access.RoleEditor}); err != nil {
		t.Fatalf("seed group binding: %v", err)
	}
	token := testAPIToken(t, ctx, store, owner.ID, "test")
	auth := testAuth(store, "test", AuthConfig{APITokenOnly: true})
	server := NewWithOptions(fakeMetrics{}, Options{Store: store, Auth: auth, Agent: agent.NewService(fakeMetrics{}, testAgentRepository(store), agent.Config{APIKey: "key", Model: "fake-model"}), DefaultWorkspaceID: "test"})

	cases := []struct {
		path string
		want []string
	}{
		{path: "/admin", want: []string{"<ld-admin-page", `section="general"`, `/updates?route=admin&amp;section=general`}},
		{path: "/admin/principals", want: []string{"<ld-admin-page", `section="principals"`, `/updates?route=admin&amp;section=principals`}},
		{path: "/admin/principals/" + analyst.ID, want: []string{"<ld-admin-page", `section="principal-detail"`, `/updates?principal=` + analyst.ID + `&amp;route=admin&amp;section=principal-detail`}},
		{path: "/admin/groups", want: []string{"<ld-admin-page", `section="groups"`, `/updates?route=admin&amp;section=groups`}},
		{path: "/admin/groups/group_finance", want: []string{"<ld-admin-page", `section="group-detail"`, `/updates?group=group_finance&amp;route=admin&amp;section=group-detail`}},
		{path: "/admin/agent", want: []string{"<ld-admin-page", `section="agent"`, `/updates?route=admin&amp;section=agent`, "/admin/agent/config"}},
		{path: "/admin/storage", want: []string{"<ld-admin-page", `section="storage"`, `/updates?route=admin&amp;section=storage`, "/admin/storage/select-table"}},
		{path: "/admin/queries", want: []string{"<ld-admin-page", `section="queries"`, `/updates?route=admin&amp;section=queries`, "/admin/queries/command"}},
	}
	for _, tc := range cases {
		req := httptest.NewRequest(http.MethodGet, tc.path, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		server.Routes().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status = %d body=%s", tc.path, rec.Code, rec.Body.String())
		}
		body := rec.Body.String()
		for _, want := range tc.want {
			if !strings.Contains(body, want) {
				t.Fatalf("%s missing %q:\n%s", tc.path, want, body)
			}
		}
		for _, notWant := range []string{"/admin/access", "Assign role", "Remove access", "<form", "data-on:ld-workspace-access-upsert", "refresh-materializations"} {
			if strings.Contains(body, notWant) {
				t.Fatalf("%s rendered write control %q:\n%s", tc.path, notWant, body)
			}
		}
	}
}

func TestAdminQueryHistoryCommandPublishesLoadMorePatch(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	owner := testPlatformPrincipal(t, ctx, store, "owner@example.com", "Owner", access.RoleAdmin)
	token := testAPIToken(t, ctx, store, owner.ID, "test")
	auth := testAuth(store, "test", AuthConfig{APITokenOnly: true})
	server := NewWithOptions(fakeMetrics{}, Options{Store: store, Auth: auth, DefaultWorkspaceID: "test"})
	repo, err := server.queryAuditRepository()
	if err != nil || repo == nil {
		t.Fatalf("query audit repository: %v", err)
	}
	for _, event := range []queryaudit.EventInput{
		{WorkspaceID: "sales", PrincipalID: owner.ID, Surface: "api", Operation: "api_query", QueryKind: "semantic_rows", ModelID: "sales", Target: "orders", Status: "success", SQL: "select 1"},
		{WorkspaceID: "sales", PrincipalID: owner.ID, Surface: "dashboard", Operation: "dashboard_table", QueryKind: "semantic_rows", ModelID: "sales", Target: "customers", Status: "success", SQL: "select 2"},
		{WorkspaceID: "operations", PrincipalID: owner.ID, Surface: "agent", Operation: "agent_query", QueryKind: "semantic_rows", ModelID: "operations", Target: "reviews", Status: "error", SQL: "select 3"},
	} {
		if err := repo.RecordQueryEvent(ctx, event); err != nil {
			t.Fatalf("record query event: %v", err)
		}
		time.Sleep(time.Millisecond)
	}
	first, err := repo.ListQueryEvents(ctx, queryaudit.Filter{Limit: 2})
	if err != nil || len(first) != 2 {
		t.Fatalf("first page = %d, err=%v", len(first), err)
	}
	nextCursor := encodeAdminQueryCursor(first[1].CreatedAt, first[1].ID)
	expectedNext, err := repo.ListQueryEvents(ctx, queryaudit.Filter{PageToken: nextCursor, Limit: 2})
	if err != nil || len(expectedNext) != 1 {
		t.Fatalf("next page = %d, err=%v", len(expectedNext), err)
	}
	updates, unsubscribe := server.broker.Subscribe("admin-queries:test-client")
	defer unsubscribe()

	body := strings.NewReader(`{"adminQueryHistory":{"table":{"rows":[{"id":"existing","query":{"label":"select 1","expandedContent":"select 1"}}]}},"adminQueryHistoryCommand":{"action":"load_more","pageToken":"` + nextCursor + `","limit":2}}`)
	req := httptest.NewRequest(http.MethodPost, "/admin/queries/command", body)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "ld_client_id", Value: "test-client"})
	rec := httptest.NewRecorder()
	server.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	select {
	case patch := <-updates:
		history, ok := patch["adminQueryHistory"].(uisignals.AdminQueryHistorySignal)
		if !ok {
			t.Fatalf("patch missing adminQueryHistory: %#v", patch)
		}
		if len(history.Table.Rows) != 2 || history.Table.Rows[0]["id"] != "existing" || history.Table.Rows[1]["id"] != expectedNext[0].ID {
			t.Fatalf("rows were not appended correctly: %#v", history.Table.Rows)
		}
		if history.HasMore || history.NextCursor != "" || history.LoadedCountLabel != "2 queries loaded" || history.Loading {
			t.Fatalf("unexpected pagination state: %#v", history)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for query history patch")
	}
}

func encodeAdminQueryCursor(createdAt, id string) string {
	if strings.TrimSpace(createdAt) == "" || strings.TrimSpace(id) == "" {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString([]byte(createdAt + "\x00" + id))
}

func TestAdminQueryHistoryCommandPublishesFilteredResetPatch(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	owner := testPlatformPrincipal(t, ctx, store, "owner@example.com", "Owner", access.RoleAdmin)
	token := testAPIToken(t, ctx, store, owner.ID, "test")
	auth := testAuth(store, "test", AuthConfig{APITokenOnly: true})
	server := NewWithOptions(fakeMetrics{}, Options{Store: store, Auth: auth, DefaultWorkspaceID: "test"})
	repo, err := server.queryAuditRepository()
	if err != nil || repo == nil {
		t.Fatalf("query audit repository: %v", err)
	}
	for _, event := range []queryaudit.EventInput{
		{WorkspaceID: "sales", PrincipalID: owner.ID, Surface: "api", Operation: "api_query", QueryKind: "semantic_rows", ModelID: "sales", Target: "orders", Status: "success", SQL: "select orders"},
		{WorkspaceID: "operations", PrincipalID: owner.ID, Surface: "agent", Operation: "agent_query", QueryKind: "semantic_rows", ModelID: "operations", Target: "reviews", Status: "error", SQL: "select reviews"},
	} {
		if err := repo.RecordQueryEvent(ctx, event); err != nil {
			t.Fatalf("record query event: %v", err)
		}
		time.Sleep(time.Millisecond)
	}
	updates, unsubscribe := server.broker.Subscribe("admin-queries:test-client")
	defer unsubscribe()

	body := strings.NewReader(`{"adminQueryHistoryCommand":{"action":"reset","limit":50,"filters":{"workspaces":["sales"],"surfaces":["api"],"statuses":["success"],"search":"orders"}}}`)
	req := httptest.NewRequest(http.MethodPost, "/admin/queries/command", body)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "ld_client_id", Value: "test-client"})
	rec := httptest.NewRecorder()
	server.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	select {
	case patch := <-updates:
		history, ok := patch["adminQueryHistory"].(uisignals.AdminQueryHistorySignal)
		if !ok {
			t.Fatalf("patch missing adminQueryHistory: %#v", patch)
		}
		if len(history.Table.Rows) != 1 || history.Table.Rows[0]["runtime"] != "sales" || history.Table.Rows[0]["target"] != "orders" {
			t.Fatalf("filtered reset rows = %#v", history.Table.Rows)
		}
		workspaces := uisignals.ValueOrZero(history.Filters.Workspaces)
		surfaces := uisignals.ValueOrZero(history.Filters.Surfaces)
		statuses := uisignals.ValueOrZero(history.Filters.Statuses)
		if len(workspaces) != 1 || workspaces[0] != "sales" || len(surfaces) != 1 || surfaces[0] != "api" || len(statuses) != 1 || statuses[0] != "success" || uisignals.ValueOrZero(history.Filters.Search) != "orders" {
			t.Fatalf("filters were not preserved: %#v", history.Filters)
		}
		filterMenus := uisignals.ValueOrZero(history.FilterMenus)
		if len(filterMenus) == 0 || uisignals.ValueOrZero(filterMenus[0].SummaryLabel) == "" {
			t.Fatalf("filter menus were not patched: %#v", history.FilterMenus)
		}
		command, ok := patch["adminQueryHistoryCommand"].(uisignals.AdminQueryHistoryCommand)
		if !ok || uisignals.ValueOrZero(command.PageToken) != history.NextCursor || command.Action != "load_more" {
			t.Fatalf("command patch = %#v", patch["adminQueryHistoryCommand"])
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for query history patch")
	}
}

func TestAdminQueryHistoryCommandSearchesFilterMenuOptions(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	owner := testPlatformPrincipal(t, ctx, store, "owner@example.com", "Owner", access.RoleAdmin)
	token := testAPIToken(t, ctx, store, owner.ID, "test")
	auth := testAuth(store, "test", AuthConfig{APITokenOnly: true})
	server := NewWithOptions(fakeMetrics{}, Options{Store: store, Auth: auth, DefaultWorkspaceID: "test"})
	repo, err := server.queryAuditRepository()
	if err != nil || repo == nil {
		t.Fatalf("query audit repository: %v", err)
	}
	for _, event := range []queryaudit.EventInput{
		{WorkspaceID: "sales", PrincipalID: owner.ID, Surface: "api", Operation: "api_query", QueryKind: "semantic_rows", ModelID: "sales", Target: "orders", Status: "success", SQL: "select orders"},
		{WorkspaceID: "operations", PrincipalID: owner.ID, Surface: "agent", Operation: "agent_query", QueryKind: "semantic_rows", ModelID: "operations", Target: "reviews", Status: "error", SQL: "select reviews"},
	} {
		if err := repo.RecordQueryEvent(ctx, event); err != nil {
			t.Fatalf("record query event: %v", err)
		}
		time.Sleep(time.Millisecond)
	}
	updates, unsubscribe := server.broker.Subscribe("admin-queries:test-client")
	defer unsubscribe()

	body := strings.NewReader(`{"adminQueryHistory":{"filterMenus":[{"id":"workspace","label":"Workspace"}]},"adminQueryHistoryCommand":{"action":"filter_search","limit":50,"filterMenu":{"menuId":"workspace","action":"search","search":"oper"}}}`)
	req := httptest.NewRequest(http.MethodPost, "/admin/queries/command", body)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "ld_client_id", Value: "test-client"})
	rec := httptest.NewRecorder()
	server.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	select {
	case patch := <-updates:
		history, ok := patch["adminQueryHistory"].(uisignals.AdminQueryHistorySignal)
		if !ok {
			t.Fatalf("patch missing adminQueryHistory: %#v", patch)
		}
		workspaceMenu := queryHistoryMenuForTest(uisignals.ValueOrZero(history.FilterMenus), "workspace")
		workspaceOptions := uisignals.ValueOrZero(workspaceMenu.Options)
		if uisignals.ValueOrZero(workspaceMenu.Search) != "oper" || len(workspaceOptions) != 1 || workspaceOptions[0].Value != "operations" {
			t.Fatalf("workspace menu = %#v", workspaceMenu)
		}
		if len(history.Table.Rows) != 0 {
			t.Fatalf("filter search should not patch table rows: %#v", history.Table.Rows)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for query history patch")
	}
}

func TestAdminQueryHistoryCommandTogglesFilterAndResetsTable(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	owner := testPlatformPrincipal(t, ctx, store, "owner@example.com", "Owner", access.RoleAdmin)
	token := testAPIToken(t, ctx, store, owner.ID, "test")
	auth := testAuth(store, "test", AuthConfig{APITokenOnly: true})
	server := NewWithOptions(fakeMetrics{}, Options{Store: store, Auth: auth, DefaultWorkspaceID: "test"})
	repo, err := server.queryAuditRepository()
	if err != nil || repo == nil {
		t.Fatalf("query audit repository: %v", err)
	}
	for _, event := range []queryaudit.EventInput{
		{WorkspaceID: "sales", PrincipalID: owner.ID, Surface: "api", Operation: "api_query", QueryKind: "semantic_rows", ModelID: "sales", Target: "orders", Status: "success", SQL: "select orders"},
		{WorkspaceID: "operations", PrincipalID: owner.ID, Surface: "agent", Operation: "agent_query", QueryKind: "semantic_rows", ModelID: "operations", Target: "reviews", Status: "error", SQL: "select reviews"},
	} {
		if err := repo.RecordQueryEvent(ctx, event); err != nil {
			t.Fatalf("record query event: %v", err)
		}
		time.Sleep(time.Millisecond)
	}
	updates, unsubscribe := server.broker.Subscribe("admin-queries:test-client")
	defer unsubscribe()

	body := strings.NewReader(`{"adminQueryHistory":{"table":{"rows":[{"id":"old"}]}},"adminQueryHistoryCommand":{"action":"filter_toggle","limit":50,"filterMenu":{"menuId":"surface","action":"toggle","value":"agent","selected":[]}}}`)
	req := httptest.NewRequest(http.MethodPost, "/admin/queries/command", body)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "ld_client_id", Value: "test-client"})
	rec := httptest.NewRecorder()
	server.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	select {
	case patch := <-updates:
		history, ok := patch["adminQueryHistory"].(uisignals.AdminQueryHistorySignal)
		if !ok {
			t.Fatalf("patch missing adminQueryHistory: %#v", patch)
		}
		surfaces := uisignals.ValueOrZero(history.Filters.Surfaces)
		if len(surfaces) != 1 || surfaces[0] != "agent" {
			t.Fatalf("surface filter = %#v", history.Filters)
		}
		if len(history.Table.Rows) != 1 || history.Table.Rows[0]["target"] != "reviews" {
			t.Fatalf("filtered table rows = %#v", history.Table.Rows)
		}
		surfaceMenu := queryHistoryMenuForTest(uisignals.ValueOrZero(history.FilterMenus), "surface")
		selected := uisignals.ValueOrZero(surfaceMenu.Selected)
		if uisignals.ValueOrZero(surfaceMenu.SummaryLabel) != "agent" || len(selected) != 1 || selected[0] != "agent" {
			t.Fatalf("surface menu = %#v", surfaceMenu)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for query history patch")
	}
}

func TestAdminQueryHistoryCommandPublishesDetailPatch(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	owner := testPlatformPrincipal(t, ctx, store, "owner@example.com", "Owner", access.RoleAdmin)
	token := testAPIToken(t, ctx, store, owner.ID, "test")
	auth := testAuth(store, "test", AuthConfig{APITokenOnly: true})
	server := NewWithOptions(fakeMetrics{}, Options{Store: store, Auth: auth, DefaultWorkspaceID: "test"})
	repo, err := server.queryAuditRepository()
	if err != nil || repo == nil {
		t.Fatalf("query audit repository: %v", err)
	}
	if err := repo.RecordQueryEvent(ctx, queryaudit.EventInput{
		WorkspaceID:   "sales",
		PrincipalID:   owner.ID,
		Surface:       "api",
		Operation:     "api_query",
		QueryKind:     "semantic_rows",
		ModelID:       "sales",
		Target:        "orders",
		ObjectType:    "semantic_dataset",
		ObjectID:      "sales:orders",
		RequestID:     "req_detail",
		CorrelationID: "corr_detail",
		Status:        "success",
		DurationMS:    17,
		RowsReturned:  3,
		SQL:           "select * from orders",
		PlanText:      "orders plan",
		QueryJSON:     `{"target":"orders"}`,
	}); err != nil {
		t.Fatalf("record query event: %v", err)
	}
	events, err := repo.ListQueryEvents(ctx, queryaudit.Filter{Search: "orders", Limit: 1})
	if err != nil || len(events) != 1 {
		t.Fatalf("query events = %d, err=%v", len(events), err)
	}
	updates, unsubscribe := server.broker.Subscribe("admin-queries:test-client")
	defer unsubscribe()

	body := strings.NewReader(`{"adminQueryHistoryCommand":{"action":"select_detail","eventId":"` + events[0].ID + `","limit":50}}`)
	req := httptest.NewRequest(http.MethodPost, "/admin/queries/command", body)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "ld_client_id", Value: "test-client"})
	rec := httptest.NewRecorder()
	server.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	select {
	case patch := <-updates:
		detail, ok := patch["adminQueryDetail"].(uisignals.AdminQueryDetailSignal)
		if !ok {
			t.Fatalf("patch missing adminQueryDetail: %#v", patch)
		}
		if uisignals.ValueOrZero(detail.EventID) != events[0].ID || uisignals.ValueOrZero(detail.WorkspaceID) != "sales" || uisignals.ValueOrZero(detail.SQL) != "select * from orders" || uisignals.ValueOrZero(detail.PlanText) != "orders plan" || uisignals.ValueOrZero(detail.QueryJSON) == "" {
			t.Fatalf("detail patch = %#v", detail)
		}
		if _, ok := patch["adminQueryHistory"]; ok {
			t.Fatalf("detail selection should not patch history: %#v", patch)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for query detail patch")
	}
}

func queryHistoryMenuForTest(menus []uisignals.FilterMenuSignal, id string) uisignals.FilterMenuSignal {
	for _, menu := range menus {
		if menu.ID == id {
			return menu
		}
	}
	return uisignals.FilterMenuSignal{}
}

func TestAdminQueryHistoryCommandRequiresCSRF(t *testing.T) {
	store := testStore(t)
	auth := testAuth(store, "test", AuthConfig{DevBypass: true})
	server := NewWithOptions(fakeMetrics{}, Options{Store: store, Auth: auth, DefaultWorkspaceID: "test"})

	req := httptest.NewRequest(http.MethodPost, "http://localhost:8150/admin/queries/command", strings.NewReader(`{"adminQueryHistoryCommand":{"action":"reset","limit":50}}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Referer", "http://localhost:8150/admin/queries")
	req.AddCookie(&http.Cookie{Name: "ld_client_id", Value: "test-client"})
	rec := httptest.NewRecorder()
	server.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("POST without CSRF status = %d, want %d body=%s", rec.Code, http.StatusForbidden, rec.Body.String())
	}
}

func TestAdminQueryHistoryUpdatesForwardsPatches(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	owner := testPlatformPrincipal(t, ctx, store, "owner@example.com", "Owner", access.RoleAdmin)
	token := testAPIToken(t, ctx, store, owner.ID, "test")
	auth := testAuth(store, "test", AuthConfig{APITokenOnly: true})
	server := NewWithOptions(fakeMetrics{}, Options{Store: store, Auth: auth, DefaultWorkspaceID: "test"})

	reqCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req := httptest.NewRequestWithContext(reqCtx, http.MethodGet, "/updates?route=admin&section=queries", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.AddCookie(&http.Cookie{Name: "ld_client_id", Value: "test-client"})
	rec := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		defer close(done)
		server.Routes().ServeHTTP(rec, req)
	}()

	deadline := time.After(time.Second)
	for server.broker.SubscriberCount("admin-queries:test-client") == 0 {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for query history updates subscriber")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}
	server.broker.Publish("admin-queries:test-client", pagestream.SignalPatch{"adminQueryHistory": map[string]any{"loadedCountLabel": "sentinel"}})
	deadline = time.After(time.Second)
	for !strings.Contains(rec.Body.String(), "sentinel") {
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for forwarded patch:\n%s", rec.Body.String())
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}
	cancel()
	<-done
	if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/event-stream") {
		t.Fatalf("content type = %q, want text/event-stream", got)
	}
}

func TestAdminStorageDetailRouteIsDropped(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	owner := testPlatformPrincipal(t, ctx, store, "owner@example.com", "Owner", access.RoleAdmin)
	token := testAPIToken(t, ctx, store, owner.ID, "test")
	auth := testAuth(store, "test", AuthConfig{APITokenOnly: true})
	server := NewWithOptions(fakeMetrics{}, Options{Store: store, Auth: auth, DefaultWorkspaceID: "test"})

	req := httptest.NewRequest(http.MethodGet, "/admin/storage/libredash-test.duckdb/model/orders", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	server.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d body=%s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

func TestAdminStorageUpdatesSubscribesWithoutInitialRescan(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	owner := testPlatformPrincipal(t, ctx, store, "owner@example.com", "Owner", access.RoleAdmin)
	token := testAPIToken(t, ctx, store, owner.ID, "test")
	auth := testAuth(store, "test", AuthConfig{APITokenOnly: true})
	server := NewWithOptions(fakeMetrics{}, Options{Store: store, Auth: auth, DefaultWorkspaceID: "test"})

	reqCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req := httptest.NewRequestWithContext(reqCtx, http.MethodGet, "/updates?route=admin&section=storage", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.AddCookie(&http.Cookie{Name: "ld_client_id", Value: "test-client"})
	rec := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		defer close(done)
		server.Routes().ServeHTTP(rec, req)
	}()

	deadline := time.After(time.Second)
	for server.broker.SubscriberCount("admin-storage:test-client") == 0 {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for storage updates subscriber")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}
	server.broker.Publish("admin-storage:test-client", pagestream.SignalPatch{"adminStorage": map[string]any{"selectedKey": "sentinel"}})
	deadline = time.After(time.Second)
	for !strings.Contains(rec.Body.String(), "sentinel") {
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for forwarded patch:\n%s", rec.Body.String())
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}
	cancel()
	<-done
	if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/event-stream") {
		t.Fatalf("content type = %q, want text/event-stream", got)
	}
}

func TestAdminStorageSelectTablePublishesSelectedTablePatch(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	owner := testPlatformPrincipal(t, ctx, store, "owner@example.com", "Owner", access.RoleAdmin)
	token := testAPIToken(t, ctx, store, owner.ID, "test")
	auth := testAuth(store, "test", AuthConfig{APITokenOnly: true})
	dir := t.TempDir()
	catalogPath := filepath.Join(dir, "libredash.db")
	dataPath := filepath.Join(dir, "data")
	seedAdminStorageDuckLakeAt(t, catalogPath, dataPath)
	server := NewWithOptions(fakeMetrics{}, Options{Store: store, Auth: auth, DefaultWorkspaceID: "test", DuckLakeCatalogPath: catalogPath, DuckLakeDataPath: dataPath})
	updates, unsubscribe := server.broker.Subscribe("admin-storage:test-client")
	defer unsubscribe()

	body := strings.NewReader(`{"adminStorageCommand":{"databaseId":"ducklake-catalog","schema":"model","table":"orders"}}`)
	req := httptest.NewRequest(http.MethodPost, "/admin/storage/select-table", body)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "ld_client_id", Value: "test-client"})
	rec := httptest.NewRecorder()
	server.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d body=%s", rec.Code, http.StatusNoContent, rec.Body.String())
	}
	select {
	case patch := <-updates:
		storage, ok := patch["adminStorage"].(map[string]any)
		if !ok {
			t.Fatalf("patch missing adminStorage: %#v", patch)
		}
		if storage["selectedKey"] != "ducklake-catalog\x00model\x00orders" {
			t.Fatalf("selectedKey = %#v", storage["selectedKey"])
		}
		table, ok := storage["selectedTable"].(*ui.AdminStorageTableSignal)
		if !ok {
			t.Fatalf("selectedTable = %#v, want *ui.AdminStorageTableSignal", storage["selectedTable"])
		}
		if table.Name != "orders" || table.Schema != "model" || len(uisignals.ValueOrZero(table.Columns)) != 3 || len(uisignals.ValueOrZero(table.Files)) == 0 {
			t.Fatalf("selectedTable = %#v", table)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for selected table patch")
	}
}

func TestAdminStorageReadsDuckLakeMetadata(t *testing.T) {
	dir := t.TempDir()
	catalogPath := filepath.Join(dir, "libredash.db")
	dataPath := filepath.Join(dir, "data")
	seedAdminStorageDuckLakeAt(t, catalogPath, dataPath)
	legacyDir := filepath.Join(dir, "duckdb")
	if err := os.MkdirAll(legacyDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacyDir, "libredash-stale.duckdb"), []byte("stale"), 0o644); err != nil {
		t.Fatal(err)
	}
	server := NewWithOptions(fakeMetrics{}, Options{DefaultWorkspaceID: "test", DuckDBDir: legacyDir, DuckLakeCatalogPath: catalogPath, DuckLakeDataPath: dataPath})

	data := server.storageReadModel().Data(httptest.NewRequest(http.MethodGet, "/admin/storage", nil).Context())
	if data.Status != "" {
		t.Fatalf("status = %q", data.Status)
	}
	if data.CatalogPath != catalogPath || data.DataPath != dataPath {
		t.Fatalf("paths = %q %q, want %q %q", data.CatalogPath, data.DataPath, catalogPath, dataPath)
	}
	if data.DatabaseCount != 1 || data.TableCount != 1 || data.SnapshotCount == 0 || data.DataFileCount == 0 {
		t.Fatalf("summary = %#v, want one DuckLake catalog with snapshots and data files", data)
	}
	if data.TotalDataSizeBytes == 0 || data.TotalSizeBytes == 0 {
		t.Fatalf("summary sizes = %#v, want DuckLake file sizes", data)
	}
	if len(data.Tables) != 1 {
		t.Fatalf("tables = %#v, want only DuckLake metadata tables and no legacy duckdb entries", data.Tables)
	}
	table := data.Tables[0]
	if table.DatabaseID != "ducklake-catalog" || table.DatabaseName != "DuckLake catalog" {
		t.Fatalf("table database = %#v, want DuckLake catalog identity", table)
	}
	if table.TableUUID == "" || table.DuckLakePath != "model/orders/" {
		t.Fatalf("table identity = %#v, want DuckLake uuid and metadata path", table)
	}
	if table.Schema != "model" || table.Name != "orders" || table.RowCountLabel != "10,000" || table.ColumnCount != 3 {
		t.Fatalf("table = %#v, want DuckLake row/column metadata", table)
	}
	if table.FileCount == 0 || table.SizeBytes == 0 || table.SizeLabel == "0 B" || len(table.Files) == 0 {
		t.Fatalf("table storage = %#v, want DuckLake data-file metadata", table)
	}
	if table.Files[0].RecordCountLabel != "10,000" {
		t.Fatalf("file record count label = %q, want thousands separator", table.Files[0].RecordCountLabel)
	}
	if table.Columns[0].ID == 0 || table.Columns[0].BeginSnapshot == 0 || table.Columns[0].DefaultValueType == "" || table.Columns[0].DefaultValueDialect == "" {
		t.Fatalf("column metadata = %#v, want DuckLake column id, snapshot, default type, and dialect", table.Columns[0])
	}
	if table.Columns[0].ContainsNull == "" || table.Columns[0].ContainsNaN == "" || table.Columns[0].MinValue == "" || table.Columns[0].MaxValue == "" {
		t.Fatalf("column stats = %#v, want DuckLake table column stats", table.Columns[0])
	}
	if len(table.History) == 0 || table.History[0].SnapshotID != table.BeginSnapshot || !strings.Contains(table.History[0].Source, "table") {
		t.Fatalf("table history = %#v, want table-scoped DuckLake snapshot metadata", table.History)
	}
}

func TestAdminStorageIncludesDeploymentSnapshotContext(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	catalogPath := filepath.Join(dir, "libredash.db")
	dataPath := filepath.Join(dir, "data")
	store, err := platform.Open(ctx, catalogPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	seedAdminStorageDuckLakeAt(t, catalogPath, dataPath)
	snapshotID := latestAdminStorageDuckLakeSnapshot(t, catalogPath)
	if _, err := store.SQLDB().ExecContext(ctx, `
INSERT INTO workspaces (id, title) VALUES ('test', 'Test') ON CONFLICT(id) DO NOTHING;
INSERT INTO serving_states (id, workspace_id, environment, status, digest, ducklake_snapshot_id, created_by, activated_at)
VALUES ('dep_test', 'test', 'dev', 'active', 'digest_test', ?, 'tester', CURRENT_TIMESTAMP);
INSERT INTO workspace_active_serving_states (workspace_id, environment, serving_state_id)
VALUES ('test', 'dev', 'dep_test')`, snapshotID); err != nil {
		t.Fatal(err)
	}

	server := NewWithOptions(fakeMetrics{}, Options{
		Store:               store,
		DefaultWorkspaceID:  "test",
		DuckLakeCatalogPath: catalogPath,
		DuckLakeDataPath:    dataPath,
	})

	data := server.storageReadModel().Data(httptest.NewRequest(http.MethodGet, "/admin/storage", nil).Context())
	if len(data.ServingStates) != 1 {
		t.Fatalf("serving_states = %#v, want active serving state context", data.ServingStates)
	}
	state := data.ServingStates[0]
	if state.WorkspaceID != "test" || state.Environment != "dev" || state.ServingStateID != "dep_test" || state.SnapshotID != snapshotID || !state.Active {
		t.Fatalf("serving state = %#v, want active snapshot serving state", state)
	}
	if len(data.Snapshots) == 0 || data.Snapshots[len(data.Snapshots)-1].ID != snapshotID {
		t.Fatalf("snapshots = %#v, want latest snapshot metadata", data.Snapshots)
	}
}

func TestAdminStorageSelectTableRejectsInvalidCommand(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	owner := testPlatformPrincipal(t, ctx, store, "owner@example.com", "Owner", access.RoleAdmin)
	token := testAPIToken(t, ctx, store, owner.ID, "test")
	auth := testAuth(store, "test", AuthConfig{APITokenOnly: true})
	dir := t.TempDir()
	catalogPath := filepath.Join(dir, "libredash.db")
	dataPath := filepath.Join(dir, "data")
	seedAdminStorageDuckLakeAt(t, catalogPath, dataPath)
	server := NewWithOptions(fakeMetrics{}, Options{Store: store, Auth: auth, DefaultWorkspaceID: "test", DuckLakeCatalogPath: catalogPath, DuckLakeDataPath: dataPath})
	updates, unsubscribe := server.broker.Subscribe("admin-storage:test-client")
	defer unsubscribe()

	body := strings.NewReader(`{"adminStorageCommand":{"databaseId":"libredash-test.duckdb","schema":"model","table":"missing"}}`)
	req := httptest.NewRequest(http.MethodPost, "/admin/storage/select-table", body)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "ld_client_id", Value: "test-client"})
	rec := httptest.NewRecorder()
	server.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	select {
	case patch := <-updates:
		t.Fatalf("unexpected selected table patch for invalid command: %#v", patch)
	default:
	}
}

func TestAdminAccessRouteIsDropped(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	owner := testPlatformPrincipal(t, ctx, store, "owner@example.com", "Owner", access.RoleAdmin)
	token := testAPIToken(t, ctx, store, owner.ID, "test")
	auth := testAuth(store, "test", AuthConfig{APITokenOnly: true})
	server := NewWithOptions(fakeMetrics{}, Options{Store: store, Auth: auth, DefaultWorkspaceID: "test"})

	req := httptest.NewRequest(http.MethodGet, "/admin/access", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	server.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d body=%s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

func TestAdminPrincipalDetailReturnsNotFoundForMissingPrincipal(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	owner := testPlatformPrincipal(t, ctx, store, "owner@example.com", "Owner", access.RoleAdmin)
	token := testAPIToken(t, ctx, store, owner.ID, "test")
	auth := testAuth(store, "test", AuthConfig{APITokenOnly: true})
	server := NewWithOptions(fakeMetrics{}, Options{Store: store, Auth: auth, DefaultWorkspaceID: "test"})

	req := httptest.NewRequest(http.MethodGet, "/admin/principals/missing", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	server.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d body=%s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

func TestAdminGroupDetailReturnsNotFoundForMissingGroup(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	owner := testPlatformPrincipal(t, ctx, store, "owner@example.com", "Owner", access.RoleAdmin)
	token := testAPIToken(t, ctx, store, owner.ID, "test")
	auth := testAuth(store, "test", AuthConfig{APITokenOnly: true})
	server := NewWithOptions(fakeMetrics{}, Options{Store: store, Auth: auth, DefaultWorkspaceID: "test"})

	req := httptest.NewRequest(http.MethodGet, "/admin/groups/missing", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	server.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d body=%s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

func TestAdminGeneralRendersWithoutStore(t *testing.T) {
	server := NewWithOptions(fakeMetrics{}, Options{DefaultWorkspaceID: "test"})
	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	rec := httptest.NewRecorder()
	server.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{"<ld-admin-page", `section="general"`, `/updates?route=admin&amp;section=general`} {
		if !strings.Contains(body, want) {
			t.Fatalf("admin general missing %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "Access store is not configured") || strings.Contains(body, "data-signals=") {
		t.Fatalf("admin general should stream read-model state instead of embedding it:\n%s", body)
	}
}

func TestAdminStorageRendersEmptyStateWithoutDuckDBFiles(t *testing.T) {
	server := NewWithOptions(fakeMetrics{}, Options{DefaultWorkspaceID: "test", DuckDBDir: t.TempDir()})
	req := httptest.NewRequest(http.MethodGet, "/admin/storage", nil)
	rec := httptest.NewRecorder()
	server.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{"<ld-admin-page", `section="storage"`, `/updates?route=admin&amp;section=storage`} {
		if !strings.Contains(body, want) {
			t.Fatalf("admin storage missing %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "No DuckLake catalog has been initialized.") || strings.Contains(body, "data-signals=") {
		t.Fatalf("admin storage should stream read-model state instead of embedding it:\n%s", body)
	}
}

func latestAdminStorageDuckLakeSnapshot(t *testing.T, catalogPath string) int64 {
	t.Helper()
	db, err := sql.Open("sqlite", catalogPath)
	if err != nil {
		t.Fatalf("open sqlite catalog: %v", err)
	}
	defer db.Close()
	var snapshotID int64
	if err := db.QueryRow(`SELECT max(snapshot_id) FROM ducklake_snapshot`).Scan(&snapshotID); err != nil {
		t.Fatalf("latest DuckLake snapshot: %v", err)
	}
	return snapshotID
}

func seedAdminStorageDuckLakeAt(t *testing.T, catalogPath, dataPath string) {
	t.Helper()
	db, err := sql.Open("duckdb", ":memory:")
	if err != nil {
		t.Fatalf("open duckdb: %v", err)
	}
	defer db.Close()
	for _, stmt := range []string{
		"LOAD sqlite",
		"LOAD ducklake",
		"ATTACH 'ducklake:sqlite:" + strings.ReplaceAll(catalogPath, "'", "''") + "' AS lake (DATA_PATH '" + strings.ReplaceAll(dataPath, "'", "''") + "')",
		"USE lake",
		"CREATE SCHEMA model",
		`CREATE TABLE model.orders AS
		 SELECT i AS id, 'c_' || i::VARCHAR AS customer_id, i * 1.5 AS amount
		 FROM range(1, 10001) t(i)`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("seed ducklake %q: %v", stmt, err)
		}
	}
}
