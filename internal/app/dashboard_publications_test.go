package app

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Yacobolo/leapview/internal/access"
	"github.com/Yacobolo/leapview/internal/dashboard"
	"github.com/Yacobolo/leapview/internal/dashboard/command"
	lddatastar "github.com/Yacobolo/leapview/internal/dashboard/datastar"
	"github.com/Yacobolo/leapview/internal/dashboard/publication"
	publicationsqlite "github.com/Yacobolo/leapview/internal/dashboard/publication/sqlite"
	"github.com/Yacobolo/leapview/internal/dataquery"
	"github.com/Yacobolo/leapview/internal/deployment/apiadapter"
	"github.com/Yacobolo/leapview/internal/platform"
	"github.com/Yacobolo/leapview/internal/servingstate"
	servingstatesqlite "github.com/Yacobolo/leapview/internal/servingstate/sqlite"
	"github.com/Yacobolo/leapview/internal/workspace"
	"github.com/Yacobolo/leapview/pkg/pagestream"
)

func TestPublicDashboardDocumentsAreAnonymousAndRouteAware(t *testing.T) {
	store := testStore(t)
	seedActivePublication(t, store, "opaque-public-id-12345678901234")
	server := NewWithOptions(fakeMetrics{}, Options{
		Store: store, DefaultWorkspaceID: "test-workspace", SecurityHeaders: SecurityHeaders(false),
	})

	public := httptest.NewRecorder()
	server.Routes().ServeHTTP(public, httptest.NewRequest(http.MethodGet, "/public/dashboards/opaque-public-id-12345678901234", nil))
	if public.Code != http.StatusOK {
		t.Fatalf("public status = %d, body=%s", public.Code, public.Body.String())
	}
	for _, want := range []string{
		`<lv-dashboard-page`, `presentation="public"`, `/public/dashboards/opaque-public-id-12345678901234/updates?`,
		`/public/dashboards/opaque-public-id-12345678901234/commands/filter`,
	} {
		if !strings.Contains(public.Body.String(), want) {
			t.Fatalf("public document missing %q:\n%s", want, public.Body.String())
		}
	}
	if strings.Contains(public.Body.String(), "lv-app-shell") || public.Header().Get("Set-Cookie") != "" {
		t.Fatalf("public document exposed app shell or cookie: headers=%v", public.Header())
	}
	if got := public.Header().Get("X-Frame-Options"); got != "DENY" {
		t.Fatalf("public X-Frame-Options = %q", got)
	}
	if csp := public.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "frame-ancestors 'none'") {
		t.Fatalf("public CSP = %q", csp)
	}
	if public.Header().Get("Referrer-Policy") != "no-referrer" || public.Header().Get("X-Robots-Tag") != "noindex" {
		t.Fatalf("public privacy headers = %v", public.Header())
	}
	if public.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("public Cache-Control = %q, want no-store", public.Header().Get("Cache-Control"))
	}

	embed := httptest.NewRecorder()
	server.Routes().ServeHTTP(embed, httptest.NewRequest(http.MethodGet, "/embed/dashboards/opaque-public-id-12345678901234", nil))
	if embed.Code != http.StatusOK {
		t.Fatalf("embed status = %d, body=%s", embed.Code, embed.Body.String())
	}
	if got := embed.Header().Get("X-Frame-Options"); got != "" {
		t.Fatalf("embed X-Frame-Options = %q", got)
	}
	if csp := embed.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "frame-ancestors https://leapview.dev https://partner.example") {
		t.Fatalf("embed CSP = %q", csp)
	}
	if !strings.Contains(embed.Body.String(), `presentation="embed"`) {
		t.Fatalf("embed document routes/presentation are wrong:\n%s", embed.Body.String())
	}
}

func TestPublicDashboardExecutionContextUsesPublicationPrincipal(t *testing.T) {
	server := &Server{}
	resolved := resolvedPublicDashboard{publication: publication.Publication{
		WorkspaceID: "visuals",
		Name:        "website-showcase",
		Dashboard:   "visual-showcase",
	}}

	metadata := dataquery.MetadataFromContext(server.publicDashboardExecutionContext(context.Background(), resolved))
	want := access.DashboardPublicationSubjectID("visuals", "website-showcase")
	if metadata.PrincipalID != want {
		t.Fatalf("public principal id = %q, want %q", metadata.PrincipalID, want)
	}
	if metadata.Surface != dataquery.SurfacePublicDashboard || metadata.ObjectType != "dashboard_publication" || metadata.ObjectID != "website-showcase" {
		t.Fatalf("public metadata = %#v", metadata)
	}
}

func TestPublicationDeploymentRequiresManagementPrivilege(t *testing.T) {
	ctx := context.Background()
	store := testStore(t)
	states := servingstatesqlite.NewRepository(store.SQLDB())
	created, err := states.Create(ctx, servingstate.CreateInput{WorkspaceID: "test", ProjectID: "project", CreatedBy: "tester"})
	if err != nil {
		t.Fatal(err)
	}
	validation := completeTestValidation("test", servingstate.Validation{
		Digest: "digest", ManifestJSON: "{}", ProjectID: "project",
		DashboardPublications: map[string]workspace.DashboardPublication{"website": {Name: "website", Dashboard: "showcase"}},
	})
	if _, err := states.SaveValidated(ctx, created.ID, validation, zeroArtifact(created.ID, "test")); err != nil {
		t.Fatal(err)
	}
	server := NewWithOptions(fakeMetrics{}, Options{Store: store, ServingStateRepo: states, DefaultWorkspaceID: "test", DefaultEnvironment: "prod"})
	targets := []apiadapter.TargetRequest{{Workspace: "test", CandidateID: string(created.ID)}}

	viewer := testPrincipal(t, ctx, store, "viewer-publication@example.com", "Viewer", "viewer")
	if err := server.authorizePublicationDeployment(ctx, Principal{ID: viewer.ID}, "prod", targets); !errors.Is(err, errPublicationDeploymentForbidden) {
		t.Fatalf("viewer authorization error = %v", err)
	}
	owner := testPrincipal(t, ctx, store, "owner-publication@example.com", "Owner", "owner")
	if err := server.authorizePublicationDeployment(ctx, Principal{ID: owner.ID}, "prod", targets); err != nil {
		t.Fatalf("owner authorization: %v", err)
	}
	if err := server.authorizePublicationDeployment(ctx, Principal{ID: viewer.ID}, "dev", targets); err != nil {
		t.Fatalf("development deployment authorization = %v, want publication check skipped", err)
	}
}

func TestPublicationAdminAuthorizationUsesAnyManagedWorkspace(t *testing.T) {
	ctx := context.Background()
	store := testStore(t)
	if _, err := store.SQLDB().ExecContext(ctx, `INSERT INTO workspaces (id, title) VALUES ('secondary', 'Secondary')`); err != nil {
		t.Fatal(err)
	}
	repo := testAccessRepository(store)
	principal, err := repo.SetPrincipalRole(ctx, access.PrincipalRoleInput{
		WorkspaceID: "secondary", Email: "secondary-owner@example.com", DisplayName: "Secondary Owner", Role: access.RoleOwner,
	})
	if err != nil {
		t.Fatal(err)
	}
	token, err := repo.CreateAPIToken(ctx, principal.ID, "test")
	if err != nil {
		t.Fatal(err)
	}
	server := NewWithOptions(fakeMetrics{}, Options{
		Store: store, DefaultWorkspaceID: "test", Auth: testAuth(store, "test", AuthConfig{APITokenOnly: true}),
	})

	request := httptest.NewRequest(http.MethodGet, "/admin/publications", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	recorder := httptest.NewRecorder()
	server.Routes().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("secondary workspace publication manager status = %d, body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestDisabledSuspendedAndRotatedPublicationIDsReturnNotFound(t *testing.T) {
	store := testStore(t)
	seedActivePublication(t, store, "opaque-public-id-12345678901234")
	server := NewWithOptions(fakeMetrics{}, Options{Store: store, DefaultWorkspaceID: "test-workspace"})
	request := func(path string) int {
		recorder := httptest.NewRecorder()
		server.Routes().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		return recorder.Code
	}
	path := "/public/dashboards/opaque-public-id-12345678901234"
	if _, err := store.SQLDB().Exec(`UPDATE dashboard_publications SET suspended_at = CURRENT_TIMESTAMP WHERE name = 'website'`); err != nil {
		t.Fatal(err)
	}
	if got := request(path); got != http.StatusNotFound {
		t.Fatalf("suspended status = %d", got)
	}
	if _, err := store.SQLDB().Exec(`UPDATE dashboard_publications SET suspended_at = NULL, public_id = 'rotated-public-id-123456789012345' WHERE name = 'website'`); err != nil {
		t.Fatal(err)
	}
	if got := request(path); got != http.StatusNotFound {
		t.Fatalf("rotated old id status = %d", got)
	}
	if _, err := store.SQLDB().Exec(`UPDATE dashboard_publications SET configured = 0, active_serving_state_id = NULL WHERE name = 'website'`); err != nil {
		t.Fatal(err)
	}
	if got := request("/public/dashboards/rotated-public-id-123456789012345"); got != http.StatusNotFound {
		t.Fatalf("disabled status = %d", got)
	}
}

func TestEmbedWithNoAllowedOriginsOmitsLegacyFrameHeaderAndDeniesFraming(t *testing.T) {
	header := http.Header{}
	setPublicDashboardSecurityHeaders(header, "embed", nil)
	if got := header.Get("X-Frame-Options"); got != "" {
		t.Fatalf("X-Frame-Options = %q, want omitted", got)
	}
	if got := header.Get("Content-Security-Policy"); !strings.Contains(got, "frame-ancestors 'none'") {
		t.Fatalf("Content-Security-Policy = %q", got)
	}
}

func TestPublicDashboardDocumentsUseDedicatedRateLimitBucket(t *testing.T) {
	store := testStore(t)
	seedActivePublication(t, store, "opaque-public-id-12345678901234")
	server := NewWithOptions(fakeMetrics{}, Options{
		Store: store, DefaultWorkspaceID: "test-workspace",
		RateLimits: RateLimitConfig{Enabled: true, PublicPageLimit: 1, PublicPageWindow: time.Minute},
	})
	handler := server.Routes()
	path := "/public/dashboards/opaque-public-id-12345678901234"
	request := func() int {
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.RemoteAddr = "192.0.2.10:1234"
		handler.ServeHTTP(recorder, req)
		return recorder.Code
	}
	if first, second := request(), request(); first != http.StatusOK || second != http.StatusTooManyRequests {
		t.Fatalf("public page rate limit statuses = %d, %d", first, second)
	}
}

func TestDashboardPublicationManagementAPIRequiresAndReplaysIdempotencyKeys(t *testing.T) {
	store := testStore(t)
	seedActivePublication(t, store, "opaque-public-id-12345678901234")
	server := NewWithOptions(fakeMetrics{}, Options{
		Store: store, DefaultWorkspaceID: "test-workspace", PublicURL: "https://app.leapview.dev",
		Auth: testAuth(store, "test-workspace", AuthConfig{DevBypass: true, DevAPIToken: "local-secret"}),
	})

	list := httptest.NewRecorder()
	listRequest := httptest.NewRequest(http.MethodGet, "/api/v1/workspaces/test-workspace/dashboard-publications", nil)
	listRequest.Header.Set("Authorization", "Bearer local-secret")
	server.Routes().ServeHTTP(list, listRequest)
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), `"publicUrl":"https://app.leapview.dev/public/dashboards/opaque-public-id-12345678901234"`) {
		t.Fatalf("list response = %d %s", list.Code, list.Body.String())
	}

	path := "/api/v1/workspaces/test-workspace/dashboard-publications/website/suspend"
	missing := httptest.NewRecorder()
	missingRequest := httptest.NewRequest(http.MethodPost, path, nil)
	missingRequest.Header.Set("Authorization", "Bearer local-secret")
	server.Routes().ServeHTTP(missing, missingRequest)
	if missing.Code != http.StatusBadRequest {
		t.Fatalf("missing idempotency status = %d, body=%s", missing.Code, missing.Body.String())
	}

	request := func() *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, path, nil)
		r.Header.Set("Idempotency-Key", "suspend-website-1")
		r.Header.Set("Authorization", "Bearer local-secret")
		server.Routes().ServeHTTP(recorder, r)
		return recorder
	}
	first, replay := request(), request()
	if first.Code != http.StatusOK || replay.Code != first.Code || replay.Body.String() != first.Body.String() {
		t.Fatalf("idempotent suspend first=%d %s replay=%d %s", first.Code, first.Body.String(), replay.Code, replay.Body.String())
	}
	if !strings.Contains(first.Body.String(), `"status":"suspended"`) || replay.Header().Get("Idempotency-Replayed") != "true" {
		t.Fatalf("idempotent response headers=%v body=%s", replay.Header(), replay.Body.String())
	}
}

func TestPublicCommandsRequireMatchingLiveStreamAndSuspensionCancelsIt(t *testing.T) {
	store := testStore(t)
	seedActivePublication(t, store, "opaque-public-id-12345678901234")
	server := NewWithOptions(fakeMetrics{}, Options{Store: store, DefaultWorkspaceID: "test-workspace"})
	resolved, err := server.resolvePublicDashboard(context.Background(), "opaque-public-id-12345678901234")
	if err != nil {
		t.Fatal(err)
	}
	clientID, instanceID, pageID := "client-a", "stream-a", "overview"
	streamID := lddatastar.StreamID(clientID, resolved.publication.Dashboard, pageID, instanceID)
	version := publication.StreamVersion{PublicID: resolved.publication.PublicID, ServingStateID: resolved.publication.ServingStateID}
	streamContext, unregister, err := server.publicationStreams.Register(context.Background(), resolved.publication.ID, streamID, version)
	if err != nil {
		t.Fatal(err)
	}
	defer unregister()
	guard := server.publicDashboardHTTP(resolved).CommandGuard
	request := command.Request{DashboardID: resolved.publication.Dashboard, ModelID: resolved.modelID, PageID: pageID}
	signals := dashboard.Signals{Runtime: dashboard.Runtime{ClientID: clientID, StreamInstanceID: instanceID}}
	if err := guard(httptest.NewRequest(http.MethodPost, "/", nil), resolved.metrics, request, signals); err != nil {
		t.Fatalf("matching stream rejected: %v", err)
	}
	secondServer := NewWithOptions(fakeMetrics{}, Options{Store: store, DefaultWorkspaceID: "test-workspace"})
	secondResolved, err := secondServer.resolvePublicDashboard(context.Background(), "opaque-public-id-12345678901234")
	if err != nil {
		t.Fatal(err)
	}
	if err := secondServer.publicDashboardHTTP(secondResolved).CommandGuard(httptest.NewRequest(http.MethodPost, "/", nil), secondResolved.metrics, request, signals); err != nil {
		t.Fatalf("matching stream was rejected by a second replica: %v", err)
	}
	signals.Runtime.StreamInstanceID = "other-stream"
	if err := guard(httptest.NewRequest(http.MethodPost, "/", nil), resolved.metrics, request, signals); err == nil {
		t.Fatal("mismatched stream was accepted")
	}
	if _, err := store.SQLDB().Exec(`UPDATE dashboard_publications SET suspended_at = CURRENT_TIMESTAMP WHERE id = ?`, resolved.publication.ID); err != nil {
		t.Fatal(err)
	}
	server.publicationStreams.ClosePublication(resolved.publication.ID)
	select {
	case <-streamContext.Done():
	default:
		t.Fatal("suspension did not cancel the live publication stream")
	}
	signals.Runtime.StreamInstanceID = instanceID
	if err := guard(httptest.NewRequest(http.MethodPost, "/", nil), resolved.metrics, request, signals); err == nil {
		t.Fatal("suspended publication command was accepted")
	}
}

func TestPublicationBrokerRelaysEventsAcrossReplicas(t *testing.T) {
	store := testStore(t)
	first := publicationsqlite.NewBroker(store.SQLDB(), nil, nil)
	second := publicationsqlite.NewBroker(store.SQLDB(), nil, nil)
	updates, unsubscribe := first.Subscribe("shared-public-stream")
	defer unsubscribe()

	second.PublishEnvelope("shared-public-stream", pagestream.Envelope{
		Signals:  pagestream.SignalPatch{"status": map[string]any{"generation": 2}},
		Delivery: pagestream.DeliveryMetadata{Generation: 2, Boundary: true},
	})
	select {
	case patch := <-updates:
		status, ok := patch["status"].(map[string]any)
		if !ok || status["generation"] != float64(2) {
			t.Fatalf("relayed patch = %#v", patch)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("publication event was not relayed across replicas")
	}
}

func TestPublicationCommandGenerationAdvancesAcrossReplicas(t *testing.T) {
	store := testStore(t)
	seedActivePublication(t, store, "opaque-public-id-12345678901234")
	first := publicationsqlite.NewStreamRegistry(store.SQLDB())
	second := publicationsqlite.NewStreamRegistry(store.SQLDB())
	version := publication.StreamVersion{PublicID: "opaque-public-id-12345678901234", ServingStateID: "state_public"}
	_, unregister, err := first.Register(context.Background(), "pub_website", "shared-command-stream", version)
	if err != nil {
		t.Fatal(err)
	}
	defer unregister()
	prepare := func(filters dashboard.Filters) (command.PreparedRefresh, error) {
		return command.PreparedRefresh{Filters: filters}, nil
	}
	_, firstGeneration, err := first.PrepareCommand(context.Background(), "pub_website", "shared-command-stream", version, prepare)
	if err != nil {
		t.Fatal(err)
	}
	_, secondGeneration, err := second.PrepareCommand(context.Background(), "pub_website", "shared-command-stream", version, prepare)
	if err != nil {
		t.Fatal(err)
	}
	if firstGeneration != 2 || secondGeneration != 3 {
		t.Fatalf("distributed generations = %d, %d; want 2, 3", firstGeneration, secondGeneration)
	}
}

func seedActivePublication(t *testing.T, store *platform.Store, publicID string) {
	t.Helper()
	statements := []struct {
		query string
		args  []any
	}{
		{query: `INSERT INTO workspaces (id, title) VALUES ('test-workspace', 'Test Workspace')`},
		{query: `INSERT INTO serving_states (id, workspace_id, project_id, environment, status, source) VALUES ('state_public', 'test-workspace', 'site', 'prod', 'active', 'publish')`},
		{query: `INSERT INTO dashboard_publications (id, project_id, workspace_id, name, public_id, dashboard, default_page, configuration_digest, allowed_origins_json, dependency_asset_ids_json, configured, active_serving_state_id, configured_at) VALUES ('pub_website', 'site', 'test-workspace', 'website', ?, 'executive-sales', 'overview', 'sha256:test', '["https://partner.example","https://leapview.dev"]', '["dashboard:test-workspace.executive-sales","semantic_model:test-workspace.test"]', 1, 'state_public', CURRENT_TIMESTAMP)`, args: []any{publicID}},
	}
	for _, statement := range statements {
		if _, err := store.SQLDB().Exec(statement.query, statement.args...); err != nil {
			t.Fatal(err)
		}
	}
}
