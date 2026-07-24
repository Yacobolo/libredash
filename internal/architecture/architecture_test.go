package architecture

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const modulePath = "github.com/Yacobolo/leapview"

type goFile struct {
	path    string
	pkgDir  string
	imports []string
	body    string
}

var targetCapabilities = map[string]struct{}{
	"project": {}, "workspace": {}, "access": {}, "manageddata": {}, "analytics": {},
	"dashboard": {}, "agent": {}, "release": {}, "deployment": {}, "servingstate": {},
	"refresh": {}, "runtimehost": {}, "workload": {}, "platform": {},
}

func TestTargetCapabilityGraphDeclaresWorkload(t *testing.T) {
	if _, ok := targetCapabilities["workload"]; !ok {
		t.Fatal("workload is absent from the target capability graph")
	}
	if !packageDirExists(repoRoot(t), "internal/workload") {
		t.Fatal("declared workload capability package does not exist")
	}
}

func TestRefreshOwnsDurableRunState(t *testing.T) {
	if !packageDirExists(repoRoot(t), "internal/refresh/run") {
		t.Fatal("refresh run contract package does not exist")
	}
	for _, file := range productionGoFiles(t) {
		if file.pkgDir != "internal/analytics/materialize" {
			continue
		}
		for _, declaration := range []string{"type RunRecord struct", "type RunInput struct", "RunStatusQueued"} {
			if strings.Contains(file.body, declaration) {
				t.Errorf("%s retains refresh lifecycle declaration %q", file.path, declaration)
			}
		}
	}
}

func TestCapabilityModuleSurfacesExist(t *testing.T) {
	root := repoRoot(t)
	for _, capability := range []string{"access", "analytics", "workspace", "manageddata", "release", "deployment", "refresh", "dashboard", "agent", "runtimehost", "servingstate", "workload", "admin"} {
		dir := "internal/" + capability + "/module"
		if !packageDirExists(root, dir) {
			t.Errorf("capability composition package %s does not exist", dir)
		}
	}
}

func TestCapabilityModulesUseBuildAsTheirConstructor(t *testing.T) {
	for _, file := range productionGoFiles(t) {
		rule, ok := ClassifyPackage(file.pkgDir)
		if !ok || rule.Layer != LayerModule {
			continue
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), file.path, file.body, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", file.path, err)
		}
		for _, declaration := range parsed.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if ok && function.Recv == nil && function.Name.Name == "New" {
				t.Errorf("%s exports New; capability modules expose Build(ctx, Config)", file.path)
			}
		}
	}
}

func TestCapabilityModulesDoNotExposeRepositories(t *testing.T) {
	for _, file := range productionGoFiles(t) {
		rule, ok := ClassifyPackage(file.pkgDir)
		if !ok || rule.Layer != LayerModule {
			continue
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), file.path, file.body, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", file.path, err)
		}
		for _, declaration := range parsed.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Recv == nil {
				continue
			}
			if function.Name.Name == "Repository" {
				t.Errorf("%s exposes a repository from a capability module; export a named read or write port", file.path)
			}
		}
	}
}

func TestApplicationAPIGenHandlerIsDirectDelegation(t *testing.T) {
	found := false
	for _, file := range productionGoFiles(t) {
		if file.pkgDir != "internal/app" {
			continue
		}
		for _, forbidden := range []string{
			"apigenOperationPrivilege",
			"apigenOperationObjectResolver",
			"apiGenObjectScopes",
			"isGlobalAgentOperation",
		} {
			if strings.Contains(file.body, forbidden) {
				t.Errorf("%s retains APIGen authorization behavior %q; access owns authorization", file.path, forbidden)
			}
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), file.path, file.body, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", file.path, err)
		}
		for _, declaration := range parsed.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Name.Name != "HandleAPIGen" {
				continue
			}
			found = true
			if function.Body == nil || len(function.Body.List) != 1 {
				t.Errorf("%s HandleAPIGen must contain one direct delegation", file.path)
				continue
			}
			statement, ok := function.Body.List[0].(*ast.ExprStmt)
			if !ok {
				t.Errorf("%s HandleAPIGen contains non-delegation logic", file.path)
				continue
			}
			call, ok := statement.X.(*ast.CallExpr)
			if !ok {
				t.Errorf("%s HandleAPIGen is not a direct call", file.path)
				continue
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || selector.Sel.Name != "HandleAPIGen" {
				t.Errorf("%s HandleAPIGen does not delegate to an owned handler", file.path)
			}
		}
	}
	if !found {
		t.Fatal("internal/app APIGen delegation method is missing")
	}
}

func TestApplicationHasNoServerShapedDependencyContainer(t *testing.T) {
	for _, file := range productionGoFiles(t) {
		if file.pkgDir != "internal/app" {
			continue
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), file.path, file.body, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", file.path, err)
		}
		localStructs := map[string]*ast.StructType{}
		for _, declaration := range parsed.Decls {
			generic, ok := declaration.(*ast.GenDecl)
			if !ok || generic.Tok != token.TYPE {
				continue
			}
			for _, spec := range generic.Specs {
				typeSpec, ok := spec.(*ast.TypeSpec)
				if !ok {
					continue
				}
				if structure, ok := typeSpec.Type.(*ast.StructType); ok {
					localStructs[typeSpec.Name.Name] = structure
				}
			}
		}
		for name, structure := range localStructs {
			switch name {
			case "runtimeRouter", "assemblyConfig", "capabilityConstruction", "applicationAssembly", "assemblyInputs", "moduleAssemblyInputs":
				t.Errorf("%s retains transitional dependency container %s", file.path, name)
			}
			fields := expandedStructFieldCount(structure, localStructs, map[string]bool{name: true})
			if fields > 12 {
				t.Errorf("%s struct %s has %d transitive fields; split composition state into narrow route, lifecycle, health, and cleanup surfaces", file.path, name, fields)
			}
		}
	}
}

func expandedStructFieldCount(structure *ast.StructType, localStructs map[string]*ast.StructType, visiting map[string]bool) int {
	fields := 0
	for _, field := range structure.Fields.List {
		fieldCount := len(field.Names)
		if fieldCount == 0 {
			fieldCount = 1
		}
		identifier, ok := localStructIdentifier(field.Type)
		if !ok {
			fields += fieldCount
			continue
		}
		embedded, ok := localStructs[identifier.Name]
		if !ok || visiting[identifier.Name] {
			fields += fieldCount
			continue
		}
		visiting[identifier.Name] = true
		fields += fieldCount * expandedStructFieldCount(embedded, localStructs, visiting)
		delete(visiting, identifier.Name)
	}
	return fields
}

func localStructIdentifier(expression ast.Expr) (*ast.Ident, bool) {
	switch value := expression.(type) {
	case *ast.Ident:
		return value, true
	case *ast.StarExpr:
		identifier, ok := value.X.(*ast.Ident)
		return identifier, ok
	default:
		return nil, false
	}
}

func TestGeneratedQueryPackagesDoNotCombineCapabilitySQL(t *testing.T) {
	body, err := os.ReadFile(filepath.Join(repoRoot(t), "sqlc.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	blocks := strings.Split(string(body), "\n  - engine:")
	for _, forbidden := range []struct {
		generatedPackage string
		queryPath        string
	}{
		{generatedPackage: `package: "deploymentdb"`, queryPath: `"internal/servingstate/sqlite/queries`},
		{generatedPackage: `package: "servingdb"`, queryPath: `"internal/access/sqlite/queries`},
	} {
		for _, block := range blocks {
			if strings.Contains(block, forbidden.generatedPackage) && strings.Contains(block, forbidden.queryPath) {
				t.Errorf("sqlc package %s includes cross-capability query input %s", forbidden.generatedPackage, forbidden.queryPath)
			}
		}
	}
}

func TestCompositionDoesNotUseTestTransports(t *testing.T) {
	for _, file := range productionGoFiles(t) {
		if file.pkgDir != "internal/app" {
			continue
		}
		for _, imported := range file.imports {
			if imported == "net/http/httptest" {
				t.Errorf("%s uses httptest in process composition; response capture belongs to the consuming transport adapter", file.path)
			}
		}
	}
}

func TestRefreshPersistenceIsConstructedOnlyByItsModule(t *testing.T) {
	constructors := 0
	for _, file := range productionGoFiles(t) {
		for _, imported := range file.imports {
			if imported != modulePath+"/internal/refresh/sqlite" {
				continue
			}
			if file.pkgDir != "internal/refresh/module" {
				t.Errorf("%s imports refresh persistence outside refresh/module", file.path)
			}
		}
		if file.pkgDir == "internal/refresh/module" {
			constructors += strings.Count(file.body, "refreshsqlite.NewSQLRunRepository(")
			constructors += strings.Count(file.body, "refreshsqlite.NewRepository(")
		}
	}
	if constructors != 3 {
		t.Fatalf("refresh/module persistence constructors = %d, want 3 (run, schedule, recovery)", constructors)
	}
}

func TestPlatformJobModuleSurfaceExists(t *testing.T) {
	if !packageDirExists(repoRoot(t), "internal/platform/jobs/module") {
		t.Fatal("platform durable job module does not exist")
	}
}

func TestCapabilityModulesDoNotImportOtherModules(t *testing.T) {
	for _, file := range productionGoFiles(t) {
		source, ok := ClassifyPackage(file.pkgDir)
		if !ok || source.Layer != LayerModule {
			continue
		}
		for _, imported := range file.imports {
			if !strings.HasPrefix(imported, modulePath+"/internal/") || !strings.HasSuffix(imported, "/module") {
				continue
			}
			packagePath := strings.TrimPrefix(imported, modulePath+"/")
			target, ok := ClassifyPackage(packagePath)
			if ok && target.Capability != source.Capability {
				t.Errorf("%s imports capability module %s; only internal/app may assemble modules", file.path, packagePath)
			}
		}
	}
}

func TestCapabilityModulesDoNotImportOtherCapabilityAdapters(t *testing.T) {
	for _, file := range productionGoFiles(t) {
		source, ok := ClassifyPackage(file.pkgDir)
		if !ok || source.Layer != LayerModule {
			continue
		}
		for _, imported := range file.imports {
			if !strings.HasPrefix(imported, modulePath+"/internal/") {
				continue
			}
			packagePath := strings.TrimPrefix(imported, modulePath+"/")
			target, ok := ClassifyPackage(packagePath)
			if !ok || target.Layer != LayerAdapter || target.Capability == source.Capability {
				continue
			}
			if target.Capability == "platform" || target.Capability == "api" || target.Capability == "ui" {
				continue
			}
			t.Errorf("%s imports another capability's adapter %s; accept a consumer-owned port", file.path, packagePath)
		}
	}
}

func TestCapabilityModulesDoNotImportOtherCapabilityPersistenceAdapters(t *testing.T) {
	for _, file := range productionGoFiles(t) {
		source, ok := ClassifyPackage(file.pkgDir)
		if !ok || source.Layer != LayerModule {
			continue
		}
		for _, imported := range file.imports {
			if !strings.HasPrefix(imported, modulePath+"/internal/") {
				continue
			}
			packagePath := strings.TrimPrefix(imported, modulePath+"/")
			target, ok := ClassifyPackage(packagePath)
			if !ok || target.Layer != LayerAdapter || target.Capability == source.Capability || !strings.Contains(packagePath, "/sqlite") {
				continue
			}
			if target.Capability == "platform" || target.Capability == "api" || target.Capability == "ui" {
				continue
			}
			t.Errorf("%s imports another capability's adapter %s; receive a contract through Config instead", file.path, packagePath)
		}
	}
}

func TestCapabilityModulesDoNotImportOtherCapabilityTransportAdapters(t *testing.T) {
	for _, file := range productionGoFiles(t) {
		source, ok := ClassifyPackage(file.pkgDir)
		if !ok || source.Layer != LayerModule {
			continue
		}
		for _, imported := range file.imports {
			if !strings.HasPrefix(imported, modulePath+"/internal/") {
				continue
			}
			packagePath := strings.TrimPrefix(imported, modulePath+"/")
			target, ok := ClassifyPackage(packagePath)
			if !ok || target.Layer != LayerAdapter || target.Capability == source.Capability {
				continue
			}
			if target.Capability == "platform" || target.Capability == "api" || target.Capability == "ui" {
				continue
			}
			if strings.Contains(packagePath, "/http") || strings.Contains(packagePath, "/datastar") {
				t.Errorf("%s imports another capability's transport adapter %s; accept a consumer-owned port", file.path, packagePath)
			}
		}
	}
}

func TestCompositionOwnershipIsAnExplicitClosedSet(t *testing.T) {
	allowed := []string{
		"cmd",
		"internal/app",
		"internal/cli",
		"internal/tools",
		// Transitional packages removed before PR #124 is marked ready.
		"internal/dashboard/analyticsduckdb",
		"internal/refresh/analyticsduckdb",
	}
	for _, file := range productionGoFiles(t) {
		rule, ok := ClassifyPackage(file.pkgDir)
		if !ok || rule.Layer != LayerComposition {
			continue
		}
		permitted := false
		for _, prefix := range allowed {
			if file.pkgDir == prefix || strings.HasPrefix(file.pkgDir, prefix+"/") {
				permitted = true
				break
			}
		}
		if !permitted {
			t.Errorf("%s claims undeclared composition ownership", file.path)
		}
	}
}

func TestEveryProductionPackageHasAnArchitecturalOwner(t *testing.T) {
	seen := map[string]bool{}
	for _, file := range productionGoFiles(t) {
		if seen[file.pkgDir] {
			continue
		}
		seen[file.pkgDir] = true
		if _, ok := ClassifyPackage(file.pkgDir); !ok {
			t.Errorf("%s has no declared capability owner and layer", file.pkgDir)
		}
	}
}

func TestDeclaredCapabilityGraphHasNoReciprocalEdges(t *testing.T) {
	for source, dependencies := range CapabilityDependencies {
		for target := range dependencies {
			if CapabilityDependencies[target][source] {
				t.Errorf("capability graph contains reciprocal edges %s -> %s and %s -> %s", source, target, target, source)
			}
		}
	}
}

func TestProductionImportsFollowCapabilityGraph(t *testing.T) {
	for _, file := range productionGoFiles(t) {
		if file.pkgDir == "internal/testutil" || strings.HasPrefix(file.pkgDir, "internal/testutil/") {
			continue
		}
		source, ok := ClassifyPackage(file.pkgDir)
		if !ok {
			continue
		}
		for _, imported := range file.imports {
			if !strings.HasPrefix(imported, modulePath+"/") {
				continue
			}
			packagePath := strings.TrimPrefix(imported, modulePath+"/")
			target, ok := ClassifyPackage(packagePath)
			if !ok || source.Capability == target.Capability {
				continue
			}
			_, sourceIsProductCapability := targetCapabilities[source.Capability]
			if !sourceIsProductCapability || source.Layer == LayerComposition || source.Layer == LayerModule || target.Capability == "platform" || target.Capability == "api" || target.Capability == "ui" {
				continue
			}
			if target.Capability == "workload" && AllowsWorkloadImport(file.pkgDir) {
				// The narrower workload allowlist test below validates exactly which
				// execution and worker adapters may use admission.
				continue
			}
			if IsDeferredPackageEdge(file.pkgDir, target.Capability) {
				continue
			}
			if !CapabilityDependencies[source.Capability][target.Capability] {
				t.Errorf("%s imports %s: undeclared capability edge %s -> %s", file.path, packagePath, source.Capability, target.Capability)
				continue
			}
			if target.Layer == LayerAdapter {
				t.Errorf("%s imports adapter package %s owned by capability %s", file.path, packagePath, target.Capability)
				continue
			}
			if source.Layer != LayerModule && !IsPublicContractImport(target.Capability, packagePath) {
				t.Errorf("%s imports non-contract package %s from capability %s", file.path, packagePath, target.Capability)
			}
		}
	}
}

func TestApplicationImportsProductCapabilitiesOnlyThroughModules(t *testing.T) {
	for _, file := range productionGoFiles(t) {
		if file.pkgDir != "internal/app" {
			continue
		}
		for _, imported := range file.imports {
			if !strings.HasPrefix(imported, modulePath+"/internal/") {
				continue
			}
			packagePath := strings.TrimPrefix(imported, modulePath+"/")
			target, ok := ClassifyPackage(packagePath)
			if !ok || target.Capability == "platform" || target.Capability == "composition" || target.Capability == "api" || target.Capability == "ui" {
				continue
			}
			if target.Layer != LayerModule {
				t.Errorf("%s imports product package %s instead of its module surface", file.path, packagePath)
			}
		}
	}
}

func TestApplicationDoesNotReclaimAccessOrAnalyticsConstruction(t *testing.T) {
	for _, file := range productionGoFiles(t) {
		if file.pkgDir != "internal/app" {
			continue
		}
		for _, forbidden := range []string{"analyticsducklake.Open(", "accesssqlite.NewRepository("} {
			if strings.Contains(file.body, forbidden) {
				t.Errorf("%s constructs a migrated capability adapter via %s", file.path, forbidden)
			}
		}
		if strings.HasSuffix(file.path, "/auth.go") {
			t.Errorf("%s owns authentication behavior; move it to access/module", file.path)
		}
	}
}

func TestAppDoesNotRetainPlatformStore(t *testing.T) {
	root := repoRoot(t)
	for _, file := range productionGoFiles(t) {
		if file.pkgDir != "internal/app" {
			continue
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), filepath.Join(root, filepath.FromSlash(file.path)), nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(parsed, func(node ast.Node) bool {
			selector, ok := node.(*ast.SelectorExpr)
			if !ok || selector.Sel.Name != "Store" {
				return true
			}
			if ident, ok := selector.X.(*ast.Ident); ok && ident.Name == "platform" {
				t.Errorf("%s retains platform.Store; keep the store local to application assembly", file.path)
			}
			return true
		})
	}
}

func TestOnlyCompositionImportsApplicationPackage(t *testing.T) {
	for _, file := range productionGoFiles(t) {
		for _, imported := range file.imports {
			if imported != modulePath+"/internal/app" {
				continue
			}
			rule, ok := ClassifyPackage(file.pkgDir)
			if !ok || rule.Layer != LayerComposition {
				t.Errorf("%s imports internal/app outside process composition", file.path)
			}
		}
	}
}

func TestLegacyApplicationContainerAPIIsAbsent(t *testing.T) {
	root := repoRoot(t)
	for _, file := range productionGoFiles(t) {
		if file.pkgDir != "internal/app" {
			continue
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), filepath.Join(root, filepath.FromSlash(file.path)), nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, declaration := range parsed.Decls {
			switch value := declaration.(type) {
			case *ast.GenDecl:
				for _, specification := range value.Specs {
					if named, ok := specification.(*ast.TypeSpec); ok {
						switch named.Name.Name {
						case "Server", "server", "Options", "serverOptions", "Host", "host":
							t.Errorf("%s declares legacy application container type %s", file.path, named.Name.Name)
						}
					}
				}
			case *ast.FuncDecl:
				if value.Recv == nil {
					switch value.Name.Name {
					case "New", "NewWithOptions", "newServer", "newServerWithOptions", "buildServer":
						t.Errorf("%s declares legacy application constructor %s", file.path, value.Name.Name)
					}
				}
			}
		}
	}
}

func TestRequestRuntimeDoesNotRetainConstructionDependencies(t *testing.T) {
	root := repoRoot(t)
	for _, file := range productionGoFiles(t) {
		if file.pkgDir != "internal/app" {
			continue
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), filepath.Join(root, filepath.FromSlash(file.path)), nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, declaration := range parsed.Decls {
			generic, ok := declaration.(*ast.GenDecl)
			if !ok {
				continue
			}
			for _, specification := range generic.Specs {
				named, ok := specification.(*ast.TypeSpec)
				if !ok || named.Name.Name != "runtimeRouter" {
					continue
				}
				structure, ok := named.Type.(*ast.StructType)
				if !ok {
					t.Fatalf("%s runtimeRouter must be a struct", file.path)
				}
				for _, field := range structure.Fields.List {
					for _, name := range field.Names {
						switch name.Name {
						case "adminDatabase", "servingStateRepo", "managedDataResolver",
							"workspaceRepo", "workspacePersistence", "workspaceAssetCatalog",
							"accessRepo", "reloader", "duckLakeCatalogPath", "duckLakeDataPath",
							"jobLeaseTimeout", "deploymentConfig":
							t.Errorf("%s runtimeRouter retains construction dependency %s", file.path, name.Name)
						}
					}
				}
			}
		}
	}
}

func TestAppDoesNotConstructRepositoriesFromSQLDB(t *testing.T) {
	for _, file := range productionGoFiles(t) {
		if file.pkgDir == "internal/app" && file.path != "internal/app/composition.go" && strings.Contains(file.body, ".SQLDB()") {
			t.Errorf("%s constructs adapters from platform.Store; capability modules must receive construction ownership", file.path)
		}
	}
}

func TestWorkloadImportsNoProductCapabilities(t *testing.T) {
	for _, file := range productionGoFiles(t) {
		if file.pkgDir != "internal/workload" {
			continue
		}
		for _, imported := range file.imports {
			if strings.HasPrefix(imported, modulePath+"/internal/") {
				t.Fatalf("%s imports product capability %s", file.path, imported)
			}
		}
	}
}

func TestOnlyWorkloadAdaptersAndCompositionDependOnWorkload(t *testing.T) {
	for _, file := range productionGoFiles(t) {
		for _, imported := range file.imports {
			if imported != modulePath+"/internal/workload" {
				continue
			}
			if !AllowsWorkloadImport(file.pkgDir) {
				t.Fatalf("%s depends on workload outside composition or an execution/worker adapter", file.path)
			}
		}
	}
}

func TestArrowImportsStayInsideAnalyticalDataPlaneAndExplicitEncoders(t *testing.T) {
	allowed := []string{
		"internal/analytics/arrowquery",
		"internal/analytics/arrowresult",
		"internal/analytics/resultcache",
		"internal/analytics/materialize",
		"internal/analytics/ducklake",
		"internal/dashboard/semanticapi",
		"internal/dashboard/http",
	}
	for _, file := range productionGoFiles(t) {
		for _, imported := range file.imports {
			if !strings.HasPrefix(imported, "github.com/apache/arrow-go/") {
				continue
			}
			permitted := false
			for _, prefix := range allowed {
				if file.pkgDir == prefix || strings.HasPrefix(file.pkgDir, prefix+"/") {
					permitted = true
					break
				}
			}
			if !permitted {
				t.Fatalf("%s imports Arrow outside the analytical data plane or an explicit Arrow encoder", file.path)
			}
		}
	}
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

func TestStaticSQLiteAdaptersUseGeneratedQueries(t *testing.T) {
	generatedOnly := map[string]bool{
		"internal/agent/sqlite":        true,
		"internal/deployment/sqlite":   true,
		"internal/manageddata/sqlite":  true,
		"internal/servingstate/sqlite": true,
		"internal/workspace/sqlite":    true,
	}
	generatedOnlyFiles := map[string]bool{
		"internal/access/sqlite/api_symmetry.go":             true,
		"internal/access/sqlite/authorization.go":            true,
		"internal/refresh/sqlite/runs.go":                    true,
		"internal/analytics/queryaudit/sqlite/repository.go": true,
	}
	for _, file := range productionGoFiles(t) {
		if !generatedOnly[file.pkgDir] && !generatedOnlyFiles[file.path] {
			continue
		}
		for _, directCall := range []string{".QueryContext(", ".QueryRowContext(", ".ExecContext("} {
			if strings.Contains(file.body, directCall) {
				t.Fatalf("%s bypasses sqlc via %s", file.path, directCall)
			}
		}
	}
}

func TestCapabilitySQLiteAdaptersDoNotImportOtherSQLiteAdapters(t *testing.T) {
	for _, file := range productionGoFiles(t) {
		if !strings.Contains(file.pkgDir, "/sqlite") {
			continue
		}
		for _, imported := range file.imports {
			if !strings.HasPrefix(imported, modulePath+"/internal/") || !strings.Contains(imported, "/sqlite") {
				continue
			}
			packagePath := strings.TrimPrefix(imported, modulePath+"/")
			source, sourceOK := ClassifyPackage(file.pkgDir)
			target, targetOK := ClassifyPackage(packagePath)
			if sourceOK && targetOK && source.Capability == target.Capability {
				continue
			}
			t.Errorf("%s imports persistence implementation %s; use a consumer-owned port or module bridge", file.path, imported)
		}
	}
}

func TestGeneratedPlatformQueriesStayInsidePlatform(t *testing.T) {
	const sharedQueries = modulePath + "/internal/platform/db"
	for _, file := range productionGoFiles(t) {
		if file.pkgDir == "internal/platform" || strings.HasPrefix(file.pkgDir, "internal/platform/db") {
			continue
		}
		for _, imported := range file.imports {
			if imported == sharedQueries {
				t.Errorf("%s imports the shared generated query package; generate capability-private queries instead", file.path)
			}
		}
	}
}

func TestFixedOperationalRetentionQueriesUseSQLC(t *testing.T) {
	for _, file := range productionGoFiles(t) {
		if file.path != "internal/admin/sqlite/retention.go" {
			continue
		}
		if strings.Contains(file.body, "DELETE FROM api_async_events") {
			t.Fatalf("%s embeds the fixed async-event retention query instead of using sqlc", file.path)
		}
	}
}

func TestSQLCQueriesAreSplitByDomain(t *testing.T) {
	root := repoRoot(t)
	for _, domain := range []string{
		"internal/admin/sqlite/queries/retention.sql",
		"internal/access/sqlite/queries/access.sql",
		"internal/agent/sqlite/queries/agent.sql",
		"internal/apiidempotency/sqlite/queries/idempotency.sql",
		"internal/cursorsigning/sqlite/queries/cursor_signing.sql",
		"internal/deployment/sqlite/queries/deployment.sql",
		"internal/manageddata/sqlite/queries/managed_data.sql",
		"internal/refresh/sqlite/runqueries/materialization.sql",
		"internal/platform/jobs/sqlite/queries/async_job.sql",
		"internal/platform/db/queries/platform.sql",
		"internal/analytics/queryaudit/sqlite/queries/query_history.sql",
		"internal/refresh/sqlite/schedulequeries/refresh_pipeline.sql",
		"internal/release/sqlite/queries/release.sql",
		"internal/servingstate/sqlite/queries/serving_state.sql",
		"internal/workspace/sqlite/queries/workspace.sql",
	} {
		contents, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(domain)))
		if err != nil {
			t.Fatalf("read sqlc query domain %s: %v", domain, err)
		}
		if !strings.Contains(string(contents), "-- name:") {
			t.Fatalf("sqlc query domain %s contains no named queries", domain)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "internal", "platform", "db", "queries.sql")); !os.IsNotExist(err) {
		t.Fatal("legacy sqlc query monolith must not exist")
	}
}

func TestSQLCUsesRuntimeMigrationsAsItsSchemaSource(t *testing.T) {
	root := repoRoot(t)
	config, err := os.ReadFile(filepath.Join(root, "sqlc.yaml"))
	if err != nil {
		t.Fatalf("read sqlc config: %v", err)
	}
	if !strings.Contains(string(config), `schema: "internal/platform/migrations"`) {
		t.Fatal("sqlc must compile against the runtime Goose migrations")
	}
	if _, err := os.Stat(filepath.Join(root, "internal", "platform", "db", "schema.sql")); !os.IsNotExist(err) {
		t.Fatal("duplicate sqlc schema snapshot must not exist")
	}
}

func TestRequiredCapabilityAdaptersExist(t *testing.T) {
	root := repoRoot(t)
	for _, dir := range []string{
		"internal/access/http",
		"internal/admin/http",
		"internal/agent/http",
		"internal/analytics/connectors",
		"internal/refresh/http",
		"internal/dashboard/semanticapi",
		"internal/dashboard/http",
		"internal/workspace/datastar",
		"internal/workspace/http",
	} {
		if !packageDirExists(root, dir) {
			t.Fatalf("required capability adapter package %s does not exist", dir)
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
		"FROM node:24-bookworm@sha256:",
		"FROM golang:1.25-bookworm@sha256:",
		"COPY --from=node /usr/local/bin/node /usr/local/bin/node",
		"COPY --from=node /usr/local/lib/node_modules /usr/local/lib/node_modules",
		"ln -sf ../lib/node_modules/npm/bin/npm-cli.js /usr/local/bin/npm",
		"./scripts/generate_build_sources.sh",
		"go run ./internal/tools/mapassets --out .data/map-assets",
		"FROM oven/bun:1.3.7@sha256:",
		"COPY --from=sourcegen /src/api/gen ./api/gen",
		"COPY --from=sourcegen /src/api/visualization ./api/visualization",
		"COPY --from=sourcegen /src/web/generated ./web/generated",
		"RUN bun install --frozen-lockfile --no-cache",
		"bun scripts/generate_visualization_validator.ts",
		"bun scripts/generate_vega_lite_validator.ts",
		"bun run build",
		"FROM golang:1.25-bookworm@sha256:",
		"COPY --from=sourcegen /src/internal/api/gen ./internal/api/gen",
		"COPY --from=sourcegen /src/internal/ui/signals/models.gen.go ./internal/ui/signals/models.gen.go",
		"CGO_ENABLED=1 go build",
		"FROM debian:bookworm-slim@sha256:",
		"USER leapview",
		"WORKDIR /app",
		"COPY --from=web /src/static ./static",
		"COPY --from=sourcegen /src/.data/map-assets ./.data/map-assets",
		"LEAPVIEW_HOME=/var/lib/leapview/home",
		"LEAPVIEW_MANAGED_DATA_DIR=/var/lib/leapview/home/managed-data",
		"LEAPVIEW_PRODUCTION=1",
		"HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 CMD [\"leapview\", \"healthcheck\"]",
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
	for _, want := range []string{".data", ".leapview", "node_modules", "api/gen", "internal/api/gen", "static/chunks"} {
		if !strings.Contains(ignoreText, want) {
			t.Fatalf(".dockerignore missing generated or runtime path %q", want)
		}
	}
}

func TestGeographicRendererDecisionIsExplicitAndNavigable(t *testing.T) {
	root := repoRoot(t)
	decision, err := os.ReadFile(filepath.Join(root, "docs", "articles", "architecture", "geographic-rendering.md"))
	if err != nil {
		t.Fatalf("read geographic rendering decision: %v", err)
	}
	text := string(decision)
	for _, want := range []string{
		"# Geographic rendering decision",
		"Status: accepted",
		"MapLibre is the sole geographic renderer",
		"ECharts `geo`",
		"one geographic camera",
		"same-origin",
		"spatial-windowed",
		"| Capability | MapLibre | ECharts `geo` |",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("geographic rendering decision missing %q", want)
		}
	}
	navigation, err := os.ReadFile(filepath.Join(root, "docs", "navigation.yaml"))
	if err != nil {
		t.Fatalf("read docs navigation: %v", err)
	}
	if !strings.Contains(string(navigation), "source: articles/architecture/geographic-rendering.md") {
		t.Fatal("geographic rendering decision is not registered in documentation navigation")
	}
}

func TestPublicSiteProductionContainerContractExists(t *testing.T) {
	root := repoRoot(t)
	dockerfile, err := os.ReadFile(filepath.Join(root, "Dockerfile.site"))
	if err != nil {
		t.Fatalf("read Dockerfile.site: %v", err)
	}
	text := string(dockerfile)
	for _, want := range []string{
		"FROM node:24-bookworm@sha256:",
		"FROM golang:1.25-bookworm@sha256:",
		"./scripts/generate_build_sources.sh",
		"FROM oven/bun:1.3.7@sha256:",
		"COPY --from=sourcegen /src/api/gen ./api/gen",
		"COPY --from=sourcegen /src/api/visualization ./api/visualization",
		"COPY --from=sourcegen /src/web/generated ./web/generated",
		"RUN bun install --frozen-lockfile --no-cache",
		"bun scripts/generate_visualization_validator.ts",
		"bun scripts/generate_vega_lite_validator.ts",
		"bun run build:site",
		"FROM golang:1.25-bookworm@sha256:",
		"CGO_ENABLED=0 go build -trimpath",
		"./cmd/leapview-site",
		"FROM gcr.io/distroless/static-debian12:nonroot@sha256:",
		"USER nonroot:nonroot",
		"ENV LEAPVIEW_SITE_BASE_URL=",
		"ENTRYPOINT [\"/leapview-site\"]",
		"CMD [\"-addr=:8081\"]",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("Dockerfile.site missing production container contract fragment %q", want)
		}
	}
	if strings.Contains(text, "apigen@v0.4.0") || strings.Contains(text, "apigenpostprocess") {
		t.Error("Dockerfile.site still uses the retired APIGen v0.4 generation pipeline")
	}
}

func TestBuildSourceGenerationContract(t *testing.T) {
	root := repoRoot(t)
	path := filepath.Join(root, "scripts", "generate_build_sources.sh")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat shared build source generator: %v", err)
	}
	if info.Mode()&0o111 == 0 {
		t.Fatal("shared build source generator is not executable")
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read shared build source generator: %v", err)
	}
	text := string(body)
	commands := []string{
		"go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.30.0 generate",
		"go run ./internal/tools/configgen",
		"typespec-compile -manifest api/apigen.yaml -target leapview-v1",
		"typespec-compile -manifest api/apigen.yaml -target ui-signals",
		"typespec-compile -manifest api/apigen.yaml -target visualization-ir",
		"all -manifest api/apigen.yaml -target visualization-ir",
		"schema export --format json-schema --out schemas/json",
	}
	previous := -1
	for _, command := range commands {
		current := strings.Index(text, command)
		if current < 0 {
			t.Fatalf("shared build source generator missing command %q", command)
		}
		if current <= previous {
			t.Fatalf("shared build source generator command %q is out of order", command)
		}
		previous = current
	}
}

func TestCoreProceduralGuidesUseTheOperationalTemplate(t *testing.T) {
	root := repoRoot(t)
	guides := []string{
		"docs/articles/start/installation.md",
		"docs/articles/start/first-dashboard.md",
		"docs/articles/build/connect-data.md",
		"docs/articles/build/model-tables.md",
		"docs/articles/build/semantic-model.md",
		"docs/articles/build/dashboard.md",
		"docs/guides/cli/validate-deploy.md",
		"docs/articles/operate/self-hosting.md",
		"docs/articles/security/oidc.md",
		"docs/articles/integrate/api-quickstart.md",
	}
	for _, guide := range guides {
		body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(guide)))
		if err != nil {
			t.Errorf("read %s: %v", guide, err)
			continue
		}
		text := string(body)
		for _, section := range []string{"\n## Before you begin\n", "\n## Validate", "\n## Verify", "\n## Troubleshooting\n", "\n## Next steps\n"} {
			if !strings.Contains(text, section) {
				t.Errorf("%s missing procedural section %q", guide, strings.TrimSpace(section))
			}
		}
		if !strings.Contains(text, "\n1. ") {
			t.Errorf("%s does not contain a numbered procedure", guide)
		}
	}
}

func TestDevelopmentServerTracksCompiledFallbackProcess(t *testing.T) {
	root := repoRoot(t)
	server, err := os.ReadFile(filepath.Join(root, "scripts", "dev-server.sh"))
	if err != nil {
		t.Fatalf("read development server script: %v", err)
	}
	serverText := string(server)
	for _, want := range []string{
		`go build -tags=duckdb_arrow -o "$TMP_DIR/leapview-dev" ./cmd/leapview`,
		`"$TMP_DIR/leapview-dev" >> "$LOG_FILE" 2>&1 &`,
		`LEAPVIEW_MANAGED_DATA_MIN_FREE_BYTES="${LEAPVIEW_MANAGED_DATA_MIN_FREE_BYTES:-67108864}"`,
	} {
		if !strings.Contains(serverText, want) {
			t.Fatalf("development server script missing tracked binary fragment %q", want)
		}
	}
	if strings.Contains(serverText, `go run ./cmd/leapview >> "$LOG_FILE" 2>&1 &`) {
		t.Fatal("development server must not track the go run wrapper as the server process")
	}

	qa, err := os.ReadFile(filepath.Join(root, "scripts", "qa_ui_framework.ts"))
	if err != nil {
		t.Fatalf("read UI framework QA script: %v", err)
	}
	qaText := string(qa)
	if !strings.Contains(qaText, "const managedServerReadyAttempts = 1800") ||
		!strings.Contains(qaText, "attempt < managedServerReadyAttempts") {
		t.Fatal("UI framework QA must allow a cold Go build before checking server readiness")
	}
	for _, want := range []string{
		"LEAPVIEW_MANAGED_DATA_DIR: `${qaHome}/managed-data`",
		"['chmod', '-R', 'u+w', qaHome]",
	} {
		if !strings.Contains(qaText, want) {
			t.Fatalf("UI framework QA must isolate and clean managed-data state: missing %q", want)
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
		"actions/checkout@",
		"actions/setup-go@",
		"go-version-file: go.mod",
		"oven-sh/setup-bun@",
		"bun-version: 1.3.7",
		"prepare:",
		"name: Prepare generated assets",
		"go install github.com/go-task/task/v3/cmd/task@v3.50.0",
		"task config:check",
		"task generate",
		"task build",
		"actions/upload-artifact@",
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
		"docker build --pull --tag leapview:ci .",
		"./scripts/smoke_production_image.sh leapview:ci",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("CI workflow missing production gate fragment %q", want)
		}
	}
	taskText := string(taskfile)
	for _, want := range []string{
		"config:generate:",
		"go run ./internal/tools/configgen",
		"config:check:",
		"go run ./internal/tools/configgen --check",
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
		"LEAPVIEW_API_TOKEN_ONLY_AUTH=1",
		"LEAPVIEW_CSRF_KEY=",
		"LEAPVIEW_METRICS_BEARER_TOKEN=",
		"LEAPVIEW_ALLOWED_HOSTS=",
		"LEAPVIEW_PUBLIC_URL=",
		"/healthz",
		"/readyz",
		"/metrics",
		"Authorization: Bearer",
		".State.Health.Status",
		"--read-only",
		"--tmpfs \"/var/lib/leapview:rw,exec,nosuid,nodev,mode=0700,uid=${runtime_uid},gid=${runtime_gid},size=128m\"",
		"--tmpfs /tmp:rw,nosuid,nodev,mode=1777",
		"--entrypoint id",
		"\"$image\" -u",
		"\"$image\" -g",
		"-o /tmp/leapview-metrics-authorized.out",
		"grep -q '^# HELP leapview_http_request_duration_seconds ' /tmp/leapview-metrics-authorized.out",
	} {
		if !strings.Contains(scriptText, want) {
			t.Fatalf("production image smoke script missing fragment %q", want)
		}
	}
}

func TestSQLCOutputsAreGeneratedBuildInputs(t *testing.T) {
	root := repoRoot(t)
	files := map[string][]string{
		"Taskfile.yml": {
			"db:generate:",
			"go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.30.0 generate",
			"- task: db:generate",
		},
		".gitignore": {
			"internal/platform/db/db.go",
			"internal/platform/db/models.go",
			"internal/platform/db/*.sql.go",
		},
		".dockerignore": {
			"internal/platform/db/db.go",
			"internal/platform/db/models.go",
			"internal/platform/db/*.sql.go",
		},
		filepath.Join(".github", "workflows", "ci.yml"): {
			"Check generated database code is untracked",
			"git ls-files -- internal/platform/db/db.go internal/platform/db/models.go 'internal/platform/db/*.sql.go'",
			"internal/platform/db/db.go",
			"internal/platform/db/models.go",
			"internal/platform/db/*.sql.go",
		},
		filepath.Join("scripts", "generate_build_sources.sh"): {
			"go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.30.0 generate",
		},
		"Dockerfile": {
			"./scripts/generate_build_sources.sh",
			"COPY --from=sourcegen /src/internal/platform/db/db.go ./internal/platform/db/db.go",
			"COPY --from=sourcegen /src/internal/platform/db/models.go ./internal/platform/db/models.go",
			"COPY --from=sourcegen /src/internal/platform/db/*.sql.go ./internal/platform/db/",
		},
	}
	for name, fragments := range files {
		body, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		for _, fragment := range fragments {
			if !strings.Contains(string(body), fragment) {
				t.Errorf("%s missing sqlc generation contract fragment %q", name, fragment)
			}
		}
	}
}

func TestDerivedArtifactsAreGeneratedBuildInputs(t *testing.T) {
	root := repoRoot(t)
	files := map[string][]string{
		".gitignore": {
			"internal/config/config_gen.go",
			"internal/configspec/names_gen.go",
			"web/generated/",
			"docs/catalog.json",
			"docs/search-index.sqlite3",
			"docs/configuration.md",
			"docs/api/*.md",
			"docs/api/operations.json",
			"docs/reference/cli/",
			"docs/reference/config/",
		},
		".dockerignore": {
			"internal/config/config_gen.go",
			"internal/configspec/names_gen.go",
			"web/generated",
			"docs/catalog.json",
			"docs/search-index.sqlite3",
			"docs/configuration.md",
			"docs/api/*.md",
			"docs/api/operations.json",
			"docs/reference/cli",
			"docs/reference/config",
		},
		filepath.Join(".github", "workflows", "ci.yml"): {
			"Check generated build inputs are untracked",
			"docs/catalog.json docs/search-index.sqlite3 docs/configuration.md",
			"'docs/api/*.md' docs/api/operations.json docs/reference/cli docs/reference/config",
			"internal/config/config_gen.go internal/configspec/names_gen.go web/generated",
			"Check public contract snapshots",
			".env.example docs/api/openapi.yaml schemas/config schemas/json",
			"Check generation is deterministic",
		},
		"Dockerfile.site": {
			"AS sourcegen",
			"./scripts/generate_build_sources.sh",
			"go run ./internal/tools/clidocgen",
			"go run ./internal/tools/schemadocgen",
			"go run ./internal/tools/openapidocgen",
			"go run ./internal/tools/docsitegen",
			"FROM sourcegen AS build",
			"COPY --from=sourcegen /src/web/generated ./web/generated",
		},
		"Dockerfile": {
			"COPY --from=sourcegen /src/internal/config/config_gen.go ./internal/config/config_gen.go",
			"COPY --from=sourcegen /src/internal/configspec/names_gen.go ./internal/configspec/names_gen.go",
		},
		filepath.Join("scripts", "generate_build_sources.sh"): {
			"go run ./internal/tools/configgen",
		},
		"Taskfile.yml": {
			"desc: Build the LeapView public site assets from generated contracts",
			"desc: Build the independently deployable public site from generated documentation",
			"desc: Start the public site from generated documentation on http://localhost:8081",
		},
	}
	for name, fragments := range files {
		body, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		for _, fragment := range fragments {
			if !strings.Contains(string(body), fragment) {
				t.Errorf("%s missing generated-input contract fragment %q", name, fragment)
			}
		}
	}

	workflow, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "ci.yml"))
	if err != nil {
		t.Fatal(err)
	}
	uploadStep := workflowStep(string(workflow), "      - name: Upload generated assets", "\n  go-tests:")
	for _, artifact := range []string{"docs/api/operations.json", "docs/reference/cli/"} {
		if !strings.Contains(uploadStep, artifact) {
			t.Errorf("generated asset upload is missing build-only machine documentation %q", artifact)
		}
	}

	gitignore, err := os.ReadFile(filepath.Join(root, ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(gitignore), "!docs/reference/cli/manifest.json") {
		t.Error("generated CLI manifest must not be exempted from Git ignore rules")
	}

	taskfile, err := os.ReadFile(filepath.Join(root, "Taskfile.yml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, generated := range []string{"docs/api/operations.json", "docs/reference/cli/manifest.json"} {
		if strings.Contains(generatedCheckCommand(string(taskfile)), generated) {
			t.Errorf("generated:check treats build-only artifact %q as a public snapshot", generated)
		}
	}
}

func TestArrowResponseContractDeclaresCursorTrailer(t *testing.T) {
	root := repoRoot(t)
	body, err := os.ReadFile(filepath.Join(root, "api", "typespec", "common.tsp"))
	if err != nil {
		t.Fatal(err)
	}
	contract := string(body)
	for _, fragment := range []string{
		`@extension("x-leapview-response-trailers", #["X-Next-Cursor"])`,
		`@header("Trailer") trailers: "X-Next-Cursor";`,
	} {
		if !strings.Contains(contract, fragment) {
			t.Errorf("Arrow response contract missing trailer declaration %q", fragment)
		}
	}
	if strings.Contains(contract, `@header("X-Next-Cursor")`) {
		t.Error("Arrow response contract still advertises X-Next-Cursor as an initial header")
	}
	operations, err := os.ReadFile(filepath.Join(root, "api", "typespec", "bi.tsp"))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(string(operations), `@extension("x-leapview-response-trailers", #["X-Next-Cursor"])`); got != 3 {
		t.Errorf("Arrow operation trailer declarations = %d, want 3", got)
	}
	openAPI, err := os.ReadFile(filepath.Join(root, "docs", "api", "openapi.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(string(openAPI), "x-leapview-response-trailers:"); got != 3 {
		t.Errorf("generated OpenAPI trailer declarations = %d, want 3", got)
	}
}

func workflowStep(workflow, startMarker, endMarker string) string {
	start := strings.Index(workflow, startMarker)
	if start < 0 {
		return ""
	}
	rest := workflow[start+len(startMarker):]
	end := strings.Index(rest, endMarker)
	if end < 0 {
		return rest
	}
	return rest[:end]
}

func generatedCheckCommand(taskfile string) string {
	start := strings.Index(taskfile, "  generated:check:")
	if start < 0 {
		return ""
	}
	rest := taskfile[start+len("  generated:check:"):]
	end := strings.Index(rest, "\n  api:generate:")
	if end < 0 {
		return rest
	}
	return rest[:end]
}

func TestFixedPlatformSQLiteQueriesUseSQLC(t *testing.T) {
	root := repoRoot(t)
	queryContracts := map[string][]string{
		filepath.Join("internal", "access", "sqlite", "queries", "access.sql"): {
			"-- name: DeleteRoleGrantTemplates :exec",
			"-- name: InsertRoleGrantTemplate :exec",
		},
		filepath.Join("internal", "platform", "db", "queries", "platform.sql"): {
			"-- name: InsertPlatformSettingIfMissing :exec",
		},
		filepath.Join("internal", "manageddata", "sqlite", "queries", "managed_data.sql"): {
			"-- name: ListManagedDataReachabilitySources :many",
		},
	}
	for name, markers := range queryContracts {
		body, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		for _, marker := range markers {
			if !strings.Contains(string(body), marker) {
				t.Errorf("%s missing sqlc query %q", name, marker)
			}
		}
	}

	handwrittenSQL := map[string][]string{
		filepath.Join("internal", "platform", "store.go"): {
			"DELETE FROM role_grant_templates",
			"INSERT INTO role_grant_templates",
			"INSERT INTO securable_objects",
			"INSERT INTO platform_settings",
		},
		filepath.Join("internal", "manageddata", "maintenance", "sqlite", "source.go"): {
			"const reachabilityQuery",
			"QueryContext(ctx, reachabilityQuery)",
		},
	}
	for name, fragments := range handwrittenSQL {
		body, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		for _, fragment := range fragments {
			if strings.Contains(string(body), fragment) {
				t.Errorf("%s retains fixed-shape SQLite query %q instead of using sqlc", name, fragment)
			}
		}
	}
}

func TestAPIv1SQLiteAdaptersUseSQLC(t *testing.T) {
	packages := map[string]struct{}{
		"internal/apiidempotency/sqlite": {},
		"internal/jobs/sqlite":           {},
		"internal/cursorsigning/sqlite":  {},
		"internal/release/sqlite":        {},
	}
	for _, file := range productionGoFiles(t) {
		if _, ok := packages[file.pkgDir]; !ok {
			continue
		}
		for _, forbidden := range []string{".ExecContext(", ".QueryContext(", ".QueryRowContext("} {
			if strings.Contains(file.body, forbidden) {
				t.Errorf("%s bypasses sqlc via %s", file.path, forbidden)
			}
		}
	}
}

func TestStorageArchitectureSpecDocumentsProcessOwnedDuckDB(t *testing.T) {
	root := repoRoot(t)
	spec, err := os.ReadFile(filepath.Join(root, "docs", "storage-architecture-spec.md"))
	if err != nil {
		t.Fatalf("read storage architecture spec: %v", err)
	}
	text := string(spec)
	for _, want := range []string{
		"one process-owned DuckDB `DatabaseInstance`",
		"leapview.db               # node-local control-plane state",
		"ducklake/catalog.duckdb   # DuckDB-backed DuckLake metadata catalog",
		"Every physical relation in a serving plan",
		"AT (VERSION => 42)",
		"Runtime retirement closes generation-scoped cache state",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("storage architecture spec missing global catalog contract fragment %q", want)
		}
	}
	for _, forbidden := range []string{
		"ducklake:sqlite:",
		"PostgreSQL as the server/multi-user DuckLake catalog backend",
		"one DuckDB file per semantic model",
	} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("storage architecture spec still contains obsolete shared-catalog contract fragment %q", forbidden)
		}
	}
}

func TestAnalyticsModuleConstructsTheProcessDuckDBExactlyOnce(t *testing.T) {
	constructors := []string{}
	for _, file := range productionGoFiles(t) {
		if file.pkgDir == "internal/analytics/module" && strings.Contains(file.body, "analyticsducklake.Open(") {
			constructors = append(constructors, file.path)
		}
	}
	if len(constructors) != 1 {
		t.Fatalf("analytics module constructs DuckDB in %v, want exactly one constructor", constructors)
	}
	root := repoRoot(t)
	for _, path := range []string{
		"internal/runtimehost/module/factory.go",
		"internal/analytics/duckdb/materialize.go",
		"internal/refresh/analyticsduckdb/materializer.go",
		"internal/dashboard/analyticsduckdb/factory.go",
		"internal/runtimehost/manager.go",
	} {
		body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(body), "analyticsducklake.Open(") || strings.Contains(string(body), "OpenSnapshot(") {
			t.Errorf("%s constructs a runtime-owned DuckDB instance", path)
		}
	}
}

func TestGovernedAnalyticalSessionBoundaryHasNoLegacyServingEscape(t *testing.T) {
	root := repoRoot(t)
	for _, path := range []string{
		"internal/analytics/ducklake/environment.go",
		"internal/dashboard/analyticsduckdb/factory.go",
		"internal/dashboard/runtime/service.go",
		"internal/dataquery/query.go",
	} {
		body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
		if err != nil {
			t.Fatal(err)
		}
		text := string(body)
		for _, forbidden := range []string{"func (e *Environment) SQLDB(", "OpenMaterializeRuntime", "OpenDashboardDataRuntime", "KindSourceRows"} {
			if strings.Contains(text, forbidden) {
				t.Errorf("%s retains legacy analytical escape %q", path, forbidden)
			}
		}
	}
}

func TestCurrentConnectorRegistryExcludesFutureQuackProduct(t *testing.T) {
	root := repoRoot(t)
	for _, path := range []string{
		"internal/analytics/connectors/registry.go",
		"internal/configschema/contracts/contracts.cue",
		"schemas/json/connection.schema.json",
	} {
		body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(strings.ToLower(string(body)), "quack") {
			t.Errorf("%s exposes future Quack product as a current connector", path)
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
	if rule, ok := ClassifyPackage(file.pkgDir); ok && (rule.Layer == LayerComposition || rule.Layer == LayerModule) {
		return true
	}
	if file.pkgDir == "internal/app" {
		switch file.path {
		case "internal/app/build.go",
			"internal/app/server.go",
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
		strings.HasPrefix(file.pkgDir, "internal/admin/storage") ||
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
	if rule, ok := ClassifyPackage(pkgDir); ok {
		switch rule.Layer {
		case LayerAdapter, LayerModule, LayerComposition, LayerPlatform:
			return true
		}
	}
	if pkgDir == "internal/app" ||
		pkgDir == "internal/api" ||
		strings.HasPrefix(pkgDir, "internal/api/") ||
		pkgDir == "internal/cli" ||
		pkgDir == "internal/integration" ||
		pkgDir == "internal/platform" ||
		strings.HasPrefix(pkgDir, "internal/platform/") ||
		pkgDir == "internal/storage" ||
		strings.HasPrefix(pkgDir, "internal/storage/") ||
		pkgDir == "internal/analytics/resource" ||
		pkgDir == "internal/access/oidc" ||
		pkgDir == "internal/access/httpauth" ||
		pkgDir == "internal/access/scimprov" ||
		pkgDir == "internal/admin/storage" ||
		pkgDir == "internal/agent/tools" ||
		strings.HasPrefix(pkgDir, "internal/tools/") ||
		strings.HasPrefix(pkgDir, "internal/testutil/") {
		return true
	}
	if strings.HasSuffix(pkgDir, "/module") {
		return true
	}
	for _, suffix := range []string{"/http", "/sqlite", "/filesystem", "/s3", "/tus", "/duckdb", "/ducklake", "/datastar", "/openai", "/ui"} {
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
	for _, segment := range []string{"/sqlite", "/filesystem", "/s3", "/tus", "/duckdb", "/ducklake", "/datastar", "/http", "/openai"} {
		if strings.Contains(strings.TrimPrefix(imported, modulePath), segment) {
			return true
		}
	}
	return false
}
