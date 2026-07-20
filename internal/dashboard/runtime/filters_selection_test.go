package runtime

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	semanticmodel "github.com/Yacobolo/leapview/internal/analytics/model"
	"github.com/Yacobolo/leapview/internal/dashboard"
	reportdef "github.com/Yacobolo/leapview/internal/dashboard/report"
	"github.com/Yacobolo/leapview/internal/dashboard/reportmodel"
)

func TestSemanticFiltersTranslateConformedAndFactLocalSelections(t *testing.T) {
	report, model := selectionFilterFixture()
	runtime := &modelRuntime{model: model}
	service := &FilterService{}

	for _, test := range []struct {
		name      string
		selection dashboard.InteractionSelection
		wantField string
		wantFact  string
		wantValue any
		wantOp    string
	}{
		{
			name:      "conformed propagates without fact",
			selection: filterSelection("decades", dashboard.InteractionSelectionMapping{Field: "release_decade", Value: "1990s"}),
			wantField: "release_decade", wantValue: "1990s", wantOp: "equals",
		},
		{
			name:      "local remains fact scoped",
			selection: filterSelection("buckets", dashboard.InteractionSelectionMapping{Field: "ratings.rating_bucket", Fact: "ratings", Value: "5"}),
			wantField: "ratings.rating_bucket", wantFact: "ratings", wantValue: "5", wantOp: "equals",
		},
		{
			name:      "null uses is null",
			selection: filterSelection("decades", dashboard.InteractionSelectionMapping{Field: "release_decade", Value: nil}),
			wantField: "release_decade", wantOp: "is_null",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			filters, err := service.semanticFilters(context.Background(), runtime, report, dashboard.Filters{Selections: []dashboard.InteractionSelection{test.selection}}, "visual", "cross")
			if err != nil {
				t.Fatal(err)
			}
			if len(filters) != 1 {
				t.Fatalf("filters = %#v", filters)
			}
			got := filters[0]
			if got.Field != test.wantField || got.Fact != test.wantFact || got.Operator != test.wantOp {
				t.Fatalf("filter = %#v", got)
			}
			if test.wantOp == "is_null" {
				if len(got.Values) != 0 {
					t.Fatalf("null filter values = %#v", got.Values)
				}
			} else if len(got.Values) != 1 || got.Values[0] != test.wantValue {
				t.Fatalf("filter values = %#v, want %#v", got.Values, test.wantValue)
			}
		})
	}
}

func TestSelectionMappingFiltersBuildHalfOpenRangesForEveryTimeGrain(t *testing.T) {
	for _, test := range []struct {
		grain string
		value string
		start string
		end   string
	}{
		{grain: "day", value: "2026-02-03", start: "2026-02-03", end: "2026-02-04"},
		{grain: "week", value: "2026-02-02", start: "2026-02-02", end: "2026-02-09"},
		{grain: "month", value: "2026-02", start: "2026-02-01", end: "2026-03-01"},
		{grain: "quarter", value: "2026-Q2", start: "2026-04-01", end: "2026-07-01"},
		{grain: "year", value: "2026", start: "2026-01-01", end: "2027-01-01"},
	} {
		t.Run(test.grain, func(t *testing.T) {
			filters, err := selectionMappingFilters(reportmodel.ResolvedSelectionMapping{Field: "activity_date", Grain: test.grain}, test.value)
			if err != nil {
				t.Fatal(err)
			}
			if len(filters) != 2 || filters[0].Operator != "greater_than_or_equal" || filters[1].Operator != "less_than" {
				t.Fatalf("filters = %#v", filters)
			}
			start := filters[0].Values[0].(time.Time).Format(time.DateOnly)
			end := filters[1].Values[0].(time.Time).Format(time.DateOnly)
			if start != test.start || end != test.end {
				t.Fatalf("range = [%s,%s), want [%s,%s)", start, end, test.start, test.end)
			}
		})
	}
}

func TestSemanticFiltersEmitConformedHalfOpenTimeRange(t *testing.T) {
	report, model := selectionFilterFixture()
	selection := filterSelection("months", dashboard.InteractionSelectionMapping{Field: "activity_date", Grain: "month", Value: "2026-02"})
	filters, err := (&FilterService{}).semanticFilters(context.Background(), &modelRuntime{model: model}, report, dashboard.Filters{Selections: []dashboard.InteractionSelection{selection}}, "visual", "cross")
	if err != nil {
		t.Fatal(err)
	}
	if len(filters) != 2 || filters[0].Field != "activity_date" || filters[0].Fact != "" || filters[1].Fact != "" {
		t.Fatalf("conformed time filters = %#v", filters)
	}
	if filters[0].Operator != "greater_than_or_equal" || filters[1].Operator != "less_than" {
		t.Fatalf("time operators = %#v", filters)
	}
}

func TestSemanticFiltersPreserveOROfCompositeEntries(t *testing.T) {
	report, model := selectionFilterFixture()
	report.Visuals["composite"] = reportdef.Visual{
		Query: reportdef.VisualQuery{
			Dimensions: []reportdef.FieldRef{{Field: "release_decade", Alias: "label"}, {Field: "activity_date", Alias: "series"}},
			Measures:   []reportdef.FieldRef{{Field: "rating_count", Alias: "value"}},
		},
		Interaction: reportdef.Interaction{PointSelection: reportdef.SelectionInteraction{
			Mappings: []reportdef.SelectionMapping{{Field: "release_decade", Value: "label"}, {Field: "activity_date", Value: "series"}},
			Targets:  []string{"cross"},
		}},
	}
	selection := dashboard.InteractionSelection{
		SourceKind: "visual", SourceID: "composite", InteractionKind: "point_selection",
		Entries: []dashboard.InteractionSelectionEntry{
			{Mappings: []dashboard.InteractionSelectionMapping{{Field: "activity_date", Value: "2026-01-01"}, {Field: "release_decade", Value: "1990s"}}},
			{Mappings: []dashboard.InteractionSelectionMapping{{Field: "release_decade", Value: "2000s"}, {Field: "activity_date", Value: "2026-02-01"}}},
		},
	}
	filters, err := (&FilterService{}).semanticFilters(context.Background(), &modelRuntime{model: model}, report, dashboard.Filters{Selections: []dashboard.InteractionSelection{selection}}, "visual", "cross")
	if err != nil {
		t.Fatal(err)
	}
	if len(filters) != 1 || len(filters[0].Groups) != 2 || len(filters[0].Groups[0].Filters) != 2 || len(filters[0].Groups[1].Filters) != 2 {
		t.Fatalf("composite OR filters = %#v", filters)
	}
}

func TestSemanticFiltersIgnoreUIOnlyRowSelections(t *testing.T) {
	report, model := selectionFilterFixture()
	selection := dashboard.InteractionSelection{
		SourceKind: "visual", SourceID: "plain_table", InteractionKind: "row_selection",
		Entries: []dashboard.InteractionSelectionEntry{{Mappings: []dashboard.InteractionSelectionMapping{{Field: dashboard.UIRowSelectionField, Value: "row-1"}}}},
	}
	filters, err := (&FilterService{}).semanticFilters(context.Background(), &modelRuntime{model: model}, report, dashboard.Filters{Selections: []dashboard.InteractionSelection{selection}}, "visual", "cross")
	if err != nil {
		t.Fatal(err)
	}
	if len(filters) != 0 {
		t.Fatalf("UI-only row filters = %#v, want none", filters)
	}
}

func TestSemanticFiltersRejectStoredSelectionWithOmittedJSONValue(t *testing.T) {
	report, model := selectionFilterFixture()
	var selection dashboard.InteractionSelection
	if err := json.Unmarshal([]byte(`{
		"sourceKind":"visual",
		"sourceId":"decades",
		"interactionKind":"point_selection",
		"entries":[{"mappings":[{"field":"release_decade"}]}]
	}`), &selection); err != nil {
		t.Fatal(err)
	}
	_, err := (&FilterService{}).semanticFilters(context.Background(), &modelRuntime{model: model}, report, dashboard.Filters{Selections: []dashboard.InteractionSelection{selection}}, "visual", "cross")
	if err == nil || !strings.Contains(err.Error(), "must include value") {
		t.Fatalf("semanticFilters() error = %v, want omitted-value rejection", err)
	}
}

func selectionFilterFixture() (*reportdef.Dashboard, *semanticmodel.Model) {
	model := &semanticmodel.Model{
		Tables: map[string]semanticmodel.Table{
			"ratings": {Dimensions: map[string]semanticmodel.MetricDimension{"rating_bucket": {Type: "string"}, "rated_at": {Type: "timestamp"}, "release_decade": {Type: "string"}}},
			"tags":    {Dimensions: map[string]semanticmodel.MetricDimension{"tagged_at": {Type: "timestamp"}, "release_decade": {Type: "string"}}},
		},
		Dimensions: map[string]semanticmodel.SemanticDimension{
			"release_decade": {Type: "string", Bindings: map[string]semanticmodel.DimensionBinding{"ratings": {Field: "ratings.release_decade"}, "tags": {Field: "tags.release_decade"}}},
			"activity_date":  {Type: "timestamp", Grains: []string{"day", "week", "month", "quarter", "year"}, Bindings: map[string]semanticmodel.DimensionBinding{"ratings": {Field: "ratings.rated_at"}, "tags": {Field: "tags.tagged_at"}}},
		},
		Measures: map[string]semanticmodel.MetricMeasure{"rating_count": {Fact: "ratings"}, "tag_count": {Fact: "tags"}},
	}
	report := &reportdef.Dashboard{
		Visuals: map[string]reportdef.Visual{
			"decades": selectionFilterVisual([]reportdef.FieldRef{{Field: "release_decade", Alias: "label"}}, reportdef.QueryTime{}, []reportdef.SelectionMapping{{Field: "release_decade", Value: "label"}}),
			"buckets": selectionFilterVisual([]reportdef.FieldRef{{Field: "ratings.rating_bucket", Alias: "label"}}, reportdef.QueryTime{}, []reportdef.SelectionMapping{{Field: "ratings.rating_bucket", Fact: "ratings", Value: "label"}}),
			"months":  selectionFilterVisual(nil, reportdef.QueryTime{Field: "activity_date", Grain: "month", Alias: "label"}, []reportdef.SelectionMapping{{Field: "activity_date", Grain: "month", Value: "label"}}),
			"cross":   {Query: reportdef.VisualQuery{Measures: []reportdef.FieldRef{{Field: "rating_count"}, {Field: "tag_count"}}}},
		},
		Tables: map[string]reportdef.TableVisual{"plain_table": {Query: reportdef.TableQuery{Table: "ratings"}}},
	}
	return report, model
}

func selectionFilterVisual(dimensions []reportdef.FieldRef, queryTime reportdef.QueryTime, mappings []reportdef.SelectionMapping) reportdef.Visual {
	return reportdef.Visual{
		Query:       reportdef.VisualQuery{Dimensions: dimensions, Time: queryTime, Measures: []reportdef.FieldRef{{Field: "rating_count"}}},
		Interaction: reportdef.Interaction{PointSelection: reportdef.SelectionInteraction{Mappings: mappings, Targets: []string{"cross"}}},
	}
}

func filterSelection(source string, mapping dashboard.InteractionSelectionMapping) dashboard.InteractionSelection {
	return dashboard.InteractionSelection{SourceKind: "visual", SourceID: source, InteractionKind: "point_selection", Entries: []dashboard.InteractionSelectionEntry{{Mappings: []dashboard.InteractionSelectionMapping{mapping}}}}
}
