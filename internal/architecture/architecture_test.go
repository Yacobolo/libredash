package architecture

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const modulePath = "github.com/Yacobolo/libredash"

type goFile struct {
	path    string
	pkgDir  string
	imports []string
	body    string
}

func TestUseCasesDoNotImportAdapters(t *testing.T) {
	for _, file := range productionGoFiles(t) {
		if !isInternalPackage(file.pkgDir) || isAdapterOrCompositionPackage(file.pkgDir) {
			continue
		}
		for _, imported := range file.imports {
			if isForbiddenUseCaseImport(imported) {
				t.Fatalf("%s imports adapter or transport package %s", file.path, imported)
			}
		}
	}
}

func TestAPIPackageIsTransportContractOnly(t *testing.T) {
	for _, file := range productionGoFiles(t) {
		if file.pkgDir != "internal/api" {
			continue
		}
		for _, imported := range file.imports {
			if imported == modulePath+"/internal/app" ||
				imported == modulePath+"/internal/ui" ||
				imported == "net/http" ||
				imported == "github.com/go-chi/chi/v5" ||
				strings.Contains(imported, "datastar") ||
				strings.Contains(imported, "gomponents") {
				t.Fatalf("%s imports forbidden API dependency %s", file.path, imported)
			}
		}
	}
}

func TestUIPackageIsRenderOnly(t *testing.T) {
	for _, file := range productionGoFiles(t) {
		if file.pkgDir != "internal/ui" {
			continue
		}
		for _, imported := range file.imports {
			if imported == modulePath+"/internal/api" ||
				imported == modulePath+"/internal/platform/db" ||
				imported == "database/sql" ||
				imported == "net/http" ||
				imported == "github.com/go-chi/chi/v5" {
				t.Fatalf("%s imports forbidden UI dependency %s", file.path, imported)
			}
		}
		for _, forbidden := range []string{".QueryContext(", ".QueryRowContext(", ".ExecContext("} {
			if strings.Contains(file.body, forbidden) {
				t.Fatalf("%s performs storage access via %s", file.path, forbidden)
			}
		}
	}
}

func TestAppIsCompositionOnly(t *testing.T) {
	for _, file := range productionGoFiles(t) {
		if file.pkgDir != "internal/app" {
			continue
		}
		for _, forbidden := range []string{
			".SQLDB().QueryContext(",
			".SQLDB().QueryRowContext(",
			".SQLDB().ExecContext(",
			"func (s *Server) api",
			"func (s *Server) list",
			"func (s *Server) get",
			"func (s *Server) create",
			"func (s *Server) update",
			"func (s *Server) delete",
			"func (s *Server) upload",
			"func (s *Server) validate",
			"func (s *Server) activate",
			"type dataAuthorizationMetrics",
			"func routeObjectRefs(",
			"func authObjectsForRequest(",
			"func dataQueryObjects(",
			"func dataQueryPrivilege(",
			"func rowFiltersFromPolicy(",
			"func columnMaskFromPolicy(",
		} {
			if strings.Contains(file.body, forbidden) {
				t.Fatalf("%s contains product behavior marker %q", file.path, forbidden)
			}
		}
	}
}

func TestRequiredCapabilityAdaptersExist(t *testing.T) {
	root := repoRoot(t)
	for _, dir := range []string{
		"internal/access/http",
		"internal/admin/http",
		"internal/agent/http",
		"internal/analytics/connectors",
		"internal/analytics/materialize/http",
		"internal/analytics/query/http",
		"internal/dashboard/http",
		"internal/servingstate/http",
		"internal/workspace/datastar",
		"internal/workspace/http",
	} {
		if !packageDirExists(root, dir) {
			t.Fatalf("required capability adapter package %s does not exist", dir)
		}
	}
}

func TestAppDoesNotOwnKnownProductRouteFamilies(t *testing.T) {
	for _, file := range productionGoFiles(t) {
		if file.pkgDir != "internal/app" {
			continue
		}
		for _, forbidden := range []string{
			"func (s *Server) workspaceAPI",
			"func (s *Server) workspaces(",
			"func (s *Server) workspaceAssets(",
			"func (s *Server) workspaceAsset(",
			"func (s *Server) workspaceAssetSection(",
			"func (s *Server) connections(",
			"func (s *Server) connectionAsset(",
			"func (s *Server) connectionAssetSection(",
			"func (s *Server) connectionSourceAsset(",
			"func (s *Server) connectionSourceAssetSection(",
			"func (s *Server) workspacePermissions(",
			"func (s *Server) workspacePermissionUpdate(",
			"func (s *Server) workspaceAssetUpdates(",
			"func (s *Server) refreshWorkspaceAsset(",
			"func (s *Server) refreshWorkspaceAssetMaterializations(",
			"func (s *Server) adminGeneral(",
			"func (s *Server) adminPrincipals(",
			"func (s *Server) adminPrincipalDetail(",
			"func (s *Server) adminGroups(",
			"func (s *Server) adminGroupDetail(",
			"func (s *Server) adminStorage(",
			"func (s *Server) adminQueries(",
			"func (s *Server) chat(",
			"func (s *Server) chatNew(",
			"func (s *Server) chatConversation(",
			"func (s *Server) chatTurn(",
			"func (s *Server) chatUpdates(",
			"func (s *Server) dataExplorer(",
			"func (s *Server) dataExplorerUpdates(",
			"func (s *Server) dataExplorerCommand(",
			"func (s *Server) workspaceDataExplorerRedirect(",
			"func (s *Server) searchWorkspace(",
			"func (s *Server) renderWorkspacesPage(",
			"func (s *Server) renderWorkspaceAssetsPage(",
			"func (s *Server) renderConnectionsPage(",
			"func (s *Server) renderWorkspaceAssetRedirect(",
			"func (s *Server) renderWorkspaceAssetSection(",
			"func (s *Server) renderConnectionAssetRedirect(",
			"func (s *Server) renderConnectionAssetSection(",
			"func (s *Server) renderConnectionSourceAssetRedirect(",
			"func (s *Server) renderConnectionSourceAssetSection(",
			"func (s *Server) assetRefreshPost(",
			"func (s *Server) assetUpdatesStream(",
			"func (s *Server) refreshWorkspaceAssetWithPatches(",
			"func (s *Server) refreshWorkspaceAssetDeploymentWithPatches(",
			"func (s *Server) openWorkspaceRefreshRuntime(",
			"func (s *Server) runWorkspaceAssetRefreshWithPatches(",
			"func (s *Server) queueWorkspaceAssetRefreshWithPatches(",
			"func (s *Server) refreshSemanticModelAssetWithPatches(",
			"func (s *Server) refreshModelTableAssetWithPatches(",
			"func (s *Server) publishWorkspaceAssetRefreshPatch(",
			"func (s *Server) publishModelRefreshPatches(",
			"func (s *Server) publishWorkspaceAssetRefreshPatchesForTarget(",
			"func (s *Server) assetRefreshStateForContext(",
		} {
			if strings.Contains(file.body, forbidden) {
				t.Fatalf("%s still owns product route family %q", file.path, forbidden)
			}
		}
	}
}

func TestAppDoesNotOwnAgentToolBehavior(t *testing.T) {
	for _, file := range productionGoFiles(t) {
		if file.pkgDir != "internal/app" {
			continue
		}
		for _, forbidden := range []string{
			"func (s *Server) agentAPIGenToolDefinitions(",
			"func (s *Server) runAPIGenAgentTool(",
			"func (s *Server) agentVisualToolDefinitions(",
			"func (s *Server) runAgentVisualTool(",
			"func (s *Server) queryAgentVisual(",
		} {
			if strings.Contains(file.body, forbidden) {
				t.Fatalf("%s still owns agent tool behavior %q", file.path, forbidden)
			}
		}
	}
}

func TestAppDoesNotKeepStaleBIAPIHelpers(t *testing.T) {
	for _, file := range productionGoFiles(t) {
		if file.pkgDir != "internal/app" {
			continue
		}
		for _, forbidden := range []string{
			"func boundedPatch(",
			"func boundedVisual(",
			"func boundedTable(",
			"func dashboardSummaryDTO(",
			"func semanticModelSummaryDTO(",
			"func (s *Server) semanticModelForRequest(",
			"func (s *Server) semanticDatasetForRequest(",
			"func semanticDatasetDTO(",
			"func semanticAggregateRequest(",
			"func semanticRowRequest(",
		} {
			if strings.Contains(file.body, forbidden) {
				t.Fatalf("%s still keeps stale BI API helper %q", file.path, forbidden)
			}
		}
	}
}

func TestAppDoesNotOwnRemainingAdminWorkspaceBehavior(t *testing.T) {
	for _, file := range productionGoFiles(t) {
		if file.pkgDir != "internal/app" {
			continue
		}
		for _, forbidden := range []string{
			"database/sql",
			"github.com/duckdb/duckdb-go",
			"datastar.ReadSignals(",
			"MarshalAndPatchSignals(",
			".QueryContext(",
			".QueryRowContext(",
			".ExecContext(",
			"func (s *Server) adminStorage",
			"func (s *Server) adminQueryHistory",
			"func (s *Server) adminData(",
			"func (s *Server) adminAgentData(",
			"func (s *Server) adminPrincipalsData(",
			"func (s *Server) adminGroupsData(",
			"func (s *Server) adminGroupMembersData(",
			"func (s *Server) adminRoleBindings",
			"func buildAdmin",
			"func (s *Server) upsertWorkspaceAccess(",
			"func (s *Server) removeWorkspaceAccess(",
			"func (s *Server) workspaceList(",
			"func (s *Server) workspaceResponse(",
			"func (s *Server) workspaceAssetsAndEdges(",
			"func (s *Server) platformConnectionAssetsAndEdges(",
			"func (s *Server) roleBindingsAndRoles(",
			"func (s *Server) workspaceAccessResponse(",
			"func (s *Server) canManageWorkspaceAccess(",
			"func apiWorkspaceDTOs(",
			"func apiAssetDTOs(",
			"func apiWorkspaceAssetGraphDTO(",
		} {
			if strings.Contains(file.body, forbidden) || importListContains(file.imports, forbidden) {
				t.Fatalf("%s still owns app behavior marker %q", file.path, forbidden)
			}
		}
	}
}

func TestAdminHTTPDoesNotDelegateStorageAndQueryHistoryBackToApp(t *testing.T) {
	for _, file := range productionGoFiles(t) {
		if file.pkgDir != "internal/admin/http" {
			continue
		}
		for _, forbidden := range []string{
			"QueryHistoryUpdates nethttp.HandlerFunc",
			"QueryHistoryCommand nethttp.HandlerFunc",
			"StorageUpdates      nethttp.HandlerFunc",
			"StorageSelectTable  nethttp.HandlerFunc",
		} {
			if strings.Contains(file.body, forbidden) {
				t.Fatalf("%s delegates admin behavior through %q", file.path, forbidden)
			}
		}
	}
}

func TestWorkspaceHTTPDoesNotDelegateProductRoutesBackToApp(t *testing.T) {
	for _, file := range productionGoFiles(t) {
		if file.pkgDir != "internal/workspace/http" {
			continue
		}
		for _, forbidden := range []string{
			"WorkspaceCatalogPage   nethttp.HandlerFunc",
			"WorkspaceAssetsPage    nethttp.HandlerFunc",
			"WorkspaceAssetPage     nethttp.HandlerFunc",
			"WorkspaceAssetDetail   nethttp.HandlerFunc",
			"ConnectionsPage        nethttp.HandlerFunc",
			"ConnectionSourcePage   nethttp.HandlerFunc",
			"ConnectionSourceDetail nethttp.HandlerFunc",
			"ConnectionAssetPage    nethttp.HandlerFunc",
			"ConnectionAssetDetail  nethttp.HandlerFunc",
			"AssetUpdates           nethttp.HandlerFunc",
			"AssetRefresh           nethttp.HandlerFunc",
			"AssetMaterialize       nethttp.HandlerFunc",
		} {
			if strings.Contains(file.body, forbidden) {
				t.Fatalf("%s delegates product route behavior through %q", file.path, forbidden)
			}
		}
	}
}

func TestPlatformStoreSQLDBDoesNotLeakPastCompositionAndAdapters(t *testing.T) {
	for _, file := range productionGoFiles(t) {
		if !strings.Contains(file.body, ".SQLDB()") {
			continue
		}
		if isSQLDBAllowedFile(file) {
			continue
		}
		t.Fatalf("%s calls platform Store SQLDB outside composition or adapter construction", file.path)
	}
}

func TestRemovedLegacyAgentPackagesAreNotImported(t *testing.T) {
	for _, file := range productionGoFiles(t) {
		for _, imported := range file.imports {
			switch imported {
			case modulePath + "/internal/agentapp",
				modulePath + "/internal/agentapp/sqlite",
				modulePath + "/internal/agenttools",
				modulePath + "/internal/agentconfig":
				t.Fatalf("%s imports legacy agent package %s", file.path, imported)
			}
		}
	}
}

func TestSecretComparisonsGoThroughSecretPackage(t *testing.T) {
	for _, file := range productionGoFiles(t) {
		if file.pkgDir == "internal/secret" {
			continue
		}
		for _, imported := range file.imports {
			if imported == "crypto/subtle" {
				t.Fatalf("%s imports crypto/subtle directly; use internal/secret for fixed-size secret comparisons", file.path)
			}
		}
	}
}

func TestProductionContainerContractExists(t *testing.T) {
	root := repoRoot(t)
	dockerfile, err := os.ReadFile(filepath.Join(root, "Dockerfile"))
	if err != nil {
		t.Fatalf("read Dockerfile: %v", err)
	}
	text := string(dockerfile)
	for _, want := range []string{
		"FROM node:24-bookworm AS node",
		"FROM golang:1.25-bookworm AS sourcegen",
		"COPY --from=node /usr/local/bin/node /usr/local/bin/node",
		"COPY --from=node /usr/local/lib/node_modules /usr/local/lib/node_modules",
		"ln -sf ../lib/node_modules/npm/bin/npm-cli.js /usr/local/bin/npm",
		"go run github.com/Yacobolo/toolbelt/apigen/cmd/apigen@v0.4.0",
		"go run ./internal/tools/uisignalsgen",
		"FROM oven/bun:1.3.7 AS web",
		"COPY --from=sourcegen /src/web/generated ./web/generated",
		"RUN bun install --frozen-lockfile --no-cache",
		"RUN bun run build",
		"FROM golang:1.25-bookworm AS build",
		"COPY --from=sourcegen /src/internal/api/gen ./internal/api/gen",
		"CGO_ENABLED=1 go build",
		"FROM debian:bookworm-slim AS runtime",
		"USER libredash",
		"WORKDIR /app",
		"COPY --from=web /src/static ./static",
		"LIBREDASH_HOME=/var/lib/libredash",
		"LIBREDASH_PRODUCTION=1",
		"HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 CMD [\"libredash\", \"healthcheck\"]",
		"CMD [\"serve\", \"--production\"]",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("Dockerfile missing production container contract fragment %q", want)
		}
	}

	ignored, err := os.ReadFile(filepath.Join(root, ".dockerignore"))
	if err != nil {
		t.Fatalf("read .dockerignore: %v", err)
	}
	ignoreText := string(ignored)
	for _, want := range []string{".data", ".libredash", "node_modules", "api/gen", "internal/api/gen", "static/chunks"} {
		if !strings.Contains(ignoreText, want) {
			t.Fatalf(".dockerignore missing generated or runtime path %q", want)
		}
	}
}

func TestContinuousIntegrationWorkflowRunsProductionGates(t *testing.T) {
	root := repoRoot(t)
	workflow, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "ci.yml"))
	if err != nil {
		t.Fatalf("read CI workflow: %v", err)
	}
	taskfile, err := os.ReadFile(filepath.Join(root, "Taskfile.yml"))
	if err != nil {
		t.Fatalf("read Taskfile.yml: %v", err)
	}
	text := string(workflow)
	for _, want := range []string{
		"name: CI",
		"pull_request:",
		"push:",
		"actions/checkout@v7",
		"actions/setup-go@v6",
		"go-version-file: go.mod",
		"oven-sh/setup-bun@v2",
		"bun-version: 1.3.7",
		"prepare:",
		"name: Prepare generated assets",
		"go install github.com/go-task/task/v3/cmd/task@v3.50.0",
		"task generate",
		"task build",
		"actions/upload-artifact@v4",
		"name: generated-assets",
		"go-tests:",
		"name: Go tests",
		"needs: prepare",
		"go test ./...",
		"frontend-tests:",
		"name: Frontend tests",
		"bun run test:semantic-model-graph",
		"ui-route-qa:",
		"name: UI route QA",
		"task qa:ui-framework",
		"node-audit:",
		"name: JavaScript dependency audit",
		"bun audit",
		"go-vuln:",
		"name: Go vulnerability scan",
		"golang.org/x/vuln/cmd/govulncheck@v1.5.0 ./...",
		"production-image:",
		"name: Production image",
		"docker build --pull --tag libredash:ci .",
		"./scripts/smoke_production_image.sh libredash:ci",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("CI workflow missing production gate fragment %q", want)
		}
	}
	taskText := string(taskfile)
	for _, want := range []string{
		"node:audit:",
		"bun audit",
		"vuln:",
		"golang.org/x/vuln/cmd/govulncheck@v1.5.0 ./...",
	} {
		if !strings.Contains(taskText, want) {
			t.Fatalf("Taskfile missing vulnerability gate fragment %q", want)
		}
	}

	script, err := os.ReadFile(filepath.Join(root, "scripts", "smoke_production_image.sh"))
	if err != nil {
		t.Fatalf("read production image smoke script: %v", err)
	}
	scriptText := string(script)
	for _, want := range []string{
		"LIBREDASH_API_TOKEN_ONLY_AUTH=1",
		"LIBREDASH_CSRF_KEY=",
		"LIBREDASH_METRICS_BEARER_TOKEN=",
		"LIBREDASH_ALLOWED_HOSTS=",
		"/healthz",
		"/readyz",
		"/metrics",
		"Authorization: Bearer",
		".State.Health.Status",
		"--read-only",
		"--tmpfs \"/var/lib/libredash:rw,exec,nosuid,nodev,mode=0700,uid=${runtime_uid},gid=${runtime_gid},size=128m\"",
		"--tmpfs /tmp:rw,nosuid,nodev,mode=1777",
		"--entrypoint id",
		"\"$image\" -u",
		"\"$image\" -g",
		"-o /tmp/libredash-metrics-authorized.out",
		"grep -q '^# HELP libredash_http_request_duration_seconds ' /tmp/libredash-metrics-authorized.out",
	} {
		if !strings.Contains(scriptText, want) {
			t.Fatalf("production image smoke script missing fragment %q", want)
		}
	}
}

func TestStorageArchitectureSpecDocumentsGlobalDuckLakeCatalog(t *testing.T) {
	root := repoRoot(t)
	spec, err := os.ReadFile(filepath.Join(root, "docs", "storage-architecture-spec.md"))
	if err != nil {
		t.Fatalf("read storage architecture spec: %v", err)
	}
	text := string(spec)
	for _, want := range []string{
		"one global DuckLake catalog",
		"libredash.db              # LibreDash control-plane tables",
		"ducklake/catalog.sqlite   # global DuckLake analytical metadata catalog",
		"data/                     # DuckLake-managed Parquet files",
		"ATTACH 'ducklake:sqlite:.libredash/ducklake/catalog.sqlite' AS lake",
		"Use one global DuckLake catalog per LibreDash instance.",
		"Do not create per-workspace DuckLake catalogs.",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("storage architecture spec missing global catalog contract fragment %q", want)
		}
	}
	for _, forbidden := range []string{
		"LibreDash control-plane tables + DuckLake metadata tables",
		"ducklake:sqlite:.libredash/libredash.db",
		"Use one metadata catalog per LibreDash instance.",
	} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("storage architecture spec still contains obsolete shared-catalog contract fragment %q", forbidden)
		}
	}
}

func TestProductionUIDoesNotDependOnCDNScripts(t *testing.T) {
	root := repoRoot(t)
	forbiddenHosts := []string{"cdn.jsdelivr.net", "unpkg.com", "esm.sh", "skypack.dev"}

	for _, dir := range []string{"internal/ui", "internal/dashboard/ui", "internal/app"} {
		err := filepath.WalkDir(filepath.Join(root, filepath.FromSlash(dir)), func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			body, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			text := string(body)
			for _, forbidden := range forbiddenHosts {
				if strings.Contains(text, forbidden) {
					rel, _ := filepath.Rel(root, path)
					t.Fatalf("%s references external script host %q; production UI assets must be served from /static", filepath.ToSlash(rel), forbidden)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	staticFiles, err := filepath.Glob(filepath.Join(root, "static", "*.js"))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range staticFiles {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		text := string(body)
		for _, forbidden := range forbiddenHosts {
			if strings.Contains(text, forbidden) {
				rel, _ := filepath.Rel(root, path)
				t.Fatalf("%s references external asset host %q; production bundles must be self-contained", filepath.ToSlash(rel), forbidden)
			}
		}
	}
}

func isSQLDBAllowedFile(file goFile) bool {
	if file.pkgDir == "internal/app" {
		switch file.path {
		case "internal/app/server.go",
			"internal/app/publishes.go",
			"internal/app/refresh_runs.go",
			"internal/app/query_audit.go":
			return true
		default:
			return false
		}
	}
	if file.pkgDir == "internal/cli" ||
		file.pkgDir == "internal/integration" ||
		strings.HasPrefix(file.pkgDir, "internal/analytics/duckdb") ||
		strings.HasPrefix(file.pkgDir, "internal/analytics/ducklake") ||
		strings.HasSuffix(file.pkgDir, "/sqlite") ||
		strings.Contains(file.pkgDir, "/sqlite/") {
		return true
	}
	return false
}

func importListContains(imports []string, value string) bool {
	for _, imported := range imports {
		if imported == value || strings.Contains(imported, value) {
			return true
		}
	}
	return false
}

func productionGoFiles(t *testing.T) []goFile {
	t.Helper()
	root := repoRoot(t)
	files := []goFile{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", "node_modules", "static", "web", "dashboards":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		imports := make([]string, 0, len(parsed.Imports))
		for _, imported := range parsed.Imports {
			imports = append(imports, strings.Trim(imported.Path.Value, `"`))
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		files = append(files, goFile{
			path:    rel,
			pkgDir:  strings.TrimSuffix(rel, "/"+filepath.Base(rel)),
			imports: imports,
			body:    string(body),
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func packageDirExists(root, dir string) bool {
	entries, err := os.ReadDir(filepath.Join(root, filepath.FromSlash(dir)))
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		return true
	}
	return false
}

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		next := filepath.Dir(dir)
		if next == dir {
			t.Fatal("go.mod not found")
		}
		dir = next
	}
}

func isInternalPackage(pkgDir string) bool {
	return pkgDir == "internal" || strings.HasPrefix(pkgDir, "internal/")
}

func isAdapterOrCompositionPackage(pkgDir string) bool {
	if pkgDir == "internal/app" ||
		pkgDir == "internal/api" ||
		strings.HasPrefix(pkgDir, "internal/api/") ||
		pkgDir == "internal/cli" ||
		pkgDir == "internal/integration" ||
		pkgDir == "internal/platform" ||
		strings.HasPrefix(pkgDir, "internal/platform/") ||
		pkgDir == "internal/storage" ||
		strings.HasPrefix(pkgDir, "internal/storage/") ||
		pkgDir == "internal/access/oidc" ||
		pkgDir == "internal/access/httpauth" ||
		pkgDir == "internal/access/scimprov" ||
		pkgDir == "internal/admin/storage" ||
		pkgDir == "internal/agent/tools" ||
		strings.HasPrefix(pkgDir, "internal/tools/") ||
		strings.HasPrefix(pkgDir, "internal/testutil/") {
		return true
	}
	for _, suffix := range []string{"/http", "/sqlite", "/filesystem", "/duckdb", "/ducklake", "/datastar", "/openai", "/ui"} {
		if strings.HasSuffix(pkgDir, suffix) || strings.Contains(pkgDir, suffix+"/") {
			return true
		}
	}
	return false
}

func isForbiddenUseCaseImport(imported string) bool {
	if imported == "net/http" ||
		imported == "database/sql" ||
		imported == "github.com/go-chi/chi/v5" ||
		strings.Contains(imported, "datastar") ||
		strings.Contains(imported, "gomponents") {
		return true
	}
	if imported == modulePath+"/internal/platform/db" {
		return true
	}
	if !strings.HasPrefix(imported, modulePath+"/internal/") {
		return false
	}
	for _, segment := range []string{"/sqlite", "/filesystem", "/duckdb", "/ducklake", "/datastar", "/http", "/openai"} {
		if strings.Contains(strings.TrimPrefix(imported, modulePath), segment) {
			return true
		}
	}
	return false
}
