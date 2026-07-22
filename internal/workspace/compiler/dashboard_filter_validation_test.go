package compiler

import (
	"testing"

	semanticmodel "github.com/Yacobolo/leapview/internal/analytics/model"
	"github.com/Yacobolo/leapview/internal/dashboard"
	"github.com/Yacobolo/leapview/internal/dashboard/report"
)

func TestValidateDashboardPreservesFactOnLocalFilterForMultiFactTarget(t *testing.T) {
	model := &semanticmodel.Model{
		Name: "model",
		Tables: map[string]semanticmodel.Table{
			"ratings": {Dimensions: map[string]semanticmodel.MetricDimension{
				"rating_bucket": {Type: "number"},
			}},
			"tags": {},
		},
		Measures: map[string]semanticmodel.MetricMeasure{
			"rating_count": {Fact: "ratings", Aggregation: "count", Empty: "zero"},
			"tag_count":    {Fact: "tags", Aggregation: "count", Empty: "zero"},
		},
		Metrics: map[string]semanticmodel.Metric{
			"tags_per_rating": {Expression: "safe_divide(${tag_count}, ${rating_count})"},
		},
	}
	dashboardDefinition := &report.Dashboard{
		ID: "dashboard", Title: "Dashboard", SemanticModel: "model",
		Filters: map[string]report.FilterDefinition{
			"rating_bucket": {
				Type: "multi_select", Label: "Rating", Dimension: "ratings.rating_bucket",
				Fact: "ratings", Operator: "in",
			},
		},
		Visuals: map[string]report.Visual{
			"target": {
				Kind:  "kpi",
				Shape: "single_value",
				Query: report.VisualQuery{Measures: []report.FieldRef{{Field: "tags_per_rating"}}},
			},
		},
		Pages: []dashboard.Page{{ID: "overview", Title: "Overview"}},
	}

	if err := ValidateDashboard(dashboardDefinition, map[string]*semanticmodel.Model{"model": model}); err != nil {
		t.Fatalf("ValidateDashboard() error = %v", err)
	}
}
