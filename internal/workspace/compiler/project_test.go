package compiler

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Yacobolo/libredash/internal/configschema"
	"github.com/Yacobolo/libredash/internal/workspace"
)

func TestCompileProjectSupportsTwoWorkspacesSharingSources(t *testing.T) {
	projectPath := writeProjectFixture(t, map[string]string{
		"libredash.yaml": projectYAML(),
		"connections/olist.yaml": `
apiVersion: libredash.dev/v1
kind: Connection
metadata:
  name: olist
spec:
  kind: local
`,
		"sources/olist.orders.yaml":                                    sourceYAML("olist.orders", "orders.csv", "order_id"),
		"sources/olist.customers.yaml":                                 sourceYAML("olist.customers", "customers.csv", "customer_id"),
		"workspaces/sales/workspace.yaml":                              workspaceYAML("sales"),
		"workspaces/sales/models/orders.yaml":                          modelTableYAML("sales", "orders", "olist.orders", "order_id", "SELECT order_id, order_status AS status FROM source.\"olist.orders\""),
		"workspaces/sales/semantic-models/sales.yaml":                  semanticModelYAML("sales", "orders", "order_count"),
		"workspaces/sales/dashboards/executive-sales.yaml":             dashboardYAML("sales", "executive-sales", "sales"),
		"workspaces/operations/workspace.yaml":                         workspaceYAML("operations"),
		"workspaces/operations/models/orders.yaml":                     modelTableYAML("operations", "orders", "olist.orders", "order_id", "SELECT order_id, order_status AS status FROM source.\"olist.orders\""),
		"workspaces/operations/semantic-models/operations.yaml":        semanticModelYAML("operations", "orders", "order_count"),
		"workspaces/operations/dashboards/fulfillment-operations.yaml": dashboardYAML("operations", "fulfillment-operations", "operations"),
	})

	compiled, err := CompileProject(projectPath, Options{DeploymentID: "dep_test"})
	if err != nil {
		t.Fatalf("CompileProject() error = %v", err)
	}
	if len(compiled.Workspaces) != 2 {
		t.Fatalf("compiled workspaces = %d, want 2", len(compiled.Workspaces))
	}
	for _, id := range []string{"sales", "operations"} {
		compiledWorkspace := compiled.Workspaces[id]
		if compiledWorkspace.Definition == nil {
			t.Fatalf("workspace %q has nil definition", id)
		}
		if _, ok := compiledWorkspace.Definition.Models[id]; !ok {
			t.Fatalf("workspace %q missing semantic model %q", id, id)
		}
		if len(compiledWorkspace.Workspace.Graph.Assets) == 0 {
			t.Fatalf("workspace %q has empty asset graph", id)
		}
	}
	assertGraphAsset(t, compiled.Workspaces["sales"].Workspace.Graph, "connection:olist")
	assertGraphAsset(t, compiled.Workspaces["sales"].Workspace.Graph, "source:olist.orders")
	assertGraphAsset(t, compiled.Workspaces["sales"].Workspace.Graph, "model_table:sales.orders")
	assertGraphAsset(t, compiled.Workspaces["sales"].Workspace.Graph, "dashboard:sales.executive-sales")
	assertGraphMissingAsset(t, compiled.Workspaces["sales"].Workspace.Graph, "source:sales.olist_orders")
	assertGraphAsset(t, compiled.Workspaces["operations"].Workspace.Graph, "source:olist.orders")
	assertGraphAsset(t, compiled.Workspaces["operations"].Workspace.Graph, "model_table:operations.orders")
	assertGraphAsset(t, compiled.Workspaces["operations"].Workspace.Graph, "dashboard:operations.fulfillment-operations")
}

func TestCompileProjectRejectsWorkspaceReadsOutsideAllowlist(t *testing.T) {
	projectPath := writeProjectFixture(t, map[string]string{
		"libredash.yaml": projectYAML(),
		"connections/olist.yaml": `
apiVersion: libredash.dev/v1
kind: Connection
metadata:
  name: olist
spec:
  kind: local
`,
		"sources/olist.orders.yaml":                        sourceYAML("olist.orders", "orders.csv", "order_id"),
		"sources/olist.customers.yaml":                     sourceYAML("olist.customers", "customers.csv", "customer_id"),
		"workspaces/sales/workspace.yaml":                  strings.Replace(workspaceYAML("sales"), "      - olist.customers\n", "", 1),
		"workspaces/sales/models/orders.yaml":              modelTableYAML("sales", "orders", "olist.customers", "customer_id", "SELECT customer_id AS order_id FROM source.\"olist.customers\""),
		"workspaces/sales/semantic-models/sales.yaml":      semanticModelYAML("sales", "orders", "order_count"),
		"workspaces/sales/dashboards/executive-sales.yaml": dashboardYAML("sales", "executive-sales", "sales"),
	})

	_, err := CompileProject(projectPath, Options{DeploymentID: "dep_test"})
	if err == nil {
		t.Fatal("CompileProject() error = nil, want allowlist failure")
	}
	if !strings.Contains(err.Error(), "outside uses.sources") {
		t.Fatalf("CompileProject() error = %v, want outside uses.sources", err)
	}
	diagnostic := configschema.Diagnostics(err)[0]
	if diagnostic.ResourceID != "model_table:sales.orders" || diagnostic.FieldPath != "spec.sources" || diagnostic.File == "" {
		t.Fatalf("diagnostic = %#v, want resource, field, and file context", diagnostic)
	}
}

func TestCompileShowcaseProject(t *testing.T) {
	projectPath := filepath.Join("..", "..", "..", "dashboards", "libredash.yaml")
	compiled, err := CompileProject(projectPath, Options{})
	if err != nil {
		t.Fatalf("CompileProject() error = %v", err)
	}
	if _, ok := compiled.Workspaces["sales"]; !ok {
		t.Fatalf("compiled workspaces = %#v, want sales", compiled.Workspaces)
	}
	if _, ok := compiled.Workspaces["operations"]; !ok {
		t.Fatalf("compiled workspaces = %#v, want operations", compiled.Workspaces)
	}
	if _, ok := compiled.Workspaces["sales"].Definition.Dashboards["executive-sales"]; !ok {
		t.Fatalf("sales dashboards = %#v, want executive-sales", compiled.Workspaces["sales"].Definition.Dashboards)
	}
	if _, ok := compiled.Workspaces["operations"].Definition.Dashboards["fulfillment-operations"]; !ok {
		t.Fatalf("operations dashboards = %#v, want fulfillment-operations", compiled.Workspaces["operations"].Definition.Dashboards)
	}
}

func TestPlanProjectIsStableAndSorted(t *testing.T) {
	projectPath := writeProjectFixture(t, map[string]string{
		"libredash.yaml":                                   projectYAML(),
		"connections/olist.yaml":                           connectionYAML("olist"),
		"sources/olist.orders.yaml":                        sourceYAML("olist.orders", "orders.csv", "order_id"),
		"sources/olist.customers.yaml":                     sourceYAML("olist.customers", "customers.csv", "customer_id"),
		"workspaces/sales/workspace.yaml":                  workspaceYAML("sales"),
		"workspaces/sales/models/z-orders.yaml":            modelTableYAML("sales", "orders", "olist.orders", "order_id", "SELECT order_id, order_status AS status FROM source.\"olist.orders\""),
		"workspaces/sales/models/a-customers.yaml":         modelTableYAML("sales", "customers", "olist.customers", "customer_id", "SELECT customer_id, order_status AS status FROM source.\"olist.customers\""),
		"workspaces/sales/semantic-models/sales.yaml":      semanticModelYAML("sales", "orders", "order_count"),
		"workspaces/sales/dashboards/executive-sales.yaml": dashboardYAML("sales", "executive-sales", "sales"),
	})

	first, err := PlanProject(projectPath)
	if err != nil {
		t.Fatalf("PlanProject() error = %v", err)
	}
	second, err := PlanProject(projectPath)
	if err != nil {
		t.Fatalf("PlanProject() second error = %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("PlanProject() unstable:\nfirst=%#v\nsecond=%#v", first, second)
	}
	if got := first.Workspaces[0].ModelTables; !reflect.DeepEqual(got, []string{"customers", "orders"}) {
		t.Fatalf("model tables = %#v, want sorted customers/orders", got)
	}
}

func TestPlanProjectAgainstGraphReportsStableDiff(t *testing.T) {
	projectPath := writeProjectFixture(t, map[string]string{
		"libredash.yaml":                                   projectYAML(),
		"connections/olist.yaml":                           connectionYAML("olist"),
		"sources/olist.orders.yaml":                        sourceYAML("olist.orders", "orders.csv", "order_id"),
		"sources/olist.customers.yaml":                     sourceYAML("olist.customers", "customers.csv", "customer_id"),
		"workspaces/sales/workspace.yaml":                  workspaceYAML("sales"),
		"workspaces/sales/models/orders.yaml":              modelTableYAML("sales", "orders", "olist.orders", "order_id", "SELECT order_id, order_status AS status FROM source.\"olist.orders\""),
		"workspaces/sales/semantic-models/sales.yaml":      semanticModelYAML("sales", "orders", "order_count"),
		"workspaces/sales/dashboards/executive-sales.yaml": dashboardYAML("sales", "executive-sales", "sales"),
	})
	active, err := CompileProject(projectPath, Options{DeploymentID: "dep_active"})
	if err != nil {
		t.Fatalf("CompileProject() error = %v", err)
	}
	activeGraph := active.Workspaces["sales"].Workspace.Graph
	for index := range activeGraph.Assets {
		if activeGraph.Assets[index].ID == "model_table:sales.orders" {
			activeGraph.Assets[index].ContentHash = "changed"
		}
	}
	activeGraph.Assets = append(activeGraph.Assets, workspace.Asset{
		ID:            "dashboard:sales.removed",
		WorkspaceID:   "sales",
		DeploymentID:  "dep_active",
		Type:          workspace.AssetTypeDashboard,
		Key:           "sales.removed",
		PayloadSchema: workspace.PayloadSchemaForAssetType(workspace.AssetTypeDashboard),
		ContentHash:   "removed",
	})
	if len(activeGraph.Edges) == 0 {
		t.Fatal("fixture graph has no edges")
	}
	activeGraph.Edges = activeGraph.Edges[:len(activeGraph.Edges)-1]

	first, err := PlanProjectAgainstGraph(projectPath, "sales", activeGraph)
	if err != nil {
		t.Fatalf("PlanProjectAgainstGraph() error = %v", err)
	}
	second, err := PlanProjectAgainstGraph(projectPath, "sales", activeGraph)
	if err != nil {
		t.Fatalf("PlanProjectAgainstGraph() second error = %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("plan diff unstable:\nfirst=%#v\nsecond=%#v", first, second)
	}
	workspacePlan := first.Workspaces[0]
	if workspacePlan.Summary.Changed != 1 || workspacePlan.Summary.Removed != 1 || workspacePlan.Summary.DependencyChanges != 1 {
		t.Fatalf("summary = %#v, want one changed, one removed, one dependency change", workspacePlan.Summary)
	}
	if !workspacePlan.Summary.Breaking || !workspacePlan.Summary.MaterializationImpact {
		t.Fatalf("summary impact = %#v, want breaking and materialization impact", workspacePlan.Summary)
	}
}

func TestCompileProjectSupportsMultipleSemanticModelsInWorkspace(t *testing.T) {
	projectPath := writeProjectFixture(t, map[string]string{
		"libredash.yaml":                                   projectYAML(),
		"connections/olist.yaml":                           connectionYAML("olist"),
		"sources/olist.orders.yaml":                        sourceYAML("olist.orders", "orders.csv", "order_id"),
		"sources/olist.customers.yaml":                     sourceYAML("olist.customers", "customers.csv", "customer_id"),
		"workspaces/sales/workspace.yaml":                  workspaceYAML("sales"),
		"workspaces/sales/models/orders.yaml":              modelTableYAML("sales", "orders", "olist.orders", "order_id", "SELECT order_id, order_status AS status FROM source.\"olist.orders\""),
		"workspaces/sales/semantic-models/sales.yaml":      semanticModelNamedYAML("sales", "sales", "orders", "order_count"),
		"workspaces/sales/semantic-models/finance.yaml":    semanticModelNamedYAML("sales", "finance", "orders", "finance_order_count"),
		"workspaces/sales/dashboards/executive-sales.yaml": dashboardYAML("sales", "executive-sales", "sales"),
	})

	compiled, err := CompileProject(projectPath, Options{DeploymentID: "dep_test"})
	if err != nil {
		t.Fatalf("CompileProject() error = %v", err)
	}
	definition := compiled.Workspaces["sales"].Definition
	if len(definition.Models) != 2 {
		t.Fatalf("semantic model count = %d, want 2", len(definition.Models))
	}
	assertGraphAsset(t, compiled.Workspaces["sales"].Workspace.Graph, "semantic_model:sales.sales")
	assertGraphAsset(t, compiled.Workspaces["sales"].Workspace.Graph, "semantic_model:sales.finance")
}

func TestCompileProjectAllowsDuplicateDashboardIDsAcrossWorkspaces(t *testing.T) {
	projectPath := writeProjectFixture(t, map[string]string{
		"libredash.yaml":                                        projectYAML(),
		"connections/olist.yaml":                                connectionYAML("olist"),
		"sources/olist.orders.yaml":                             sourceYAML("olist.orders", "orders.csv", "order_id"),
		"sources/olist.customers.yaml":                          sourceYAML("olist.customers", "customers.csv", "customer_id"),
		"workspaces/sales/workspace.yaml":                       workspaceYAML("sales"),
		"workspaces/sales/models/orders.yaml":                   modelTableYAML("sales", "orders", "olist.orders", "order_id", "SELECT order_id, order_status AS status FROM source.\"olist.orders\""),
		"workspaces/sales/semantic-models/sales.yaml":           semanticModelYAML("sales", "orders", "order_count"),
		"workspaces/sales/dashboards/executive-sales.yaml":      dashboardYAML("sales", "executive-sales", "sales"),
		"workspaces/operations/workspace.yaml":                  workspaceYAML("operations"),
		"workspaces/operations/models/orders.yaml":              modelTableYAML("operations", "orders", "olist.orders", "order_id", "SELECT order_id, order_status AS status FROM source.\"olist.orders\""),
		"workspaces/operations/semantic-models/operations.yaml": semanticModelYAML("operations", "orders", "order_count"),
		"workspaces/operations/dashboards/executive-sales.yaml": dashboardYAML("operations", "executive-sales", "operations"),
	})

	compiled, err := CompileProject(projectPath, Options{DeploymentID: "dep_test"})
	if err != nil {
		t.Fatalf("CompileProject() error = %v", err)
	}
	assertGraphAsset(t, compiled.Workspaces["sales"].Workspace.Graph, "dashboard:sales.executive-sales")
	assertGraphAsset(t, compiled.Workspaces["operations"].Workspace.Graph, "dashboard:operations.executive-sales")
}

func TestCompileProjectRejectsDuplicateDashboardIDsWithinWorkspace(t *testing.T) {
	projectPath := writeProjectFixture(t, map[string]string{
		"libredash.yaml":                              projectYAML(),
		"connections/olist.yaml":                      connectionYAML("olist"),
		"sources/olist.orders.yaml":                   sourceYAML("olist.orders", "orders.csv", "order_id"),
		"sources/olist.customers.yaml":                sourceYAML("olist.customers", "customers.csv", "customer_id"),
		"workspaces/sales/workspace.yaml":             workspaceYAML("sales"),
		"workspaces/sales/models/orders.yaml":         modelTableYAML("sales", "orders", "olist.orders", "order_id", "SELECT order_id, order_status AS status FROM source.\"olist.orders\""),
		"workspaces/sales/semantic-models/sales.yaml": semanticModelYAML("sales", "orders", "order_count"),
		"workspaces/sales/dashboards/one.yaml":        dashboardYAML("sales", "executive-sales", "sales"),
		"workspaces/sales/dashboards/two.yaml":        dashboardYAML("sales", "executive-sales", "sales"),
	})

	_, err := CompileProject(projectPath, Options{DeploymentID: "dep_test"})
	assertCompileErrorContains(t, err, `duplicate Dashboard "executive-sales"`)
}

func TestCompileProjectRejectsUnknownReferences(t *testing.T) {
	t.Run("source connection", func(t *testing.T) {
		projectPath := writeProjectFixture(t, map[string]string{
			"libredash.yaml":                                   projectYAML(),
			"connections/olist.yaml":                           connectionYAML("olist"),
			"sources/olist.orders.yaml":                        strings.Replace(sourceYAML("olist.orders", "orders.csv", "order_id"), "connection: olist", "connection: missing", 1),
			"sources/olist.customers.yaml":                     sourceYAML("olist.customers", "customers.csv", "customer_id"),
			"workspaces/sales/workspace.yaml":                  workspaceYAML("sales"),
			"workspaces/sales/models/orders.yaml":              modelTableYAML("sales", "orders", "olist.orders", "order_id", "SELECT order_id, order_status AS status FROM source.\"olist.orders\""),
			"workspaces/sales/semantic-models/sales.yaml":      semanticModelYAML("sales", "orders", "order_count"),
			"workspaces/sales/dashboards/executive-sales.yaml": dashboardYAML("sales", "executive-sales", "sales"),
		})

		_, err := CompileProject(projectPath, Options{DeploymentID: "dep_test"})
		assertCompileErrorContains(t, err, `Source "olist.orders" references unknown Connection "missing"`)
	})

	t.Run("dashboard semantic model", func(t *testing.T) {
		projectPath := writeProjectFixture(t, map[string]string{
			"libredash.yaml":                                   projectYAML(),
			"connections/olist.yaml":                           connectionYAML("olist"),
			"sources/olist.orders.yaml":                        sourceYAML("olist.orders", "orders.csv", "order_id"),
			"sources/olist.customers.yaml":                     sourceYAML("olist.customers", "customers.csv", "customer_id"),
			"workspaces/sales/workspace.yaml":                  workspaceYAML("sales"),
			"workspaces/sales/models/orders.yaml":              modelTableYAML("sales", "orders", "olist.orders", "order_id", "SELECT order_id, order_status AS status FROM source.\"olist.orders\""),
			"workspaces/sales/semantic-models/sales.yaml":      semanticModelYAML("sales", "orders", "order_count"),
			"workspaces/sales/dashboards/executive-sales.yaml": dashboardYAML("sales", "executive-sales", "missing"),
		})

		_, err := CompileProject(projectPath, Options{DeploymentID: "dep_test"})
		assertCompileErrorContains(t, err, `references unknown SemanticModel "missing"`)
	})
}

func TestCompileProjectRejectsHiddenImportsAndUnsafeIncludes(t *testing.T) {
	t.Run("raw relation", func(t *testing.T) {
		projectPath := writeProjectFixture(t, map[string]string{
			"libredash.yaml":                                   projectYAML(),
			"connections/olist.yaml":                           connectionYAML("olist"),
			"sources/olist.orders.yaml":                        sourceYAML("olist.orders", "orders.csv", "order_id"),
			"sources/olist.customers.yaml":                     sourceYAML("olist.customers", "customers.csv", "customer_id"),
			"workspaces/sales/workspace.yaml":                  workspaceYAML("sales"),
			"workspaces/sales/models/orders.yaml":              modelTableYAML("sales", "orders", "olist.orders", "order_id", "SELECT order_id FROM raw.\"olist.orders\""),
			"workspaces/sales/semantic-models/sales.yaml":      semanticModelYAML("sales", "orders", "order_count"),
			"workspaces/sales/dashboards/executive-sales.yaml": dashboardYAML("sales", "executive-sales", "sales"),
		})

		_, err := CompileProject(projectPath, Options{DeploymentID: "dep_test"})
		assertCompileErrorContains(t, err, "raw.<name> is internal")
	})

	t.Run("unqualified relation", func(t *testing.T) {
		projectPath := writeProjectFixture(t, map[string]string{
			"libredash.yaml":                                   projectYAML(),
			"connections/olist.yaml":                           connectionYAML("olist"),
			"sources/olist.orders.yaml":                        sourceYAML("olist.orders", "orders.csv", "order_id"),
			"sources/olist.customers.yaml":                     sourceYAML("olist.customers", "customers.csv", "customer_id"),
			"workspaces/sales/workspace.yaml":                  workspaceYAML("sales"),
			"workspaces/sales/models/orders.yaml":              modelTableYAML("sales", "orders", "olist.orders", "order_id", "SELECT order_id FROM orders"),
			"workspaces/sales/semantic-models/sales.yaml":      semanticModelYAML("sales", "orders", "order_count"),
			"workspaces/sales/dashboards/executive-sales.yaml": dashboardYAML("sales", "executive-sales", "sales"),
		})

		_, err := CompileProject(projectPath, Options{DeploymentID: "dep_test"})
		assertCompileErrorContains(t, err, "found unqualified relation")
	})

	t.Run("escaping include", func(t *testing.T) {
		projectPath := writeProjectFixture(t, map[string]string{
			"libredash.yaml":         strings.Replace(projectYAML(), "connections/*.yaml", "../*.yaml", 1),
			"connections/olist.yaml": connectionYAML("olist"),
		})

		_, err := CompileProject(projectPath, Options{DeploymentID: "dep_test"})
		assertCompileErrorContains(t, err, "escapes project boundary")
	})

	t.Run("recursive include", func(t *testing.T) {
		projectPath := writeProjectFixture(t, map[string]string{
			"libredash.yaml":         strings.Replace(projectYAML(), "connections/*.yaml", "connections/**/*.yaml", 1),
			"connections/olist.yaml": connectionYAML("olist"),
		})

		_, err := CompileProject(projectPath, Options{DeploymentID: "dep_test"})
		assertCompileErrorContains(t, err, "unsupported ** glob")
	})
}

func TestCompileProjectRejectsSQLSourceMismatchAndCycles(t *testing.T) {
	t.Run("source mismatch", func(t *testing.T) {
		projectPath := writeProjectFixture(t, map[string]string{
			"libredash.yaml":                                   projectYAML(),
			"connections/olist.yaml":                           connectionYAML("olist"),
			"sources/olist.orders.yaml":                        sourceYAML("olist.orders", "orders.csv", "order_id"),
			"sources/olist.customers.yaml":                     sourceYAML("olist.customers", "customers.csv", "customer_id"),
			"workspaces/sales/workspace.yaml":                  workspaceYAML("sales"),
			"workspaces/sales/models/orders.yaml":              modelTableYAML("sales", "orders", "olist.orders", "order_id", "SELECT customer_id AS order_id FROM source.\"olist.customers\""),
			"workspaces/sales/semantic-models/sales.yaml":      semanticModelYAML("sales", "orders", "order_count"),
			"workspaces/sales/dashboards/executive-sales.yaml": dashboardYAML("sales", "executive-sales", "sales"),
		})

		_, err := CompileProject(projectPath, Options{DeploymentID: "dep_test"})
		assertCompileErrorContains(t, err, "SQL source references")
	})

	t.Run("model table cycle", func(t *testing.T) {
		projectPath := writeProjectFixture(t, map[string]string{
			"libredash.yaml":                                   projectYAML(),
			"connections/olist.yaml":                           connectionYAML("olist"),
			"sources/olist.orders.yaml":                        sourceYAML("olist.orders", "orders.csv", "order_id"),
			"sources/olist.customers.yaml":                     sourceYAML("olist.customers", "customers.csv", "customer_id"),
			"workspaces/sales/workspace.yaml":                  workspaceYAML("sales"),
			"workspaces/sales/models/orders.yaml":              sqlModelTableYAML("sales", "orders", "order_id", "SELECT order_id, status FROM model.customers"),
			"workspaces/sales/models/customers.yaml":           sqlModelTableYAML("sales", "customers", "customer_id", "SELECT customer_id, status FROM model.orders"),
			"workspaces/sales/semantic-models/sales.yaml":      semanticModelYAMLWithTables("sales", []string{"orders", "customers"}, "order_count"),
			"workspaces/sales/dashboards/executive-sales.yaml": dashboardYAML("sales", "executive-sales", "sales"),
		})

		_, err := CompileProject(projectPath, Options{DeploymentID: "dep_test"})
		assertCompileErrorContains(t, err, "model table dependency cycle")
	})
}

func writeProjectFixture(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		writeCompilerFixture(t, path, content)
	}
	return filepath.Join(dir, "libredash.yaml")
}

func connectionYAML(name string) string {
	return `
apiVersion: libredash.dev/v1
kind: Connection
metadata:
  name: ` + name + `
spec:
  kind: local
`
}

func projectYAML() string {
	return `
apiVersion: libredash.dev/v1
kind: Project
metadata:
  name: test
spec:
  connections:
    include:
      - connections/*.yaml
  sources:
    include:
      - sources/*.yaml
  workspaces:
    include:
      - workspaces/*/workspace.yaml
`
}

func sourceYAML(name, path, key string) string {
	return `
apiVersion: libredash.dev/v1
kind: Source
metadata:
  name: ` + name + `
spec:
  connection: olist
  path: ` + path + `
  fields:
    ` + key + `:
      type: string
    order_status:
      type: string
`
}

func workspaceYAML(name string) string {
	return `
apiVersion: libredash.dev/v1
kind: Workspace
metadata:
  name: ` + name + `
  title: ` + name + `
spec:
  uses:
    sources:
      - olist.orders
      - olist.customers
  models:
    include:
      - models/*.yaml
  semanticModels:
    include:
      - semantic-models/*.yaml
  dashboards:
    include:
      - dashboards/*.yaml
`
}

func modelTableYAML(workspace, name, source, key, sql string) string {
	return `
apiVersion: libredash.dev/v1
kind: ModelTable
metadata:
  workspace: ` + workspace + `
  name: ` + name + `
spec:
  primary_key: ` + key + `
  sources:
    - ` + source + `
  fields:
    ` + key + `:
      label: ID
  transform:
    sql: |
      ` + sql + `
`
}

func sqlModelTableYAML(workspace, name, key, sql string) string {
	return `
apiVersion: libredash.dev/v1
kind: ModelTable
metadata:
  workspace: ` + workspace + `
  name: ` + name + `
spec:
  primary_key: ` + key + `
  fields:
    ` + key + `:
      label: ID
  transform:
    sql: |
      ` + sql + `
`
}

func semanticModelYAML(workspace, table, measure string) string {
	return semanticModelNamedYAML(workspace, workspace, table, measure)
}

func semanticModelNamedYAML(workspace, name, table, measure string) string {
	return `
apiVersion: libredash.dev/v1
kind: SemanticModel
metadata:
  workspace: ` + workspace + `
  name: ` + name + `
spec:
  baseTable: ` + table + `
  tables:
    - ` + table + `
  measures:
    defaults:
      table: ` + table + `
    ` + measure + `:
      expression: count(` + table + `.order_id)
`
}

func semanticModelYAMLWithTables(workspace string, tables []string, measure string) string {
	return `
apiVersion: libredash.dev/v1
kind: SemanticModel
metadata:
  workspace: ` + workspace + `
  name: ` + workspace + `
spec:
  baseTable: ` + tables[0] + `
  tables:
` + semanticTableListYAML(tables) + `  measures:
    defaults:
      table: ` + tables[0] + `
    ` + measure + `:
      expression: count(` + tables[0] + `.order_id)
`
}

func semanticTableListYAML(tables []string) string {
	var builder strings.Builder
	for _, table := range tables {
		builder.WriteString("    - ")
		builder.WriteString(table)
		builder.WriteByte('\n')
	}
	return builder.String()
}

func dashboardYAML(workspace, name, model string) string {
	return `
apiVersion: libredash.dev/v1
kind: Dashboard
metadata:
  workspace: ` + workspace + `
  name: ` + name + `
  title: ` + name + `
spec:
  semanticModel: ` + model + `
  visuals:
    total:
      kind: kpi
      query:
        measures:
          order_count:
  pages:
    - id: overview
      title: Overview
      visuals:
        - id: total
          kind: kpi_card
          visual: total
          placement:
            col: 1
            row: 1
            col_span: 3
            row_span: 2
`
}

func writeCompilerFixture(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(strings.TrimSpace(content)+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func assertCompileErrorContains(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatalf("CompileProject() error = nil, want %q", want)
	}
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("CompileProject() error = %v, want %q", err, want)
	}
}

func assertGraphAsset(t *testing.T, graph workspace.AssetGraph, id string) {
	t.Helper()
	for _, asset := range graph.Assets {
		if string(asset.ID) == id {
			return
		}
	}
	t.Fatalf("asset %q missing from graph", id)
}

func assertGraphMissingAsset(t *testing.T, graph workspace.AssetGraph, id string) {
	t.Helper()
	for _, asset := range graph.Assets {
		if string(asset.ID) == id {
			t.Fatalf("asset %q unexpectedly present in graph", id)
		}
	}
}
