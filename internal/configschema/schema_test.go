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
)

func TestValidateBytesRejectsUnknownField(t *testing.T) {
	err := ValidateBytes(KindCatalog, "catalog.yaml", []byte(`
workspace:
  id: libredash
semantic_models:
  - id: olist
    title: Olist
    path: model.yaml
dashboards: []
surprise: true
`))
	assertDiagnostic(t, err, "schema.unknown_field", "field not allowed")
}

func TestValidateBytesRejectsWrongType(t *testing.T) {
	err := ValidateBytes(KindCatalog, "catalog.yaml", []byte(`
semantic_models:
  - id: 12
    title: Olist
    path: model.yaml
dashboards: []
`))
	assertDiagnostic(t, err, "schema.type", "mismatched types")
}

func TestValidateBytesRejectsUnsupportedEnum(t *testing.T) {
	err := ValidateBytes(KindDashboard, "dashboard.yaml", []byte(`
id: sales
title: Sales
semantic_model: olist
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

func TestValidateBytesRejectsInvalidIdentifierKey(t *testing.T) {
	err := ValidateBytes(KindSemanticModel, "model.yaml", []byte(`
name: olist
sources:
  invalid-name:
    connection: olist
    path: orders.csv
models:
  orders:
    source: invalid-name
    primary_key: order_id
semantic_models:
  olist:
    base_table: orders
    tables: [orders]
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
			name: "catalog semantic models",
			kind: KindCatalog,
			content: `
dashboards: []
`,
			contains: "semantic_models",
		},
		{
			name: "semantic model sources",
			kind: KindSemanticModel,
			content: `
name: olist
models: {}
semantic_models: {}
`,
			contains: "sources",
		},
		{
			name: "dashboard semantic model",
			kind: KindDashboard,
			content: `
id: sales
title: Sales
visuals: {}
pages: []
`,
			contains: "semantic_model",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateBytes(tt.kind, tt.name+".yaml", []byte(tt.content))
			assertDiagnosticMessage(t, err, "schema.contract", tt.contains)
		})
	}
}

func TestValidateFileAcceptsOlistContracts(t *testing.T) {
	root := filepath.Join("..", "..")
	tests := []struct {
		kind Kind
		path string
	}{
		{KindCatalog, filepath.Join(root, "dashboards", "catalog.yaml")},
		{KindSemanticModel, filepath.Join(root, "dashboards", "olist", "model.yaml")},
		{KindDashboard, filepath.Join(root, "dashboards", "olist", "executive-sales.yaml")},
	}
	for _, tt := range tests {
		t.Run(string(tt.kind), func(t *testing.T) {
			if err := ValidateFile(tt.kind, tt.path); err != nil {
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
			name: "catalog missing semantic models",
			kind: KindCatalog,
			instance: map[string]any{
				"dashboards": []any{},
			},
		},
		{
			name: "catalog empty dashboards",
			kind: KindCatalog,
			instance: map[string]any{
				"semantic_models": []any{map[string]any{"id": "olist", "title": "Olist", "path": "model.yaml"}},
				"dashboards":      []any{},
			},
		},
		{
			name: "semantic model missing sources",
			kind: KindSemanticModel,
			instance: map[string]any{
				"name":            "olist",
				"models":          map[string]any{"orders": map[string]any{"primary_key": "order_id"}},
				"semantic_models": map[string]any{"olist": map[string]any{"base_table": "orders", "tables": []any{"orders"}}},
			},
		},
		{
			name: "semantic model invalid source key",
			kind: KindSemanticModel,
			instance: map[string]any{
				"name": "olist",
				"sources": map[string]any{
					"invalid-name": map[string]any{"path": "orders.csv"},
				},
				"models":          map[string]any{"orders": map[string]any{"primary_key": "order_id"}},
				"semantic_models": map[string]any{"olist": map[string]any{"base_table": "orders", "tables": []any{"orders"}}},
			},
		},
		{
			name: "dashboard missing semantic model",
			kind: KindDashboard,
			instance: map[string]any{
				"id":      "sales",
				"title":   "Sales",
				"visuals": map[string]any{"revenue": map[string]any{"query": map[string]any{}}},
				"pages":   []any{map[string]any{"id": "overview", "title": "Overview", "visuals": []any{}}},
			},
		},
		{
			name: "dashboard empty visuals",
			kind: KindDashboard,
			instance: map[string]any{
				"id":             "sales",
				"title":          "Sales",
				"semantic_model": "olist",
				"visuals":        map[string]any{},
				"pages":          []any{map[string]any{"id": "overview", "title": "Overview", "visuals": []any{}}},
			},
		},
		{
			name: "dashboard invalid visual key",
			kind: KindDashboard,
			instance: map[string]any{
				"id":             "sales",
				"title":          "Sales",
				"semantic_model": "olist",
				"visuals": map[string]any{
					"bad-visual": map[string]any{"query": map[string]any{}},
				},
				"pages": []any{map[string]any{"id": "overview", "title": "Overview", "visuals": []any{}}},
			},
		},
		{
			name: "dashboard empty pages",
			kind: KindDashboard,
			instance: map[string]any{
				"id":             "sales",
				"title":          "Sales",
				"semantic_model": "olist",
				"visuals":        map[string]any{"revenue": map[string]any{"query": map[string]any{}}},
				"pages":          []any{},
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
			t.Fatalf("%s is stale; run libredash schema export --format json-schema --out schemas/json", path)
		}
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
