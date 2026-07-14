package materialize

import (
	"context"
	"errors"
	"math"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	semanticmodel "github.com/Yacobolo/libredash/internal/analytics/model"
	semanticquery "github.com/Yacobolo/libredash/internal/analytics/query"
	"github.com/Yacobolo/libredash/internal/dataquery"
)

func TestRuntimeProjectsAuthorizedCrossFactScalarFromCompleteGroupedBranch(t *testing.T) {
	database := &projectionBundleDatabase{}
	runtime := projectionBundleRuntime(database)
	governor := &projectionGovernor{}
	result, err := runtime.ExecuteDataQueryBundle(dataquery.WithGovernor(context.Background(), governor), projectionBundleRequests(360))
	if err != nil {
		t.Fatal(err)
	}
	if governor.calls.Load() != 2 {
		t.Fatalf("governance calls = %d, want both branches", governor.calls.Load())
	}
	if database.queries.Load() != 1 {
		t.Fatalf("physical queries = %d, want grouped source only", database.queries.Load())
	}
	rows := result.Results["tags_per_rating"].Rows
	if len(rows) != 1 || rows[0]["value"] != 0.4 {
		t.Fatalf("projected scalar rows = %#v, want 0.4", rows)
	}
	if got := len(result.Results["activity_by_month"].Rows); got != 2 {
		t.Fatalf("grouped rows = %d, want original two rows", got)
	}
	if _, err := runtime.ExecuteDataQueryBundle(dataquery.WithGovernor(context.Background(), governor), projectionBundleRequests(360)); err != nil {
		t.Fatal(err)
	}
	if database.queries.Load() != 1 {
		t.Fatalf("physical queries after warm repeat = %d, want projected results cached under ordinary branch keys", database.queries.Load())
	}
}

func TestRuntimeProjectionAuthorizesScalarSemanticFieldIndependently(t *testing.T) {
	database := &projectionBundleDatabase{}
	runtime := projectionBundleRuntime(database)
	governor := &projectionGovernor{denyScalar: true}
	_, err := runtime.ExecuteDataQueryBundle(dataquery.WithGovernor(context.Background(), governor), projectionBundleRequests(360))
	var branchErr *dataquery.BundleBranchError
	if !errors.As(err, &branchErr) || branchErr.ID != "tags_per_rating" {
		t.Fatalf("error = %v, want scalar branch denial", err)
	}
	if governor.calls.Load() != 1 {
		t.Fatalf("governance calls = %d, want scalar field independently checked first", governor.calls.Load())
	}
	if database.queries.Load() != 0 {
		t.Fatalf("physical queries = %d, want denial before SQL", database.queries.Load())
	}
}

func TestRuntimeProjectionRejectsDifferentGovernedRowScopes(t *testing.T) {
	database := &projectionBundleDatabase{}
	runtime := projectionBundleRuntime(database)
	governor := &projectionGovernor{scalarFilter: true}
	_, err := runtime.ExecuteDataQueryBundle(dataquery.WithGovernor(context.Background(), governor), projectionBundleRequests(360))
	if err == nil || !dataquery.IsBundleIncompatible(err) {
		t.Fatalf("error = %v, want incompatible governed scopes", err)
	}
	if governor.calls.Load() != 2 {
		t.Fatalf("governance calls = %d, want both branches", governor.calls.Load())
	}
	if database.queries.Load() != 0 {
		t.Fatalf("physical queries = %d, want rejection before SQL", database.queries.Load())
	}
}

func TestRuntimeProjectionRejectsDifferentGovernedMasks(t *testing.T) {
	database := &projectionBundleDatabase{}
	runtime := projectionBundleRuntime(database)
	governor := &projectionGovernor{scalarMask: true}
	_, err := runtime.ExecuteDataQueryBundle(dataquery.WithGovernor(context.Background(), governor), projectionBundleRequests(360))
	if err == nil || !dataquery.IsBundleIncompatible(err) {
		t.Fatalf("error = %v, want incompatible governed masks", err)
	}
	if database.queries.Load() != 0 {
		t.Fatalf("physical queries = %d, want mask mismatch rejection before SQL", database.queries.Load())
	}
}

func TestRuntimeProjectionOverfetchesToDetectTruncatedGroupedBranch(t *testing.T) {
	database := &projectionBundleDatabase{extraRow: true}
	runtime := projectionBundleRuntime(database)
	_, err := runtime.ExecuteDataQueryBundle(context.Background(), projectionBundleRequests(2))
	if err == nil || !dataquery.IsBundleIncompatible(err) {
		t.Fatalf("error = %v, want safe fallback for truncated source", err)
	}
	if database.lastLimit.Load() != 3 {
		t.Fatalf("grouped physical limit = %d, want authored limit+1", database.lastLimit.Load())
	}
}

func TestRuntimeProjectionAppliesBranchTransformsOnceAndCachesRawResults(t *testing.T) {
	database := &projectionBundleDatabase{}
	runtime := projectionBundleRuntime(database)
	governor := &projectionTransformGovernor{}
	for iteration := range 2 {
		result, err := runtime.ExecuteDataQueryBundle(dataquery.WithGovernor(context.Background(), governor), projectionBundleRequests(360))
		if err != nil {
			t.Fatal(err)
		}
		if got := result.Results["activity_by_month"].Rows[0]["value_0"]; got != int64(108) {
			t.Fatalf("iteration %d grouped transformed value = %#v, want raw 8 + 100", iteration, got)
		}
		if got := result.Results["tags_per_rating"].Rows[0]["value"]; got != 0.8 {
			t.Fatalf("iteration %d scalar transformed value = %#v, want raw 0.4 * 2", iteration, got)
		}
	}
	if database.queries.Load() != 1 {
		t.Fatalf("physical queries = %d, want one projection miss then raw branch-cache hits", database.queries.Load())
	}
	if governor.groupedTransforms.Load() != 2 || governor.scalarTransforms.Load() != 2 {
		t.Fatalf("transform calls grouped=%d scalar=%d, want one per branch response", governor.groupedTransforms.Load(), governor.scalarTransforms.Load())
	}
}

func TestRuntimeProjectionUsesMemberNameWhenScalarAliasIsEmpty(t *testing.T) {
	database := &projectionBundleDatabase{}
	runtime := projectionBundleRuntime(database)
	requests := projectionBundleRequests(360)
	requests[0].Query.Measures[0].Alias = ""
	result, err := runtime.ExecuteDataQueryBundle(context.Background(), requests)
	if err != nil {
		t.Fatal(err)
	}
	scalar := result.Results["tags_per_rating"]
	if len(scalar.Columns) != 1 || scalar.Columns[0].Name != "tags_per_rating" || scalar.Rows[0]["tags_per_rating"] != 0.4 {
		t.Fatalf("scalar result = %#v, want consistent member-name output", scalar)
	}
}

func TestRuntimeProjectionBundlesMultipleGroupedSourcesAndProjectsCrossFactAndNarrowScalars(t *testing.T) {
	database := &projectionBundleDatabase{}
	runtime := projectionBundleRuntime(database)
	governor := &projectionGovernor{}
	requests := multiProjectionBundleRequests(360, 360)
	result, err := runtime.ExecuteDataQueryBundle(dataquery.WithGovernor(context.Background(), governor), requests)
	if err != nil {
		t.Fatal(err)
	}
	if governor.calls.Load() != 4 {
		t.Fatalf("governance calls = %d, want every grouped and scalar branch independently checked", governor.calls.Load())
	}
	if database.queries.Load() != 1 {
		t.Fatalf("physical queries = %d, want grouped sources in one normal bundle", database.queries.Load())
	}
	if database.bundleQueries.Load() != 1 {
		t.Fatalf("physical grouped bundles = %d, want one", database.bundleQueries.Load())
	}
	if got := result.Results["tags_per_rating"].Rows; len(got) != 1 || got[0]["value"] != 0.4 {
		t.Fatalf("cross-fact scalar = %#v, want first authored complete source ratio 0.4", got)
	}
	if got := result.Results["rating_total"].Rows; len(got) != 1 || got[0]["value"] != int64(10) {
		t.Fatalf("narrow rating scalar = %#v, want projection from grouped atomic measure", got)
	}
	if len(result.Results["activity_by_month"].Rows) != 2 || len(result.Results["activity_by_year"].Rows) != 2 {
		t.Fatalf("grouped results = %#v", result.Results)
	}

	if _, err := runtime.ExecuteDataQueryBundle(dataquery.WithGovernor(context.Background(), governor), requests); err != nil {
		t.Fatal(err)
	}
	if database.queries.Load() != 1 {
		t.Fatalf("physical queries after warm repeat = %d, want raw results cached under all original branch keys", database.queries.Load())
	}
}

func TestRuntimeProjectionChoosesLaterCompleteGroupedSourceAndTrimsTruncatedOutput(t *testing.T) {
	database := &projectionBundleDatabase{bundleRowsByBranch: []int{3, 2}}
	runtime := projectionBundleRuntime(database)
	result, err := runtime.ExecuteDataQueryBundle(context.Background(), multiProjectionBundleRequests(2, 360))
	if err != nil {
		t.Fatal(err)
	}
	if got := len(result.Results["activity_by_month"].Rows); got != 2 {
		t.Fatalf("truncated grouped output rows = %d, want original authored limit", got)
	}
	if got := result.Results["tags_per_rating"].Rows; len(got) != 1 || got[0]["value"] != 0.5 {
		t.Fatalf("cross-fact scalar = %#v, want later complete grouped source ratio 0.5", got)
	}
	if got := result.Results["rating_total"].Rows; len(got) != 1 || got[0]["value"] != int64(100) {
		t.Fatalf("narrow rating scalar = %#v, want later complete grouped source total 100", got)
	}
	if database.lastLimit.Load() != 3 {
		t.Fatalf("first grouped physical limit = %d, want authored limit+1", database.lastLimit.Load())
	}
}

func TestRuntimeProjectionDoesNotTreatMaxIntGroupedLimitAsComplete(t *testing.T) {
	database := &projectionBundleDatabase{}
	runtime := projectionBundleRuntime(database)
	_, err := runtime.ExecuteDataQueryBundle(context.Background(), multiProjectionBundleRequests(math.MaxInt, math.MaxInt))
	if err == nil || !dataquery.IsBundleIncompatible(err) {
		t.Fatalf("error = %v, want fail-closed projection for non-overfetchable sources", err)
	}
}

func TestRuntimeMultiGroupedProjectionAppliesTransformsOnceAndCachesRawBranches(t *testing.T) {
	database := &projectionBundleDatabase{}
	runtime := projectionBundleRuntime(database)
	governor := &multiProjectionTransformGovernor{}
	requests := multiProjectionBundleRequests(360, 360)
	for iteration := range 2 {
		result, err := runtime.ExecuteDataQueryBundle(dataquery.WithGovernor(context.Background(), governor), requests)
		if err != nil {
			t.Fatal(err)
		}
		if got := result.Results["activity_by_month"].Rows[0]["value_0"]; got != int64(108) {
			t.Fatalf("iteration %d month transformed value = %#v, want raw 8 + 100", iteration, got)
		}
		if got := result.Results["activity_by_year"].Rows[0]["value_0"]; got != int64(160) {
			t.Fatalf("iteration %d year transformed value = %#v, want raw 60 + 100", iteration, got)
		}
		if got := result.Results["tags_per_rating"].Rows[0]["value"]; got != 0.8 {
			t.Fatalf("iteration %d ratio transformed value = %#v, want raw 0.4 * 2", iteration, got)
		}
		if got := result.Results["rating_total"].Rows[0]["value"]; got != int64(20) {
			t.Fatalf("iteration %d total transformed value = %#v, want raw 10 * 2", iteration, got)
		}
	}
	if database.queries.Load() != 1 {
		t.Fatalf("physical queries = %d, want one miss followed by raw original-branch cache hits", database.queries.Load())
	}
	if governor.groupedTransforms.Load() != 4 || governor.scalarTransforms.Load() != 4 {
		t.Fatalf("transform calls grouped=%d scalar=%d, want exactly once per branch response", governor.groupedTransforms.Load(), governor.scalarTransforms.Load())
	}
}

func TestRuntimeCanceledMultiGroupedProjectionDoesNotCacheResults(t *testing.T) {
	database := &cancelingProjectionBundleDatabase{
		projectionBundleDatabase: projectionBundleDatabase{},
		started:                  make(chan struct{}),
		release:                  make(chan struct{}),
	}
	runtime := projectionBundleRuntime(database)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := runtime.ExecuteDataQueryBundle(ctx, multiProjectionBundleRequests(360, 360))
		done <- err
	}()
	<-database.started
	cancel()
	close(database.release)
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled projection error = %v", err)
	}
	if _, err := runtime.ExecuteDataQueryBundle(context.Background(), multiProjectionBundleRequests(360, 360)); err != nil {
		t.Fatal(err)
	}
	if got := database.queries.Load(); got != 2 {
		t.Fatalf("physical queries = %d, want canceled miss plus uncached retry", got)
	}
}

type projectionGovernor struct {
	calls        atomic.Int32
	denyScalar   bool
	scalarFilter bool
	scalarMask   bool
}

type projectionTransformGovernor struct {
	groupedTransforms atomic.Int32
	scalarTransforms  atomic.Int32
}

type multiProjectionTransformGovernor struct {
	groupedTransforms atomic.Int32
	scalarTransforms  atomic.Int32
}

func (g *multiProjectionTransformGovernor) GovernDataQuery(_ context.Context, request dataquery.Query) (dataquery.Query, dataquery.ResultTransformer, error) {
	if len(request.Fields) == 0 && request.Time.Field == "" {
		return request, func(result *dataquery.Result, err error) error {
			g.scalarTransforms.Add(1)
			if err == nil && len(result.Rows) > 0 {
				switch value := result.Rows[0]["value"].(type) {
				case float64:
					result.Rows[0]["value"] = value * 2
				case int64:
					result.Rows[0]["value"] = value * 2
				}
			}
			return nil
		}, nil
	}
	return request, func(result *dataquery.Result, err error) error {
		g.groupedTransforms.Add(1)
		if err == nil && len(result.Rows) > 0 {
			result.Rows[0]["value_0"] = result.Rows[0]["value_0"].(int64) + 100
		}
		return nil
	}, nil
}

func (g *projectionTransformGovernor) GovernDataQuery(_ context.Context, request dataquery.Query) (dataquery.Query, dataquery.ResultTransformer, error) {
	if len(request.Measures) == 1 && request.Measures[0].Field == "tags_per_rating" {
		return request, func(result *dataquery.Result, err error) error {
			g.scalarTransforms.Add(1)
			if err == nil && len(result.Rows) > 0 {
				result.Rows[0]["value"] = result.Rows[0]["value"].(float64) * 2
			}
			return nil
		}, nil
	}
	return request, func(result *dataquery.Result, err error) error {
		g.groupedTransforms.Add(1)
		if err == nil && len(result.Rows) > 0 {
			result.Rows[0]["value_0"] = result.Rows[0]["value_0"].(int64) + 100
		}
		return nil
	}, nil
}

func (g *projectionGovernor) GovernDataQuery(_ context.Context, request dataquery.Query) (dataquery.Query, dataquery.ResultTransformer, error) {
	g.calls.Add(1)
	if len(request.Measures) == 1 && request.Measures[0].Field == "tags_per_rating" {
		if g.denyScalar {
			return dataquery.Query{}, nil, errors.New("metric denied")
		}
		if g.scalarFilter {
			request.Filters = append(request.Filters, dataquery.Filter{Field: "activity_date", Operator: "greater_than_or_equal", Values: []any{"2024-01-01"}})
		}
		if g.scalarMask {
			request.ColumnMasks = append(request.ColumnMasks, dataquery.ColumnMask{Field: "tag_count", Mask: "null"})
		}
	}
	return request, nil, nil
}

func projectionBundleRuntime(database Database) *Runtime {
	model := &semanticmodel.Model{
		Name: "movie_ratings",
		Tables: map[string]semanticmodel.Table{
			"ratings": {Dimensions: map[string]semanticmodel.MetricDimension{"rated_at": {Field: "ratings.rated_at", Table: "ratings", Name: "rated_at", Type: "timestamp"}}},
			"tags":    {Dimensions: map[string]semanticmodel.MetricDimension{"tagged_at": {Field: "tags.tagged_at", Table: "tags", Name: "tagged_at", Type: "timestamp"}}},
		},
		Dimensions: map[string]semanticmodel.SemanticDimension{
			"activity_date": {Name: "activity_date", Type: "timestamp", Grains: []string{"month", "year"}, Bindings: map[string]semanticmodel.DimensionBinding{
				"ratings": {Field: "ratings.rated_at"},
				"tags":    {Field: "tags.tagged_at"},
			}},
		},
		Measures: map[string]semanticmodel.MetricMeasure{
			"rating_count": {Fact: "ratings", Aggregation: "count", Empty: "zero"},
			"tag_count":    {Fact: "tags", Aggregation: "count", Empty: "zero"},
		},
		Metrics: map[string]semanticmodel.Metric{
			"tags_per_rating": {Expression: "safe_divide(${tag_count}, ${rating_count})"},
		},
	}
	return &Runtime{modelID: "movie_ratings", model: model, db: database, queryCache: newQueryResultCache(256, "projection")}
}

func projectionBundleRequests(limit int) []dataquery.BundleRequest {
	base := dataquery.Query{Surface: dataquery.SurfaceDashboard, Operation: dataquery.OperationDashboardAggregate, ModelID: "movie_ratings", Kind: dataquery.KindSemanticAggregate}
	scalar := base
	scalar.Measures = []dataquery.Field{{Field: "tags_per_rating", Alias: "value"}}
	grouped := base
	grouped.Time = dataquery.Time{Field: "activity_date", Grain: "month", Alias: "label"}
	grouped.Measures = []dataquery.Field{{Field: "rating_count", Alias: "value_0"}, {Field: "tag_count", Alias: "value_1"}}
	grouped.Sort = []dataquery.Sort{{Field: "label", Direction: "asc"}}
	grouped.Limit = limit
	return []dataquery.BundleRequest{{ID: "tags_per_rating", Query: scalar}, {ID: "activity_by_month", Query: grouped}}
}

func multiProjectionBundleRequests(monthLimit, yearLimit int) []dataquery.BundleRequest {
	base := dataquery.Query{Surface: dataquery.SurfaceDashboard, Operation: dataquery.OperationDashboardAggregate, ModelID: "movie_ratings", Kind: dataquery.KindSemanticAggregate}
	ratio := base
	ratio.Measures = []dataquery.Field{{Field: "tags_per_rating", Alias: "value"}}
	ratingTotal := base
	ratingTotal.Measures = []dataquery.Field{{Field: "rating_count", Alias: "value"}}
	month := base
	month.Time = dataquery.Time{Field: "activity_date", Grain: "month", Alias: "label"}
	month.Measures = []dataquery.Field{{Field: "rating_count", Alias: "value_0"}, {Field: "tag_count", Alias: "value_1"}}
	month.Sort = []dataquery.Sort{{Field: "label", Direction: "asc"}}
	month.Limit = monthLimit
	year := base
	year.Time = dataquery.Time{Field: "activity_date", Grain: "year", Alias: "label"}
	year.Measures = []dataquery.Field{{Field: "rating_count", Alias: "value_0"}, {Field: "tag_count", Alias: "value_1"}}
	year.Sort = []dataquery.Sort{{Field: "label", Direction: "asc"}}
	year.Limit = yearLimit
	return []dataquery.BundleRequest{
		{ID: "tags_per_rating", Query: ratio},
		{ID: "rating_total", Query: ratingTotal},
		{ID: "activity_by_month", Query: month},
		{ID: "activity_by_year", Query: year},
	}
}

type projectionBundleDatabase struct {
	cacheRuntimeDatabase
	queries            atomic.Int32
	bundleQueries      atomic.Int32
	lastLimit          atomic.Int64
	extraRow           bool
	bundleRowsByBranch []int
}

func (d *projectionBundleDatabase) Query(_ context.Context, plan semanticquery.Plan) (semanticquery.Rows, error) {
	d.queries.Add(1)
	if strings.Contains(plan.SQL, "LIMIT 3") {
		d.lastLimit.Store(3)
	}
	if len(plan.Columns) > 0 && plan.Columns[0] == semanticquery.BundleBranchColumn {
		d.bundleQueries.Add(1)
		rowCounts := d.bundleRowsByBranch
		if len(rowCounts) == 0 {
			rowCounts = []int{2, 2}
		}
		rows := semanticquery.Rows{}
		for ordinal, rowCount := range rowCounts {
			for rowIndex := 0; rowIndex < rowCount; rowIndex++ {
				row := semanticquery.Row{}
				for _, column := range plan.Columns {
					switch {
					case column == semanticquery.BundleBranchColumn:
						row[column] = int64(ordinal)
					case column == semanticquery.BundleRowColumn:
						row[column] = int64(rowIndex + 1)
					case strings.HasPrefix(column, "__d"):
						row[column] = "2024-01-01"
					case column == "__o0":
						row[column] = projectionBundleValue(ordinal, rowIndex, 0)
					case column == "__o1":
						row[column] = projectionBundleValue(ordinal, rowIndex, 1)
					default:
						row[column] = nil
					}
				}
				rows = append(rows, row)
			}
		}
		return rows, nil
	}
	rows := semanticquery.Rows{
		{"label": "2024-01-01", "value_0": int64(8), "value_1": int64(3)},
		{"label": "2024-02-01", "value_0": int64(2), "value_1": int64(1)},
	}
	if d.extraRow {
		rows = append(rows, semanticquery.Row{"label": "2024-03-01", "value_0": int64(1), "value_1": int64(1)})
	}
	return rows, nil
}

func projectionBundleValue(branch, row, member int) int64 {
	values := [][][]int64{
		{{8, 3}, {2, 1}, {1, 1}},
		{{60, 30}, {40, 20}, {10, 5}},
	}
	return values[branch][row][member]
}

type cancelingProjectionBundleDatabase struct {
	projectionBundleDatabase
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (d *cancelingProjectionBundleDatabase) Query(ctx context.Context, plan semanticquery.Plan) (semanticquery.Rows, error) {
	wait := false
	d.once.Do(func() {
		wait = true
		close(d.started)
	})
	if wait {
		<-d.release
	}
	return d.projectionBundleDatabase.Query(ctx, plan)
}
