// Command visualdocgen compiles executable visual examples embedded in the
// public Markdown documentation into deterministic browser payloads.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"

	dashboardadapter "github.com/Yacobolo/libredash/internal/analytics/duckdb/dashboardadapter"
	"github.com/Yacobolo/libredash/internal/configschema"
	"github.com/Yacobolo/libredash/internal/dashboard"
	reportdef "github.com/Yacobolo/libredash/internal/dashboard/report"
	dashboardruntime "github.com/Yacobolo/libredash/internal/dashboard/runtime"
	"github.com/Yacobolo/libredash/internal/visualdocs"
	"github.com/Yacobolo/libredash/internal/workspace"
	workspacecompiler "github.com/Yacobolo/libredash/internal/workspace/compiler"
	"gopkg.in/yaml.v3"
)

var visualShortcodePattern = regexp.MustCompile(`^\s*\{\{<\s*visual\s+id="([a-z0-9_]+)"\s*>}}\s*$`)
var visualFencePattern = regexp.MustCompile("^```ya?ml[ \\t]+visual-example=([a-z0-9_]+)[ \\t]*$")

type visualExample struct {
	ID      string
	Source  string
	Line    int
	Chart   *reportdef.Visual
	Tabular *reportdef.TableVisual
}

type visualExampleFragment struct {
	Visuals map[string]yaml.Node `yaml:"visuals"`
}

type visualCatalog struct {
	Documents []struct {
		Source string `json:"source"`
		Title  string `json:"title"`
	} `json:"documents"`
}

type visualExamplesArtifact = visualdocs.Artifact
type visualDocumentReference = visualdocs.DocumentReference
type visualExampleReference = visualdocs.ExampleReference

func main() {
	docsDir := flag.String("docs", "docs/visuals", "visual documentation directory")
	project := flag.String("project", "internal/tools/visualdocgen/testdata/project/libredash.yaml", "fixture project")
	data := flag.String("data", "internal/tools/visualdocgen/testdata/data", "fixture managed-data root")
	out := flag.String("out", "docs/visuals/examples.gen.json", "generated artifact")
	check := flag.Bool("check", false, "verify the generated artifact is current")
	flag.Parse()

	artifact, err := generateVisualExamples(*docsDir, *project, *data)
	if err == nil {
		err = persistVisualExamples(*out, artifact, *check)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "generate visual documentation: %v\n", err)
		os.Exit(1)
	}
}

func persistVisualExamples(path string, artifact visualExamplesArtifact, check bool) error {
	encoded, err := json.MarshalIndent(artifact, "", "  ")
	if err != nil {
		return err
	}
	encoded = append(encoded, '\n')
	if !check {
		return os.WriteFile(path, encoded, 0o644)
	}
	current, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if !bytes.Equal(current, encoded) {
		return fmt.Errorf("%s is out of date; run task docs:generate", path)
	}
	return nil
}

func generateVisualExamples(docsDir, projectPath, dataRoot string) (visualExamplesArtifact, error) {
	catalogContents, err := os.ReadFile(filepath.Join(docsDir, "catalog.json"))
	if err != nil {
		return visualExamplesArtifact{}, err
	}
	var catalog visualCatalog
	if err := json.Unmarshal(catalogContents, &catalog); err != nil {
		return visualExamplesArtifact{}, fmt.Errorf("decode visual catalog: %w", err)
	}

	examplesByPage := make(map[string][]visualExample, len(catalog.Documents))
	globalIDs := map[string]string{}
	for _, document := range catalog.Documents {
		path := filepath.Join(docsDir, document.Source+".md")
		contents, err := os.ReadFile(path)
		if err != nil {
			return visualExamplesArtifact{}, err
		}
		examples, err := parseVisualExamples(path, contents)
		if err != nil {
			return visualExamplesArtifact{}, err
		}
		if len(examples) == 0 {
			return visualExamplesArtifact{}, fmt.Errorf("%s: visual document has no executable examples", path)
		}
		for _, example := range examples {
			if previous := globalIDs[example.ID]; previous != "" {
				return visualExamplesArtifact{}, fmt.Errorf("%s:%d: visual example %q is already declared in %s", path, example.Line, example.ID, previous)
			}
			globalIDs[example.ID] = path
		}
		examplesByPage[document.Source] = examples
	}

	compiled, err := workspacecompiler.CompileProject(projectPath, workspacecompiler.Options{WorkspaceID: "visual_examples"})
	if err != nil {
		return visualExamplesArtifact{}, fmt.Errorf("compile fixture project: %w", err)
	}
	compiledWorkspace, ok := compiled.Workspaces["visual_examples"]
	if !ok {
		return visualExamplesArtifact{}, fmt.Errorf("fixture project has no visual_examples workspace")
	}
	definition := compiledWorkspace.Definition
	report := buildExampleDashboard(catalog, examplesByPage)
	if err := workspacecompiler.ValidateDashboard(report, definition.Models); err != nil {
		return visualExamplesArtifact{}, fmt.Errorf("validate executable examples: %w", err)
	}
	definition.Dashboards = map[string]*reportdef.Dashboard{report.ID: report}
	definition.Catalog.Dashboards = []workspace.CatalogDashboard{{ID: report.ID, Title: report.Title, Path: "docs/visuals", Description: report.Description}}
	if err := bindFixtureRoot(definition, dataRoot); err != nil {
		return visualExamplesArtifact{}, err
	}

	runtimeDir, err := os.MkdirTemp("", "libredash-visual-docs-*")
	if err != nil {
		return visualExamplesArtifact{}, err
	}
	defer os.RemoveAll(runtimeDir)
	service, err := dashboardruntime.NewFromDefinition(runtimeDir, dashboardadapter.NewFactory(dashboardadapter.Options{}), definition)
	if err != nil {
		return visualExamplesArtifact{}, fmt.Errorf("open fixture runtime: %w", err)
	}
	defer service.Close()

	artifact := visualExamplesArtifact{Version: visualdocs.ArtifactVersion, Documents: map[string][]visualdocs.Payload{}, References: map[string]visualDocumentReference{}, Showcase: make([]visualdocs.Payload, 0, len(catalog.Documents))}
	for _, document := range catalog.Documents {
		patch, err := service.QueryDashboardPage(context.Background(), report.ID, document.Source, dashboard.Filters{})
		if err != nil {
			return visualExamplesArtifact{}, fmt.Errorf("query %s examples: %w", document.Source, err)
		}
		if patch.Status.Error != "" {
			return visualExamplesArtifact{}, fmt.Errorf("query %s examples: %s", document.Source, patch.Status.Error)
		}
		payloads := make([]visualdocs.Payload, 0, len(examplesByPage[document.Source]))
		for _, example := range examplesByPage[document.Source] {
			if example.Chart != nil {
				payload, ok := patch.Visuals[example.ID]
				if !ok || len(payload.Data) == 0 {
					return visualExamplesArtifact{}, fmt.Errorf("query %s did not return visual %q data", document.Source, example.ID)
				}
				if err := validateVisualPayload(example, payload); err != nil {
					return visualExamplesArtifact{}, err
				}
				canonicalizePayloadData(*example.Chart, &payload)
				payloads = append(payloads, visualdocs.ChartPayload(payload))
				continue
			}
			table, err := service.QueryTablePage(context.Background(), report.ID, document.Source, dashboard.Filters{}, dashboard.TableRequest{Table: example.ID, Block: "a", Start: 0, Count: dashboard.TableMaxRequestCount})
			if err != nil {
				return visualExamplesArtifact{}, fmt.Errorf("query %s visual %q: %w", document.Source, example.ID, err)
			}
			if len(table.Blocks["a"].Rows) == 0 {
				return visualExamplesArtifact{}, fmt.Errorf("visual example %q returned no rows", example.ID)
			}
			payloads = append(payloads, visualdocs.TabularPayload(dashboard.NewTabularVisual(example.ID, table)))
		}
		slug := "visuals/" + document.Source
		artifact.Documents[slug] = payloads
		reference, err := buildVisualDocumentReference(examplesByPage[document.Source])
		if err != nil {
			return visualExamplesArtifact{}, fmt.Errorf("build %s field reference: %w", document.Source, err)
		}
		artifact.References[slug] = reference
		artifact.Showcase = append(artifact.Showcase, payloads[0])
	}
	return artifact, nil
}

var visualDocMapRegions = map[string]map[string]struct{}{
	"brazil_states": stringSet("RR", "AP", "AM", "PA", "AC", "RO", "MT", "TO", "MA", "PI", "CE", "RN", "PB", "PE", "AL", "SE", "BA", "GO", "DF", "MS", "MG", "ES", "RJ", "SP", "PR", "SC", "RS"),
}

func stringSet(values ...string) map[string]struct{} {
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		result[value] = struct{}{}
	}
	return result
}

func validateVisualPayload(example visualExample, payload dashboard.Visual) error {
	finiteNumbers := 0
	for index, datum := range payload.Data {
		if len(datum) == 0 {
			return fmt.Errorf("visual example %q has an empty row at data[%d]", example.ID, index)
		}
		if err := inspectPayloadValue(datum, fmt.Sprintf("data[%d]", index), &finiteNumbers); err != nil {
			return fmt.Errorf("visual example %q %w", example.ID, err)
		}
	}
	if finiteNumbers == 0 {
		return fmt.Errorf("visual example %q has no finite numeric values", example.ID)
	}
	if example.Chart.ShapeOrDefault() != "geo" {
		return nil
	}
	mapID, _ := example.Chart.Options["map"].(string)
	regions, ok := visualDocMapRegions[mapID]
	if !ok {
		return fmt.Errorf("visual example %q uses unsupported documentation map %q", example.ID, mapID)
	}
	seenRegions := make(map[string]struct{}, len(payload.Data))
	for index, datum := range payload.Data {
		region, _ := datum["name"].(string)
		if _, ok := regions[region]; !ok {
			return fmt.Errorf("visual example %q region %q is not defined by map %q at data[%d].name", example.ID, region, mapID, index)
		}
		seenRegions[region] = struct{}{}
	}
	for _, region := range sortedSet(regions) {
		if _, ok := seenRegions[region]; !ok {
			return fmt.Errorf("visual example %q does not provide data for map region %q in %q", example.ID, region, mapID)
		}
	}
	return nil
}

func inspectPayloadValue(value any, path string, finiteNumbers *int) error {
	switch typed := value.(type) {
	case float64:
		if math.IsNaN(typed) || math.IsInf(typed, 0) {
			return fmt.Errorf("contains a non-finite number at %s", path)
		}
		*finiteNumbers++
	case float32:
		if math.IsNaN(float64(typed)) || math.IsInf(float64(typed), 0) {
			return fmt.Errorf("contains a non-finite number at %s", path)
		}
		*finiteNumbers++
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		*finiteNumbers++
	case dashboard.Datum:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			if err := inspectPayloadValue(typed[key], path+"."+key, finiteNumbers); err != nil {
				return err
			}
		}
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			if err := inspectPayloadValue(typed[key], path+"."+key, finiteNumbers); err != nil {
				return err
			}
		}
	case []any:
		for index, item := range typed {
			if err := inspectPayloadValue(item, fmt.Sprintf("%s[%d]", path, index), finiteNumbers); err != nil {
				return err
			}
		}
	}
	return nil
}

func buildVisualDocumentReference(examples []visualExample) (visualDocumentReference, error) {
	if len(examples) > 0 && examples[0].Tabular != nil {
		visualType := dashboard.NewTabularVisual("reference", dashboard.Table{Kind: examples[0].Tabular.Kind}).Type
		return visualDocumentReference{
			Kind: "visual", Renderer: "tabular", Shapes: []string{visualType},
			QueryFields: []string{"table", "fields", "rows", "columns", "measures"},
			Fields: []visualdocs.FieldReference{
				{Path: "type", Type: "string", AllowedValues: []string{"table", "matrix", "pivot"}, Description: "Selects the tabular visual behavior."},
				{Path: "query", Type: "tabular query", Description: "Selects record fields or grouped row, column, and measure fields."},
				{Path: "cardinality", Type: "string", AllowedValues: []string{"bounded", "exact"}, Description: "Controls whether the visual resolves an exact row count."},
			},
			Accessibility: "Tabular visuals expose semantic headers and virtualized rows while preserving keyboard navigation.",
			Examples:      map[string]visualExampleReference{examples[0].ID: {KeyFields: []string{"type", "query", "columns"}}},
		}, nil
	}
	kinds := map[string]struct{}{}
	renderers := map[string]struct{}{}
	shapes := map[string]struct{}{}
	queryFields := map[string]struct{}{}
	options := map[string]struct{}{}
	reference := visualDocumentReference{Examples: make(map[string]visualExampleReference, len(examples))}
	var previous *reportdef.Visual
	for index := range examples {
		visual := *examples[index].Chart
		kinds[visual.KindOrDefault()] = struct{}{}
		renderers[visual.RendererOrDefault()] = struct{}{}
		shapes[visual.ShapeOrDefault()] = struct{}{}
		collectQueryFields(visual.Query, queryFields)
		for key := range visual.Options {
			options[key] = struct{}{}
		}
		reference.Examples[examples[index].ID] = visualExampleReference{KeyFields: visualKeyFields(previous, visual)}
		previous = examples[index].Chart
	}
	reference.Kind = strings.Join(sortedSet(kinds), ", ")
	reference.Renderer = strings.Join(sortedSet(renderers), ", ")
	reference.Shapes = sortedSet(shapes)
	reference.QueryFields = sortedSet(queryFields)
	reference.Options = sortedSet(options)
	fields, err := visualFieldReferences(reference.QueryFields, reference.Options, examples[0].Chart.Type)
	if err != nil {
		return visualDocumentReference{}, err
	}
	reference.Fields = fields
	reference.Accessibility = visualAccessibilityGuidance(*examples[0].Chart)
	return reference, nil
}

func collectQueryFields(query reportdef.VisualQuery, fields map[string]struct{}) {
	if query.Table != "" {
		fields["table"] = struct{}{}
	}
	if len(query.Dimensions) > 0 {
		fields["dimensions"] = struct{}{}
	}
	if !query.Series.IsZero() {
		fields["series"] = struct{}{}
	}
	if len(query.Measures) > 0 {
		fields["measures"] = struct{}{}
	}
	if query.Time.Field != "" {
		fields["time"] = struct{}{}
	}
	if len(query.Sort) > 0 {
		fields["sort"] = struct{}{}
	}
	if query.Limit > 0 {
		fields["limit"] = struct{}{}
	}
}

func visualKeyFields(previous *reportdef.Visual, visual reportdef.Visual) []string {
	fields := make([]string, 0, 12)
	changedToValue := func(before, after any) bool {
		return valueIsSet(after) && (previous == nil || !reflect.DeepEqual(before, after))
	}
	if changedToValue(valueOrZero(previous, func(item reportdef.Visual) any { return item.Shape }), visual.Shape) {
		fields = append(fields, "shape")
	}
	queryChecks := []struct {
		name string
		get  func(reportdef.VisualQuery) any
	}{
		{"table", func(query reportdef.VisualQuery) any { return query.Table }},
		{"dimensions", func(query reportdef.VisualQuery) any { return query.Dimensions }},
		{"series", func(query reportdef.VisualQuery) any { return query.Series }},
		{"measures", func(query reportdef.VisualQuery) any { return query.Measures }},
		{"time", func(query reportdef.VisualQuery) any { return query.Time }},
	}
	for _, check := range queryChecks {
		var before any
		if previous != nil {
			before = check.get(previous.Query)
		}
		after := check.get(visual.Query)
		if changedToValue(before, after) {
			fields = append(fields, "query."+check.name)
		}
	}
	optionKeys := make(map[string]struct{}, len(visual.Options))
	for key := range visual.Options {
		optionKeys[key] = struct{}{}
	}
	for _, key := range sortedSet(optionKeys) {
		if previous == nil || !reflect.DeepEqual(previous.Options[key], visual.Options[key]) {
			fields = append(fields, "options."+key)
		}
	}
	return fields
}

func valueIsSet(value any) bool {
	if value == nil {
		return false
	}
	reflected := reflect.ValueOf(value)
	return reflected.IsValid() && !reflected.IsZero()
}

func valueOrZero(previous *reportdef.Visual, get func(reportdef.Visual) any) any {
	if previous == nil {
		return nil
	}
	return get(*previous)
}

func sortedSet(values map[string]struct{}) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func visualAccessibilityGuidance(visual reportdef.Visual) string {
	if visual.KindOrDefault() == "kpi" {
		return "Keep the note concise and do not use tone as the only indication of status."
	}
	switch visual.Type {
	case "map":
		return "Region values must match the selected map identifiers. Add labels when boundaries or color differences may be difficult to distinguish."
	case "graph", "sankey", "tree", "sunburst", "treemap":
		return "Use meaningful node labels and keep the hierarchy or flow small enough to follow without relying on color alone."
	default:
		return "Use a descriptive title and unit, and do not rely on color alone to distinguish series or values."
	}
}

func canonicalizePayloadData(visual reportdef.Visual, payload *dashboard.Visual) {
	if payload == nil || len(payload.Data) < 2 || payload.Shape == "binned_measure" || payload.Shape == "single_value" {
		return
	}
	sort.SliceStable(payload.Data, func(left, right int) bool {
		for _, order := range visual.Query.Sort {
			leftValue := payloadSortValue(visual, payload.Data[left], order.Field)
			rightValue := payloadSortValue(visual, payload.Data[right], order.Field)
			comparison := comparePayloadValues(leftValue, rightValue)
			if comparison == 0 {
				continue
			}
			if strings.EqualFold(order.Direction, "desc") {
				return comparison > 0
			}
			return comparison < 0
		}
		leftJSON, _ := json.Marshal(payload.Data[left])
		rightJSON, _ := json.Marshal(payload.Data[right])
		return bytes.Compare(leftJSON, rightJSON) < 0
	})
}

func payloadSortValue(visual reportdef.Visual, datum dashboard.Datum, field string) any {
	if value, ok := datum[field]; ok {
		return value
	}
	for index, dimension := range visual.Query.Dimensions {
		if field != dimension.Field && field != dimension.Alias {
			continue
		}
		switch visual.ShapeOrDefault() {
		case "matrix":
			if index == 0 {
				return datum["row"]
			}
			return datum["column"]
		case "graph":
			if index == 0 {
				return datum["source"]
			}
			return datum["target"]
		case "geo":
			return datum["name"]
		default:
			return datum["label"]
		}
	}
	return nil
}

func comparePayloadValues(left, right any) int {
	leftNumber, leftIsNumber := payloadNumber(left)
	rightNumber, rightIsNumber := payloadNumber(right)
	if leftIsNumber && rightIsNumber {
		switch {
		case leftNumber < rightNumber:
			return -1
		case leftNumber > rightNumber:
			return 1
		default:
			return 0
		}
	}
	return strings.Compare(fmt.Sprint(left), fmt.Sprint(right))
}

func payloadNumber(value any) (float64, bool) {
	switch typed := value.(type) {
	case int:
		return float64(typed), true
	case int32:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case uint:
		return float64(typed), true
	case uint32:
		return float64(typed), true
	case uint64:
		return float64(typed), true
	case float32:
		return float64(typed), true
	case float64:
		return typed, true
	default:
		return 0, false
	}
}

func buildExampleDashboard(catalog visualCatalog, examplesByPage map[string][]visualExample) *reportdef.Dashboard {
	report := &reportdef.Dashboard{ID: "visual-docs", Title: "Visual documentation", Description: "Executable documentation examples.", SemanticModel: "visual_examples", Visuals: map[string]reportdef.Visual{}, Tables: map[string]reportdef.TableVisual{}, Pages: make([]dashboard.Page, 0, len(catalog.Documents))}
	for _, document := range catalog.Documents {
		page := dashboard.Page{ID: document.Source, Title: document.Title, Canvas: dashboard.PageCanvas{Width: 1366, Height: 3000}, Grid: dashboard.PageGrid{Columns: 12, RowHeight: 48, Gap: 16, Padding: 16}, Visuals: make([]dashboard.PageVisual, 0, len(examplesByPage[document.Source]))}
		for index, example := range examplesByPage[document.Source] {
			component := dashboard.PageVisual{ID: example.ID, Placement: dashboard.PagePlacement{Col: 1, Row: 1 + index*8, ColSpan: 6, RowSpan: 7}}
			if example.Chart != nil {
				report.Visuals[example.ID] = *example.Chart
				component.Kind, component.Visual = example.Chart.Type+"_chart", example.ID
				if example.Chart.KindOrDefault() == "kpi" {
					component.Kind = "kpi_card"
				}
			} else {
				report.Tables[example.ID] = *example.Tabular
				component.Kind, component.Table = "table", example.ID
			}
			page.Visuals = append(page.Visuals, component)
		}
		report.Pages = append(report.Pages, page)
	}
	return report
}

func bindFixtureRoot(definition *workspace.Definition, dataRoot string) error {
	root, err := filepath.Abs(dataRoot)
	if err != nil {
		return err
	}
	for _, model := range definition.Models {
		for name, connection := range model.Connections {
			if connection.Kind != "managed" {
				continue
			}
			connection.Root = root
			connection.Scope = ""
			model.Connections[name] = connection
		}
	}
	return nil
}

func parseVisualExamples(filename string, source []byte) ([]visualExample, error) {
	lines := strings.Split(strings.ReplaceAll(string(source), "\r\n", "\n"), "\n")
	shortcodes := map[string]int{}
	examples := make([]visualExample, 0)
	seenExamples := map[string]int{}

	for index := 0; index < len(lines); index++ {
		line := lines[index]
		if match := visualShortcodePattern.FindStringSubmatch(line); len(match) == 2 {
			id := match[1]
			if previous := shortcodes[id]; previous > 0 {
				return nil, fmt.Errorf("%s:%d: duplicate visual shortcode %q (first declared on line %d)", filename, index+1, id, previous)
			}
			shortcodes[id] = index + 1
			continue
		}

		match := visualFencePattern.FindStringSubmatch(line)
		if len(match) != 2 {
			continue
		}
		id := match[1]
		if previous := seenExamples[id]; previous > 0 {
			return nil, fmt.Errorf("%s:%d: duplicate visual example %q (first declared on line %d)", filename, index+1, id, previous)
		}
		closing := -1
		for candidate := index + 1; candidate < len(lines); candidate++ {
			if strings.TrimSpace(lines[candidate]) == "```" {
				closing = candidate
				break
			}
		}
		if closing < 0 {
			return nil, fmt.Errorf("%s:%d: unclosed visual example %q", filename, index+1, id)
		}
		body := strings.Join(lines[index+1:closing], "\n") + "\n"
		decoder := yaml.NewDecoder(bytes.NewBufferString(body))
		decoder.KnownFields(true)
		var fragment visualExampleFragment
		if err := decoder.Decode(&fragment); err != nil {
			return nil, fmt.Errorf("%s:%d: decode visual example %q: %w", filename, index+2, id, err)
		}
		if len(fragment.Visuals) != 1 {
			return nil, fmt.Errorf("%s:%d: visual example %q must contain exactly one visual", filename, index+1, id)
		}
		visualNode, ok := fragment.Visuals[id]
		if !ok {
			keys := make([]string, 0, len(fragment.Visuals))
			for key := range fragment.Visuals {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			return nil, fmt.Errorf("%s:%d: visual example %q must use visual key %q, got %q", filename, index+1, id, id, strings.Join(keys, ", "))
		}
		seenExamples[id] = index + 1
		example, err := decodeVisualExample(id, filename, index+1, visualNode)
		if err != nil {
			return nil, err
		}
		examples = append(examples, example)
		index = closing
	}

	for id, line := range shortcodes {
		if seenExamples[id] == 0 {
			return nil, fmt.Errorf("%s:%d: shortcode %q has no matching visual example", filename, line, id)
		}
	}
	for id, line := range seenExamples {
		if shortcodes[id] == 0 {
			return nil, fmt.Errorf("%s:%d: visual example %q has no matching shortcode", filename, line, id)
		}
	}
	return examples, nil
}

func decodeVisualExample(id, filename string, line int, node yaml.Node) (visualExample, error) {
	var tag struct {
		Type string `yaml:"type"`
	}
	if err := node.Decode(&tag); err != nil {
		return visualExample{}, err
	}
	for index := 0; index+1 < len(node.Content); index += 2 {
		if node.Content[index].Value == "kind" {
			return visualExample{}, fmt.Errorf("%s:%d: visual %q uses removed field kind; use type", filename, line, id)
		}
	}
	if strings.TrimSpace(tag.Type) == "" {
		return visualExample{}, fmt.Errorf("%s:%d: visual %q requires type", filename, line, id)
	}
	if err := validateVisualExampleContract(id, filename, node); err != nil {
		return visualExample{}, fmt.Errorf("%s:%d: visual %q: %w", filename, line, id, err)
	}
	example := visualExample{ID: id, Source: filename, Line: line}
	switch tag.Type {
	case "table", "matrix", "pivot":
		var value reportdef.TableVisual
		if err := node.Decode(&value); err != nil {
			return visualExample{}, fmt.Errorf("%s:%d: decode visual %q: %w", filename, line, id, err)
		}
		value.Kind = map[string]string{"table": "data_table", "matrix": "matrix_table", "pivot": "pivot_table"}[tag.Type]
		example.Tabular = &value
	default:
		var value reportdef.Visual
		if err := node.Decode(&value); err != nil {
			return visualExample{}, fmt.Errorf("%s:%d: decode visual %q: %w", filename, line, id, err)
		}
		example.Chart = &value
	}
	return example, nil
}

func validateVisualExampleContract(id, filename string, node yaml.Node) error {
	var visual any
	if err := node.Decode(&visual); err != nil {
		return err
	}
	resource := map[string]any{
		"apiVersion": "libredash.dev/v1",
		"kind":       "Dashboard",
		"metadata":   map[string]any{"name": "visual-doc-example"},
		"spec": map[string]any{
			"semanticModel": "visual_examples",
			"visuals":       map[string]any{id: visual},
			"pages": []any{map[string]any{
				"id": "example", "title": "Example",
				"components": []any{map[string]any{
					"id": id, "kind": "visual", "visual": id,
					"placement": map[string]int{"col": 1, "row": 1, "col_span": 6, "row_span": 4},
				}},
			}},
		},
	}
	content, err := yaml.Marshal(resource)
	if err != nil {
		return err
	}
	return configschema.ValidateBytes(configschema.KindDashboardResource, filename, content)
}
