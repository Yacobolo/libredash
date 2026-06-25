package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Yacobolo/libredash/internal/configschema"
)

func TestRunValidateAcceptsValidCatalog(t *testing.T) {
	catalog := writeValidateWorkspace(t, validValidateDashboardYAML())
	var out bytes.Buffer
	err := runValidate(context.Background(), &rootOptions{catalog: catalog}, &out)
	if err != nil {
		t.Fatalf("runValidate() error = %v", err)
	}
	if !strings.Contains(out.String(), "ok ") {
		t.Fatalf("output = %q, want ok", out.String())
	}
}

func TestRunValidateReportsSchemaDiagnostic(t *testing.T) {
	catalog := writeValidateWorkspace(t, strings.Replace(validValidateDashboardYAML(), "visuals:\n", "unexpected: true\nvisuals:\n", 1))
	var out bytes.Buffer
	err := runValidate(context.Background(), &rootOptions{catalog: catalog}, &out)
	if err == nil {
		t.Fatal("runValidate() error = nil, want validation failure")
	}
	output := out.String()
	if !strings.Contains(output, "schema.unknown_field") || !strings.Contains(output, "unexpected") {
		t.Fatalf("output = %q, want unknown-field diagnostic", output)
	}
}

func TestRunValidateJSONReportsCompilerDiagnostic(t *testing.T) {
	catalog := writeValidateWorkspace(t, strings.Replace(validValidateDashboardYAML(), "orders.status", "orders.missing", 1))
	var out bytes.Buffer
	err := runValidate(context.Background(), &rootOptions{catalog: catalog, jsonOutput: true}, &out)
	if err == nil {
		t.Fatal("runValidate() error = nil, want validation failure")
	}
	var response validateResponse
	if err := json.Unmarshal(out.Bytes(), &response); err != nil {
		t.Fatalf("json output: %v\n%s", err, out.String())
	}
	if response.OK {
		t.Fatalf("response.OK = true, want false")
	}
	if len(response.Diagnostics) != 1 {
		t.Fatalf("diagnostics len = %d, want 1", len(response.Diagnostics))
	}
	if response.Diagnostics[0].Code != "compiler.reference" {
		t.Fatalf("diagnostic code = %q, want compiler.reference", response.Diagnostics[0].Code)
	}
}

func TestRunValidateCollectsReferencedFileSchemaDiagnostics(t *testing.T) {
	dir := t.TempDir()
	writeValidateFixture(t, filepath.Join(dir, "catalog.yaml"), `
semantic_models:
  - id: bad_model
    title: Bad Model
    path: model.yaml
dashboards:
  - id: bad_dashboard
    title: Bad Dashboard
    path: dashboard.yaml
`)
	writeValidateFixture(t, filepath.Join(dir, "model.yaml"), `
name: bad_model
sources: {}
models: {}
semantic_models: {}
`)
	writeValidateFixture(t, filepath.Join(dir, "dashboard.yaml"), `
id: bad_dashboard
title: Bad Dashboard
semantic_model: bad_model
visuals: {}
pages: []
`)
	var out bytes.Buffer
	err := runValidate(context.Background(), &rootOptions{catalog: filepath.Join(dir, "catalog.yaml"), jsonOutput: true}, &out)
	if err == nil {
		t.Fatal("runValidate() error = nil, want validation failure")
	}
	var response validateResponse
	if err := json.Unmarshal(out.Bytes(), &response); err != nil {
		t.Fatalf("json output: %v\n%s", err, out.String())
	}
	if len(response.Diagnostics) < 2 {
		t.Fatalf("diagnostics len = %d, want diagnostics from multiple files: %#v", len(response.Diagnostics), response.Diagnostics)
	}
	files := map[string]bool{}
	for _, diagnostic := range response.Diagnostics {
		files[filepath.Base(diagnostic.File)] = true
	}
	if !files["model.yaml"] || !files["dashboard.yaml"] {
		t.Fatalf("diagnostic files = %#v, want model.yaml and dashboard.yaml", files)
	}
}

func TestValidateCommandAcceptsPositionalCatalog(t *testing.T) {
	catalog := writeValidateWorkspace(t, validValidateDashboardYAML())
	opts := &rootOptions{}
	cmd := validateCommand(context.Background(), opts)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{catalog})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("validate command error = %v", err)
	}
	if !strings.Contains(out.String(), "ok "+catalog) {
		t.Fatalf("output = %q, want positional catalog path", out.String())
	}
}

func TestValidateCommandRejectsAmbiguousCatalogArgs(t *testing.T) {
	catalog := writeValidateWorkspace(t, validValidateDashboardYAML())
	opts := &rootOptions{}
	cmd := validateCommand(context.Background(), opts)
	cmd.SetArgs([]string{"--catalog", catalog, catalog})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("validate command error = nil, want ambiguity error")
	}
	if !strings.Contains(err.Error(), "either --catalog or positional catalog") {
		t.Fatalf("error = %v, want ambiguity message", err)
	}
}

func TestSchemaExportRejectsUnexpectedArgs(t *testing.T) {
	opts := &rootOptions{schemaOut: t.TempDir()}
	cmd := schemaCommand(opts)
	cmd.SetArgs([]string{"export", "unexpected"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("schema export error = nil, want unexpected arg error")
	}
	if !strings.Contains(err.Error(), "unknown command") && !strings.Contains(err.Error(), "accepts 0 arg") {
		t.Fatalf("error = %v, want argument error", err)
	}
}

func TestRunSchemaExportWritesJSONSchemas(t *testing.T) {
	outDir := t.TempDir()
	err := runSchemaExport(&rootOptions{schemaFormat: "json-schema", schemaOut: outDir})
	if err != nil {
		t.Fatalf("runSchemaExport() error = %v", err)
	}
	for _, name := range []string{
		configschema.JSONSchemaFilename(configschema.KindCatalog),
		configschema.JSONSchemaFilename(configschema.KindSemanticModel),
		configschema.JSONSchemaFilename(configschema.KindDashboard),
	} {
		content, err := os.ReadFile(filepath.Join(outDir, name))
		if err != nil {
			t.Fatalf("read exported schema %s: %v", name, err)
		}
		if !bytes.Contains(content, []byte(`"$schema"`)) {
			t.Fatalf("%s does not look like a JSON Schema: %s", name, content)
		}
	}
}

func writeValidateWorkspace(t *testing.T, dashboardYAML string) string {
	t.Helper()
	dir := t.TempDir()
	writeValidateFixture(t, filepath.Join(dir, "catalog.yaml"), `
workspace:
  id: libredash
  title: LibreDash Workspace
semantic_models:
  - id: olist
    title: Olist
    path: model.yaml
dashboards:
  - id: sales
    title: Sales
    path: dashboard.yaml
`)
	writeValidateFixture(t, filepath.Join(dir, "model.yaml"), `
name: olist
title: Olist
connections:
  olist:
    kind: local
sources:
  orders:
    connection: olist
    path: orders.csv
    format: csv
models:
  orders:
    source: orders
    primary_key: order_id
    fields:
      order_id: {label: Order ID}
      status: {label: Status}
      revenue: {label: Revenue}
semantic_models:
  olist:
    base_table: orders
    tables:
      - orders
    measures:
      defaults: {table: orders, grain: order_id}
      revenue: {expr: SUM(orders.revenue), format: currency}
`)
	writeValidateFixture(t, filepath.Join(dir, "dashboard.yaml"), dashboardYAML)
	return filepath.Join(dir, "catalog.yaml")
}

func validValidateDashboardYAML() string {
	return `
id: sales
title: Sales
semantic_model: olist
filters:
  status:
    type: multi_select
    label: Status
    operator: in
    field: orders.status
visuals:
  revenue:
    kind: kpi
    query:
      measures:
        revenue:
tables: {}
pages:
  - id: overview
    title: Overview
    visuals: []
`
}

func writeValidateFixture(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(strings.TrimSpace(content)+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}
