package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGoFieldNamePreservesInitialisms(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"id":             "ID",
		"dashboardId":    "DashboardID",
		"urlParams":      "URLParams",
		"queryJson":      "QueryJSON",
		"sql":            "SQL",
		"tableUuid":      "TableUUID",
		"ssoAuth":        "SSOAuth",
		"containsNan":    "ContainsNaN",
		"durationMs":     "DurationMS",
		"workspaceTitle": "WorkspaceTitle",
	}
	for input, want := range tests {
		input, want := input, want
		t.Run(input, func(t *testing.T) {
			t.Parallel()
			if got := goFieldName(input); got != want {
				t.Fatalf("goFieldName(%q) = %q, want %q", input, got, want)
			}
		})
	}
}

func TestPostprocessGeneratedModelsIsIdempotent(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "models.gen.go")
	source := "package signals\n\ntype Runtime struct {\n\tDashboardId string `json:\"dashboardId\"`\n}\n"
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := postprocessGeneratedModels(path); err != nil {
			t.Fatalf("postprocess generated models: %v", err)
		}
	}
	generated, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(generated)
	if !strings.Contains(text, "DashboardID string `json:\"dashboardId\"`") {
		t.Fatalf("postprocessed model did not preserve the wire name and Go initialism:\n%s", text)
	}
}

func TestPostprocessGeneratedTypescriptNarrowsInteractionValues(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "index.ts")
	source := `export interface DashboardInteractionCommandMapping {
  field: string
  value: unknown
}

export interface DashboardInteractionSelectionMapping {
  field: string
  value: unknown
}
`
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := postprocessGeneratedTypescript(path); err != nil {
			t.Fatalf("postprocess generated TypeScript: %v", err)
		}
	}
	generated, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(string(generated), "value: string | number | boolean | null"); got != 2 {
		t.Fatalf("typed scalar fields = %d, want 2:\n%s", got, generated)
	}
	if strings.Contains(string(generated), "value: unknown") {
		t.Fatalf("unknown interaction value remains:\n%s", generated)
	}
}
