package app

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"testing"

	"github.com/Yacobolo/leapview/internal/access"
	agenttools "github.com/Yacobolo/leapview/internal/agent/tools"
	semanticmodel "github.com/Yacobolo/leapview/internal/analytics/model"
	"github.com/Yacobolo/leapview/internal/dashboard"
	dashboarddefinition "github.com/Yacobolo/leapview/internal/dashboard/definition"
	"github.com/Yacobolo/leapview/internal/servingstate"
	servingstatesqlite "github.com/Yacobolo/leapview/internal/servingstate/sqlite"
	"github.com/Yacobolo/leapview/internal/workspace"
	workspacecompiler "github.com/Yacobolo/leapview/internal/workspace/compiler"
	workspacesqlite "github.com/Yacobolo/leapview/internal/workspace/sqlite"
	agentcore "github.com/Yacobolo/leapview/pkg/agent"
)

type sharedCatalogMetrics struct{ fakeMetrics }

func (sharedCatalogMetrics) Pages(dashboardID string) []dashboard.Page {
	pages := fakeMetrics{}.Pages(dashboardID)
	if len(pages) == 2 {
		pages[1].Visuals = append(pages[1].Visuals, dashboard.PageVisual{
			ID: "shared-orders-chart", Kind: "donut_chart", Visual: "orders",
		})
	}
	return pages
}

func (m sharedCatalogMetrics) Report(dashboardID string) (dashboarddefinition.Definition, *semanticmodel.Model, bool) {
	report, model, ok := m.fakeMetrics.Report(dashboardID)
	if ok {
		report.Pages = m.Pages(dashboardID)
	}
	return report, model, ok
}

func TestAgentCatalogBrowsesEverySupportedRelationship(t *testing.T) {
	server := catalogTestServer(t)
	service := agentCatalogService{server: server}
	scope := agenttools.Scope{PrincipalID: "dev", DevAuthBypass: true}
	ctx := context.Background()

	root := catalogListForTest(t, service, ctx, scope, agenttools.CatalogListRequest{Limit: 50})
	assertCatalogRefs(t, root.Items, "workspace:test", "workspace:test-workspace")

	workspacePage := catalogListForTest(t, service, ctx, scope, agenttools.CatalogListRequest{
		Parent: catalogRefPointer("test-workspace", agenttools.CatalogTypeWorkspace, "test-workspace"), Limit: 50,
	})
	assertCatalogRefs(t, workspacePage.Items, "dashboard:executive-sales", "semantic_model:test")

	dashboardPage := catalogListForTest(t, service, ctx, scope, agenttools.CatalogListRequest{
		Parent: catalogRefPointer("test-workspace", agenttools.CatalogTypeDashboard, "executive-sales"), Limit: 50,
	})
	assertCatalogRefs(t, dashboardPage.Items, "page:executive-sales.operations", "page:executive-sales.overview")

	pageChildren := catalogListForTest(t, service, ctx, scope, agenttools.CatalogListRequest{
		Parent: catalogRefPointer("test-workspace", agenttools.CatalogTypePage, "executive-sales.overview"), Limit: 50,
	})
	assertCatalogRefs(t, pageChildren.Items,
		"filter:executive-sales.state",
		"visual:executive-sales.order_rows",
		"visual:executive-sales.orders",
	)

	modelChildren := catalogListForTest(t, service, ctx, scope, agenttools.CatalogListRequest{
		Parent: catalogRefPointer("test-workspace", agenttools.CatalogTypeSemanticModel, "test"), Limit: 50,
	})
	assertCatalogRefs(t, modelChildren.Items, "measure:test.order_count", "semantic_table:test.orders")

	tableChildren := catalogListForTest(t, service, ctx, scope, agenttools.CatalogListRequest{
		Parent: catalogRefPointer("test-workspace", agenttools.CatalogTypeSemanticTable, "test.orders"), Limit: 50,
	})
	assertCatalogRefs(t, tableChildren.Items, "field:test.orders.order_id", "field:test.orders.status")
}

func TestAgentCatalogGetRequiresSharedLocationAndReturnsTypedDetails(t *testing.T) {
	server := catalogTestServer(t)
	service := agentCatalogService{server: server}
	scope := agenttools.Scope{PrincipalID: "dev", DevAuthBypass: true}
	ctx := context.Background()
	visualRef := agenttools.CatalogRef{WorkspaceID: "test-workspace", Type: agenttools.CatalogTypeVisual, ID: "executive-sales.orders"}

	_, err := service.Get(ctx, scope, agenttools.CatalogGetRequest{Ref: visualRef})
	var catalogErr *agenttools.CatalogError
	if !errors.As(err, &catalogErr) || catalogErr.Code != "catalog_location_required" {
		t.Fatalf("shared visual error = %v, want catalog_location_required", err)
	}
	result, err := service.Get(ctx, scope, agenttools.CatalogGetRequest{
		Ref: visualRef,
		Location: &agenttools.CatalogLocation{
			DashboardID: "executive-sales",
			PageID:      "overview",
		},
	})
	if err != nil {
		t.Fatalf("get shared visual: %v", err)
	}
	if result.Details["type"] != "visual" || result.Details["visualType"] != "donut" || result.Details["query"] == nil {
		t.Fatalf("visual details = %#v", result.Details)
	}

	for _, ref := range []agenttools.CatalogRef{
		{WorkspaceID: "test-workspace", Type: agenttools.CatalogTypeWorkspace, ID: "test-workspace"},
		{WorkspaceID: "test-workspace", Type: agenttools.CatalogTypeDashboard, ID: "executive-sales"},
		{WorkspaceID: "test-workspace", Type: agenttools.CatalogTypePage, ID: "executive-sales.overview"},
		{WorkspaceID: "test-workspace", Type: agenttools.CatalogTypeSemanticModel, ID: "test"},
		{WorkspaceID: "test-workspace", Type: agenttools.CatalogTypeSemanticTable, ID: "test.orders"},
		{WorkspaceID: "test-workspace", Type: agenttools.CatalogTypeField, ID: "test.orders.status"},
		{WorkspaceID: "test-workspace", Type: agenttools.CatalogTypeMeasure, ID: "test.order_count"},
	} {
		got, err := service.Get(ctx, scope, agenttools.CatalogGetRequest{Ref: ref})
		if err != nil {
			t.Fatalf("get %s %s: %v", ref.Type, ref.ID, err)
		}
		if got.Details["type"] != string(ref.Type) {
			t.Fatalf("get %s details = %#v", ref.Type, got.Details)
		}
	}
}

func TestAgentCatalogSearchIsGlobalNormalizedAndBounded(t *testing.T) {
	server := catalogTestServer(t)
	service := agentCatalogService{server: server}
	page, err := service.Search(context.Background(), agenttools.Scope{PrincipalID: "dev", DevAuthBypass: true}, agenttools.CatalogSearchRequest{
		Query: "orders", Limit: 10,
	})
	if err != nil {
		t.Fatalf("catalog search: %v", err)
	}
	if len(page.Items) == 0 || len(page.Items) > 10 {
		t.Fatalf("catalog search items = %#v", page.Items)
	}
	for _, item := range page.Items {
		if item.Ref.WorkspaceID != "test-workspace" || item.Workspace.Ref.Type != agenttools.CatalogTypeWorkspace || len(item.Capabilities) == 0 {
			t.Fatalf("unnormalized catalog item = %#v", item)
		}
	}
}

func TestAgentCatalogListCursorBindsScopeRequestAndSnapshot(t *testing.T) {
	scope := agenttools.Scope{PrincipalID: "p1"}
	request := agenttools.CatalogListRequest{Limit: 1}
	items := []agenttools.CatalogItem{
		{Ref: agenttools.CatalogRef{WorkspaceID: "a", Type: agenttools.CatalogTypeWorkspace, ID: "a"}, Name: "A"},
		{Ref: agenttools.CatalogRef{WorkspaceID: "b", Type: agenttools.CatalogTypeWorkspace, ID: "b"}, Name: "B"},
	}
	snapshot := catalogItemsSnapshot(items)
	cursor := encodeCatalogListCursor(scope, request, snapshot, 1)
	if offset, err := decodeCatalogListCursor(cursor, scope, request, snapshot); err != nil || offset != 1 {
		t.Fatalf("decode cursor = %d, %v", offset, err)
	}
	if _, err := decodeCatalogListCursor(cursor, agenttools.Scope{PrincipalID: "p2"}, request, snapshot); err == nil {
		t.Fatal("cursor accepted a different principal")
	}
	if _, err := decodeCatalogListCursor(cursor, scope, request, catalogItemsSnapshot(items[:1])); err == nil {
		t.Fatal("cursor accepted a changed snapshot")
	} else {
		var catalogErr *agenttools.CatalogError
		if !errors.As(err, &catalogErr) || catalogErr.Code != "catalog_snapshot_changed" {
			t.Fatalf("snapshot error = %v", err)
		}
	}
	metadataChanged := append([]agenttools.CatalogItem(nil), items...)
	metadataChanged[1].Description = "Changed after the first page"
	if _, err := decodeCatalogListCursor(cursor, scope, request, catalogItemsSnapshot(metadataChanged)); err == nil {
		t.Fatal("cursor accepted changed item metadata")
	}
}

func TestAgentCatalogCredentialRestrictionDoesNotLeakOtherWorkspaces(t *testing.T) {
	server := catalogTestServer(t)
	principal := testPrincipal(t, context.Background(), server.store, "catalog-owner@example.com", "Catalog Owner", access.RoleOwner)
	scope := agenttools.Scope{
		PrincipalID: principal.ID,
		Credential: agenttools.CredentialScope{
			WorkspaceID: "test-workspace",
			Restricted:  true,
			Privileges:  []string{string(access.PrivilegeViewItem)},
		},
	}
	service := agentCatalogService{server: server}
	root, err := service.List(context.Background(), scope, agenttools.CatalogListRequest{Limit: 50})
	if err != nil {
		t.Fatalf("restricted root list: %v", err)
	}
	assertCatalogRefs(t, root.Items, "workspace:test-workspace")

	_, inaccessibleErr := service.Get(context.Background(), scope, agenttools.CatalogGetRequest{
		Ref: agenttools.CatalogRef{WorkspaceID: "test", Type: agenttools.CatalogTypeWorkspace, ID: "test"},
	})
	_, missingErr := service.Get(context.Background(), scope, agenttools.CatalogGetRequest{
		Ref: agenttools.CatalogRef{WorkspaceID: "test-workspace", Type: agenttools.CatalogTypeDashboard, ID: "missing"},
	})
	for name, err := range map[string]error{"inaccessible": inaccessibleErr, "missing": missingErr} {
		var catalogErr *agenttools.CatalogError
		if !errors.As(err, &catalogErr) || catalogErr.Code != "catalog_not_found" {
			t.Fatalf("%s ref error = %v, want catalog_not_found", name, err)
		}
	}
}

func TestAgentCatalogSemanticModelUsageFiltersUnauthorizedDashboards(t *testing.T) {
	server := catalogTestServer(t)
	ctx := context.Background()
	repository := testAccessRepository(server.store)
	principal, err := repository.UpsertPrincipal(ctx, access.PrincipalInput{
		ID: "catalog-model-only", Email: "catalog-model-only@example.com", DisplayName: "Catalog Model Only",
	})
	if err != nil {
		t.Fatalf("upsert principal: %v", err)
	}
	modelObject := access.ItemObjectWithParent(
		access.SecurableSemanticModel,
		"test-workspace",
		"test",
		access.WorkspaceObject("test-workspace"),
	)
	if _, err := repository.CreateGrant(ctx, access.GrantInput{
		Object: modelObject, SubjectType: access.SubjectPrincipal, SubjectID: principal.ID, Privilege: access.PrivilegeViewItem,
	}); err != nil {
		t.Fatalf("grant semantic model view: %v", err)
	}

	result, err := (agentCatalogService{server: server}).Get(ctx, agenttools.Scope{PrincipalID: principal.ID}, agenttools.CatalogGetRequest{
		Ref: agenttools.CatalogRef{WorkspaceID: "test-workspace", Type: agenttools.CatalogTypeSemanticModel, ID: "test"},
	})
	if err != nil {
		t.Fatalf("get semantic model: %v", err)
	}
	if got := result.Details["dashboardCount"]; got != 0 {
		t.Fatalf("dashboardCount = %#v, want 0 for unauthorized dashboard", got)
	}
	if got, ok := result.Details["dashboardUsage"].([]agenttools.CatalogRef); !ok || len(got) != 0 {
		t.Fatalf("dashboardUsage = %#v, want no unauthorized refs", result.Details["dashboardUsage"])
	}
}

func TestAgentCatalogWorkspaceLookupPropagatesRepositoryFailures(t *testing.T) {
	sentinel := errors.New("workspace repository unavailable")
	server := NewWithOptions(fakeMetrics{}, Options{
		WorkspaceRepo:      activeMetadataWorkspaceRepo{err: sentinel},
		DefaultWorkspaceID: "test",
	})
	_, err := (agentCatalogService{server: server}).Get(
		context.Background(),
		agenttools.Scope{PrincipalID: "dev", DevAuthBypass: true},
		agenttools.CatalogGetRequest{
			Ref: agenttools.CatalogRef{WorkspaceID: "test", Type: agenttools.CatalogTypeWorkspace, ID: "test"},
		},
	)
	if !errors.Is(err, sentinel) {
		t.Fatalf("workspace lookup error = %v, want %v", err, sentinel)
	}
}

func TestAgentCatalogProviderOutputMatchesClosedSchema(t *testing.T) {
	server := catalogTestServer(t)
	definitions := server.agentCatalogToolProvider().Definitions(agenttools.Scope{PrincipalID: "dev", DevAuthBypass: true})
	catalog, err := agentcore.NewToolCatalog(definitions)
	if err != nil {
		t.Fatalf("compile catalog tools: %v", err)
	}
	result, err := catalog.Execute(context.Background(), agentcore.ToolCall{
		ID: "catalog-list", Name: agenttools.CatalogListToolName, Arguments: json.RawMessage(`{}`),
	})
	if err != nil || result.IsError {
		t.Fatalf("execute catalog_list: result=%#v err=%v", result, err)
	}
	result, err = catalog.Execute(context.Background(), agentcore.ToolCall{
		ID: "catalog-get", Name: agenttools.CatalogGetToolName,
		Arguments: json.RawMessage(`{
			"ref":{"workspaceId":"test-workspace","type":"visual","id":"executive-sales.orders"},
			"location":{"dashboardId":"executive-sales","pageId":"overview"}
		}`),
	})
	if err != nil || result.IsError {
		t.Fatalf("execute catalog_get: result=%#v err=%v", result, err)
	}
}

func catalogTestServer(t *testing.T) *Server {
	t.Helper()
	ctx := context.Background()
	store := testStore(t)
	workspaceID := workspace.WorkspaceID("test-workspace")
	repository := workspacesqlite.NewRepository(store.SQLDB())
	if err := repository.Ensure(ctx, workspace.EnsureInput{ID: workspaceID, Title: "Test Workspace", Description: "Fixture workspace"}); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	servingRepository := servingstatesqlite.NewRepository(store.SQLDB())
	created, err := servingRepository.Create(ctx, servingstate.CreateInput{
		WorkspaceID: servingstate.WorkspaceID(workspaceID),
		Environment: servingstate.DefaultEnvironment,
		CreatedBy:   "tester",
	})
	if err != nil {
		t.Fatalf("create serving state: %v", err)
	}
	metrics := sharedCatalogMetrics{}
	report, model, ok := metrics.Report("executive-sales")
	if !ok {
		t.Fatal("fixture report missing")
	}
	definition := &workspace.Definition{
		Catalog: workspace.Catalog{
			Workspace:      workspace.CatalogWorkspace{ID: string(workspaceID), Title: "Test Workspace", Description: "Fixture workspace"},
			SemanticModels: []workspace.CatalogModel{{ID: "test", Title: "Test Model", Description: "Fixture model"}},
			Dashboards:     []workspace.CatalogDashboard{{ID: "executive-sales", Title: report.Title, Description: "Fixture report"}},
		},
		Models:     map[string]*semanticmodel.Model{"test": model},
		Dashboards: map[string]dashboarddefinition.Definition{"executive-sales": report},
	}
	graph, err := workspacecompiler.ExtractLineage(workspaceID, workspace.ServingStateID(created.ID), definition)
	if err != nil {
		t.Fatalf("extract catalog lineage: %v", err)
	}
	for index := range graph.Assets {
		graph.Assets[index].SourceFile = "dashboards/leapview.yaml"
	}
	artifact := zeroArtifact(created.ID, string(workspaceID))
	artifact.Environment = servingstate.DefaultEnvironment
	validation := completeTestValidation(string(workspaceID), servingstate.Validation{
		Digest: "catalog-test", ManifestJSON: "{}", ProjectID: "catalog-test", Graph: graph,
	})
	if _, err := servingRepository.SaveValidated(ctx, created.ID, validation, artifact); err != nil {
		t.Fatalf("save catalog serving state: %v", err)
	}
	if _, err := servingRepository.Activate(ctx, servingstate.WorkspaceID(workspaceID), servingstate.DefaultEnvironment, created.ID); err != nil {
		t.Fatalf("activate catalog serving state: %v", err)
	}
	return NewWithOptions(metrics, Options{
		Store: store, WorkspaceRepo: repository, DefaultWorkspaceID: string(workspaceID), DefaultEnvironment: string(servingstate.DefaultEnvironment),
	})
}

func catalogListForTest(t *testing.T, service agentCatalogService, ctx context.Context, scope agenttools.Scope, request agenttools.CatalogListRequest) agenttools.CatalogPage {
	t.Helper()
	page, err := service.List(ctx, scope, request)
	if err != nil {
		t.Fatalf("catalog list %#v: %v", request.Parent, err)
	}
	return page
}

func catalogRefPointer(workspaceID string, typ agenttools.CatalogType, id string) *agenttools.CatalogRef {
	return &agenttools.CatalogRef{WorkspaceID: workspaceID, Type: typ, ID: id}
}

func assertCatalogRefs(t *testing.T, items []agenttools.CatalogItem, want ...string) {
	t.Helper()
	got := make([]string, 0, len(items))
	for _, item := range items {
		got = append(got, string(item.Ref.Type)+":"+item.Ref.ID)
	}
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("catalog refs = %#v, want %#v", got, want)
	}
}
