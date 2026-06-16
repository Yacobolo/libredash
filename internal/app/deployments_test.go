package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/Yacobolo/libredash/internal/api"
	"github.com/Yacobolo/libredash/internal/deploy"
	"github.com/Yacobolo/libredash/internal/platform"
	platformdb "github.com/Yacobolo/libredash/internal/platform/db"
	"github.com/Yacobolo/libredash/internal/runtime"
	"github.com/gorilla/csrf"
)

type fakeReloader struct {
	prepareCalls int
	commitCalls  int
	prepareErr   error
}

func (r *fakeReloader) Reload(context.Context) error {
	r.prepareCalls++
	r.commitCalls++
	return nil
}

func (r *fakeReloader) PrepareDeployment(context.Context, string) (*runtime.Prepared, error) {
	r.prepareCalls++
	if r.prepareErr != nil {
		return nil, r.prepareErr
	}
	return &runtime.Prepared{}, nil
}

func (r *fakeReloader) CommitPrepared(*runtime.Prepared) error {
	r.commitCalls++
	return nil
}

func TestDeploymentAPIRequiresAuthentication(t *testing.T) {
	store := testStore(t)
	auth := NewAuth(store, "test", AuthConfig{APITokenOnly: true})
	server := NewWithOptions(fakeMetrics{}, Options{Store: store, Auth: auth, ArtifactDir: t.TempDir(), DefaultWorkspaceID: "test"})

	req := httptest.NewRequest(http.MethodGet, "/api/deployments", nil)
	req.Header.Set("Accept", "application/json")
	rec := httptest.NewRecorder()
	server.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestDeploymentAPIRejectsBrowserPostWithoutCSRF(t *testing.T) {
	t.Setenv("LIBREDASH_DEV_AUTH_BYPASS", "1")
	store := testStore(t)
	auth := NewAuth(store, "test", AuthConfig{DevBypass: true})
	server := NewWithOptions(fakeMetrics{}, Options{Store: store, Auth: auth, ArtifactDir: t.TempDir(), DefaultWorkspaceID: "test"})

	req := httptest.NewRequest(http.MethodPost, "/api/deployments", bytes.NewBufferString(`{"workspaceId":"test"}`))
	req.Header.Set("Accept", "application/json")
	rec := httptest.NewRecorder()
	server.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestCSRFMiddlewareAllowsBrowserPostWithToken(t *testing.T) {
	store := testStore(t)
	auth := NewAuth(store, "test", AuthConfig{DevBypass: true})
	handler := auth.CSRFMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(csrf.Token(r)))
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	getReq := httptest.NewRequest(http.MethodGet, "/form", nil)
	getRec := httptest.NewRecorder()
	handler.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("GET status = %d, want %d", getRec.Code, http.StatusOK)
	}

	postReq := httptest.NewRequest(http.MethodPost, "/form", nil)
	postReq.Header.Set("X-CSRF-Token", getRec.Body.String())
	postReq.Header.Set("Origin", "https://example.com")
	for _, cookie := range getRec.Result().Cookies() {
		postReq.AddCookie(cookie)
	}
	postRec := httptest.NewRecorder()
	handler.ServeHTTP(postRec, postReq)
	if postRec.Code != http.StatusNoContent {
		t.Fatalf("POST status = %d, want %d, body=%s", postRec.Code, http.StatusNoContent, postRec.Body.String())
	}
}

func TestSessionCookieUsesConfiguredSecureFlag(t *testing.T) {
	store := testStore(t)
	auth := NewAuth(store, "test", AuthConfig{DevBypass: true, CookieSecure: true})
	cookie := auth.sessionCookie("token", time.Now().Add(time.Hour))
	if !cookie.Secure {
		t.Fatal("session cookie Secure = false, want true")
	}
}

func TestDeploymentAPIRejectsViewer(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	principal, err := store.UpsertPrincipal(ctx, platform.PrincipalInput{Email: "viewer@example.com", DisplayName: "Viewer"})
	if err != nil {
		t.Fatalf("upsert principal: %v", err)
	}
	if err := store.BindRole(ctx, "test", principal.ID, "viewer"); err != nil {
		t.Fatalf("bind role: %v", err)
	}
	token, err := store.CreateAPIToken(ctx, principal.ID, "test")
	if err != nil {
		t.Fatalf("create api token: %v", err)
	}
	auth := NewAuth(store, "test", AuthConfig{APITokenOnly: true})
	server := NewWithOptions(fakeMetrics{}, Options{Store: store, Auth: auth, ArtifactDir: t.TempDir(), DefaultWorkspaceID: "test"})

	req := httptest.NewRequest(http.MethodPost, "/api/deployments", bytes.NewBufferString(`{"workspaceId":"test"}`))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	rec := httptest.NewRecorder()
	server.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestDeploymentAPIValidatesAndActivatesBundle(t *testing.T) {
	t.Setenv("LIBREDASH_DEV_AUTH_BYPASS", "1")
	store := testStore(t)
	reloader := &fakeReloader{}
	artifactDir := t.TempDir()
	auth := NewAuth(store, "test", AuthConfig{DevBypass: true})
	server := NewWithOptions(fakeMetrics{}, Options{Store: store, Auth: auth, Reloader: reloader, ArtifactDir: artifactDir, DefaultWorkspaceID: "test"})

	createReq := httptest.NewRequest(http.MethodPost, "/api/deployments", bytes.NewBufferString(`{"workspaceId":"test","title":"Test"}`))
	createReq.Header.Set("Authorization", "Bearer dev")
	createReq.Header.Set("Accept", "application/json")
	createRec := httptest.NewRecorder()
	server.Routes().ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create status = %d body=%s", createRec.Code, createRec.Body.String())
	}
	var created api.DeploymentResponse
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create: %v", err)
	}

	var bundle bytes.Buffer
	if _, _, err := deploy.PackCatalog(filepath.Join("..", "..", "dashboards", "catalog.yaml"), &bundle); err != nil {
		t.Fatalf("pack catalog: %v", err)
	}
	uploadReq := httptest.NewRequest(http.MethodPut, "/api/deployments/"+created.ID+"/artifact", bytes.NewReader(bundle.Bytes()))
	uploadReq.Header.Set("Authorization", "Bearer dev")
	uploadReq.Header.Set("Accept", "application/json")
	uploadRec := httptest.NewRecorder()
	server.Routes().ServeHTTP(uploadRec, uploadReq)
	if uploadRec.Code != http.StatusOK {
		t.Fatalf("upload status = %d body=%s", uploadRec.Code, uploadRec.Body.String())
	}

	validateReq := httptest.NewRequest(http.MethodPost, "/api/deployments/"+created.ID+"/validate", nil)
	validateReq.Header.Set("Authorization", "Bearer dev")
	validateReq.Header.Set("Accept", "application/json")
	validateRec := httptest.NewRecorder()
	server.Routes().ServeHTTP(validateRec, validateReq)
	if validateRec.Code != http.StatusOK {
		t.Fatalf("validate status = %d body=%s", validateRec.Code, validateRec.Body.String())
	}

	activateReq := httptest.NewRequest(http.MethodPost, "/api/deployments/"+created.ID+"/activate", nil)
	activateReq.Header.Set("Authorization", "Bearer dev")
	activateReq.Header.Set("Accept", "application/json")
	activateRec := httptest.NewRecorder()
	server.Routes().ServeHTTP(activateRec, activateReq)
	if activateRec.Code != http.StatusOK {
		t.Fatalf("activate status = %d body=%s", activateRec.Code, activateRec.Body.String())
	}
	if reloader.prepareCalls != 1 {
		t.Fatalf("prepare calls = %d, want 1", reloader.prepareCalls)
	}
	if reloader.commitCalls != 1 {
		t.Fatalf("commit calls = %d, want 1", reloader.commitCalls)
	}
}

func TestDeploymentActivationPrepareFailureLeavesDeploymentInactive(t *testing.T) {
	t.Setenv("LIBREDASH_DEV_AUTH_BYPASS", "1")
	store := testStore(t)
	ctx := context.Background()
	deployment, err := store.CreateDeployment(ctx, "test", "tester")
	if err != nil {
		t.Fatalf("create deployment: %v", err)
	}
	if err := store.ValidateDeployment(ctx, deployment.ID, "digest", "{}", zeroArtifact(deployment.ID, "test"), nil, nil); err != nil {
		t.Fatalf("validate deployment: %v", err)
	}
	auth := NewAuth(store, "test", AuthConfig{DevBypass: true})
	reloader := &fakeReloader{prepareErr: errors.New("runtime load failed")}
	server := NewWithOptions(fakeMetrics{}, Options{Store: store, Auth: auth, Reloader: reloader, ArtifactDir: t.TempDir(), DefaultWorkspaceID: "test"})

	req := httptest.NewRequest(http.MethodPost, "/api/deployments/"+deployment.ID+"/activate", nil)
	req.Header.Set("Authorization", "Bearer dev")
	req.Header.Set("Accept", "application/json")
	rec := httptest.NewRecorder()
	server.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d body=%s", rec.Code, http.StatusInternalServerError, rec.Body.String())
	}
	after, err := store.Queries().GetDeployment(ctx, deployment.ID)
	if err != nil {
		t.Fatalf("get deployment: %v", err)
	}
	if after.Status != "validated" {
		t.Fatalf("status = %q, want validated", after.Status)
	}
	if reloader.commitCalls != 0 {
		t.Fatalf("commit calls = %d, want 0", reloader.commitCalls)
	}
}

func testStore(t *testing.T) *platform.Store {
	t.Helper()
	store, err := platform.Open(context.Background(), filepath.Join(t.TempDir(), "libredash.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.EnsureWorkspace(context.Background(), platform.WorkspaceInput{ID: "test", Title: "Test"}); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	return store
}

func zeroArtifact(deploymentID, workspaceID string) platformdb.InsertDeploymentArtifactParams {
	return platformdb.InsertDeploymentArtifactParams{
		ID:           "artifact_" + deploymentID,
		DeploymentID: deploymentID,
		WorkspaceID:  workspaceID,
		Digest:       "digest",
		Format:       "tar.gz",
		Path:         "artifact.tar.gz",
		ManifestJson: "{}",
	}
}
