package integration

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	accesssqlite "github.com/Yacobolo/leapview/internal/access/sqlite"
	analyticsduckdb "github.com/Yacobolo/leapview/internal/analytics/duckdb"
	materializeruntime "github.com/Yacobolo/leapview/internal/analytics/materialize"
	semanticmodel "github.com/Yacobolo/leapview/internal/analytics/model"
	"github.com/Yacobolo/leapview/internal/app"
	"github.com/Yacobolo/leapview/internal/dashboard"
	"github.com/Yacobolo/leapview/internal/dashboard/consumer"
	reportdef "github.com/Yacobolo/leapview/internal/dashboard/report"
	dashboardruntime "github.com/Yacobolo/leapview/internal/dashboard/runtime"
	"github.com/Yacobolo/leapview/internal/dataquery"
	"github.com/Yacobolo/leapview/internal/manageddata"
	"github.com/Yacobolo/leapview/internal/platform"
	servingstate "github.com/Yacobolo/leapview/internal/servingstate"
	servingstatefs "github.com/Yacobolo/leapview/internal/servingstate/filesystem"
	servingstatesqlite "github.com/Yacobolo/leapview/internal/servingstate/sqlite"
	"github.com/Yacobolo/leapview/internal/testutil/ssetest"
	"github.com/Yacobolo/leapview/internal/workspace"
	workspacecompiler "github.com/Yacobolo/leapview/internal/workspace/compiler"
	workspacesqlite "github.com/Yacobolo/leapview/internal/workspace/sqlite"
)

type harness struct {
	handler     http.Handler
	server      *httptest.Server
	store       *platform.Store
	workspaceID string
}

var integrationOlistManagedDataRevision = integrationManagedDataManifest().RevisionID()

func integrationManagedDataManifest() manageddata.Manifest {
	return manageddata.Manifest{Files: []manageddata.File{{
		Path: "fixture.csv", Size: 1, SHA256: strings.Repeat("a", 64),
	}}}
}

var integrationDataInitUpdatesPattern = regexp.MustCompile(`data-init="@get\('([^']+)'`)

type harnessConfig struct {
	catalogPath string
	fixture     func(t *testing.T, dir string)
	wrapMetrics func(*dashboardruntime.Service) integrationMetrics
}

type harnessOption func(*harnessConfig)

type integrationMetrics interface {
	consumer.Executor
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
	ExecuteDataQuery(ctx context.Context, request dataquery.Query) (dataquery.Result, error)
	QuerySemantic(ctx context.Context, modelID string, request reportdef.AggregateQuery) (reportdef.QueryRows, error)
	PreviewSemantic(ctx context.Context, modelID string, request reportdef.RowQuery) (reportdef.QueryRows, error)
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

	metrics, err := newHarnessRuntime(dataDir, config.catalogPath, duckDBDir)
	if err != nil {
		t.Fatalf("create dashboard runtime: %v", err)
	}
	t.Cleanup(func() { _ = metrics.Close() })

	metricsForApp := integrationMetrics(metrics)
	if config.wrapMetrics != nil {
		metricsForApp = config.wrapMetrics(metrics)
	}

	h := &harness{
		handler:     app.New(metricsForApp).Routes(),
		workspaceID: metricsForApp.Catalog().Workspace.ID,
	}
	h.server = httptest.NewServer(h.handler)
	t.Cleanup(h.server.Close)
	return h
}

func newStoreBackedHarness(t *testing.T, opts ...harnessOption) *harness {
	t.Helper()

	h, metrics, catalogPath := newHarnessWithMetrics(t, opts...)
	ctx := context.Background()
	store, err := platform.Open(ctx, filepath.Join(t.TempDir(), "leapview.db"))
	if err != nil {
		t.Fatalf("open platform store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	workspaceID := metrics.Catalog().Workspace.ID
	if workspaceID == "" {
		workspaceID = platform.DefaultWorkspaceID
	}
	workspaceRepo := workspacesqlite.NewRepository(store.SQLDB())
	if err := workspaceRepo.Ensure(ctx, workspace.EnsureInput{ID: workspace.WorkspaceID(workspaceID), Title: metrics.Catalog().Workspace.Title, Description: metrics.Catalog().Workspace.Description}); err != nil {
		t.Fatalf("ensure integration workspace: %v", err)
	}
	accessRepo := accesssqlite.NewRepository(store.SQLDB())
	if err := app.SeedLocalDeveloperPlatformAdmin(ctx, accessRepo); err != nil {
		t.Fatalf("seed local developer: %v", err)
	}
	seedIntegrationActiveDeployment(t, store, workspaceID, catalogPath)

	auth := app.NewAuth(accessRepo, workspaceID, app.AuthConfig{DevBypass: true})
	server := app.NewWithOptions(metrics, app.Options{
		Store:              store,
		Auth:               auth,
		DefaultWorkspaceID: workspaceID,
	})
	h.handler = server.Routes()
	h.store = store
	h.server = httptest.NewServer(h.handler)
	t.Cleanup(h.server.Close)
	return h
}

func newHarnessWithMetrics(t *testing.T, opts ...harnessOption) (*harness, integrationMetrics, string) {
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

	metrics, err := newHarnessRuntime(dataDir, config.catalogPath, duckDBDir)
	if err != nil {
		t.Fatalf("create dashboard runtime: %v", err)
	}
	t.Cleanup(func() { _ = metrics.Close() })

	metricsForApp := integrationMetrics(metrics)
	if config.wrapMetrics != nil {
		metricsForApp = config.wrapMetrics(metrics)
	}
	return &harness{workspaceID: metricsForApp.Catalog().Workspace.ID}, metricsForApp, config.catalogPath
}

func newHarnessRuntime(dataDir, catalogPath, duckDBDir string) (*dashboardruntime.Service, error) {
	compiled, err := workspacecompiler.CompileProject(catalogPath, workspacecompiler.Options{})
	if err != nil {
		return nil, err
	}
	compiledWorkspace, ok := compiled.Workspaces["sales"]
	if !ok {
		return nil, fmt.Errorf("project has no sales workspace")
	}
	if err := bindManagedConnectionRoots(compiledWorkspace.Definition, dataDir); err != nil {
		return nil, err
	}
	service, err := dashboardruntime.NewFromDefinition(filepath.Join(duckDBDir, "sales"), integrationDataRuntimeFactory{}, compiledWorkspace.Definition)
	if err != nil {
		return nil, fmt.Errorf("loading workspace %q: %w", "sales", err)
	}
	return service, nil
}

func integrationOlistManagedDataRevisions() map[string]string {
	return map[string]string{"olist": integrationOlistManagedDataRevision}
}

func bindManagedConnectionRoots(definition *workspace.Definition, root string) error {
	if definition == nil {
		return fmt.Errorf("managed connection root binding requires a workspace definition")
	}
	if root == "" || !filepath.IsAbs(root) {
		return fmt.Errorf("managed connection root must be absolute: %q", root)
	}
	for modelID, model := range definition.Models {
		if model == nil {
			return fmt.Errorf("workspace definition contains nil model %q", modelID)
		}
		for connectionName, connection := range model.Connections {
			if connection.Kind != "managed" {
				continue
			}
			connection.Root = root
			model.Connections[connectionName] = connection
		}
	}
	return nil
}

func bindManagedConnectionRootsInArtifact(t *testing.T, artifactPath, root string) {
	t.Helper()

	extractedRoot := t.TempDir()
	if err := servingstatefs.ExtractArtifact(artifactPath, extractedRoot); err != nil {
		t.Fatalf("extract integration artifact: %v", err)
	}
	compiled, manifest, err := servingstatefs.LoadCompiledWorkspaceArtifact(extractedRoot)
	if err != nil {
		t.Fatalf("load integration artifact: %v", err)
	}
	if err := bindManagedConnectionRoots(compiled.Definition, root); err != nil {
		t.Fatalf("bind integration artifact managed roots: %v", err)
	}
	compiledBytes, err := json.MarshalIndent(compiled, "", "  ")
	if err != nil {
		t.Fatalf("marshal integration artifact: %v", err)
	}
	compiledPath := filepath.Join(extractedRoot, filepath.FromSlash(manifest.CompiledPath))
	if err := os.WriteFile(compiledPath, compiledBytes, 0o644); err != nil {
		t.Fatalf("write integration artifact compiled definition: %v", err)
	}
	manifest.GraphHash = integrationDigestBytes(compiledBytes)
	manifestBytes, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatalf("marshal integration artifact manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(extractedRoot, "manifest.json"), manifestBytes, 0o644); err != nil {
		t.Fatalf("write integration artifact manifest: %v", err)
	}

	temporaryPath, err := os.CreateTemp(filepath.Dir(artifactPath), ".integration-artifact-*.tar.gz")
	if err != nil {
		t.Fatalf("create rewritten integration artifact: %v", err)
	}
	temporaryName := temporaryPath.Name()
	defer os.Remove(temporaryName)
	gzipWriter := gzip.NewWriter(temporaryPath)
	tarWriter := tar.NewWriter(gzipWriter)
	if err := filepath.WalkDir(extractedRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(extractedRoot, path)
		if err != nil {
			return err
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := tarWriter.WriteHeader(&tar.Header{Name: filepath.ToSlash(relative), Mode: 0o644, Size: int64(len(content))}); err != nil {
			return err
		}
		_, err = tarWriter.Write(content)
		return err
	}); err != nil {
		_ = tarWriter.Close()
		_ = gzipWriter.Close()
		_ = temporaryPath.Close()
		t.Fatalf("rewrite integration artifact: %v", err)
	}
	if err := tarWriter.Close(); err != nil {
		_ = gzipWriter.Close()
		_ = temporaryPath.Close()
		t.Fatalf("close rewritten integration artifact tar: %v", err)
	}
	if err := gzipWriter.Close(); err != nil {
		_ = temporaryPath.Close()
		t.Fatalf("close rewritten integration artifact gzip: %v", err)
	}
	if err := temporaryPath.Close(); err != nil {
		t.Fatalf("close rewritten integration artifact: %v", err)
	}
	if err := os.Rename(temporaryName, artifactPath); err != nil {
		t.Fatalf("replace integration artifact: %v", err)
	}
}

func integrationDigestBytes(content []byte) string {
	digest := sha256.Sum256(content)
	return hex.EncodeToString(digest[:])
}

func (h *harness) getUpdates(t *testing.T, dashboardID, pageID string, signals map[string]any) string {
	t.Helper()

	return h.getUpdatesWithQuery(t, dashboardID, pageID, signals, nil)
}

func (h *harness) getUpdatesWithQuery(t *testing.T, dashboardID, pageID string, signals map[string]any, query url.Values) string {
	t.Helper()

	encodedSignals, err := json.Marshal(signals)
	if err != nil {
		t.Fatalf("marshal Datastar signals: %v", err)
	}
	values := url.Values{}
	values.Set("route", "dashboard")
	values.Set("workspace", h.workspaceIDOrDefault())
	values.Set("dashboard", dashboardID)
	values.Set("page", pageID)
	if streamInstanceID := streamInstanceIDFromSignals(signals); streamInstanceID != "" {
		values.Set("streamInstance", streamInstanceID)
	}
	values.Set("datastar", string(encodedSignals))
	for key, vals := range query {
		for _, value := range vals {
			values.Add(key, value)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()
	req := httptest.NewRequestWithContext(ctx, http.MethodGet, h.workspaceUpdatesPath()+"?"+values.Encode(), nil)
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
	return patchSignalsFromBody(t, body)
}

func (h *harness) getUpdatesSignalsWithQuery(t *testing.T, dashboardID, pageID string, signals map[string]any, query url.Values) []map[string]any {
	t.Helper()

	body := h.getUpdatesWithQuery(t, dashboardID, pageID, signals, query)
	return patchSignalsFromBody(t, body)
}

func patchSignalsFromBody(t *testing.T, body string) []map[string]any {
	t.Helper()

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
	values.Set("route", "dashboard")
	values.Set("workspace", h.workspaceIDOrDefault())
	values.Set("dashboard", dashboardID)
	values.Set("page", pageID)
	if streamInstanceID := streamInstanceIDFromSignals(signals); streamInstanceID != "" {
		values.Set("streamInstance", streamInstanceID)
	}
	values.Set("datastar", string(encodedSignals))

	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, serverURL+h.workspaceUpdatesPath()+"?"+values.Encode(), nil)
	if err != nil {
		cancel()
		t.Fatalf("create updates request: %v", err)
	}
	if clientID := clientIDFromSignals(signals); clientID != "" {
		req.AddCookie(&http.Cookie{Name: "lv_client_id", Value: clientID})
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
	req, err := http.NewRequest(http.MethodPost, h.serverURL(t)+h.workspaceCommandPath(path), bytes.NewReader(encodedSignals))
	if err != nil {
		t.Fatalf("create command request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if clientID := clientIDFromSignals(signals); clientID != "" {
		req.AddCookie(&http.Cookie{Name: "lv_client_id", Value: clientID})
	}
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

func (h *harness) getAuthenticated(t *testing.T, path string) string {
	t.Helper()

	req, err := http.NewRequest(http.MethodGet, h.serverURL(t)+path, nil)
	if err != nil {
		t.Fatalf("create GET %s request: %v", path, err)
	}
	req.Header.Set("Authorization", "Bearer dev")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	if res.StatusCode >= 400 {
		t.Fatalf("GET %s status = %d, body:\n%s", path, res.StatusCode, string(body))
	}
	return string(body)
}

func (h *harness) getAuthenticatedHydrated(t *testing.T, path string) string {
	t.Helper()
	body := h.getAuthenticated(t, path)
	return html.UnescapeString(body) + h.streamPageBootstrap(t, body)
}

func (h *harness) streamPageBootstrap(t *testing.T, pageBody string) string {
	t.Helper()
	decoded := html.UnescapeString(pageBody)
	matches := integrationDataInitUpdatesPattern.FindStringSubmatch(decoded)
	if len(matches) != 2 {
		t.Fatalf("rendered page did not include literal /updates data-init:\n%s", pageBody)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, h.serverURL(t)+matches[1], nil)
	if err != nil {
		t.Fatalf("create bootstrap request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer dev")
	req.AddCookie(&http.Cookie{Name: "lv_client_id", Value: "integration-stream-first"})
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET bootstrap %s: %v", matches[1], err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(res.Body)
		t.Fatalf("GET bootstrap %s status = %d, body:\n%s", matches[1], res.StatusCode, string(body))
	}
	client := &streamClient{
		cancel:  cancel,
		body:    res.Body,
		patches: make(chan map[string]any, 16),
		errs:    make(chan error, 1),
	}
	go client.read()
	t.Cleanup(client.close)
	patch := client.nextPatch(t)
	cancel()
	return patchString(patch)
}

func patchString(patch map[string]any) string {
	encoded, err := json.Marshal(patch)
	if err != nil {
		return fmt.Sprintf("%#v", patch)
	}
	return string(encoded)
}

func (h *harness) postAuthenticated(t *testing.T, path string) int {
	t.Helper()

	req, err := http.NewRequest(http.MethodPost, h.serverURL(t)+path, nil)
	if err != nil {
		t.Fatalf("create POST %s request: %v", path, err)
	}
	req.Header.Set("Authorization", "Bearer dev")
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

func (h *harness) openAssetUpdatesStream(t *testing.T, workspaceID, assetID, section string) *streamClient {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	values := url.Values{}
	values.Set("route", "workspace_asset")
	values.Set("workspace", workspaceID)
	values.Set("asset", assetID)
	values.Set("section", section)
	path := h.serverURL(t) + "/updates?" + values.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, path, nil)
	if err != nil {
		cancel()
		t.Fatalf("create asset updates request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer dev")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		cancel()
		t.Fatalf("open asset updates stream: %v", err)
	}
	if res.StatusCode != http.StatusOK {
		defer res.Body.Close()
		body, _ := io.ReadAll(res.Body)
		cancel()
		t.Fatalf("GET asset updates status = %d, body:\n%s", res.StatusCode, string(body))
	}
	if got := res.Header.Get("Content-Type"); !strings.HasPrefix(got, "text/event-stream") {
		_ = res.Body.Close()
		cancel()
		t.Fatalf("GET asset updates content type = %q, want text/event-stream", got)
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

func (h *harness) serverURL(t *testing.T) string {
	t.Helper()
	return h.server.URL
}

func (h *harness) workspaceUpdatesPath() string {
	return "/updates"
}

func (h *harness) workspaceCommandPath(path string) string {
	if strings.HasPrefix(path, "/workspaces/") {
		return path
	}
	if strings.HasPrefix(path, "/commands/") {
		return "/workspaces/" + h.workspaceIDOrDefault() + path
	}
	return path
}

func (h *harness) workspaceIDOrDefault() string {
	if h.workspaceID != "" {
		return h.workspaceID
	}
	return platform.DefaultWorkspaceID
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
		candidate := filepath.Join(dir, "dashboards", "leapview.yaml")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find dashboards/leapview.yaml")
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

func seedIntegrationActiveDeployment(t *testing.T, store *platform.Store, workspaceID, catalogPath string) {
	t.Helper()
	ctx := context.Background()
	deploymentRepo := servingstatesqlite.NewRepository(store.SQLDB())
	created, err := deploymentRepo.Create(ctx, servingstate.CreateInput{WorkspaceID: servingstate.WorkspaceID(workspaceID), CreatedBy: "integration"})
	if err != nil {
		t.Fatalf("create deployment: %v", err)
	}
	compiled, err := workspacecompiler.CompileProject(catalogPath, workspacecompiler.Options{})
	if err != nil {
		t.Fatalf("compile project: %v", err)
	}
	selected := compiled.Workspaces[workspaceID]
	workspaceDef := selected.Definition
	if workspaceDef == nil {
		t.Fatalf("compile project: missing workspace %q", workspaceID)
	}
	graph, err := workspacecompiler.ExtractLineage(workspace.WorkspaceID(workspaceID), workspace.ServingStateID(created.ID), workspaceDef)
	if err != nil {
		t.Fatalf("extract workspace assets: %v", err)
	}
	validation := servingstate.Validation{
		Digest:            "digest-" + string(created.ID),
		ManifestJSON:      "{}",
		ProjectID:         compiled.Project.Name,
		ProjectDigest:     "sha256:" + strings.Repeat("a", 64),
		ProjectWorkspaces: []string{workspaceID},
		Graph:             graph,
	}
	if _, err := deploymentRepo.SaveValidated(ctx, created.ID, validation, integrationZeroArtifact(created.ID, workspaceID)); err != nil {
		t.Fatalf("save validated deployment: %v", err)
	}
	if _, err := deploymentRepo.Activate(ctx, servingstate.WorkspaceID(workspaceID), servingstate.DefaultEnvironment, created.ID); err != nil {
		t.Fatalf("activate serving state: %v", err)
	}
}

func integrationAssetID(t *testing.T, store *platform.Store, workspaceID, assetType, key string) string {
	t.Helper()
	repo := workspacesqlite.NewRepository(store.SQLDB())
	graph, ok, err := repo.ActiveServingStateGraph(context.Background(), workspace.WorkspaceID(workspaceID), string(servingstate.DefaultEnvironment))
	if err != nil {
		t.Fatalf("active serving-state graph: %v", err)
	}
	if !ok {
		t.Fatalf("workspace %q has no active serving-state graph", workspaceID)
	}
	for _, asset := range graph.Assets {
		if string(asset.Type) == assetType && asset.Key == key {
			return string(asset.ID)
		}
	}
	t.Fatalf("asset %s %q not found in active graph", assetType, key)
	return ""
}

func integrationZeroArtifact(deploymentID servingstate.ID, workspaceID string) servingstate.Artifact {
	return servingstate.Artifact{
		ID:             "artifact_" + string(deploymentID),
		ServingStateID: deploymentID,
		WorkspaceID:    servingstate.WorkspaceID(workspaceID),
		Digest:         "digest",
		Format:         "tar.gz",
		Path:           "artifact.tar.gz",
		ManifestJSON:   "{}",
	}
}

type integrationDataRuntimeFactory struct{}

func (integrationDataRuntimeFactory) OpenDashboardDataRuntime(ctx context.Context, config dashboardruntime.DataRuntimeConfig) (dashboardruntime.DataRuntime, error) {
	runtime, err := analyticsduckdb.OpenMaterializeRuntime(ctx, materializeruntime.RuntimeConfig{
		ModelID: config.ModelID,
		Model:   config.Model,
		DBDir:   config.DBDir,
	})
	if err != nil {
		return nil, err
	}
	return integrationDataRuntime{
		runtime: runtime,
		data:    reportdef.NewDataQueryService(config.ModelID, reportdef.NewAnalyticsDataService(runtime.Queries()), runtime),
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

func (r integrationDataRuntime) ExecuteDataQuery(ctx context.Context, request dataquery.Query) (dataquery.Result, error) {
	return r.runtime.ExecuteDataQuery(ctx, request)
}

func (r integrationDataRuntime) Refresh(ctx context.Context) error {
	return r.runtime.Refresh(ctx)
}

func (r integrationDataRuntime) RefreshTables(ctx context.Context, tableNames []string) error {
	return r.runtime.RefreshModelTables(ctx, tableNames)
}

func (r integrationDataRuntime) Close() error {
	return r.runtime.Close()
}

func (r integrationDataRuntime) LastRefresh() time.Time {
	return r.runtime.LastRefresh()
}
