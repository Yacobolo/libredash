package configschema

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"gopkg.in/yaml.v3"
)

func TestValidateBytesRejectsUnknownEnvelopeField(t *testing.T) {
	err := ValidateBytes(KindProject, "leapview.yaml", []byte(`
apiVersion: leapview.dev/v1
kind: Project
metadata:
  name: test
spec:
  connections:
    include: [connections/*.yaml]
  sources:
    include: [sources/*.yaml]
  workspaces:
    include: [workspaces/*/workspace.yaml]
surprise: true
`))
	assertDiagnostic(t, err, "schema.unknown_field", "field not allowed")
}

func TestValidateBytesRejectsRemovedWorkspaceAgentPolicyInclude(t *testing.T) {
	err := ValidateBytes(KindWorkspace, "workspace.yaml", []byte(`
apiVersion: leapview.dev/v1
kind: Workspace
metadata:
  name: sales
spec:
  uses:
    sources: []
  models:
    include: []
  semanticModels:
    include: []
  dashboards:
    include: []
  agentPolicy:
    include: [agent/*.yaml]
`))
	assertDiagnostic(t, err, "schema.unknown_field", "agentPolicy")
}

func TestValidateBytesRejectsWrongEnvelopeType(t *testing.T) {
	err := ValidateBytes(KindWorkspace, "workspace.yaml", []byte(`
apiVersion: leapview.dev/v1
kind: Workspace
metadata:
  name: sales
spec:
  uses:
    sources: olist.orders
  models:
    include: [models/*.yaml]
  semanticModels:
    include: [semantic-models/*.yaml]
  dashboards:
    include: [dashboards/*.yaml]
`))
	assertDiagnostic(t, err, "schema.type", "mismatched types")
}

func TestValidateBytesRejectsUnsupportedEnum(t *testing.T) {
	err := ValidateBytes(KindDashboardResource, "dashboard.yaml", []byte(`
apiVersion: leapview.dev/v1
kind: Dashboard
metadata:
  name: sales
spec:
  semanticModel: sales
  visuals:
    revenue:
      type: volcano
      query:
        measures:
          revenue:
  pages:
    - id: overview
      title: Overview
      visuals: []
`))
	assertDiagnostic(t, err, "schema.enum", "type")
}

func TestDashboardVisualContractUnifiesChartsAndTables(t *testing.T) {
	err := ValidateBytes(KindDashboardResource, "dashboard.yaml", []byte(`
apiVersion: leapview.dev/v1
kind: Dashboard
metadata:
  name: sales
spec:
  semanticModel: sales
  filters:
    state:
      label: State
      field: customers.state
      predicates:
        - kind: set
          operators: [in, not_in]
      options: {kind: distinct, limit: 50}
  filter_bindings:
    state:
      filter: state
      targets:
        include: [overview/revenue]
      default: {kind: unfiltered}
  visuals:
    revenue:
      type: line
      title: Revenue
      query:
        dimensions: [orders.purchase_month]
        measures: [revenue]
    total:
      type: kpi
      query:
        measures: [revenue]
    orders:
      type: table
      title: Orders
      cardinality: bounded
      query:
        table: orders
        fields: [orders.order_id, orders.revenue]
    state_status:
      type: matrix
      title: State status
      query:
        rows: [customers.state]
        columns: [orders.status]
        measures: [order_count]
    category_status:
      type: pivot
      title: Category status
      query:
        rows: [orders.category]
        columns: [orders.status]
        measures: [order_count]
  pages:
    - id: overview
      title: Overview
      components:
        - id: revenue
          kind: visual
          visual: revenue
          placement: {col: 1, row: 1, col_span: 6, row_span: 4}
        - id: state
          kind: slicer
          binding: {scope: report, id: state}
          presentation: {style: dropdown}
          placement: {col: 7, row: 1, col_span: 3, row_span: 2}
        - id: heading
          kind: header
          title: Sales
          placement: {col: 1, row: 5, col_span: 12, row_span: 1}
`))
	if err != nil {
		t.Fatalf("ValidateBytes() error = %v", err)
	}
}

func TestDashboardVisualContractRejectsLegacyChartTableSplit(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{
			name: "dashboard tables",
			body: `
  tables:
    orders:
      kind: data_table
      title: Orders
      query: {table: orders, fields: [orders.order_id]}
`,
		},
		{
			name: "visual kind",
			body: `
  visuals:
    total:
      kind: kpi
      query: {measures: [revenue]}
`,
		},
		{
			name: "page visuals",
			body: `
  pages:
    - id: overview
      title: Overview
      visuals: []
`,
		},
		{
			name: "page name",
			body: `
  pages:
    - name: overview
      title: Overview
      components: []
`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content := `
apiVersion: leapview.dev/v1
kind: Dashboard
metadata:
  name: sales
spec:
  semanticModel: sales
  visuals:
    revenue:
      type: line
      title: Revenue
      query: {dimensions: [orders.status], measures: [revenue]}
  pages:
    - id: overview
      title: Overview
      components: []
` + tt.body
			if err := ValidateBytes(KindDashboardResource, "dashboard.yaml", []byte(content)); err == nil {
				t.Fatal("ValidateBytes() unexpectedly accepted legacy dashboard syntax")
			}
		})
	}
}

func TestValidateBytesRejectsRemovedLocalConnectionKind(t *testing.T) {
	err := ValidateBytes(KindConnection, "local.yaml", []byte(`
apiVersion: leapview.dev/v1
kind: Connection
metadata:
  name: files
spec:
  kind: local
`))
	assertDiagnostic(t, err, "schema.enum", "local")
}

func TestValidateBytesRejectsInvalidIdentifierKey(t *testing.T) {
	err := ValidateBytes(KindModelTable, "orders.yaml", []byte(`
apiVersion: leapview.dev/v1
kind: ModelTable
metadata:
  name: orders
spec:
  primaryKey: order_id
  fields:
    invalid-name:
      label: Invalid
`))
	assertDiagnostic(t, err, "schema.unknown_field", "invalid-name")
}

func TestValidateBytesRejectsMissingRequiredRootFields(t *testing.T) {
	tests := []struct {
		name     string
		kind     Kind
		content  string
		contains string
	}{
		{
			name: "project spec",
			kind: KindProject,
			content: `
apiVersion: leapview.dev/v1
kind: Project
metadata:
  name: test
`,
			contains: "spec",
		},
		{
			name: "workspace uses",
			kind: KindWorkspace,
			content: `
apiVersion: leapview.dev/v1
kind: Workspace
metadata:
  name: sales
spec:
  models:
    include: [models/*.yaml]
  semanticModels:
    include: [semantic-models/*.yaml]
  dashboards:
    include: [dashboards/*.yaml]
`,
			contains: "uses",
		},
		{
			name: "dashboard semantic model",
			kind: KindDashboardResource,
			content: `
apiVersion: leapview.dev/v1
kind: Dashboard
metadata:
  name: sales
spec:
  visuals: {}
  pages: []
`,
			contains: "semanticModel",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateBytes(tt.kind, tt.name+".yaml", []byte(tt.content))
			assertDiagnosticMessage(t, err, "schema.contract", tt.contains)
		})
	}
}

func TestValidateFileAcceptsShowcaseResources(t *testing.T) {
	root := filepath.Join("..", "..", "dashboards")
	files, err := filepath.Glob(filepath.Join(root, "**", "*.yaml"))
	if err == nil && len(files) == 0 {
		err = filepath.SkipAll
	}
	if err != nil {
		files = explicitShowcaseResourceFiles(root)
	}
	for _, path := range files {
		kind, ok := kindForResourceFile(t, path)
		if !ok {
			continue
		}
		t.Run(path, func(t *testing.T) {
			if err := ValidateFile(kind, path); err != nil {
				t.Fatalf("ValidateFile() error = %v", err)
			}
		})
	}
}

func TestGeneratedJSONSchemasRejectInvalidDocuments(t *testing.T) {
	tests := []struct {
		name     string
		kind     Kind
		instance any
	}{
		{
			name: "project missing spec",
			kind: KindProject,
			instance: map[string]any{
				"apiVersion": "leapview.dev/v1",
				"kind":       "Project",
				"metadata":   map[string]any{"name": "test"},
			},
		},
		{
			name: "workspace missing uses",
			kind: KindWorkspace,
			instance: map[string]any{
				"apiVersion": "leapview.dev/v1",
				"kind":       "Workspace",
				"metadata":   map[string]any{"name": "sales"},
				"spec": map[string]any{
					"models":         map[string]any{"include": []any{"models/*.yaml"}},
					"semanticModels": map[string]any{"include": []any{"semantic-models/*.yaml"}},
					"dashboards":     map[string]any{"include": []any{"dashboards/*.yaml"}},
				},
			},
		},
		{
			name: "model table missing primary key",
			kind: KindModelTable,
			instance: map[string]any{
				"apiVersion": "leapview.dev/v1",
				"kind":       "ModelTable",
				"metadata":   map[string]any{"name": "orders"},
				"spec":       map[string]any{},
			},
		},
		{
			name: "dashboard empty pages",
			kind: KindDashboardResource,
			instance: map[string]any{
				"apiVersion": "leapview.dev/v1",
				"kind":       "Dashboard",
				"metadata":   map[string]any{"name": "sales"},
				"spec": map[string]any{
					"semanticModel": "sales",
					"visuals":       map[string]any{"revenue": map[string]any{"query": map[string]any{}}},
					"pages":         []any{},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			schema := compileGeneratedSchema(t, tt.kind)
			if err := schema.Validate(tt.instance); err == nil {
				t.Fatal("generated JSON Schema accepted invalid document")
			}
		})
	}
}

func TestJSONSchemaFilesAreFresh(t *testing.T) {
	files, err := JSONSchemaFiles()
	if err != nil {
		t.Fatalf("JSONSchemaFiles() error = %v", err)
	}
	for name, content := range files {
		path := filepath.Join("..", "..", "schemas", "json", name)
		onDisk, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read generated schema %s: %v", name, err)
		}
		if string(onDisk) != string(content) {
			t.Fatalf("%s is stale; run leapview schema export --format json-schema --out schemas/json", path)
		}
	}
}

func explicitShowcaseResourceFiles(root string) []string {
	return []string{
		filepath.Join(root, "leapview.yaml"),
		filepath.Join(root, "connections", "olist.yaml"),
		filepath.Join(root, "sources", "olist.customers.yaml"),
		filepath.Join(root, "sources", "olist.order_items.yaml"),
		filepath.Join(root, "sources", "olist.orders.yaml"),
		filepath.Join(root, "sources", "olist.payments.yaml"),
		filepath.Join(root, "sources", "olist.products.yaml"),
		filepath.Join(root, "sources", "olist.reviews.yaml"),
		filepath.Join(root, "sources", "olist.translations.yaml"),
		filepath.Join(root, "workspaces", "sales", "workspace.yaml"),
		filepath.Join(root, "workspaces", "sales", "agent", "default.yaml"),
		filepath.Join(root, "workspaces", "sales", "models", "customers.yaml"),
		filepath.Join(root, "workspaces", "sales", "models", "orders.yaml"),
		filepath.Join(root, "workspaces", "sales", "semantic-models", "sales.yaml"),
		filepath.Join(root, "workspaces", "sales", "dashboards", "executive-sales.yaml"),
		filepath.Join(root, "workspaces", "operations", "workspace.yaml"),
		filepath.Join(root, "workspaces", "operations", "agent", "default.yaml"),
		filepath.Join(root, "workspaces", "operations", "models", "customers.yaml"),
		filepath.Join(root, "workspaces", "operations", "models", "orders.yaml"),
		filepath.Join(root, "workspaces", "operations", "semantic-models", "operations.yaml"),
		filepath.Join(root, "workspaces", "operations", "dashboards", "fulfillment-operations.yaml"),
	}
}

func kindForResourceFile(t *testing.T, path string) (Kind, bool) {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var header struct {
		APIVersion string `yaml:"apiVersion"`
		Kind       string `yaml:"kind"`
	}
	if err := yaml.Unmarshal(content, &header); err != nil {
		t.Fatal(err)
	}
	if header.APIVersion != "leapview.dev/v1" {
		return "", false
	}
	switch header.Kind {
	case "Project":
		return KindProject, true
	case "Connection":
		return KindConnection, true
	case "Source":
		return KindSource, true
	case "Workspace":
		return KindWorkspace, true
	case "WorkspaceGroup":
		return KindWorkspaceGroup, true
	case "WorkspaceRoleBinding":
		return KindWorkspaceRoleBinding, true
	case "ModelTable":
		return KindModelTable, true
	case "SemanticModel":
		return KindSemanticModelResource, true
	case "Dashboard":
		return KindDashboardResource, true
	case "DashboardPublication":
		return KindDashboardPublication, true
	default:
		return "", false
	}
}

func compileGeneratedSchema(t *testing.T, kind Kind) *jsonschema.Schema {
	t.Helper()
	content, err := JSONSchema(kind)
	if err != nil {
		t.Fatalf("JSONSchema(%s): %v", kind, err)
	}
	document, err := jsonschema.UnmarshalJSON(bytes.NewReader(content))
	if err != nil {
		t.Fatalf("unmarshal JSON Schema: %v", err)
	}
	compiler := jsonschema.NewCompiler()
	location := fmt.Sprintf("memory://%s.schema.json", kind)
	if err := compiler.AddResource(location, document); err != nil {
		t.Fatalf("add schema resource: %v", err)
	}
	schema, err := compiler.Compile(location)
	if err != nil {
		t.Fatalf("compile schema: %v", err)
	}
	return schema
}

func assertDiagnostic(t *testing.T, err error, code, contains string) {
	t.Helper()
	got := assertDiagnosticMessage(t, err, code, contains)
	if got.File == "" || got.Line == 0 || got.Column == 0 {
		t.Fatalf("diagnostic lacks source position: %#v", got)
	}
}

func assertDiagnosticMessage(t *testing.T, err error, code, contains string) Diagnostic {
	t.Helper()
	if err == nil {
		t.Fatalf("ValidateBytes() error = nil, want %s", code)
	}
	var schemaErr *Error
	if !errors.As(err, &schemaErr) {
		t.Fatalf("error type = %T, want *Error: %v", err, err)
	}
	if len(schemaErr.Diagnostics) == 0 {
		t.Fatal("diagnostics empty")
	}
	got := schemaErr.Diagnostics[0]
	if got.Code != code {
		t.Fatalf("diagnostic code = %q, want %q: %#v", got.Code, code, schemaErr.Diagnostics)
	}
	if !strings.Contains(got.Message, contains) {
		t.Fatalf("diagnostic message = %q, want containing %q", got.Message, contains)
	}
	return got
}
