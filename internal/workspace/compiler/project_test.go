package compiler

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Yacobolo/leapview/internal/configschema"
	reportdef "github.com/Yacobolo/leapview/internal/dashboard/report"
	"github.com/Yacobolo/leapview/internal/workspace"
)

func TestCompileProjectSupportsTwoWorkspacesSharingSources(t *testing.T) {
	projectPath := writeProjectFixture(t, map[string]string{
		"leapview.yaml": projectYAML(),
		"connections/olist.yaml": `
apiVersion: leapview.dev/v1
kind: Connection
metadata:
  name: olist
spec:
  kind: managed
`,
		"sources/olist.orders.yaml":                                    sourceYAML("olist.orders", "orders.csv", "order_id"),
		"sources/olist.customers.yaml":                                 sourceYAML("olist.customers", "customers.csv", "customer_id"),
		"workspaces/sales/workspace.yaml":                              workspaceYAMLWithAccess("sales"),
		"workspaces/sales/access/analysts.yaml":                        workspaceGroupYAML("sales", "analysts", "analyst@example.com"),
		"workspaces/sales/access/analysts-viewer.yaml":                 workspaceRoleBindingGroupYAML("sales", "analysts-viewer", "viewer", "analysts"),
		"workspaces/sales/models/orders.yaml":                          modelTableYAML("sales", "orders", "olist.orders", "order_id", "SELECT order_id, order_status AS status FROM source.\"olist.orders\""),
		"workspaces/sales/semantic-models/sales.yaml":                  semanticModelYAML("sales", "orders", "order_count"),
		"workspaces/sales/dashboards/executive-sales.yaml":             dashboardYAML("sales", "executive-sales", "sales"),
		"workspaces/operations/workspace.yaml":                         workspaceYAMLWithAccess("operations"),
		"workspaces/operations/access/operators.yaml":                  workspaceGroupYAML("operations", "operators", "operator@example.com"),
		"workspaces/operations/access/operators-viewer.yaml":           workspaceRoleBindingGroupYAML("operations", "operators-viewer", "viewer", "operators"),
		"workspaces/operations/models/orders.yaml":                     modelTableYAML("operations", "orders", "olist.orders", "order_id", "SELECT order_id, order_status AS status FROM source.\"olist.orders\""),
		"workspaces/operations/semantic-models/operations.yaml":        semanticModelYAML("operations", "orders", "order_count"),
		"workspaces/operations/dashboards/fulfillment-operations.yaml": dashboardYAML("operations", "fulfillment-operations", "operations"),
	})

	compiled, err := CompileProject(projectPath, Options{ServingStateID: "dep_test"})
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
	assertGraphAsset(t, compiled.Workspaces["sales"].Workspace.Graph, "workspace_group:sales.analysts")
	assertGraphAsset(t, compiled.Workspaces["sales"].Workspace.Graph, "workspace_role_binding:sales.analysts-viewer")
	assertAssetSourceFileContains(t, compiled.Workspaces["sales"].Workspace.Graph, "source:olist.orders", filepath.Join("sources", "olist.orders.yaml"))
	assertAssetSourceFileContains(t, compiled.Workspaces["sales"].Workspace.Graph, "field:sales.sales.orders.order_id", filepath.Join("workspaces", "sales", "semantic-models", "sales.yaml"))
	assertGraphMissingAsset(t, compiled.Workspaces["sales"].Workspace.Graph, "source:sales.olist_orders")
	assertGraphAsset(t, compiled.Workspaces["operations"].Workspace.Graph, "source:olist.orders")
	assertGraphAsset(t, compiled.Workspaces["operations"].Workspace.Graph, "model_table:operations.orders")
	assertGraphAsset(t, compiled.Workspaces["operations"].Workspace.Graph, "dashboard:operations.fulfillment-operations")
	sourceField := compiled.Workspaces["sales"].Definition.Models["sales"].Sources["olist_orders"].Fields["order_id"]
	if sourceField.Type != "string" {
		t.Fatalf("source field type = %q, want string", sourceField.Type)
	}
	var payload sourcePayloadV1
	unmarshalGraphPayload(t, compiled.Workspaces["sales"].Workspace.Graph, "source:olist.orders", &payload)
	if payload.Fields["order_id"].Type != "string" {
		t.Fatalf("source payload field type = %q, want string", payload.Fields["order_id"].Type)
	}
	group := compiled.Workspaces["sales"].Definition.Access.Groups["analysts"]
	if group.ID != "analysts" || len(group.Members) != 1 || group.Members[0].Email != "analyst@example.com" {
		t.Fatalf("compiled access group = %#v, want analyst member", group)
	}
	binding := compiled.Workspaces["sales"].Definition.Access.RoleBindings["analysts-viewer"]
	if binding.Role != "viewer" || binding.Subject.Kind != "group" || binding.Subject.Group != "analysts" {
		t.Fatalf("compiled access binding = %#v, want analysts viewer group binding", binding)
	}
	if got := compiled.Workspaces["sales"].Definition.Dashboards["executive-sales"].Pages[0].ID; got != "overview" {
		t.Fatalf("compiled dashboard page id = %q, want authored page name overview", got)
	}
}

func TestCompileRequiresExplicitWorkspaceID(t *testing.T) {
	projectPath := writeProjectFixture(t, map[string]string{
		"leapview.yaml":                                   projectYAML(),
		"connections/olist.yaml":                           connectionYAML("olist"),
		"sources/olist.orders.yaml":                        sourceYAML("olist.orders", "orders.csv", "order_id"),
		"sources/olist.customers.yaml":                     sourceYAML("olist.customers", "customers.csv", "customer_id"),
		"workspaces/sales/workspace.yaml":                  workspaceYAML("sales"),
		"workspaces/sales/models/orders.yaml":              modelTableYAML("sales", "orders", "olist.orders", "order_id", "SELECT order_id, order_status AS status FROM source.\"olist.orders\""),
		"workspaces/sales/semantic-models/sales.yaml":      semanticModelYAML("sales", "orders", "order_count"),
		"workspaces/sales/dashboards/executive-sales.yaml": dashboardYAML("sales", "executive-sales", "sales"),
	})

	_, err := Compile(projectPath, Options{ServingStateID: "dep_test"})
	if err == nil || !strings.Contains(err.Error(), "workspace id is required") {
		t.Fatalf("Compile(blank workspace) error = %v, want workspace id required", err)
	}

	compiled, err := Compile(projectPath, Options{WorkspaceID: "sales", ServingStateID: "dep_test"})
	if err != nil {
		t.Fatalf("Compile(sales) error = %v", err)
	}
	if got := compiled.Workspace.ID; got != "sales" {
		t.Fatalf("compiled workspace = %q, want sales", got)
	}
}

func TestCompileProjectCompilesRefreshPipeline(t *testing.T) {
	projectPath := writeProjectFixture(t, map[string]string{
		"leapview.yaml":                                        projectYAML(),
		"connections/olist.yaml":                                connectionYAML("olist"),
		"sources/olist.orders.yaml":                             sourceYAML("olist.orders", "orders.csv", "order_id"),
		"sources/olist.customers.yaml":                          sourceYAML("olist.customers", "customers.csv", "customer_id"),
		"workspaces/sales/workspace.yaml":                       workspaceYAMLWithRefreshPipelines("sales"),
		"workspaces/sales/models/orders.yaml":                   modelTableYAML("sales", "orders", "olist.orders", "order_id", "SELECT order_id, order_status AS status FROM source.\"olist.orders\""),
		"workspaces/sales/semantic-models/sales.yaml":           semanticModelYAML("sales", "orders", "order_count"),
		"workspaces/sales/dashboards/executive-sales.yaml":      dashboardYAML("sales", "executive-sales", "sales"),
		"workspaces/sales/refresh-pipelines/sales-refresh.yaml": refreshPipelineYAML("sales", "sales-refresh", "sales", []string{"0 6 * * *|Europe/Copenhagen", "30 18 * * MON-FRI|"}),
	})

	compiled, err := CompileProject(projectPath, Options{ServingStateID: "dep_test"})
	if err != nil {
		t.Fatalf("CompileProject() error = %v", err)
	}
	pipeline, ok := compiled.Workspaces["sales"].Definition.RefreshPipelines["sales-refresh"]
	if !ok {
		t.Fatalf("refresh pipelines = %#v, want sales-refresh", compiled.Workspaces["sales"].Definition.RefreshPipelines)
	}
	if pipeline.SemanticModel != "sales" || len(pipeline.Schedules) != 2 {
		t.Fatalf("pipeline = %#v", pipeline)
	}
	if pipeline.Schedules[1].Timezone != "UTC" {
		t.Fatalf("second schedule timezone = %q, want UTC", pipeline.Schedules[1].Timezone)
	}
	assertGraphAsset(t, compiled.Workspaces["sales"].Workspace.Graph, "refresh_pipeline:sales.sales-refresh")
	assertAssetSourceFileContains(t, compiled.Workspaces["sales"].Workspace.Graph, "refresh_pipeline:sales.sales-refresh", filepath.Join("workspaces", "sales", "refresh-pipelines", "sales-refresh.yaml"))
}

func TestCompileProjectSupportsManualOnlyRefreshPipeline(t *testing.T) {
	projectPath := writeProjectFixture(t, map[string]string{
		"leapview.yaml":                                        projectYAML(),
		"connections/olist.yaml":                                connectionYAML("olist"),
		"sources/olist.orders.yaml":                             sourceYAML("olist.orders", "orders.csv", "order_id"),
		"sources/olist.customers.yaml":                          sourceYAML("olist.customers", "customers.csv", "customer_id"),
		"workspaces/sales/workspace.yaml":                       workspaceYAMLWithRefreshPipelines("sales"),
		"workspaces/sales/models/orders.yaml":                   modelTableYAML("sales", "orders", "olist.orders", "order_id", "SELECT order_id, order_status AS status FROM source.\"olist.orders\""),
		"workspaces/sales/semantic-models/sales.yaml":           semanticModelYAML("sales", "orders", "order_count"),
		"workspaces/sales/dashboards/executive-sales.yaml":      dashboardYAML("sales", "executive-sales", "sales"),
		"workspaces/sales/refresh-pipelines/sales-refresh.yaml": refreshPipelineYAML("sales", "sales-refresh", "sales", nil),
	})

	compiled, err := CompileProject(projectPath, Options{ServingStateID: "dep_test"})
	if err != nil {
		t.Fatalf("CompileProject() error = %v", err)
	}
	if got := len(compiled.Workspaces["sales"].Definition.RefreshPipelines["sales-refresh"].Schedules); got != 0 {
		t.Fatalf("schedules = %d, want 0", got)
	}
}

func TestCompileProjectRejectsInvalidRefreshPipeline(t *testing.T) {
	cases := []struct {
		name  string
		files map[string]string
		want  string
		field string
		id    string
	}{
		{
			name:  "unknown semantic model",
			files: map[string]string{"workspaces/sales/refresh-pipelines/sales-refresh.yaml": refreshPipelineYAML("sales", "sales-refresh", "missing", nil)},
			want:  `unknown semantic model "missing"`, field: "spec.semanticModel", id: "refresh_pipeline:sales.sales-refresh",
		},
		{
			name:  "workspace mismatch",
			files: map[string]string{"workspaces/sales/refresh-pipelines/sales-refresh.yaml": refreshPipelineYAML("operations", "sales-refresh", "sales", nil)},
			want:  `workspace = "operations", want "sales"`, field: "metadata.workspace", id: "refresh_pipeline:operations.sales-refresh",
		},
		{
			name: "one pipeline per model",
			files: map[string]string{
				"workspaces/sales/refresh-pipelines/first.yaml":  refreshPipelineYAML("sales", "first", "sales", nil),
				"workspaces/sales/refresh-pipelines/second.yaml": refreshPipelineYAML("sales", "second", "sales", nil),
			},
			want: `semantic model "sales" already has refresh pipeline`, field: "spec.semanticModel", id: "refresh_pipeline:sales.second",
		},
		{
			name:  "duplicate schedule",
			files: map[string]string{"workspaces/sales/refresh-pipelines/sales-refresh.yaml": refreshPipelineYAML("sales", "sales-refresh", "sales", []string{"0 6 * * *|UTC", "0  6  * * *|"})},
			want:  "duplicate schedule", field: "spec.on.schedule", id: "refresh_pipeline:sales.sales-refresh",
		},
		{
			name:  "cron alias",
			files: map[string]string{"workspaces/sales/refresh-pipelines/sales-refresh.yaml": refreshPipelineYAML("sales", "sales-refresh", "sales", []string{"@daily|UTC"})},
			want:  "five-field", field: "spec.on.schedule", id: "refresh_pipeline:sales.sales-refresh",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			files := map[string]string{
				"leapview.yaml":                                   projectYAML(),
				"connections/olist.yaml":                           connectionYAML("olist"),
				"sources/olist.orders.yaml":                        sourceYAML("olist.orders", "orders.csv", "order_id"),
				"sources/olist.customers.yaml":                     sourceYAML("olist.customers", "customers.csv", "customer_id"),
				"workspaces/sales/workspace.yaml":                  workspaceYAMLWithRefreshPipelines("sales"),
				"workspaces/sales/models/orders.yaml":              modelTableYAML("sales", "orders", "olist.orders", "order_id", "SELECT order_id, order_status AS status FROM source.\"olist.orders\""),
				"workspaces/sales/semantic-models/sales.yaml":      semanticModelYAML("sales", "orders", "order_count"),
				"workspaces/sales/dashboards/executive-sales.yaml": dashboardYAML("sales", "executive-sales", "sales"),
			}
			for path, content := range tc.files {
				files[path] = content
			}
			projectPath := writeProjectFixture(t, files)
			_, err := CompileProject(projectPath, Options{ServingStateID: "dep_test"})
			assertCompileErrorContains(t, err, tc.want)
			assertDiagnostic(t, err, tc.id, tc.field)
		})
	}
}

func TestCompileProjectRejectsInvalidAccessResources(t *testing.T) {
	cases := []struct {
		name  string
		files map[string]string
		want  string
		field string
	}{
		{
			name: "unknown role",
			files: map[string]string{
				"workspaces/sales/access/analysts.yaml":        workspaceGroupYAML("sales", "analysts", "analyst@example.com"),
				"workspaces/sales/access/analysts-viewer.yaml": workspaceRoleBindingGroupYAML("sales", "analysts-viewer", "superuser", "analysts"),
			},
			want:  `superuser`,
			field: "spec",
		},
		{
			name: "unknown group",
			files: map[string]string{
				"workspaces/sales/access/analysts-viewer.yaml": workspaceRoleBindingGroupYAML("sales", "analysts-viewer", "viewer", "missing"),
			},
			want:  `unknown WorkspaceGroup "missing"`,
			field: "spec.subject.group",
		},
		{
			name: "workspace mismatch",
			files: map[string]string{
				"workspaces/sales/access/analysts.yaml":        strings.Replace(workspaceGroupYAML("sales", "analysts", "analyst@example.com"), "workspace: sales", "workspace: operations", 1),
				"workspaces/sales/access/analysts-viewer.yaml": workspaceRoleBindingGroupYAML("sales", "analysts-viewer", "viewer", "analysts"),
			},
			want:  `workspace = "operations", want "sales"`,
			field: "metadata.workspace",
		},
		{
			name: "duplicate group",
			files: map[string]string{
				"workspaces/sales/access/analysts-one.yaml":    workspaceGroupYAML("sales", "analysts", "one@example.com"),
				"workspaces/sales/access/analysts-two.yaml":    workspaceGroupYAML("sales", "analysts", "two@example.com"),
				"workspaces/sales/access/analysts-viewer.yaml": workspaceRoleBindingGroupYAML("sales", "analysts-viewer", "viewer", "analysts"),
			},
			want:  `duplicate WorkspaceGroup "analysts"`,
			field: "metadata.name",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			files := map[string]string{
				"leapview.yaml":                                   projectYAML(),
				"connections/olist.yaml":                           connectionYAML("olist"),
				"sources/olist.orders.yaml":                        sourceYAML("olist.orders", "orders.csv", "order_id"),
				"sources/olist.customers.yaml":                     sourceYAML("olist.customers", "customers.csv", "customer_id"),
				"workspaces/sales/workspace.yaml":                  workspaceYAMLWithAccess("sales"),
				"workspaces/sales/models/orders.yaml":              modelTableYAML("sales", "orders", "olist.orders", "order_id", "SELECT order_id, order_status AS status FROM source.\"olist.orders\""),
				"workspaces/sales/semantic-models/sales.yaml":      semanticModelYAML("sales", "orders", "order_count"),
				"workspaces/sales/dashboards/executive-sales.yaml": dashboardYAML("sales", "executive-sales", "sales"),
			}
			for path, content := range tc.files {
				files[path] = content
			}
			projectPath := writeProjectFixture(t, files)
			_, err := CompileProject(projectPath, Options{ServingStateID: "dep_test"})
			assertCompileErrorContains(t, err, tc.want)
			if tc.field != "" {
				diagnostics := configschema.Diagnostics(err)
				if len(diagnostics) == 0 || diagnostics[0].FieldPath != tc.field {
					t.Fatalf("diagnostics = %#v, want field %q", diagnostics, tc.field)
				}
			}
		})
	}
}

func TestCompileProjectRejectsWorkspaceReadsOutsideAllowlist(t *testing.T) {
	projectPath := writeProjectFixture(t, map[string]string{
		"leapview.yaml": projectYAML(),
		"connections/olist.yaml": `
apiVersion: leapview.dev/v1
kind: Connection
metadata:
  name: olist
spec:
  kind: managed
`,
		"sources/olist.orders.yaml":                        sourceYAML("olist.orders", "orders.csv", "order_id"),
		"sources/olist.customers.yaml":                     sourceYAML("olist.customers", "customers.csv", "customer_id"),
		"workspaces/sales/workspace.yaml":                  strings.Replace(workspaceYAML("sales"), "      - olist.customers\n", "", 1),
		"workspaces/sales/models/orders.yaml":              modelTableYAML("sales", "orders", "olist.customers", "customer_id", "SELECT customer_id AS order_id FROM source.\"olist.customers\""),
		"workspaces/sales/semantic-models/sales.yaml":      semanticModelYAML("sales", "orders", "order_count"),
		"workspaces/sales/dashboards/executive-sales.yaml": dashboardYAML("sales", "executive-sales", "sales"),
	})

	_, err := CompileProject(projectPath, Options{ServingStateID: "dep_test"})
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
	projectPath := filepath.Join("..", "..", "..", "dashboards", "leapview.yaml")
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
	if _, ok := compiled.Workspaces["visuals"]; !ok {
		t.Fatalf("compiled workspaces = %#v, want visuals", compiled.Workspaces)
	}
	if _, ok := compiled.Workspaces["sales"].Definition.Dashboards["executive-sales"]; !ok {
		t.Fatalf("sales dashboards = %#v, want executive-sales", compiled.Workspaces["sales"].Definition.Dashboards)
	}
	if _, ok := compiled.Workspaces["operations"].Definition.Dashboards["fulfillment-operations"]; !ok {
		t.Fatalf("operations dashboards = %#v, want fulfillment-operations", compiled.Workspaces["operations"].Definition.Dashboards)
	}
	visuals := compiled.Workspaces["visuals"]
	showcase, ok := visuals.Definition.Dashboards["visual-showcase"]
	if !ok {
		t.Fatalf("visuals dashboards = %#v, want visual-showcase", visuals.Definition.Dashboards)
	}
	if _, ok := visuals.Definition.Models["visuals"]; !ok {
		t.Fatalf("visuals semantic models = %#v, want visuals", visuals.Definition.Models)
	}
	assertVisualShowcaseCoverage(t, showcase)
}

func TestPlanProjectIsStableAndSorted(t *testing.T) {
	projectPath := writeProjectFixture(t, map[string]string{
		"leapview.yaml":                                   projectYAML(),
		"connections/olist.yaml":                           connectionYAML("olist"),
		"sources/olist.orders.yaml":                        sourceYAML("olist.orders", "orders.csv", "order_id"),
		"sources/olist.customers.yaml":                     sourceYAML("olist.customers", "customers.csv", "customer_id"),
		"workspaces/sales/workspace.yaml":                  workspaceYAMLWithAccess("sales"),
		"workspaces/sales/access/analysts.yaml":            workspaceGroupYAML("sales", "analysts", "analyst@example.com"),
		"workspaces/sales/access/analysts-viewer.yaml":     workspaceRoleBindingGroupYAML("sales", "analysts-viewer", "viewer", "analysts"),
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
	if got := first.Workspaces[0].WorkspaceGroups; !reflect.DeepEqual(got, []string{"analysts"}) {
		t.Fatalf("workspace groups = %#v, want analysts", got)
	}
	if got := first.Workspaces[0].WorkspaceRoleBindings; !reflect.DeepEqual(got, []string{"analysts-viewer"}) {
		t.Fatalf("workspace role bindings = %#v, want analysts-viewer", got)
	}
}

func TestPlanProjectAgainstGraphReportsStableDiff(t *testing.T) {
	projectPath := writeProjectFixture(t, map[string]string{
		"leapview.yaml":                                   projectYAML(),
		"connections/olist.yaml":                           connectionYAML("olist"),
		"sources/olist.orders.yaml":                        sourceYAML("olist.orders", "orders.csv", "order_id"),
		"sources/olist.customers.yaml":                     sourceYAML("olist.customers", "customers.csv", "customer_id"),
		"workspaces/sales/workspace.yaml":                  workspaceYAML("sales"),
		"workspaces/sales/models/orders.yaml":              modelTableYAML("sales", "orders", "olist.orders", "order_id", "SELECT order_id, order_status AS status FROM source.\"olist.orders\""),
		"workspaces/sales/semantic-models/sales.yaml":      semanticModelYAML("sales", "orders", "order_count"),
		"workspaces/sales/dashboards/executive-sales.yaml": dashboardYAML("sales", "executive-sales", "sales"),
	})
	active, err := CompileProject(projectPath, Options{ServingStateID: "dep_active"})
	if err != nil {
		t.Fatalf("CompileProject() error = %v", err)
	}
	activeGraph := active.Workspaces["sales"].Workspace.Graph
	for index := range activeGraph.Assets {
		if activeGraph.Assets[index].ID == "model_table:sales.orders" {
			var payload modelTablePayloadV1
			if err := json.Unmarshal([]byte(activeGraph.Assets[index].PayloadJSON), &payload); err != nil {
				t.Fatalf("unmarshal model table payload: %v", err)
			}
			payload.SQL = "SELECT order_id, order_status AS status, 'changed' AS changed FROM source.\"olist.orders\""
			payloadBytes, err := json.Marshal(payload)
			if err != nil {
				t.Fatalf("marshal model table payload: %v", err)
			}
			activeGraph.Assets[index].PayloadJSON = string(payloadBytes)
		}
	}
	activeGraph.Assets = append(activeGraph.Assets, workspace.Asset{
		ID:             "dashboard:sales.removed",
		WorkspaceID:    "sales",
		ServingStateID: "dep_active",
		Type:           workspace.AssetTypeDashboard,
		Key:            "sales.removed",
		PayloadSchema:  workspace.PayloadSchemaForAssetType(workspace.AssetTypeDashboard),
		ContentHash:    "removed",
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

func TestPlanProjectAgainstGraphReportsSemanticAndAccessImpact(t *testing.T) {
	projectPath := writeProjectFixture(t, map[string]string{
		"leapview.yaml":                                   projectYAML(),
		"connections/olist.yaml":                           connectionYAML("olist"),
		"sources/olist.orders.yaml":                        sourceYAML("olist.orders", "orders.csv", "order_id"),
		"sources/olist.customers.yaml":                     sourceYAML("olist.customers", "customers.csv", "customer_id"),
		"workspaces/sales/workspace.yaml":                  workspaceYAMLWithAccess("sales"),
		"workspaces/sales/access/analysts.yaml":            workspaceGroupYAML("sales", "analysts", "analyst@example.com"),
		"workspaces/sales/access/analysts-viewer.yaml":     workspaceRoleBindingGroupYAML("sales", "analysts-viewer", "viewer", "analysts"),
		"workspaces/sales/models/orders.yaml":              modelTableYAML("sales", "orders", "olist.orders", "order_id", "SELECT order_id, order_status AS status FROM source.\"olist.orders\""),
		"workspaces/sales/semantic-models/sales.yaml":      semanticModelYAML("sales", "orders", "order_count"),
		"workspaces/sales/dashboards/executive-sales.yaml": dashboardYAML("sales", "executive-sales", "sales"),
	})
	active, err := CompileProject(projectPath, Options{ServingStateID: "dep_active"})
	if err != nil {
		t.Fatalf("CompileProject() error = %v", err)
	}
	activeGraph := active.Workspaces["sales"].Workspace.Graph
	for index := range activeGraph.Assets {
		switch activeGraph.Assets[index].ID {
		case "source:olist.orders":
			var payload sourcePayloadV1
			unmarshalGraphPayload(t, activeGraph, string(activeGraph.Assets[index].ID), &payload)
			field := payload.Fields["order_id"]
			field.Type = "integer"
			payload.Fields["order_id"] = field
			raw, err := json.Marshal(payload)
			if err != nil {
				t.Fatal(err)
			}
			activeGraph.Assets[index].PayloadJSON = string(raw)
			activeGraph.Assets[index].ContentHash = "old-source-hash"
		case "workspace_group:sales.analysts":
			activeGraph.Assets[index].ContentHash = "old-access-hash"
		}
	}

	plan, err := PlanProjectAgainstGraph(projectPath, "sales", activeGraph)
	if err != nil {
		t.Fatalf("PlanProjectAgainstGraph() error = %v", err)
	}
	summary := plan.Workspaces[0].Summary
	if !summary.Breaking || !summary.MaterializationImpact || !summary.AccessImpact {
		t.Fatalf("summary = %#v, want breaking, materialization, and access impact", summary)
	}
	var sourceBreaking, groupAccess bool
	for _, change := range plan.Workspaces[0].Changes {
		if change.ID == "source:olist.orders" {
			sourceBreaking = change.Breaking && change.MaterializationImpact
		}
		if change.ID == "workspace_group:sales.analysts" {
			groupAccess = change.AccessImpact
		}
	}
	if !sourceBreaking || !groupAccess {
		t.Fatalf("changes = %#v, want source breaking/materialization and group access impact", plan.Workspaces[0].Changes)
	}
}

func TestDiffAssetGraphsMarksUsedSemanticChildrenBreaking(t *testing.T) {
	workspaceID := workspace.WorkspaceID("sales")
	activeDeployment := workspace.ServingStateID("dep_active")
	authoredDeployment := workspace.ServingStateID("plan")
	field := testPlanAsset(t, workspaceID, activeDeployment, workspace.AssetTypeField, "sales.orders.order_id", "semantic_table:sales.sales.orders")
	measure := testPlanAsset(t, workspaceID, activeDeployment, workspace.AssetTypeMeasure, "sales.order_count", "semantic_model:sales.sales")
	visual := testPlanAsset(t, workspaceID, activeDeployment, workspace.AssetTypeVisual, "sales.executive-sales.total", "dashboard:sales.executive-sales")
	active := workspace.AssetGraph{
		Assets: []workspace.Asset{field, measure, visual},
		Edges: []workspace.AssetEdge{
			workspace.NewAssetEdge(workspaceID, activeDeployment, visual.ID, field.ID, workspace.AssetEdgeUsesField),
			workspace.NewAssetEdge(workspaceID, activeDeployment, visual.ID, measure.ID, workspace.AssetEdgeUsesMeasure),
		},
	}
	authored := workspace.AssetGraph{
		Assets: []workspace.Asset{testPlanAsset(t, workspaceID, authoredDeployment, workspace.AssetTypeVisual, "sales.executive-sales.total", "dashboard:sales.executive-sales")},
	}

	changes, _, summary := diffAssetGraphs(authored, active)
	if !summary.Breaking {
		t.Fatalf("summary = %#v, want breaking", summary)
	}
	for _, id := range []string{"field:sales.orders.order_id", "measure:sales.order_count"} {
		var found bool
		for _, change := range changes {
			if change.ID == id {
				found = true
				if !change.Breaking {
					t.Fatalf("change %s = %#v, want breaking", id, change)
				}
			}
		}
		if !found {
			t.Fatalf("changes = %#v, missing %s", changes, id)
		}
	}
}

func TestDiffAssetGraphsMarksRemovedDashboardDependencyBreaking(t *testing.T) {
	workspaceID := workspace.WorkspaceID("sales")
	activeDeployment := workspace.ServingStateID("dep_active")
	authoredDeployment := workspace.ServingStateID("plan")
	field := testPlanAsset(t, workspaceID, activeDeployment, workspace.AssetTypeField, "sales.orders.status", "semantic_table:sales.sales.orders")
	authoredField := field
	authoredField.ServingStateID = authoredDeployment
	visual := testPlanAsset(t, workspaceID, activeDeployment, workspace.AssetTypeVisual, "sales.executive-sales.status", "dashboard:sales.executive-sales")
	authoredVisual := visual
	authoredVisual.ServingStateID = authoredDeployment
	active := workspace.AssetGraph{
		Assets: []workspace.Asset{field, visual},
		Edges:  []workspace.AssetEdge{workspace.NewAssetEdge(workspaceID, activeDeployment, visual.ID, field.ID, workspace.AssetEdgeUsesField)},
	}
	authored := workspace.AssetGraph{Assets: []workspace.Asset{authoredField, authoredVisual}}

	changes, dependencies, summary := diffAssetGraphs(authored, active)
	if len(changes) != 0 {
		t.Fatalf("changes = %#v, want no asset changes", changes)
	}
	if !summary.Breaking {
		t.Fatalf("summary = %#v dependencies=%#v, want dependency breaking", summary, dependencies)
	}
	if len(dependencies) != 1 || dependencies[0].Action != "remove" || !dependencies[0].Breaking {
		t.Fatalf("dependency changes = %#v, want one breaking removal", dependencies)
	}
}

func TestDiffAssetGraphsMarksRemovedDashboardSemanticModelDependencyBreaking(t *testing.T) {
	workspaceID := workspace.WorkspaceID("sales")
	activeDeployment := workspace.ServingStateID("dep_active")
	authoredDeployment := workspace.ServingStateID("plan")
	model := testPlanAsset(t, workspaceID, activeDeployment, workspace.AssetTypeSemanticModel, "sales.sales", "catalog:sales")
	authoredModel := model
	authoredModel.ServingStateID = authoredDeployment
	dashboard := testPlanAsset(t, workspaceID, activeDeployment, workspace.AssetTypeDashboard, "sales.executive-sales", "catalog:sales")
	authoredDashboard := dashboard
	authoredDashboard.ServingStateID = authoredDeployment
	active := workspace.AssetGraph{
		Assets: []workspace.Asset{model, dashboard},
		Edges:  []workspace.AssetEdge{workspace.NewAssetEdge(workspaceID, activeDeployment, dashboard.ID, model.ID, workspace.AssetEdgeUsesSemanticModel)},
	}
	authored := workspace.AssetGraph{Assets: []workspace.Asset{authoredModel, authoredDashboard}}

	_, dependencies, summary := diffAssetGraphs(authored, active)
	if !summary.Breaking || len(dependencies) != 1 || !dependencies[0].Breaking {
		t.Fatalf("summary = %#v dependencies=%#v, want semantic model dependency breaking", summary, dependencies)
	}
}

func TestDiffAssetGraphsKeepsSourceDependencyRemovalMaterializationOnly(t *testing.T) {
	workspaceID := workspace.WorkspaceID("sales")
	activeDeployment := workspace.ServingStateID("dep_active")
	authoredDeployment := workspace.ServingStateID("plan")
	source := testPlanAsset(t, workspaceID, activeDeployment, workspace.AssetTypeSource, "olist.orders", "catalog:sales")
	authoredSource := source
	authoredSource.ServingStateID = authoredDeployment
	modelTable := testPlanAsset(t, workspaceID, activeDeployment, workspace.AssetTypeModelTable, "sales.orders", "catalog:sales")
	authoredModelTable := modelTable
	authoredModelTable.ServingStateID = authoredDeployment
	active := workspace.AssetGraph{
		Assets: []workspace.Asset{source, modelTable},
		Edges:  []workspace.AssetEdge{workspace.NewAssetEdge(workspaceID, activeDeployment, modelTable.ID, source.ID, workspace.AssetEdgeReadsSource)},
	}
	authored := workspace.AssetGraph{Assets: []workspace.Asset{authoredSource, authoredModelTable}}

	_, dependencies, summary := diffAssetGraphs(authored, active)
	if summary.Breaking || !summary.MaterializationImpact {
		t.Fatalf("summary = %#v dependencies=%#v, want materialization impact without breaking", summary, dependencies)
	}
	if len(dependencies) != 1 || dependencies[0].Breaking {
		t.Fatalf("dependency changes = %#v, want non-breaking source dependency removal", dependencies)
	}
}

func TestDiffAssetGraphsTreatsUsedFieldLabelChangeAsNonBreaking(t *testing.T) {
	workspaceID := workspace.WorkspaceID("sales")
	activeDeployment := workspace.ServingStateID("dep_active")
	authoredDeployment := workspace.ServingStateID("plan")
	activeField := testPlanAssetPayload(t, workspaceID, activeDeployment, workspace.AssetTypeField, "sales.sales.orders.status", "semantic_table:sales.sales.orders", fieldPayloadV1{
		Field: "orders.status", Table: "orders", Name: "status", Label: "Status", Type: "string", Expression: "status",
	})
	authoredField := testPlanAssetPayload(t, workspaceID, authoredDeployment, workspace.AssetTypeField, "sales.sales.orders.status", "semantic_table:sales.sales.orders", fieldPayloadV1{
		Field: "orders.status", Table: "orders", Name: "status", Label: "Order Status", Type: "string", Expression: "status",
	})
	visual := testPlanAsset(t, workspaceID, activeDeployment, workspace.AssetTypeVisual, "sales.executive-sales.status", "dashboard:sales.executive-sales")
	active := workspace.AssetGraph{
		Assets: []workspace.Asset{activeField, visual},
		Edges:  []workspace.AssetEdge{workspace.NewAssetEdge(workspaceID, activeDeployment, visual.ID, activeField.ID, workspace.AssetEdgeUsesField)},
	}
	authored := workspace.AssetGraph{
		Assets: []workspace.Asset{authoredField, visual},
		Edges:  []workspace.AssetEdge{workspace.NewAssetEdge(workspaceID, authoredDeployment, visual.ID, authoredField.ID, workspace.AssetEdgeUsesField)},
	}

	changes, _, summary := diffAssetGraphs(authored, active)
	if summary.Breaking {
		t.Fatalf("summary = %#v changes=%#v, want non-breaking label-only change", summary, changes)
	}
	if len(changes) != 1 || changes[0].ID != string(activeField.ID) || changes[0].Breaking {
		t.Fatalf("changes = %#v, want one non-breaking field change", changes)
	}
}

func TestDiffAssetGraphsTreatsUsedFieldExpressionChangeAsBreaking(t *testing.T) {
	workspaceID := workspace.WorkspaceID("sales")
	activeDeployment := workspace.ServingStateID("dep_active")
	authoredDeployment := workspace.ServingStateID("plan")
	activeField := testPlanAssetPayload(t, workspaceID, activeDeployment, workspace.AssetTypeField, "sales.sales.orders.status", "semantic_table:sales.sales.orders", fieldPayloadV1{
		Field: "orders.status", Table: "orders", Name: "status", Label: "Status", Type: "string", Expression: "status",
	})
	authoredField := testPlanAssetPayload(t, workspaceID, authoredDeployment, workspace.AssetTypeField, "sales.sales.orders.status", "semantic_table:sales.sales.orders", fieldPayloadV1{
		Field: "orders.status", Table: "orders", Name: "status", Label: "Status", Type: "string", Expression: "coalesce(status, 'unknown')",
	})
	visual := testPlanAsset(t, workspaceID, activeDeployment, workspace.AssetTypeVisual, "sales.executive-sales.status", "dashboard:sales.executive-sales")
	active := workspace.AssetGraph{
		Assets: []workspace.Asset{activeField, visual},
		Edges:  []workspace.AssetEdge{workspace.NewAssetEdge(workspaceID, activeDeployment, visual.ID, activeField.ID, workspace.AssetEdgeUsesField)},
	}
	authored := workspace.AssetGraph{
		Assets: []workspace.Asset{authoredField, visual},
		Edges:  []workspace.AssetEdge{workspace.NewAssetEdge(workspaceID, authoredDeployment, visual.ID, authoredField.ID, workspace.AssetEdgeUsesField)},
	}

	changes, _, summary := diffAssetGraphs(authored, active)
	if !summary.Breaking || len(changes) != 1 || !changes[0].Breaking {
		t.Fatalf("summary = %#v changes=%#v, want breaking expression change", summary, changes)
	}
}

func TestDiffAssetGraphsTreatsUnusedSourceFieldChangeAsNonBreaking(t *testing.T) {
	workspaceID := workspace.WorkspaceID("sales")
	activeDeployment := workspace.ServingStateID("dep_active")
	authoredDeployment := workspace.ServingStateID("plan")
	activeSource := testPlanAssetPayload(t, workspaceID, activeDeployment, workspace.AssetTypeSource, "olist.orders", "catalog:sales", sourcePayloadV1{
		Fields: map[string]sourceFieldPayloadV1{
			"order_id": {Name: "order_id", Type: "string"},
			"unused":   {Name: "unused", Type: "string"},
		},
	})
	authoredSource := testPlanAssetPayload(t, workspaceID, authoredDeployment, workspace.AssetTypeSource, "olist.orders", "catalog:sales", sourcePayloadV1{
		Fields: map[string]sourceFieldPayloadV1{
			"order_id": {Name: "order_id", Type: "string"},
			"unused":   {Name: "unused", Type: "integer"},
		},
	})
	modelTable := testPlanAssetPayload(t, workspaceID, activeDeployment, workspace.AssetTypeModelTable, "sales.orders", "catalog:sales", modelTablePayloadV1{
		SQL: "SELECT order_id FROM source.\"olist.orders\"",
	})
	active := workspace.AssetGraph{
		Assets: []workspace.Asset{activeSource, modelTable},
		Edges:  []workspace.AssetEdge{workspace.NewAssetEdge(workspaceID, activeDeployment, modelTable.ID, activeSource.ID, workspace.AssetEdgeReadsSource)},
	}
	authored := workspace.AssetGraph{
		Assets: []workspace.Asset{authoredSource, modelTable},
		Edges:  []workspace.AssetEdge{workspace.NewAssetEdge(workspaceID, authoredDeployment, modelTable.ID, authoredSource.ID, workspace.AssetEdgeReadsSource)},
	}

	changes, _, summary := diffAssetGraphs(authored, active)
	if summary.Breaking || len(changes) != 1 || changes[0].Breaking {
		t.Fatalf("summary = %#v changes=%#v, want non-breaking unused source field change", summary, changes)
	}
}

func TestDiffAssetGraphsTreatsUsedSourceFieldChangeAsBreaking(t *testing.T) {
	workspaceID := workspace.WorkspaceID("sales")
	activeDeployment := workspace.ServingStateID("dep_active")
	authoredDeployment := workspace.ServingStateID("plan")
	activeSource := testPlanAssetPayload(t, workspaceID, activeDeployment, workspace.AssetTypeSource, "olist.orders", "catalog:sales", sourcePayloadV1{
		Fields: map[string]sourceFieldPayloadV1{
			"order_id": {Name: "order_id", Type: "string"},
		},
	})
	authoredSource := testPlanAssetPayload(t, workspaceID, authoredDeployment, workspace.AssetTypeSource, "olist.orders", "catalog:sales", sourcePayloadV1{
		Fields: map[string]sourceFieldPayloadV1{
			"order_id": {Name: "order_id", Type: "integer"},
		},
	})
	modelTable := testPlanAssetPayload(t, workspaceID, activeDeployment, workspace.AssetTypeModelTable, "sales.orders", "catalog:sales", modelTablePayloadV1{
		SQL: "SELECT order_id FROM source.\"olist.orders\"",
	})
	active := workspace.AssetGraph{
		Assets: []workspace.Asset{activeSource, modelTable},
		Edges:  []workspace.AssetEdge{workspace.NewAssetEdge(workspaceID, activeDeployment, modelTable.ID, activeSource.ID, workspace.AssetEdgeReadsSource)},
	}
	authored := workspace.AssetGraph{
		Assets: []workspace.Asset{authoredSource, modelTable},
		Edges:  []workspace.AssetEdge{workspace.NewAssetEdge(workspaceID, authoredDeployment, modelTable.ID, authoredSource.ID, workspace.AssetEdgeReadsSource)},
	}

	changes, _, summary := diffAssetGraphs(authored, active)
	if !summary.Breaking || len(changes) != 1 || !changes[0].Breaking {
		t.Fatalf("summary = %#v changes=%#v, want breaking used source field change", summary, changes)
	}
}

func TestDiffAssetGraphsIgnoresRuntimeDiscoveredSchema(t *testing.T) {
	workspaceID := workspace.WorkspaceID("sales")
	activeDeployment := workspace.ServingStateID("dep_active")
	authoredDeployment := workspace.ServingStateID("plan")
	nullable := true
	activeSource := testPlanAssetPayload(t, workspaceID, activeDeployment, workspace.AssetTypeSource, "olist.orders", "catalog:sales", sourcePayloadV1{
		Connection: "olist",
		Path:       "orders.csv",
		Schema: schemaPayloadV1{Columns: []schemaColumnPayloadV1{
			{Name: "order_id", Ordinal: 1, PhysicalType: "VARCHAR", Nullable: &nullable},
		}},
	})
	authoredSource := testPlanAssetPayload(t, workspaceID, authoredDeployment, workspace.AssetTypeSource, "olist.orders", "catalog:sales", sourcePayloadV1{
		Connection: "olist",
		Path:       "orders.csv",
	})
	activeModel := testPlanAssetPayload(t, workspaceID, activeDeployment, workspace.AssetTypeModelTable, "sales.orders", "catalog:sales", modelTablePayloadV1{
		SQL: "SELECT order_id FROM source.\"olist.orders\"",
		Schema: schemaPayloadV1{Columns: []schemaColumnPayloadV1{
			{Name: "order_id", Ordinal: 1, PhysicalType: "VARCHAR", Nullable: &nullable},
		}},
	})
	authoredModel := testPlanAssetPayload(t, workspaceID, authoredDeployment, workspace.AssetTypeModelTable, "sales.orders", "catalog:sales", modelTablePayloadV1{
		SQL: "SELECT order_id FROM source.\"olist.orders\"",
	})
	active := workspace.AssetGraph{Assets: []workspace.Asset{activeSource, activeModel}}
	authored := workspace.AssetGraph{Assets: []workspace.Asset{authoredSource, authoredModel}}

	changes, dependencyChanges, summary := diffAssetGraphs(authored, active)
	if len(changes) != 0 || len(dependencyChanges) != 0 || summary != (ProjectPlanSummary{}) {
		t.Fatalf("changes=%#v dependencyChanges=%#v summary=%#v, want no diff", changes, dependencyChanges, summary)
	}
}

func TestDiffAssetGraphsKeepsAuthoredNestedSchemaFieldsSignificant(t *testing.T) {
	workspaceID := workspace.WorkspaceID("sales")
	activeDeployment := workspace.ServingStateID("dep_active")
	authoredDeployment := workspace.ServingStateID("plan")
	activeSource := testPlanAssetPayload(t, workspaceID, activeDeployment, workspace.AssetTypeSource, "olist.orders", "catalog:sales", map[string]any{
		"Connection": "olist",
		"Path":       "orders.csv",
		"Options": map[string]any{
			"Schema": "public",
		},
	})
	authoredSource := testPlanAssetPayload(t, workspaceID, authoredDeployment, workspace.AssetTypeSource, "olist.orders", "catalog:sales", map[string]any{
		"Connection": "olist",
		"Path":       "orders.csv",
		"Options": map[string]any{
			"Schema": "private",
		},
	})

	changes, _, summary := diffAssetGraphs(workspace.AssetGraph{Assets: []workspace.Asset{authoredSource}}, workspace.AssetGraph{Assets: []workspace.Asset{activeSource}})
	if len(changes) != 1 || summary.Changed != 1 {
		t.Fatalf("changes=%#v summary=%#v, want nested authored Schema change", changes, summary)
	}
}

func TestDiffAssetGraphsTreatsUnusedModelColumnChangeAsNonBreaking(t *testing.T) {
	workspaceID := workspace.WorkspaceID("sales")
	activeDeployment := workspace.ServingStateID("dep_active")
	authoredDeployment := workspace.ServingStateID("plan")
	activeModel := testPlanAssetPayload(t, workspaceID, activeDeployment, workspace.AssetTypeModelTable, "sales.orders", "catalog:sales", modelTablePayloadV1{
		Columns: map[string]modelColumnPayloadV1{
			"order_id": {Name: "order_id", Type: "string"},
			"unused":   {Name: "unused", Type: "string"},
		},
	})
	authoredModel := testPlanAssetPayload(t, workspaceID, authoredDeployment, workspace.AssetTypeModelTable, "sales.orders", "catalog:sales", modelTablePayloadV1{
		Columns: map[string]modelColumnPayloadV1{
			"order_id": {Name: "order_id", Type: "string"},
			"unused":   {Name: "unused", Type: "integer"},
		},
	})
	semanticTable := testPlanAsset(t, workspaceID, activeDeployment, workspace.AssetTypeSemanticTable, "sales.sales.orders", "semantic_model:sales.sales")
	active := workspace.AssetGraph{
		Assets: []workspace.Asset{activeModel, semanticTable},
		Edges:  []workspace.AssetEdge{workspace.NewAssetEdge(workspaceID, activeDeployment, semanticTable.ID, activeModel.ID, workspace.AssetEdgeUsesModelTable)},
	}
	authored := workspace.AssetGraph{
		Assets: []workspace.Asset{authoredModel, semanticTable},
		Edges:  []workspace.AssetEdge{workspace.NewAssetEdge(workspaceID, authoredDeployment, semanticTable.ID, authoredModel.ID, workspace.AssetEdgeUsesModelTable)},
	}

	changes, _, summary := diffAssetGraphs(authored, active)
	if summary.Breaking || len(changes) != 1 || changes[0].Breaking {
		t.Fatalf("summary = %#v changes=%#v, want non-breaking unused model column change", summary, changes)
	}
}

func TestDiffAssetGraphsTreatsUsedModelColumnChangeAsBreaking(t *testing.T) {
	workspaceID := workspace.WorkspaceID("sales")
	activeDeployment := workspace.ServingStateID("dep_active")
	authoredDeployment := workspace.ServingStateID("plan")
	activeModel := testPlanAssetPayload(t, workspaceID, activeDeployment, workspace.AssetTypeModelTable, "sales.orders", "catalog:sales", modelTablePayloadV1{
		Columns: map[string]modelColumnPayloadV1{"order_id": {Name: "order_id", Type: "string"}},
	})
	authoredModel := testPlanAssetPayload(t, workspaceID, authoredDeployment, workspace.AssetTypeModelTable, "sales.orders", "catalog:sales", modelTablePayloadV1{
		Columns: map[string]modelColumnPayloadV1{"order_id": {Name: "order_id", Type: "integer"}},
	})
	semanticField := testPlanAssetPayload(t, workspaceID, activeDeployment, workspace.AssetTypeField, "sales.sales.orders.order_id", "semantic_table:sales.sales.orders", fieldPayloadV1{
		Field: "orders.order_id", Table: "orders", Name: "order_id", Type: "string", Expression: "order_id",
	})
	semanticTable := testPlanAsset(t, workspaceID, activeDeployment, workspace.AssetTypeSemanticTable, "sales.sales.orders", "semantic_model:sales.sales")
	active := workspace.AssetGraph{
		Assets: []workspace.Asset{activeModel, semanticTable, semanticField},
		Edges: []workspace.AssetEdge{
			workspace.NewAssetEdge(workspaceID, activeDeployment, semanticTable.ID, activeModel.ID, workspace.AssetEdgeUsesModelTable),
			workspace.NewAssetEdge(workspaceID, activeDeployment, semanticField.ID, semanticTable.ID, workspace.AssetEdgeUsesSemanticTable),
		},
	}
	authored := workspace.AssetGraph{
		Assets: []workspace.Asset{authoredModel, semanticTable, semanticField},
		Edges: []workspace.AssetEdge{
			workspace.NewAssetEdge(workspaceID, authoredDeployment, semanticTable.ID, authoredModel.ID, workspace.AssetEdgeUsesModelTable),
			workspace.NewAssetEdge(workspaceID, authoredDeployment, semanticField.ID, semanticTable.ID, workspace.AssetEdgeUsesSemanticTable),
		},
	}

	changes, _, summary := diffAssetGraphs(authored, active)
	if !summary.Breaking || len(changes) != 1 || !changes[0].Breaking {
		t.Fatalf("summary = %#v changes=%#v, want breaking used model column change", summary, changes)
	}
}

func TestCompileProjectValidatesUnusedGlobalConnectionsAndSources(t *testing.T) {
	t.Run("unused unsupported connection", func(t *testing.T) {
		projectPath := writeProjectFixture(t, minimalProjectFiles(map[string]string{
			"connections/bad.yaml": `
apiVersion: leapview.dev/v1
kind: Connection
metadata:
  name: bad
spec:
  kind: unsupported
`,
		}))

		_, err := CompileProject(projectPath, Options{ServingStateID: "dep_test"})
		assertCompileErrorContains(t, err, `schema.enum`)
		assertDiagnostic(t, err, "connection:bad", "spec")
	})

	t.Run("unused source path outside connection scope", func(t *testing.T) {
		projectPath := writeProjectFixture(t, minimalProjectFiles(map[string]string{
			"connections/scoped.yaml": `
apiVersion: leapview.dev/v1
kind: Connection
metadata:
  name: scoped
spec:
  kind: http
  scope: https://example.com/data/allowed
`,
			"sources/unused.escape.yaml": `
apiVersion: leapview.dev/v1
kind: Source
metadata:
  name: unused.escape
spec:
  connection: scoped
  path: https://example.com/outside.csv
  format: csv
`,
		}))

		_, err := CompileProject(projectPath, Options{ServingStateID: "dep_test"})
		assertCompileErrorContains(t, err, `escapes connection scope`)
		assertDiagnostic(t, err, "source:unused.escape", "spec")
	})
}

func TestCompileProjectSupportsMultipleSemanticModelsInWorkspace(t *testing.T) {
	projectPath := writeProjectFixture(t, map[string]string{
		"leapview.yaml":                                   projectYAML(),
		"connections/olist.yaml":                           connectionYAML("olist"),
		"sources/olist.orders.yaml":                        sourceYAML("olist.orders", "orders.csv", "order_id"),
		"sources/olist.customers.yaml":                     sourceYAML("olist.customers", "customers.csv", "customer_id"),
		"workspaces/sales/workspace.yaml":                  workspaceYAML("sales"),
		"workspaces/sales/models/orders.yaml":              modelTableYAML("sales", "orders", "olist.orders", "order_id", "SELECT order_id, order_status AS status FROM source.\"olist.orders\""),
		"workspaces/sales/semantic-models/sales.yaml":      semanticModelNamedYAML("sales", "sales", "orders", "order_count"),
		"workspaces/sales/semantic-models/finance.yaml":    semanticModelNamedYAML("sales", "finance", "orders", "finance_order_count"),
		"workspaces/sales/dashboards/executive-sales.yaml": dashboardYAML("sales", "executive-sales", "sales"),
	})

	compiled, err := CompileProject(projectPath, Options{ServingStateID: "dep_test"})
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
		"leapview.yaml":                                        projectYAML(),
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

	compiled, err := CompileProject(projectPath, Options{ServingStateID: "dep_test"})
	if err != nil {
		t.Fatalf("CompileProject() error = %v", err)
	}
	assertGraphAsset(t, compiled.Workspaces["sales"].Workspace.Graph, "dashboard:sales.executive-sales")
	assertGraphAsset(t, compiled.Workspaces["operations"].Workspace.Graph, "dashboard:operations.executive-sales")
}

func TestCompileProjectRejectsDuplicateDashboardIDsWithinWorkspace(t *testing.T) {
	projectPath := writeProjectFixture(t, map[string]string{
		"leapview.yaml":                              projectYAML(),
		"connections/olist.yaml":                      connectionYAML("olist"),
		"sources/olist.orders.yaml":                   sourceYAML("olist.orders", "orders.csv", "order_id"),
		"sources/olist.customers.yaml":                sourceYAML("olist.customers", "customers.csv", "customer_id"),
		"workspaces/sales/workspace.yaml":             workspaceYAML("sales"),
		"workspaces/sales/models/orders.yaml":         modelTableYAML("sales", "orders", "olist.orders", "order_id", "SELECT order_id, order_status AS status FROM source.\"olist.orders\""),
		"workspaces/sales/semantic-models/sales.yaml": semanticModelYAML("sales", "orders", "order_count"),
		"workspaces/sales/dashboards/one.yaml":        dashboardYAML("sales", "executive-sales", "sales"),
		"workspaces/sales/dashboards/two.yaml":        dashboardYAML("sales", "executive-sales", "sales"),
	})

	_, err := CompileProject(projectPath, Options{ServingStateID: "dep_test"})
	assertCompileErrorContains(t, err, `duplicate Dashboard "executive-sales"`)
	assertDiagnostic(t, err, "dashboard:sales.executive-sales", "metadata.name")
}

func TestCompileProjectRejectsUnknownReferences(t *testing.T) {
	t.Run("source connection", func(t *testing.T) {
		projectPath := writeProjectFixture(t, map[string]string{
			"leapview.yaml":                                   projectYAML(),
			"connections/olist.yaml":                           connectionYAML("olist"),
			"sources/olist.orders.yaml":                        strings.Replace(sourceYAML("olist.orders", "orders.csv", "order_id"), "connection: olist", "connection: missing", 1),
			"sources/olist.customers.yaml":                     sourceYAML("olist.customers", "customers.csv", "customer_id"),
			"workspaces/sales/workspace.yaml":                  workspaceYAML("sales"),
			"workspaces/sales/models/orders.yaml":              modelTableYAML("sales", "orders", "olist.orders", "order_id", "SELECT order_id, order_status AS status FROM source.\"olist.orders\""),
			"workspaces/sales/semantic-models/sales.yaml":      semanticModelYAML("sales", "orders", "order_count"),
			"workspaces/sales/dashboards/executive-sales.yaml": dashboardYAML("sales", "executive-sales", "sales"),
		})

		_, err := CompileProject(projectPath, Options{ServingStateID: "dep_test"})
		assertCompileErrorContains(t, err, `Source "olist.orders" references unknown Connection "missing"`)
	})

	t.Run("dashboard semantic model", func(t *testing.T) {
		projectPath := writeProjectFixture(t, map[string]string{
			"leapview.yaml":                                   projectYAML(),
			"connections/olist.yaml":                           connectionYAML("olist"),
			"sources/olist.orders.yaml":                        sourceYAML("olist.orders", "orders.csv", "order_id"),
			"sources/olist.customers.yaml":                     sourceYAML("olist.customers", "customers.csv", "customer_id"),
			"workspaces/sales/workspace.yaml":                  workspaceYAML("sales"),
			"workspaces/sales/models/orders.yaml":              modelTableYAML("sales", "orders", "olist.orders", "order_id", "SELECT order_id, order_status AS status FROM source.\"olist.orders\""),
			"workspaces/sales/semantic-models/sales.yaml":      semanticModelYAML("sales", "orders", "order_count"),
			"workspaces/sales/dashboards/executive-sales.yaml": dashboardYAML("sales", "executive-sales", "missing"),
		})

		_, err := CompileProject(projectPath, Options{ServingStateID: "dep_test"})
		assertCompileErrorContains(t, err, `references unknown SemanticModel "missing"`)
	})
}

func TestCompileProjectRejectsInlineConnectionAuthWithResourceDiagnostic(t *testing.T) {
	projectPath := writeProjectFixture(t, map[string]string{
		"leapview.yaml": projectYAML(),
		"connections/olist.yaml": `
apiVersion: leapview.dev/v1
kind: Connection
metadata:
  name: olist
spec:
  kind: managed
  auth:
    token: secret
`,
	})

	_, err := CompileProject(projectPath, Options{ServingStateID: "dep_test"})
	assertCompileErrorContains(t, err, `field not allowed`)
	assertDiagnostic(t, err, "connection:olist", "spec")
}

func TestCompileProjectRejectsWorkspaceMismatchWithResourceDiagnostic(t *testing.T) {
	projectPath := writeProjectFixture(t, map[string]string{
		"leapview.yaml":                                   projectYAML(),
		"connections/olist.yaml":                           connectionYAML("olist"),
		"sources/olist.orders.yaml":                        sourceYAML("olist.orders", "orders.csv", "order_id"),
		"sources/olist.customers.yaml":                     sourceYAML("olist.customers", "customers.csv", "customer_id"),
		"workspaces/sales/workspace.yaml":                  workspaceYAML("sales"),
		"workspaces/sales/models/orders.yaml":              strings.Replace(modelTableYAML("sales", "orders", "olist.orders", "order_id", "SELECT order_id, order_status AS status FROM source.\"olist.orders\""), "workspace: sales", "workspace: operations", 1),
		"workspaces/sales/semantic-models/sales.yaml":      semanticModelYAML("sales", "orders", "order_count"),
		"workspaces/sales/dashboards/executive-sales.yaml": dashboardYAML("sales", "executive-sales", "sales"),
	})

	_, err := CompileProject(projectPath, Options{ServingStateID: "dep_test"})
	assertCompileErrorContains(t, err, `workspace = "operations", want "sales"`)
	assertDiagnostic(t, err, "model_table:operations.orders", "metadata.workspace")
}

func TestCompileProjectRejectsHiddenImportsAndUnsafeIncludes(t *testing.T) {
	t.Run("raw relation", func(t *testing.T) {
		projectPath := writeProjectFixture(t, map[string]string{
			"leapview.yaml":                                   projectYAML(),
			"connections/olist.yaml":                           connectionYAML("olist"),
			"sources/olist.orders.yaml":                        sourceYAML("olist.orders", "orders.csv", "order_id"),
			"sources/olist.customers.yaml":                     sourceYAML("olist.customers", "customers.csv", "customer_id"),
			"workspaces/sales/workspace.yaml":                  workspaceYAML("sales"),
			"workspaces/sales/models/orders.yaml":              modelTableYAML("sales", "orders", "olist.orders", "order_id", "SELECT order_id FROM raw.\"olist.orders\""),
			"workspaces/sales/semantic-models/sales.yaml":      semanticModelYAML("sales", "orders", "order_count"),
			"workspaces/sales/dashboards/executive-sales.yaml": dashboardYAML("sales", "executive-sales", "sales"),
		})

		_, err := CompileProject(projectPath, Options{ServingStateID: "dep_test"})
		assertCompileErrorContains(t, err, "raw.<name> is internal")
	})

	t.Run("unqualified relation", func(t *testing.T) {
		projectPath := writeProjectFixture(t, map[string]string{
			"leapview.yaml":                                   projectYAML(),
			"connections/olist.yaml":                           connectionYAML("olist"),
			"sources/olist.orders.yaml":                        sourceYAML("olist.orders", "orders.csv", "order_id"),
			"sources/olist.customers.yaml":                     sourceYAML("olist.customers", "customers.csv", "customer_id"),
			"workspaces/sales/workspace.yaml":                  workspaceYAML("sales"),
			"workspaces/sales/models/orders.yaml":              modelTableYAML("sales", "orders", "olist.orders", "order_id", "SELECT order_id FROM orders"),
			"workspaces/sales/semantic-models/sales.yaml":      semanticModelYAML("sales", "orders", "order_count"),
			"workspaces/sales/dashboards/executive-sales.yaml": dashboardYAML("sales", "executive-sales", "sales"),
		})

		_, err := CompileProject(projectPath, Options{ServingStateID: "dep_test"})
		assertCompileErrorContains(t, err, "found unqualified relation")
	})

	t.Run("escaping include", func(t *testing.T) {
		projectPath := writeProjectFixture(t, map[string]string{
			"leapview.yaml":         strings.Replace(projectYAML(), "connections/*.yaml", "../*.yaml", 1),
			"connections/olist.yaml": connectionYAML("olist"),
		})

		_, err := CompileProject(projectPath, Options{ServingStateID: "dep_test"})
		assertCompileErrorContains(t, err, "escapes project boundary")
	})

	t.Run("recursive include", func(t *testing.T) {
		projectPath := writeProjectFixture(t, map[string]string{
			"leapview.yaml":         strings.Replace(projectYAML(), "connections/*.yaml", "connections/**/*.yaml", 1),
			"connections/olist.yaml": connectionYAML("olist"),
		})

		_, err := CompileProject(projectPath, Options{ServingStateID: "dep_test"})
		assertCompileErrorContains(t, err, "unsupported ** glob")
	})
}

func TestCompileProjectRejectsSQLSourceMismatchAndCycles(t *testing.T) {
	t.Run("source mismatch", func(t *testing.T) {
		projectPath := writeProjectFixture(t, map[string]string{
			"leapview.yaml":                                   projectYAML(),
			"connections/olist.yaml":                           connectionYAML("olist"),
			"sources/olist.orders.yaml":                        sourceYAML("olist.orders", "orders.csv", "order_id"),
			"sources/olist.customers.yaml":                     sourceYAML("olist.customers", "customers.csv", "customer_id"),
			"workspaces/sales/workspace.yaml":                  workspaceYAML("sales"),
			"workspaces/sales/models/orders.yaml":              modelTableYAML("sales", "orders", "olist.orders", "order_id", "SELECT customer_id AS order_id FROM source.\"olist.customers\""),
			"workspaces/sales/semantic-models/sales.yaml":      semanticModelYAML("sales", "orders", "order_count"),
			"workspaces/sales/dashboards/executive-sales.yaml": dashboardYAML("sales", "executive-sales", "sales"),
		})

		_, err := CompileProject(projectPath, Options{ServingStateID: "dep_test"})
		assertCompileErrorContains(t, err, "SQL source references")
	})

	t.Run("model table cycle", func(t *testing.T) {
		projectPath := writeProjectFixture(t, map[string]string{
			"leapview.yaml":                                   projectYAML(),
			"connections/olist.yaml":                           connectionYAML("olist"),
			"sources/olist.orders.yaml":                        sourceYAML("olist.orders", "orders.csv", "order_id"),
			"sources/olist.customers.yaml":                     sourceYAML("olist.customers", "customers.csv", "customer_id"),
			"workspaces/sales/workspace.yaml":                  workspaceYAML("sales"),
			"workspaces/sales/models/orders.yaml":              sqlModelTableYAML("sales", "orders", "order_id", "SELECT order_id, status FROM model.customers"),
			"workspaces/sales/models/customers.yaml":           sqlModelTableYAML("sales", "customers", "customer_id", "SELECT customer_id, status FROM model.orders"),
			"workspaces/sales/semantic-models/sales.yaml":      semanticModelYAMLWithTables("sales", []string{"orders", "customers"}, "order_count"),
			"workspaces/sales/dashboards/executive-sales.yaml": dashboardYAML("sales", "executive-sales", "sales"),
		})

		_, err := CompileProject(projectPath, Options{ServingStateID: "dep_test"})
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
	return filepath.Join(dir, "leapview.yaml")
}

func assertVisualShowcaseCoverage(t *testing.T, report *reportdef.Dashboard) {
	t.Helper()
	visualTypes := map[string]struct{}{}
	for _, visual := range report.Visuals {
		if visual.Type != "" {
			visualTypes[visual.Type] = struct{}{}
		}
		if visual.Kind != "" {
			visualTypes[visual.Kind] = struct{}{}
		}
	}
	for _, typ := range []string{
		"line", "area", "bar", "column", "pie", "donut", "scatter", "funnel", "treemap", "gauge", "heatmap",
		"sankey", "graph", "map", "candlestick", "boxplot", "combo", "waterfall", "histogram", "radar", "tree", "sunburst",
	} {
		if _, ok := visualTypes[typ]; !ok {
			t.Fatalf("visual-showcase missing visual type %q", typ)
		}
	}
	tableKinds := map[string]struct{}{}
	conditionalFormatting := map[string]struct{}{}
	for _, table := range report.Tables {
		tableKinds[table.Kind] = struct{}{}
		for _, column := range table.Columns {
			for _, rule := range column.Formatting {
				conditionalFormatting[rule.Kind] = struct{}{}
			}
		}
		for _, rules := range table.MeasureFormatting {
			for _, rule := range rules {
				conditionalFormatting[rule.Kind] = struct{}{}
			}
		}
	}
	for _, kind := range []string{"data_table", "matrix_table", "pivot_table"} {
		if _, ok := tableKinds[kind]; !ok {
			t.Fatalf("visual-showcase missing table kind %q", kind)
		}
	}
	for _, kind := range []string{"badge", "data_bar", "text_color", "background_scale"} {
		if _, ok := conditionalFormatting[kind]; !ok {
			t.Fatalf("visual-showcase missing conditional formatting kind %q", kind)
		}
	}
}

func minimalProjectFiles(extra map[string]string) map[string]string {
	files := map[string]string{
		"leapview.yaml":                                   projectYAML(),
		"connections/olist.yaml":                           connectionYAML("olist"),
		"sources/olist.orders.yaml":                        sourceYAML("olist.orders", "orders.csv", "order_id"),
		"sources/olist.customers.yaml":                     sourceYAML("olist.customers", "customers.csv", "customer_id"),
		"workspaces/sales/workspace.yaml":                  workspaceYAML("sales"),
		"workspaces/sales/models/orders.yaml":              modelTableYAML("sales", "orders", "olist.orders", "order_id", "SELECT order_id, order_status AS status FROM source.\"olist.orders\""),
		"workspaces/sales/semantic-models/sales.yaml":      semanticModelYAML("sales", "orders", "order_count"),
		"workspaces/sales/dashboards/executive-sales.yaml": dashboardYAML("sales", "executive-sales", "sales"),
	}
	for path, content := range extra {
		files[path] = content
	}
	return files
}

func testPlanAsset(t *testing.T, workspaceID workspace.WorkspaceID, servingStateID workspace.ServingStateID, typ workspace.AssetType, key string, parent workspace.AssetID) workspace.Asset {
	t.Helper()
	asset, err := workspace.NewAsset(workspaceID, servingStateID, typ, key, parent, key, "", workspace.PayloadSchemaForAssetType(typ), map[string]any{"key": key})
	if err != nil {
		t.Fatal(err)
	}
	return asset
}

func testPlanAssetPayload(t *testing.T, workspaceID workspace.WorkspaceID, servingStateID workspace.ServingStateID, typ workspace.AssetType, key string, parent workspace.AssetID, payload any) workspace.Asset {
	t.Helper()
	asset, err := workspace.NewAsset(workspaceID, servingStateID, typ, key, parent, key, "", workspace.PayloadSchemaForAssetType(typ), payload)
	if err != nil {
		t.Fatal(err)
	}
	return asset
}

func connectionYAML(name string) string {
	return `
apiVersion: leapview.dev/v1
kind: Connection
metadata:
  name: ` + name + `
spec:
  kind: managed
`
}

func projectYAML() string {
	return `
apiVersion: leapview.dev/v1
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
apiVersion: leapview.dev/v1
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
apiVersion: leapview.dev/v1
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
  access:
    include: []
`
}

func workspaceYAMLWithRefreshPipelines(name string) string {
	return strings.Replace(workspaceYAML(name), "  dashboards:\n", "  refreshPipelines:\n    include:\n      - refresh-pipelines/*.yaml\n  dashboards:\n", 1)
}

func refreshPipelineYAML(workspaceID, name, semanticModel string, schedules []string) string {
	content := `
apiVersion: leapview.dev/v1
kind: RefreshPipeline
metadata:
  workspace: ` + workspaceID + `
  name: ` + name + `
spec:
  semanticModel: ` + semanticModel + `
`
	if schedules == nil {
		return content
	}
	content += "  on:\n    schedule:\n"
	for _, item := range schedules {
		cron, timezone, _ := strings.Cut(item, "|")
		content += "      - cron: " + quoteYAML(cron) + "\n"
		if timezone != "" {
			content += "        timezone: " + timezone + "\n"
		}
	}
	return content
}

func quoteYAML(value string) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return string(encoded)
}

func workspaceYAMLWithAccess(name string) string {
	return strings.Replace(workspaceYAML(name), "include: []", "include:\n      - access/*.yaml", 1)
}

func modelTableYAML(workspace, name, source, key, sql string) string {
	return `
apiVersion: leapview.dev/v1
kind: ModelTable
metadata:
  workspace: ` + workspace + `
  name: ` + name + `
spec:
  primaryKey: ` + key + `
  sources:
    - ` + source + `
  fields:
    ` + key + `:
      label: ID
      type: string
  transform:
    sql: |
      ` + sql + `
`
}

func sqlModelTableYAML(workspace, name, key, sql string) string {
	return `
apiVersion: leapview.dev/v1
kind: ModelTable
metadata:
  workspace: ` + workspace + `
  name: ` + name + `
spec:
  primaryKey: ` + key + `
  fields:
    ` + key + `:
      label: ID
      type: string
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
apiVersion: leapview.dev/v1
kind: SemanticModel
metadata:
  workspace: ` + workspace + `
  name: ` + name + `
spec:
  tables:
    - ` + table + `
  measures:
    ` + measure + `:
      fact: ` + table + `
      aggregation: count
      empty: zero
`
}

func semanticModelYAMLWithTables(workspace string, tables []string, measure string) string {
	return `
apiVersion: leapview.dev/v1
kind: SemanticModel
metadata:
  workspace: ` + workspace + `
  name: ` + workspace + `
spec:
  tables:
` + semanticTableListYAML(tables) + `  measures:
    ` + measure + `:
      fact: ` + tables[0] + `
      aggregation: count
      empty: zero
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
apiVersion: leapview.dev/v1
kind: Dashboard
metadata:
  workspace: ` + workspace + `
  name: ` + name + `
  title: ` + name + `
spec:
  semanticModel: ` + model + `
  visuals:
    total:
      type: kpi
      query:
        measures:
          order_count:
  pages:
    - id: overview
      title: Overview
      components:
        - id: total
          kind: visual
          visual: total
          placement:
            col: 1
            row: 1
            col_span: 3
            row_span: 2
`
}

func workspaceGroupYAML(workspace, name, email string) string {
	return `
apiVersion: leapview.dev/v1
kind: WorkspaceGroup
metadata:
  workspace: ` + workspace + `
  name: ` + name + `
spec:
  members:
    - email: ` + email + `
`
}

func workspaceRoleBindingGroupYAML(workspace, name, role, group string) string {
	return `
apiVersion: leapview.dev/v1
kind: WorkspaceRoleBinding
metadata:
  workspace: ` + workspace + `
  name: ` + name + `
spec:
  role: ` + role + `
  subject:
    kind: group
    group: ` + group + `
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

func assertDiagnostic(t *testing.T, err error, resourceID, fieldPath string) {
	t.Helper()
	diagnostics := configschema.Diagnostics(err)
	if len(diagnostics) == 0 {
		t.Fatalf("diagnostics empty, want resource=%q field=%q", resourceID, fieldPath)
	}
	diagnostic := diagnostics[0]
	if diagnostic.File == "" || diagnostic.ResourceID != resourceID || diagnostic.FieldPath != fieldPath {
		t.Fatalf("diagnostic = %#v, want file, resource=%q, field=%q", diagnostic, resourceID, fieldPath)
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

func assertAssetSourceFileContains(t *testing.T, graph workspace.AssetGraph, id, want string) {
	t.Helper()
	for _, asset := range graph.Assets {
		if string(asset.ID) != id {
			continue
		}
		if !strings.Contains(filepath.ToSlash(asset.SourceFile), filepath.ToSlash(want)) {
			t.Fatalf("asset %q sourceFile = %q, want contains %q", id, asset.SourceFile, want)
		}
		return
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

func unmarshalGraphPayload(t *testing.T, graph workspace.AssetGraph, id string, out any) {
	t.Helper()
	for _, asset := range graph.Assets {
		if string(asset.ID) != id {
			continue
		}
		if err := json.Unmarshal([]byte(asset.PayloadJSON), out); err != nil {
			t.Fatalf("unmarshal payload %q: %v", id, err)
		}
		return
	}
	t.Fatalf("asset %q missing from graph", id)
}
