package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Yacobolo/libredash/internal/access"
	accesssqlite "github.com/Yacobolo/libredash/internal/access/sqlite"
	"github.com/Yacobolo/libredash/internal/app"
	"github.com/Yacobolo/libredash/internal/platform"
	"github.com/Yacobolo/libredash/internal/workspace"
	workspacesqlite "github.com/Yacobolo/libredash/internal/workspace/sqlite"
)

func TestAuthSpecItemSharingAndDataPrivileges(t *testing.T) {
	h, repo := newAuthSpecHarness(t)
	ctx := context.Background()

	analyst := authSpecPrincipal(t, ctx, repo, "analyst@example.com")
	authSpecGrant(t, ctx, repo, access.ItemObject(access.SecurableDashboard, "sales", "executive-sales"), access.SubjectPrincipal, analyst.ID, access.PrivilegeViewItem)
	authSpecGrant(t, ctx, repo, access.ItemObject(access.SecurableSemanticModel, "sales", "sales"), access.SubjectPrincipal, analyst.ID, access.PrivilegeQueryData)
	token := authSpecToken(t, ctx, repo, access.APITokenInput{PrincipalID: analyst.ID, WorkspaceID: "sales", Name: "analyst"})

	status, body := h.authSpecDo(t, http.MethodGet, "/api/v1/workspaces/sales/dashboards/executive-sales", token, "")
	if status != http.StatusOK {
		t.Fatalf("dashboard metadata status=%d body=%s", status, body)
	}
	status, body = h.authSpecDo(t, http.MethodGet, "/api/v1/workspaces/sales/dashboards", token, "")
	if status != http.StatusForbidden {
		t.Fatalf("workspace dashboard list status=%d want=403 body=%s", status, body)
	}
	status, body = h.authSpecDo(t, http.MethodPost, "/api/v1/workspaces/sales/dashboards/executive-sales/pages/overview/query", token, `{}`)
	if status != http.StatusOK {
		t.Fatalf("dashboard query via semantic model grant status=%d body=%s", status, body)
	}
	status, body = h.authSpecDo(t, http.MethodPost, "/api/v1/workspaces/sales/semantic-models/sales/datasets/orders/preview", token, `{"fields":[{"field":"orders.status"}],"limit":1}`)
	if status != http.StatusForbidden {
		t.Fatalf("raw preview status=%d want=403 body=%s", status, body)
	}
}

func TestAuthSpecEffectiveAccessExplainsInheritedGrants(t *testing.T) {
	h, repo := newAuthSpecHarness(t)
	ctx := context.Background()

	principal := authSpecPrincipal(t, ctx, repo, "effective@example.com")
	authSpecGrant(t, ctx, repo, access.WorkspaceObject("sales"), access.SubjectPrincipal, principal.ID, access.PrivilegeUseWorkspace)
	authSpecGrant(t, ctx, repo, access.ItemObject(access.SecurableSemanticModel, "sales", "sales"), access.SubjectPrincipal, principal.ID, access.PrivilegeQueryData)
	token := authSpecToken(t, ctx, repo, access.APITokenInput{PrincipalID: principal.ID, WorkspaceID: "sales", Name: "effective"})

	status, body := h.authSpecDo(t, http.MethodGet, "/api/v1/workspaces/sales/effective-privileges?objectType=dataset&objectId=sales/orders", token, "")
	if status != http.StatusOK {
		t.Fatalf("effective privileges status=%d body=%s", status, body)
	}
	var decoded struct {
		Privileges      []string `json:"privileges"`
		EffectiveGrants []struct {
			Privilege     string `json:"privilege"`
			Reason        string `json:"reason"`
			Inherited     bool   `json:"inherited"`
			GrantObjectID string `json:"grantObjectId"`
		} `json:"effectiveGrants"`
	}
	if err := json.Unmarshal([]byte(body), &decoded); err != nil {
		t.Fatalf("decode effective access: %v body=%s", err, body)
	}
	if !authSpecHas(decoded.Privileges, string(access.PrivilegeQueryData)) {
		t.Fatalf("privileges=%#v missing QUERY_DATA", decoded.Privileges)
	}
	for _, grant := range decoded.EffectiveGrants {
		if grant.Privilege == string(access.PrivilegeQueryData) {
			if grant.Reason != "grant" || !grant.Inherited || grant.GrantObjectID != "semantic_model:sales:sales" {
				t.Fatalf("query grant provenance=%#v, want inherited semantic model grant", grant)
			}
			return
		}
	}
	t.Fatalf("effectiveGrants=%#v missing QUERY_DATA provenance", decoded.EffectiveGrants)
}

func TestAuthSpecServicePrincipalOAuthAndTokenAllowlist(t *testing.T) {
	h, repo := newAuthSpecHarness(t)
	ctx := context.Background()

	admin := authSpecPlatformAdmin(t, ctx, repo, "platform-admin@example.com")
	adminToken := authSpecToken(t, ctx, repo, access.APITokenInput{PrincipalID: admin.ID, Name: "platform-admin", Permissions: []access.Privilege{access.PrivilegeManagePlatform}})

	status, body := h.authSpecDo(t, http.MethodPost, "/api/v1/service-principals", adminToken, `{"id":"sp_ci","displayName":"CI"}`)
	if status != http.StatusCreated {
		t.Fatalf("create service principal status=%d body=%s", status, body)
	}
	status, body = h.authSpecDo(t, http.MethodPost, "/api/v1/service-principals/sp_ci/secrets", adminToken, `{"name":"ci"}`)
	if status != http.StatusCreated {
		t.Fatalf("create service principal secret status=%d body=%s", status, body)
	}
	var secretResponse struct {
		Secret       string `json:"secret"`
		ClientSecret struct {
			ID string `json:"id"`
		} `json:"clientSecret"`
	}
	if err := json.Unmarshal([]byte(body), &secretResponse); err != nil {
		t.Fatalf("decode service principal secret: %v body=%s", err, body)
	}
	authSpecGrant(t, ctx, repo, access.ItemObject(access.SecurableSemanticModel, "sales", "sales"), access.SubjectServicePrincipal, "sp_ci", access.PrivilegeQueryData)

	form := url.Values{}
	form.Set("grant_type", "client_credentials")
	form.Set("client_id", "sp_ci")
	form.Set("client_secret", secretResponse.Secret)
	form.Set("workspace_id", "sales")
	form.Set("scope", string(access.PrivilegeQueryData))
	status, body = h.authSpecForm(t, "/oauth/token", form)
	if status != http.StatusOK {
		t.Fatalf("oauth token status=%d body=%s", status, body)
	}
	var tokenResponse struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal([]byte(body), &tokenResponse); err != nil {
		t.Fatalf("decode oauth token: %v body=%s", err, body)
	}
	status, body = h.authSpecDo(t, http.MethodPost, "/api/v1/workspaces/sales/semantic-models/sales/datasets/orders/query", tokenResponse.AccessToken, `{"measures":[{"field":"revenue"}],"limit":1}`)
	if status != http.StatusOK {
		t.Fatalf("service principal query status=%d body=%s", status, body)
	}
	status, body = h.authSpecDo(t, http.MethodPost, "/api/v1/workspaces/sales/semantic-models/sales/datasets/orders/preview", tokenResponse.AccessToken, `{"fields":[{"field":"orders.status"}],"limit":1}`)
	if status != http.StatusForbidden {
		t.Fatalf("service principal preview status=%d want=403 body=%s", status, body)
	}

	status, body = h.authSpecDo(t, http.MethodDelete, "/api/v1/service-principals/sp_ci/secrets/"+secretResponse.ClientSecret.ID, adminToken, "")
	if status != http.StatusOK {
		t.Fatalf("revoke service principal secret status=%d body=%s", status, body)
	}
	status, body = h.authSpecForm(t, "/oauth/token", form)
	if status != http.StatusUnauthorized {
		t.Fatalf("oauth token after revoke status=%d want=401 body=%s", status, body)
	}
}

func TestAuthSpecAuditIncludesGrantRequestMetadata(t *testing.T) {
	h, repo := newAuthSpecHarness(t)
	ctx := context.Background()

	admin := authSpecPrincipal(t, ctx, repo, "grant-admin@example.com")
	authSpecGrant(t, ctx, repo, access.WorkspaceObject("sales"), access.SubjectPrincipal, admin.ID, access.PrivilegeManageGrants)
	authSpecGrant(t, ctx, repo, access.WorkspaceObject("sales"), access.SubjectPrincipal, admin.ID, access.PrivilegeViewAudit)
	token := authSpecToken(t, ctx, repo, access.APITokenInput{PrincipalID: admin.ID, WorkspaceID: "sales", Name: "grant-admin"})

	req, err := http.NewRequest(http.MethodPost, h.serverURL(t)+"/api/v1/workspaces/sales/grants", strings.NewReader(`{"objectType":"dashboard","objectId":"executive-sales","subjectType":"principal","subjectId":"email_audited","privilege":"VIEW_ITEM"}`))
	if err != nil {
		t.Fatalf("create grant request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Request-ID", "auth-spec-request")
	req.Header.Set("X-Correlation-ID", "auth-spec-correlation")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("create grant: %v", err)
	}
	bodyBytes, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("create grant status=%d body=%s", res.StatusCode, string(bodyBytes))
	}

	status, body := h.authSpecDo(t, http.MethodGet, "/api/v1/workspaces/sales/audit-events?action=grant.created&limit=10", token, "")
	if status != http.StatusOK {
		t.Fatalf("list audit status=%d body=%s", status, body)
	}
	if !strings.Contains(body, `"requestId":"auth-spec-request"`) ||
		!strings.Contains(body, `"correlationId":"auth-spec-correlation"`) ||
		!strings.Contains(body, `"privilege":"VIEW_ITEM"`) ||
		!strings.Contains(body, `"status":"success"`) {
		t.Fatalf("audit response missing auth metadata: %s", body)
	}
}

func newAuthSpecHarness(t *testing.T) (*harness, *accesssqlite.Repository) {
	t.Helper()
	h, metrics, catalogPath := newHarnessWithMetrics(t)
	ctx := context.Background()
	store, err := platform.Open(ctx, filepath.Join(t.TempDir(), "libredash.db"))
	if err != nil {
		t.Fatalf("open platform store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	workspaceID := metrics.Catalog().Workspace.ID
	if workspaceID == "" {
		workspaceID = platform.DefaultWorkspaceID
	}
	if err := workspacesqlite.NewRepository(store.SQLDB()).Ensure(ctx, workspace.EnsureInput{ID: workspace.WorkspaceID(workspaceID), Title: metrics.Catalog().Workspace.Title, Description: metrics.Catalog().Workspace.Description}); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	seedIntegrationActiveDeployment(t, store, workspaceID, catalogPath)
	repo := accesssqlite.NewRepository(store.SQLDB())
	auth := app.NewAuth(repo, workspaceID, app.AuthConfig{APITokenOnly: true})
	server := app.NewWithOptions(metrics, app.Options{Store: store, Auth: auth, DefaultWorkspaceID: workspaceID})
	h.store = store
	h.handler = server.Routes()
	h.server = httptestNewServer(t, h.handler)
	h.workspaceID = workspaceID
	return h, repo
}

func httptestNewServer(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return server
}

func authSpecPrincipal(t *testing.T, ctx context.Context, repo *accesssqlite.Repository, email string) access.Principal {
	t.Helper()
	principal, err := repo.UpsertPrincipal(ctx, access.PrincipalInput{ID: access.PrincipalIDForEmail(email), Email: email, DisplayName: email})
	if err != nil {
		t.Fatalf("upsert principal: %v", err)
	}
	return principal
}

func authSpecPlatformAdmin(t *testing.T, ctx context.Context, repo *accesssqlite.Repository, email string) access.Principal {
	t.Helper()
	principal, err := repo.SetPlatformRole(ctx, access.PlatformRoleInput{PrincipalID: access.PrincipalIDForEmail(email), Email: email, DisplayName: email, Role: access.RolePlatformAdmin})
	if err != nil {
		t.Fatalf("set platform role: %v", err)
	}
	return principal
}

func authSpecGrant(t *testing.T, ctx context.Context, repo *accesssqlite.Repository, object access.ObjectRef, subjectType access.SubjectType, subjectID string, privilege access.Privilege) {
	t.Helper()
	if _, err := repo.CreateGrant(ctx, access.GrantInput{Object: object, SubjectType: subjectType, SubjectID: subjectID, Privilege: privilege}); err != nil {
		t.Fatalf("create %s grant on %s: %v", privilege, object.CanonicalID(), err)
	}
}

func authSpecToken(t *testing.T, ctx context.Context, repo *accesssqlite.Repository, input access.APITokenInput) string {
	t.Helper()
	token, _, err := repo.CreateAPITokenWithMetadata(ctx, input)
	if err != nil {
		t.Fatalf("create api token: %v", err)
	}
	return token
}

func (h *harness) authSpecDo(t *testing.T, method, path, token, body string) (int, string) {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, h.serverURL(t)+path, reader)
	if err != nil {
		t.Fatalf("create %s %s: %v", method, path, err)
	}
	req.Header.Set("Accept", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer res.Body.Close()
	bytes, _ := io.ReadAll(res.Body)
	return res.StatusCode, string(bytes)
}

func (h *harness) authSpecForm(t *testing.T, path string, form url.Values) (int, string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, h.serverURL(t)+path, bytes.NewBufferString(form.Encode()))
	if err != nil {
		t.Fatalf("create form request %s: %v", path, err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", path, err)
	}
	defer res.Body.Close()
	bytes, _ := io.ReadAll(res.Body)
	return res.StatusCode, string(bytes)
}

func authSpecHas(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
