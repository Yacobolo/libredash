package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/Yacobolo/leapview/internal/access"
	apigenapi "github.com/Yacobolo/leapview/internal/api/gen"
	productsearch "github.com/Yacobolo/leapview/internal/search"
	servingstate "github.com/Yacobolo/leapview/internal/servingstate"
)

func TestSearchAPIResultsIncludeVisualSubtype(t *testing.T) {
	items := searchAPIResults([]productsearch.Result{{
		Reference:  productsearch.Reference{WorkspaceID: "sales", Type: productsearch.TypeVisual, ID: "orders.revenue"},
		Name:       "Revenue",
		VisualType: "line",
		Workspace:  productsearch.Workspace{ID: "sales", Name: "Sales"},
		Locations:  []productsearch.Location{},
		Context:    []productsearch.ContextTag{},
	}})
	if len(items) != 1 || items[0].VisualType == nil || *items[0].VisualType != "line" {
		t.Fatalf("search API visual subtype = %#v", items)
	}
}

func TestGlobalSearchReturnsStructuredResultsAcrossWorkspaces(t *testing.T) {
	store := testStore(t)
	seedEnvironmentAssetDeployment(t, store, "sales", servingstate.DefaultEnvironment, "Executive Sales", "Sales Warehouse")
	seedEnvironmentAssetDeployment(t, store, "operations", servingstate.DefaultEnvironment, "Fulfillment Operations", "Operations Warehouse")
	if _, err := store.SQLDB().Exec(`
		UPDATE assets
		SET asset_key = workspace_id || '.overview', payload_json = '{"key":"overview"}'
		WHERE asset_type = 'dashboard'
	`); err != nil {
		t.Fatalf("remove textual dashboard terms from fixtures: %v", err)
	}
	server := NewWithOptions(nil, Options{Store: store, DefaultEnvironment: string(servingstate.DefaultEnvironment)})

	request := newPublicAPIRequest(http.MethodGet, "/api/v1/search?q=dashboar&limit=20", nil)
	response := httptest.NewRecorder()
	server.Routes().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var body apigenapi.SearchResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode search response: %v", err)
	}
	if len(body.Items) != 2 {
		t.Fatalf("search items = %#v, want two dashboards", body.Items)
	}
	seen := map[string]bool{}
	for _, item := range body.Items {
		seen[item.Reference.WorkspaceId] = true
		if item.Reference.Type != apigenapi.SearchResultTypeDashboard || item.Reference.Id == "" || item.Href == "" || len(item.Locations) == 0 {
			t.Fatalf("incomplete structured search result: %#v", item)
		}
	}
	if !seen["sales"] || !seen["operations"] {
		t.Fatalf("search workspaces = %#v, want sales and operations", seen)
	}
}

func TestGlobalSearchRepeatedFiltersAndCursor(t *testing.T) {
	store := testStore(t)
	seedEnvironmentAssetDeployment(t, store, "sales", servingstate.DefaultEnvironment, "Executive Sales", "Sales Warehouse")
	seedEnvironmentAssetDeployment(t, store, "operations", servingstate.DefaultEnvironment, "Fulfillment Operations", "Operations Warehouse")
	server := NewWithOptions(nil, Options{Store: store, DefaultEnvironment: string(servingstate.DefaultEnvironment)})

	firstRequest := newPublicAPIRequest(http.MethodGet, "/api/v1/search?workspace=sales&workspace=operations&type=dashboard&type=connection&limit=1", nil)
	firstResponse := httptest.NewRecorder()
	server.Routes().ServeHTTP(firstResponse, firstRequest)
	if firstResponse.Code != http.StatusOK {
		t.Fatalf("first status=%d body=%s", firstResponse.Code, firstResponse.Body.String())
	}
	var first apigenapi.SearchResponse
	if err := json.Unmarshal(firstResponse.Body.Bytes(), &first); err != nil {
		t.Fatalf("decode first response: %v", err)
	}
	if len(first.Items) != 1 || first.Page.NextCursor == nil || *first.Page.NextCursor == "" {
		t.Fatalf("first search page = %#v", first)
	}

	nextRequest := newPublicAPIRequest(http.MethodGet, "/api/v1/search?workspace=sales&workspace=operations&type=dashboard&type=connection&limit=1&pageToken="+url.QueryEscape(*first.Page.NextCursor), nil)
	nextResponse := httptest.NewRecorder()
	server.Routes().ServeHTTP(nextResponse, nextRequest)
	if nextResponse.Code != http.StatusOK {
		t.Fatalf("next status=%d body=%s", nextResponse.Code, nextResponse.Body.String())
	}
	var next apigenapi.SearchResponse
	if err := json.Unmarshal(nextResponse.Body.Bytes(), &next); err != nil {
		t.Fatalf("decode next response: %v", err)
	}
	if len(next.Items) != 1 || next.Items[0].Reference == first.Items[0].Reference {
		t.Fatalf("next page repeated first result: first=%#v next=%#v", first.Items, next.Items)
	}
}

func TestWorkspaceSearchRouteWasRemoved(t *testing.T) {
	server := NewWithOptions(nil, Options{Store: testStore(t)})
	request := newPublicAPIRequest(http.MethodGet, "/api/v1/workspaces/test/search?q=orders", nil)
	response := httptest.NewRecorder()
	server.Routes().ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("legacy workspace search status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestGlobalSearchDoesNotExposeOtherWorkspacesToScopedCredential(t *testing.T) {
	ctx := context.Background()
	store := testStore(t)
	seedEnvironmentAssetDeployment(t, store, "sales", servingstate.DefaultEnvironment, "Sales Orders", "Sales Warehouse")
	seedEnvironmentAssetDeployment(t, store, "secret", servingstate.DefaultEnvironment, "Secret Orders", "Secret Warehouse")
	accessRepository := testAccessRepository(store)
	principal, err := accessRepository.SetPrincipalRole(ctx, access.PrincipalRoleInput{
		WorkspaceID: "sales", Email: "searcher@example.com", DisplayName: "Searcher", Role: access.RoleViewer,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := accessRepository.UpsertSecurableObject(ctx, access.ItemObjectWithParent(
		access.SecurableDashboard, "sales", "dev-dashboard", access.WorkspaceObject("sales"),
	), ""); err != nil {
		t.Fatal(err)
	}
	token, _ := testScopedAPIToken(t, ctx, store, access.APITokenInput{
		PrincipalID: principal.ID, WorkspaceID: "sales", Name: "search", Privileges: []access.Privilege{access.PrivilegeViewItem},
	})
	server := NewWithOptions(nil, Options{Store: store, Auth: testAuth(store, "sales", AuthConfig{APITokenOnly: true})})
	request := httptest.NewRequest(http.MethodGet, "/api/v1/search?q=orders&limit=20", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	response := httptest.NewRecorder()
	server.Routes().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var body apigenapi.SearchResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Items) == 0 || body.Page.NextCursor != nil {
		t.Fatalf("credential-scoped search = %#v", body)
	}
	for _, item := range body.Items {
		if item.Reference.WorkspaceId != "sales" {
			t.Fatalf("credential-scoped search = %#v", body)
		}
	}
	responseText := response.Body.String()
	if responseText == "" || strings.Contains(responseText, "secret") || strings.Contains(responseText, "Secret Orders") || strings.Contains(responseText, "Secret Warehouse") {
		t.Fatalf("search response leaked inaccessible catalog metadata: %s", response.Body.String())
	}
}
