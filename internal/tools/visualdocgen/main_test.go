package main

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/Yacobolo/leapview/internal/dashboard"
	reportdef "github.com/Yacobolo/leapview/internal/dashboard/report"
	"github.com/Yacobolo/leapview/internal/visualdocs"
)

func TestParseVisualExamplesUsesMarkedYAMLAsSource(t *testing.T) {
	t.Parallel()

	source := []byte("" +
		"# Line chart\n\n" +
		"## Basic\n\n" +
		"{{< visual id=\"line_basic\" >}}\n\n" +
		"```yaml visual-example=line_basic\n" +
		"visuals:\n" +
		"  line_basic:\n" +
		"    title: Revenue\n" +
		"    type: line\n" +
		"    query:\n" +
		"      dimensions:\n" +
		"        month: orders.month\n" +
		"      measures:\n" +
		"        revenue: null\n" +
		"```\n")

	examples, err := parseVisualExamples("line.md", source)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(examples), 1; got != want {
		t.Fatalf("examples = %d, want %d", got, want)
	}
	example := examples[0]
	if example.ID != "line_basic" || example.Chart == nil || example.Chart.Type != "line" {
		t.Fatalf("example = %#v", example)
	}
	if got := example.Chart.Query.Dimensions[0].Field; got != "orders.month" {
		t.Fatalf("dimension = %q, want orders.month", got)
	}
}

func TestGenerateVisualExamplesExecutesEveryDocumentedQuery(t *testing.T) {
	docsDir := filepath.Join("..", "..", "..", "docs", "visuals")
	artifact, err := generateVisualExamples(docsDir, filepath.Join("testdata", "project", "leapview.yaml"), filepath.Join("testdata", "data"))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := artifact.Version, visualdocs.ArtifactVersion; got != want {
		t.Fatalf("version = %d, want %d", got, want)
	}
	lineReference := artifact.References["visuals/line"]
	if got, want := lineReference.Kind, "chart"; got != want {
		t.Fatalf("line reference kind = %q, want %q", got, want)
	}
	if got, want := strings.Join(lineReference.Shapes, ","), "category_series_value,category_value"; got != want {
		t.Fatalf("line reference shapes = %q, want %q", got, want)
	}
	if got := strings.Join(lineReference.Examples["revenue_line_step"].KeyFields, ","); !strings.Contains(got, "options.step") || strings.Contains(got, "query.series") {
		t.Fatalf("stepped line key fields = %q", got)
	}
	fields := make(map[string]visualdocs.FieldReference, len(lineReference.Fields))
	for _, field := range lineReference.Fields {
		fields[field.Path] = field
	}
	if got, want := fields["query.dimensions"].Type, "field mapping"; got != want {
		t.Fatalf("query.dimensions type = %q, want %q", got, want)
	}
	if got, want := fields["query.limit"].Default, "no limit"; got != want {
		t.Fatalf("query.limit default = %q, want %q", got, want)
	}
	if got, want := fields["options.step"].Type, "string | boolean"; got != want {
		t.Fatalf("options.step type = %q, want %q", got, want)
	}
	if got, want := strings.Join(fields["options.step"].AllowedValues, ","), "start,middle,end,true"; got != want {
		t.Fatalf("options.step values = %q, want %q", got, want)
	}
	if fields["options.step"].Description == "" {
		t.Fatal("options.step description is empty")
	}
	if got := artifact.References["visuals/map"].Accessibility; !strings.Contains(got, "map identifiers") {
		t.Fatalf("map accessibility guidance = %q", got)
	}
	if got := artifact.References["visuals/kpi"].Accessibility; !strings.Contains(got, "tone as the only") {
		t.Fatalf("KPI accessibility guidance = %q", got)
	}
	if got, want := len(artifact.Documents), 26; got != want {
		t.Fatalf("documents = %d, want %d", got, want)
	}
	count := 0
	for slug, examples := range artifact.Documents {
		if len(examples) == 0 {
			t.Fatalf("%s has no examples", slug)
		}
		for _, example := range examples {
			count++
			if example.Chart != nil && len(example.Chart.Data) == 0 {
				t.Fatalf("%s/%s has no query data", slug, example.ID)
			}
			if example.Tabular != nil && len(example.Tabular.Blocks["a"].Rows) == 0 {
				t.Fatalf("%s/%s has no query rows", slug, example.ID)
			}
		}
	}
	if got, want := count, 72; got != want {
		t.Fatalf("examples = %d, want %d", got, want)
	}
	if got, want := len(artifact.Showcase), 26; got != want {
		t.Fatalf("showcase examples = %d, want %d", got, want)
	}
	line := artifact.Documents["visuals/line"]
	if got := line[1].Chart.Shape; got != "category_series_value" {
		t.Fatalf("series line shape = %q", got)
	}
	if got := line[2].Chart.Options["step"]; got != "middle" {
		t.Fatalf("stepped line option = %#v", got)
	}
	first, err := json.Marshal(artifact)
	if err != nil {
		t.Fatal(err)
	}
	regenerated, err := generateVisualExamples(docsDir, filepath.Join("testdata", "project", "leapview.yaml"), filepath.Join("testdata", "data"))
	if err != nil {
		t.Fatal(err)
	}
	second, err := json.Marshal(regenerated)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Fatal("artifact JSON is not deterministic")
	}
}

func TestValidateVisualPayloadRejectsInvalidGeneratedData(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		visual  visualExample
		payload dashboard.Visual
		want    string
	}{
		{
			name:    "non finite metric",
			visual:  visualExample{ID: "bad_number", Chart: reportVisualPointer("category_value", "line", nil)},
			payload: dashboard.Visual{Data: []dashboard.Datum{{"label": "Jan", "value": math.NaN()}}},
			want:    `non-finite number at data[0].value`,
		},
		{
			name:    "unknown map region",
			visual:  visualExample{ID: "bad_map", Chart: reportVisualPointer("geo", "map", map[string]any{"map": "brazil_states"})},
			payload: dashboard.Visual{Data: []dashboard.Datum{{"name": "CA", "value": 2.0}}},
			want:    `region "CA" is not defined by map "brazil_states"`,
		},
		{
			name:    "incomplete map coverage",
			visual:  visualExample{ID: "incomplete_map", Chart: reportVisualPointer("geo", "map", map[string]any{"map": "brazil_states"})},
			payload: dashboard.Visual{Data: []dashboard.Datum{{"name": "SP", "value": 2.0}}},
			want:    `does not provide data for map region`,
		},
		{
			name:    "no numeric values",
			visual:  visualExample{ID: "empty_series", Chart: reportVisualPointer("category_value", "line", nil)},
			payload: dashboard.Visual{Data: []dashboard.Datum{{"label": "Jan"}}},
			want:    `has no finite numeric values`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateVisualPayload(tt.visual, tt.payload)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func reportVisual(shape, visualType string, options map[string]any) reportdef.Visual {
	return reportdef.Visual{Shape: shape, Type: visualType, Options: options}
}

func reportVisualPointer(shape, visualType string, options map[string]any) *reportdef.Visual {
	value := reportVisual(shape, visualType, options)
	return &value
}

func TestPersistVisualExamplesCheckDetectsStaleArtifact(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "examples.gen.json")
	artifact := visualExamplesArtifact{Version: 2, Documents: map[string][]visualdocs.Payload{}, Showcase: []visualdocs.Payload{}}
	if err := persistVisualExamples(path, artifact, false); err != nil {
		t.Fatal(err)
	}
	if err := persistVisualExamples(path, artifact, true); err != nil {
		t.Fatalf("current artifact: %v", err)
	}
	if err := os.WriteFile(path, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := persistVisualExamples(path, artifact, true); err == nil || !strings.Contains(err.Error(), "out of date") {
		t.Fatalf("stale artifact error = %v", err)
	}
}

func TestVisualDocumentationUsesPatternHeadingsAndSpecificGuidance(t *testing.T) {
	t.Parallel()
	docsDir := filepath.Join("..", "..", "..", "docs", "visuals")
	files, err := filepath.Glob(filepath.Join(docsDir, "*.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		if filepath.Base(file) == "index.md" {
			continue
		}
		contents, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		source := string(contents)
		for _, boilerplate := range []string{
			"Start with the default presentation and keep the query focused",
			"to create this variation while leaving the renderer contract unchanged",
		} {
			if strings.Contains(source, boilerplate) {
				t.Errorf("%s contains generic variation guidance %q", file, boilerplate)
			}
		}
		headings := map[string]struct{}{}
		for _, line := range strings.Split(source, "\n") {
			if strings.HasPrefix(line, "## ") {
				headings[strings.TrimPrefix(line, "## ")] = struct{}{}
			}
		}
		for _, title := range regexp.MustCompile(`(?m)^    title: (.+)$`).FindAllStringSubmatch(source, -1) {
			if _, duplicate := headings[title[1]]; duplicate {
				t.Errorf("%s repeats rendered visual title %q as a variation heading", file, title[1])
			}
		}
	}
}

func TestParseVisualExamplesRejectsBrokenContracts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "missing fence",
			body: `{{< visual id="line_basic" >}}`,
			want: `shortcode "line_basic" has no matching visual example`,
		},
		{
			name: "missing shortcode",
			body: "```yaml visual-example=line_basic\nvisuals:\n  line_basic:\n    title: Line\n    type: line\n    query:\n      dimensions: [orders.month]\n      measures: [revenue]\n```",
			want: `visual example "line_basic" has no matching shortcode`,
		},
		{
			name: "multiple visuals",
			body: "{{< visual id=\"line_basic\" >}}\n```yaml visual-example=line_basic\nvisuals:\n  line_basic: {type: line}\n  other: {type: line}\n```",
			want: `must contain exactly one visual`,
		},
		{
			name: "key mismatch",
			body: "{{< visual id=\"line_basic\" >}}\n```yaml visual-example=line_basic\nvisuals:\n  other: {type: line}\n```",
			want: `must use visual key "line_basic"`,
		},
		{
			name: "duplicate shortcode",
			body: "{{< visual id=\"line_basic\" >}}\n{{< visual id=\"line_basic\" >}}\n```yaml visual-example=line_basic\nvisuals:\n  line_basic: {type: line}\n```",
			want: `duplicate visual shortcode "line_basic"`,
		},
		{
			name: "missing type",
			body: "{{< visual id=\"total\" >}}\n```yaml visual-example=total\nvisuals:\n  total:\n    shape: single_value\n    query:\n      measures: [revenue]\n```",
			want: `type`,
		},
		{
			name: "legacy kind",
			body: "{{< visual id=\"total\" >}}\n```yaml visual-example=total\nvisuals:\n  total:\n    kind: kpi\n    shape: single_value\n    query:\n      measures: [revenue]\n```",
			want: `kind`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseVisualExamples("line.md", []byte(tt.body))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want containing %q", err, tt.want)
			}
		})
	}
}
