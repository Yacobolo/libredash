package authz

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/Yacobolo/libredash/internal/access"
	accesssqlite "github.com/Yacobolo/libredash/internal/access/sqlite"
	semanticmodel "github.com/Yacobolo/libredash/internal/analytics/model"
	semanticquery "github.com/Yacobolo/libredash/internal/analytics/query"
	"github.com/Yacobolo/libredash/internal/dataquery"
	"github.com/Yacobolo/libredash/internal/platform"
	"github.com/Yacobolo/libredash/internal/queryruntime"
	"github.com/Yacobolo/libredash/internal/workspace"
	workspacesqlite "github.com/Yacobolo/libredash/internal/workspace/sqlite"
)

type semanticModelMetrics struct {
	queryruntime.Metrics
	model *semanticmodel.Model
}

func (m semanticModelMetrics) SemanticModel(modelID string) (*semanticmodel.Model, bool) {
	return m.model, modelID == m.model.Name
}

func TestGovernModelAggregateUsesTransitivePhysicalPolicies(t *testing.T) {
	ctx := context.Background()
	store, err := platform.Open(ctx, filepath.Join(t.TempDir(), "libredash.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := workspacesqlite.NewRepository(store.SQLDB()).Ensure(ctx, workspace.EnsureInput{ID: "test", Title: "Test"}); err != nil {
		t.Fatal(err)
	}
	repo := accesssqlite.NewRepository(store.SQLDB())
	principal, err := repo.UpsertPrincipal(ctx, access.PrincipalInput{ID: "analyst", Email: "analyst@example.com"})
	if err != nil {
		t.Fatal(err)
	}

	model := governanceTestModel()
	modelObject := access.ItemObject(access.SecurableSemanticModel, "test", model.Name)
	ratingsDataset := access.ItemObjectWithParent(access.SecurableDataset, "test", model.Name+"/ratings", modelObject)
	tagsDataset := access.ItemObjectWithParent(access.SecurableDataset, "test", model.Name+"/tags", modelObject)
	ratingColumn := access.ItemObjectWithParent(access.SecurableColumn, "test", model.Name+"/ratings/rating", ratingsDataset)
	for _, object := range []access.ObjectRef{
		modelObject,
		access.ItemObjectWithParent(access.SecurableSemanticField, "test", model.Name+"/average_value", modelObject),
		access.ItemObjectWithParent(access.SecurableSemanticField, "test", model.Name+"/rating_sum", modelObject),
		access.ItemObjectWithParent(access.SecurableSemanticField, "test", model.Name+"/rating_count", modelObject),
		access.ItemObjectWithParent(access.SecurableSemanticField, "test", model.Name+"/tag_count", modelObject),
		ratingsDataset,
		tagsDataset,
		ratingColumn,
	} {
		if _, err := repo.UpsertSecurableObject(ctx, object, ""); err != nil {
			t.Fatalf("upsert %s: %v", object.CanonicalID(), err)
		}
	}
	if _, err := repo.CreateGrant(ctx, access.GrantInput{
		Object: modelObject, SubjectType: access.SubjectPrincipal, SubjectID: principal.ID, Privilege: access.PrivilegeQueryData,
	}); err != nil {
		t.Fatal(err)
	}
	rowFilter, _ := json.Marshal(map[string]any{"field": "ratings.status", "operator": "equals", "values": []any{"published"}})
	if _, err := repo.UpsertDataPolicy(ctx, access.DataPolicyInput{Object: ratingsDataset, PolicyType: "row_filter", ExpressionJSON: string(rowFilter)}); err != nil {
		t.Fatal(err)
	}
	columnMask, _ := json.Marshal(map[string]any{"field": "ratings.rating", "mask": "null"})
	if _, err := repo.UpsertDataPolicy(ctx, access.DataPolicyInput{Object: ratingColumn, PolicyType: "column_mask", ExpressionJSON: string(columnMask)}); err != nil {
		t.Fatal(err)
	}

	metrics := New(semanticModelMetrics{model: model}, Options{
		Repo: repo,
		PrincipalFromContext: func(context.Context) (Principal, bool) {
			return Principal{ID: principal.ID}, true
		},
		DefaultWorkspaceID: "test",
	})
	request := dataquery.Query{
		WorkspaceID: "test",
		ModelID:     model.Name,
		Kind:        dataquery.KindSemanticAggregate,
		Measures: []dataquery.Field{
			{Field: "average_value", Alias: "average_value"},
			{Field: "tag_count", Alias: "tag_count"},
		},
	}
	governed, _, err := metrics.GovernDataQuery(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if len(governed.Filters) != 1 || governed.Filters[0].Fact != "ratings" {
		t.Fatalf("governed filters = %#v, want ratings-targeted row policy", governed.Filters)
	}
	if len(governed.ColumnMasks) != 1 || governed.ColumnMasks[0].Field != "ratings.rating" {
		t.Fatalf("governed masks = %#v, want transitive rating mask", governed.ColumnMasks)
	}

	_, err = semanticquery.NewPlanner(model).Plan(semanticquery.Request{
		Measures: []semanticquery.Field{{Field: "average_value"}, {Field: "tag_count"}},
		Filters:  dataFiltersToSemanticFilters(governed.Filters),
		ColumnMasks: []semanticquery.ColumnMask{{
			Field: governed.ColumnMasks[0].Field,
			Mask:  governed.ColumnMasks[0].Mask,
		}},
	})
	if err == nil {
		t.Fatal("masked metric dependency was accepted by the aggregate planner")
	}
}

func TestGovernDashboardCountUsesAuthorizationProjectionPolicies(t *testing.T) {
	ctx := context.Background()
	store, err := platform.Open(ctx, filepath.Join(t.TempDir(), "libredash.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := workspacesqlite.NewRepository(store.SQLDB()).Ensure(ctx, workspace.EnsureInput{ID: "test", Title: "Test"}); err != nil {
		t.Fatal(err)
	}
	repo := accesssqlite.NewRepository(store.SQLDB())
	principal, err := repo.UpsertPrincipal(ctx, access.PrincipalInput{ID: "analyst", Email: "analyst@example.com"})
	if err != nil {
		t.Fatal(err)
	}

	model := governanceTestModel()
	modelObject := access.ItemObject(access.SecurableSemanticModel, "test", model.Name)
	dataset := access.ItemObjectWithParent(access.SecurableDataset, "test", model.Name+"/ratings", modelObject)
	column := access.ItemObjectWithParent(access.SecurableColumn, "test", model.Name+"/ratings/rating", dataset)
	for _, object := range []access.ObjectRef{modelObject, dataset, column} {
		if _, err := repo.UpsertSecurableObject(ctx, object, ""); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := repo.CreateGrant(ctx, access.GrantInput{
		Object: dataset, SubjectType: access.SubjectPrincipal, SubjectID: principal.ID, Privilege: access.PrivilegeQueryData,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.UpsertDataPolicy(ctx, access.DataPolicyInput{
		Object: column, PolicyType: "row_filter", ExpressionJSON: `{"field":"ratings.status","operator":"equals","values":["published"]}`,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.UpsertDataPolicy(ctx, access.DataPolicyInput{
		Object: column, PolicyType: "column_mask", ExpressionJSON: `{"field":"ratings.rating","mask":"null"}`,
	}); err != nil {
		t.Fatal(err)
	}

	metrics := New(semanticModelMetrics{model: model}, Options{
		Repo: repo,
		PrincipalFromContext: func(context.Context) (Principal, bool) {
			return Principal{ID: principal.ID}, true
		},
		DefaultWorkspaceID: "test",
	})
	governed, _, err := metrics.GovernDataQuery(ctx, dataquery.Query{
		Surface: dataquery.SurfaceDashboard, Operation: dataquery.OperationDashboardCount,
		WorkspaceID: "test", ModelID: model.Name, Kind: dataquery.KindSemanticRows, Target: "ratings", IncludeTotal: true,
		AuthorizationFields: []dataquery.Field{{Field: "ratings.rating", Alias: "rating"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(governed.Filters) != 1 || governed.Filters[0].Field != "ratings.status" {
		t.Fatalf("governed count filters = %#v", governed.Filters)
	}
	if len(governed.ColumnMasks) != 1 || governed.ColumnMasks[0].Field != "ratings.rating" {
		t.Fatalf("governed count masks = %#v", governed.ColumnMasks)
	}
}

func governanceTestModel() *semanticmodel.Model {
	return &semanticmodel.Model{
		Name: "activity",
		Tables: map[string]semanticmodel.Table{
			"ratings": {Dimensions: map[string]semanticmodel.MetricDimension{
				"rating": {Type: "number"},
				"status": {Type: "string"},
			}},
			"tags": {Dimensions: map[string]semanticmodel.MetricDimension{
				"tag": {Type: "string"},
			}},
		},
		Measures: map[string]semanticmodel.MetricMeasure{
			"rating_sum":   {Fact: "ratings", Aggregation: "sum", Input: semanticmodel.MeasureInput{Field: "ratings.rating"}, Empty: "null"},
			"rating_count": {Fact: "ratings", Aggregation: "count", Empty: "zero"},
			"tag_count":    {Fact: "tags", Aggregation: "count", Empty: "zero"},
		},
		Metrics: map[string]semanticmodel.Metric{
			"average_value": {Expression: "safe_divide(${rating_sum}, ${rating_count})"},
		},
	}
}
