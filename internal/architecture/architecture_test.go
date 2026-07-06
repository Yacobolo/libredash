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
