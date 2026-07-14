package materialize

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	semanticmodel "github.com/Yacobolo/libredash/internal/analytics/model"
	semanticquery "github.com/Yacobolo/libredash/internal/analytics/query"
	"github.com/Yacobolo/libredash/internal/dataquery"
	"github.com/Yacobolo/libredash/internal/execution"
)

func TestQueryResultCacheUsesGovernedRequestAndReturnsDeepCopies(t *testing.T) {
	cache := newQueryResultCache(256, "")
	request := dataquery.Query{
		ModelID: "sales", Kind: dataquery.KindSemanticAggregate, Target: "orders",
		Operation:   dataquery.OperationDashboardFilterOptions,
		Fields:      []dataquery.Field{{Field: "orders.state", Alias: "value"}},
		ColumnMasks: []dataquery.ColumnMask{{Field: "orders.state", Mask: "redact"}},
	}
	var calls atomic.Int32
	execute := func() (dataquery.Result, error) {
		calls.Add(1)
		return dataquery.Result{Rows: []dataquery.Row{{"value": "SP"}}}, nil
	}
	first, err := cache.execute(context.Background(), request, execute)
	if err != nil {
		t.Fatal(err)
	}
	if first.CacheOutcome != dataquery.CacheMiss {
		t.Fatalf("first cache outcome = %q, want miss", first.CacheOutcome)
	}
	first.Rows[0]["value"] = "mutated"
	request.PrincipalID = "another-user"
	request.RequestID = "request-2"
	request.CorrelationID = "refresh-2"
	second, err := cache.execute(context.Background(), request, execute)
	if err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 {
		t.Fatalf("physical executions = %d, want 1", calls.Load())
	}
	if second.CacheOutcome != dataquery.CacheHit {
		t.Fatalf("second cache outcome = %q, want hit", second.CacheOutcome)
	}
	if second.Rows[0]["value"] != "SP" {
		t.Fatalf("cached result was aliased: %#v", second.Rows)
	}

	request.ColumnMasks[0].Mask = "null"
	if _, err := cache.execute(context.Background(), request, execute); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 {
		t.Fatalf("different governed request executions = %d, want 2", calls.Load())
	}
}

func TestQueryResultCacheEnforcesByteBudgetAndRejectsOversizedEntries(t *testing.T) {
	cache := newQueryResultCacheWithLimits(10, 1200, "bytes")
	first := dataquery.Query{ModelID: "sales", Kind: dataquery.KindSemanticAggregate, Measures: []dataquery.Field{{Field: "revenue"}}}
	second := first
	second.Measures = []dataquery.Field{{Field: "orders"}}
	large := first
	large.Measures = []dataquery.Field{{Field: "large"}}

	_, firstKey, generation, _, err := cache.lookup(first)
	if err != nil {
		t.Fatal(err)
	}
	cache.store(firstKey, generation, dataquery.Result{Rows: []dataquery.Row{{"value": strings.Repeat("a", 80)}}})
	_, secondKey, generation, _, err := cache.lookup(second)
	if err != nil {
		t.Fatal(err)
	}
	cache.store(secondKey, generation, dataquery.Result{Rows: []dataquery.Row{{"value": strings.Repeat("b", 80)}}})
	if cache.currentBytes > cache.maxBytes {
		t.Fatalf("cache bytes = %d, budget = %d", cache.currentBytes, cache.maxBytes)
	}
	if cache.lru.Len() != 1 {
		t.Fatalf("entries = %d, want byte-budget eviction", cache.lru.Len())
	}

	_, largeKey, generation, _, err := cache.lookup(large)
	if err != nil {
		t.Fatal(err)
	}
	cache.store(largeKey, generation, dataquery.Result{Rows: []dataquery.Row{{"value": strings.Repeat("x", 5000)}}})
	if _, ok := cache.get(largeKey); ok {
		t.Fatal("oversized result was cached")
	}
}

func TestQueryResultCacheKeyIncludesRawValueField(t *testing.T) {
	cache := newQueryResultCache(256, "")
	request := dataquery.Query{
		Surface: dataquery.SurfaceDashboard, Operation: dataquery.OperationDashboardHistogram,
		ModelID: "sales", Kind: dataquery.KindSemanticHistogram, Target: "orders",
		Value: dataquery.Field{Field: "order_total", Alias: "value"}, BinCount: 20,
	}
	var calls atomic.Int32
	execute := func() (dataquery.Result, error) {
		calls.Add(1)
		return dataquery.Result{Rows: []dataquery.Row{{"bucket": 0}}}, nil
	}
	if _, err := cache.execute(context.Background(), request, execute); err != nil {
		t.Fatal(err)
	}
	request.Value = dataquery.Field{Field: "shipping_total", Alias: "value"}
	if _, err := cache.execute(context.Background(), request, execute); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 {
		t.Fatalf("physical executions = %d, want distinct entries for raw value fields", calls.Load())
	}
}

func TestQueryResultCacheKeyIncludesAuthorizationProjection(t *testing.T) {
	cache := newQueryResultCache(256, "")
	request := dataquery.Query{
		Surface: dataquery.SurfaceDashboard, Operation: dataquery.OperationDashboardCount,
		ModelID: "sales", Kind: dataquery.KindSemanticRows, Target: "orders", IncludeTotal: true,
		AuthorizationFields: []dataquery.Field{{Field: "orders.customer_email"}},
	}
	first, _, err := cache.cacheKey(request)
	if err != nil {
		t.Fatal(err)
	}
	request.AuthorizationFields = []dataquery.Field{{Field: "orders.customer_id"}}
	second, _, err := cache.cacheKey(request)
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("count cache key ignored its authorization projection")
	}
}

func TestQueryResultCacheKeyIncludesRuntimeNamespace(t *testing.T) {
	request := dataquery.Query{ModelID: "sales", Kind: dataquery.KindSemanticAggregate}
	first, _, err := newQueryResultCache(256, "snapshot=1;source=old").cacheKey(request)
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := newQueryResultCache(256, "snapshot=2;source=new").cacheKey(request)
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("cache keys matched across snapshot/source namespaces")
	}
}

func TestDashboardResultCacheEligibility(t *testing.T) {
	for _, operation := range []string{
		dataquery.OperationDashboardAggregate,
		dataquery.OperationDashboardRows,
		dataquery.OperationDashboardCount,
		dataquery.OperationDashboardHistogram,
		dataquery.OperationDashboardDistribution,
		dataquery.OperationDashboardFilterOptions,
	} {
		request := dataquery.Query{Surface: dataquery.SurfaceDashboard, Operation: operation}
		if !dashboardQueryResultCacheable(request) {
			t.Errorf("operation %q was not cacheable", operation)
		}
	}
	for _, request := range []dataquery.Query{
		{Surface: dataquery.SurfaceAPI, Operation: dataquery.OperationDashboardAggregate},
		{Surface: dataquery.SurfaceDashboard, Operation: dataquery.OperationAPIQuery},
		{Operation: dataquery.OperationDashboardAggregate},
	} {
		if dashboardQueryResultCacheable(request) {
			t.Errorf("non-dashboard request was cacheable: %#v", request)
		}
	}
}

func TestQueryResultCacheDoesNotCacheErrorsAndInvalidatesGeneration(t *testing.T) {
	cache := newQueryResultCache(1, "")
	request := dataquery.Query{ModelID: "sales", Kind: dataquery.KindSemanticAggregate, Target: "orders"}
	var calls atomic.Int32
	execute := func() (dataquery.Result, error) {
		if calls.Add(1) == 1 {
			return dataquery.Result{}, errors.New("temporary")
		}
		return dataquery.Result{Rows: []dataquery.Row{{"value": "SP"}}}, nil
	}
	if _, err := cache.execute(context.Background(), request, execute); err == nil {
		t.Fatal("first cache execution error = nil")
	}
	if _, err := cache.execute(context.Background(), request, execute); err != nil {
		t.Fatal(err)
	}
	cache.clear()
	if _, err := cache.execute(context.Background(), request, execute); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 3 {
		t.Fatalf("physical executions after error and clear = %d, want 3", calls.Load())
	}
}

func TestQueryResultCacheLiveWaiterRetriesCanceledFlightAndCachesResult(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		cache := newQueryResultCache(256, "")
		request := dataquery.Query{
			ModelID: "sales", Kind: dataquery.KindSemanticAggregate, Target: "orders",
			Operation: dataquery.OperationDashboardFilterOptions,
		}

		key, generation, err := cache.cacheKey(request)
		if err != nil {
			t.Fatal(err)
		}
		flightStarted := make(chan struct{})
		releaseCanceledFlight := make(chan struct{})
		cache.group.DoChan(fmt.Sprintf("%d:%s", generation, key), func() (any, error) {
			close(flightStarted)
			<-releaseCanceledFlight
			return dataquery.Result{}, canceledQueryCacheFlightError{err: context.Canceled}
		})
		<-flightStarted

		var physicalExecutions atomic.Int32
		secondResult := make(chan dataquery.Result, 1)
		secondError := make(chan error, 1)
		go func() {
			result, executeErr := cache.execute(context.Background(), request, func() (dataquery.Result, error) {
				physicalExecutions.Add(1)
				return dataquery.Result{Rows: []dataquery.Row{{"value": "SP"}}}, nil
			})
			secondResult <- result
			secondError <- executeErr
		}()
		synctest.Wait()

		close(releaseCanceledFlight)
		if err := <-secondError; err != nil {
			t.Fatalf("live waiter inherited canceled flight: %v", err)
		}
		result := <-secondResult
		if result.CacheOutcome != dataquery.CacheMiss {
			t.Fatalf("live waiter cache outcome = %q, want miss", result.CacheOutcome)
		}
		if physicalExecutions.Load() != 1 {
			t.Fatalf("live waiter physical executions = %d, want 1", physicalExecutions.Load())
		}

		cached, err := cache.execute(context.Background(), request, func() (dataquery.Result, error) {
			physicalExecutions.Add(1)
			return dataquery.Result{}, nil
		})
		if err != nil {
			t.Fatal(err)
		}
		if cached.CacheOutcome != dataquery.CacheHit {
			t.Fatalf("follow-up cache outcome = %q, want hit", cached.CacheOutcome)
		}
		if physicalExecutions.Load() != 1 {
			t.Fatalf("physical executions after cache hit = %d, want 1", physicalExecutions.Load())
		}
		if generation != cache.generation {
			t.Fatalf("cache generation changed during flight: got %d, want %d", cache.generation, generation)
		}
	})
}

func TestObserveQueryCacheOutcomeReportsSuccessAndError(t *testing.T) {
	observed := []string{}
	ctx := dataquery.WithCacheOutcomeObserver(context.Background(), func(outcome string) {
		observed = append(observed, outcome)
	})

	for _, outcome := range []string{dataquery.CacheHit, dataquery.CacheMiss, dataquery.CacheCoalesced} {
		observeQueryCacheOutcome(ctx, dataquery.Result{CacheOutcome: outcome}, nil)
	}
	observeQueryCacheOutcome(ctx, dataquery.Result{}, errors.New("temporary"))

	want := []string{dataquery.CacheHit, dataquery.CacheMiss, dataquery.CacheCoalesced, dataquery.CacheError}
	if len(observed) != len(want) {
		t.Fatalf("observed cache outcomes = %#v, want %#v", observed, want)
	}
	for index := range want {
		if observed[index] != want[index] {
			t.Fatalf("observed cache outcomes = %#v, want %#v", observed, want)
		}
	}
}

func TestRuntimeCountsFilterOptionCacheMissAsPhysicalAndHitAsZero(t *testing.T) {
	runtime := &Runtime{
		modelID: "sales",
		model: &semanticmodel.Model{Name: "sales", Tables: map[string]semanticmodel.Table{
			"orders": {Columns: map[string]semanticmodel.ModelColumn{"id": {Name: "id"}}},
		}},
		db:         cacheRuntimeDatabase{},
		queryCache: newQueryResultCache(256, ""),
	}
	physicalQueries := 0
	cacheOutcomes := []string{}
	ctx := dataquery.WithPhysicalQueryObserver(context.Background(), func(observation dataquery.PhysicalQueryObservation) {
		physicalQueries += observation.Count
	})
	ctx = dataquery.WithCacheOutcomeObserver(ctx, func(outcome string) { cacheOutcomes = append(cacheOutcomes, outcome) })
	request := dataquery.Query{
		Surface: dataquery.SurfaceDashboard,
		ModelID: "sales", Kind: dataquery.KindModelTableRows, Target: "orders",
		Operation: dataquery.OperationDashboardFilterOptions,
		Fields:    []dataquery.Field{{Field: "id"}},
		Limit:     50,
	}

	if _, err := runtime.ExecuteDataQuery(ctx, request); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.ExecuteDataQuery(ctx, request); err != nil {
		t.Fatal(err)
	}

	if physicalQueries != 1 {
		t.Fatalf("physical queries = %d, want 1 miss and zero for hit", physicalQueries)
	}
	wantOutcomes := []string{dataquery.CacheMiss, dataquery.CacheHit}
	if len(cacheOutcomes) != len(wantOutcomes) || cacheOutcomes[0] != wantOutcomes[0] || cacheOutcomes[1] != wantOutcomes[1] {
		t.Fatalf("cache outcomes = %#v, want %#v", cacheOutcomes, wantOutcomes)
	}
}

func TestRuntimeCachesGovernedDashboardQueriesAndToggleBackExecutesZeroSQL(t *testing.T) {
	database := &countingCacheRuntimeDatabase{}
	runtime := &Runtime{
		modelID: "sales",
		model: &semanticmodel.Model{Name: "sales", Tables: map[string]semanticmodel.Table{
			"orders": {
				Columns:    map[string]semanticmodel.ModelColumn{"id": {Name: "id"}},
				Dimensions: map[string]semanticmodel.MetricDimension{"id": {Label: "ID"}},
			},
		}},
		db:         database,
		queryCache: newQueryResultCache(256, ""),
	}
	base := dataquery.Query{
		Surface: dataquery.SurfaceDashboard, Operation: dataquery.OperationDashboardAggregate,
		ModelID: "sales", Kind: dataquery.KindSemanticAggregate, Target: "orders",
		Fields: []dataquery.Field{{Field: "orders.id", Alias: "id"}}, Limit: 50,
	}
	selected := base
	selected.Filters = []dataquery.Filter{{Field: "orders.id", Operator: "equals", Values: []any{42}}}

	for _, request := range []dataquery.Query{selected, base, selected} {
		if _, err := runtime.ExecuteDataQuery(context.Background(), request); err != nil {
			t.Fatal(err)
		}
	}
	if got := database.queries.Load(); got != 2 {
		t.Fatalf("physical executions = %d, want selection miss + clear miss + toggle-back hit", got)
	}

	selected.Filters[0].Values[0] = 43
	if _, err := runtime.ExecuteDataQuery(context.Background(), selected); err != nil {
		t.Fatal(err)
	}
	if got := database.queries.Load(); got != 3 {
		t.Fatalf("physical executions after governed filter change = %d, want 3", got)
	}

	runtime.ClearQueryCache()
	selected.Filters[0].Values[0] = 42
	if _, err := runtime.ExecuteDataQuery(context.Background(), selected); err != nil {
		t.Fatal(err)
	}
	if got := database.queries.Load(); got != 4 {
		t.Fatalf("physical executions after snapshot generation invalidation = %d, want 4", got)
	}
}

func TestRuntimeBundleCacheAllHitExecutesZeroAdditionalSQL(t *testing.T) {
	database := &bundleCountingDatabase{}
	runtime := bundleCacheRuntime(database)
	requests := bundleCacheRequests()
	if _, err := runtime.ExecuteDataQueryBundle(context.Background(), requests); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.ExecuteDataQueryBundle(context.Background(), requests); err != nil {
		t.Fatal(err)
	}
	if got := database.queries.Load(); got != 1 {
		t.Fatalf("physical executions = %d, want one bundle miss and zero for all-hit", got)
	}
}

func TestRuntimeBundleRejectsNonDashboardBranchesBeforeFlightCoalescing(t *testing.T) {
	database := &bundleCountingDatabase{}
	runtime := bundleCacheRuntime(database)
	requests := bundleCacheRequests()
	for i := range requests {
		requests[i].Query.Surface = dataquery.SurfaceAPI
		requests[i].Query.Operation = dataquery.OperationAPIQuery
	}
	_, err := runtime.ExecuteDataQueryBundle(context.Background(), requests)
	if err == nil || !dataquery.IsBundleIncompatible(err) {
		t.Fatalf("error = %v, want incompatible non-dashboard bundle", err)
	}
	if database.queries.Load() != 0 {
		t.Fatalf("physical executions = %d, want fail before flight", database.queries.Load())
	}
}

func TestRuntimeBundleCacheMixedHitExecutesOnlyLoneMiss(t *testing.T) {
	database := &bundleCountingDatabase{}
	runtime := bundleCacheRuntime(database)
	requests := bundleCacheRequests()
	if _, err := runtime.ExecuteDataQuery(context.Background(), requests[0].Query); err != nil {
		t.Fatal(err)
	}
	result, err := runtime.ExecuteDataQueryBundle(context.Background(), requests)
	if err != nil {
		t.Fatal(err)
	}
	if got := database.queries.Load(); got != 2 {
		t.Fatalf("physical executions = %d, want prime plus lone miss", got)
	}
	if result.Results[requests[0].ID].CacheOutcome != dataquery.CacheHit {
		t.Fatalf("first branch outcome = %q", result.Results[requests[0].ID].CacheOutcome)
	}
}

func TestRuntimeBundleCanceledExecutionDoesNotCacheOrAuditSuccess(t *testing.T) {
	database := &cancelIgnoringBundleDatabase{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	runtime := bundleCacheRuntime(database)
	governor := &bundleAuditGovernor{}
	ctx, cancel := context.WithCancel(dataquery.WithGovernor(context.Background(), governor))
	done := make(chan error, 1)
	go func() {
		_, err := runtime.ExecuteDataQueryBundle(ctx, bundleCacheRequests())
		done <- err
	}()
	<-database.started
	cancel()
	close(database.release)
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled bundle error = %v", err)
	}
	if governor.successes.Load() != 0 {
		t.Fatalf("canceled bundle recorded %d successful branches", governor.successes.Load())
	}
	if _, err := runtime.ExecuteDataQueryBundle(dataquery.WithGovernor(context.Background(), governor), bundleCacheRequests()); err != nil {
		t.Fatal(err)
	}
	if got := database.queries.Load(); got != 2 {
		t.Fatalf("physical executions = %d, want canceled miss plus uncached retry", got)
	}
}

func TestQueryResultCacheCoalescesExactBundleFlightsAndRetriesCanceledOwner(t *testing.T) {
	cache := newQueryResultCache(256, "bundle")
	ownerCtx, cancelOwner := context.WithCancel(context.Background())
	started := make(chan struct{})
	release := make(chan struct{})
	var executions atomic.Int32
	var startedOnce sync.Once
	execute := func(ctx context.Context) (any, error) {
		executions.Add(1)
		startedOnce.Do(func() { close(started) })
		<-release
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return "fresh", nil
	}
	ownerDone := make(chan error, 1)
	go func() {
		_, _, err := cache.coalesce(ownerCtx, "exact-bundle", func() (any, error) { return execute(ownerCtx) })
		ownerDone <- err
	}()
	<-started
	waiterDone := make(chan error, 1)
	go func() {
		result, _, err := cache.coalesce(context.Background(), "exact-bundle", func() (any, error) { return execute(context.Background()) })
		if err == nil && result != "fresh" {
			err = fmt.Errorf("coalesced result = %v", result)
		}
		waiterDone <- err
	}()
	cancelOwner()
	close(release)
	if err := <-ownerDone; !errors.Is(err, context.Canceled) {
		t.Fatalf("owner error = %v", err)
	}
	if err := <-waiterDone; err != nil {
		t.Fatalf("live waiter inherited canceled owner: %v", err)
	}
	if got := executions.Load(); got != 2 {
		t.Fatalf("executions = %d, want canceled owner plus one live replacement", got)
	}
}

type bundleAuditGovernor struct {
	successes atomic.Int32
	failures  atomic.Int32
}

func (g *bundleAuditGovernor) GovernDataQuery(_ context.Context, request dataquery.Query) (dataquery.Query, dataquery.ResultTransformer, error) {
	return request, func(_ *dataquery.Result, err error) error {
		if err == nil {
			g.successes.Add(1)
		} else {
			g.failures.Add(1)
		}
		return nil
	}, nil
}

type cancelIgnoringBundleDatabase struct {
	bundleCountingDatabase
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (d *cancelIgnoringBundleDatabase) Query(ctx context.Context, plan semanticquery.Plan) (semanticquery.Rows, error) {
	d.once.Do(func() {
		close(d.started)
		<-d.release
	})
	return d.bundleCountingDatabase.Query(ctx, plan)
}

func TestRuntimeBundleGovernsEveryBranchAndFailsClosedOnMask(t *testing.T) {
	database := &bundleCountingDatabase{}
	runtime := bundleCacheRuntime(database)
	governor := &bundleMaskGovernor{}
	_, err := runtime.ExecuteDataQueryBundle(dataquery.WithGovernor(context.Background(), governor), bundleCacheRequests())
	if err == nil || !dataquery.IsBundleIncompatible(err) {
		t.Fatalf("error = %v, want incompatible masked bundle", err)
	}
	if governor.calls.Load() != 2 {
		t.Fatalf("governance calls = %d, want every branch", governor.calls.Load())
	}
	if database.queries.Load() != 0 {
		t.Fatalf("physical executions = %d, want fail before SQL", database.queries.Load())
	}
}

type bundleMaskGovernor struct{ calls atomic.Int32 }

func (g *bundleMaskGovernor) GovernDataQuery(_ context.Context, request dataquery.Query) (dataquery.Query, dataquery.ResultTransformer, error) {
	if g.calls.Add(1) == 2 {
		request.ColumnMasks = []dataquery.ColumnMask{{Field: "orders.secret", Mask: "redact"}}
	}
	return request, nil, nil
}

func bundleCacheRuntime(database Database) *Runtime {
	return &Runtime{modelID: "sales", model: &semanticmodel.Model{Name: "sales", Tables: map[string]semanticmodel.Table{"orders": {}}, Measures: map[string]semanticmodel.MetricMeasure{
		"order_count": {Fact: "orders", Aggregation: "count", Empty: "zero"},
		"event_count": {Fact: "orders", Aggregation: "count", Empty: "zero"},
	}}, db: database, queryCache: newQueryResultCache(256, "bundle-test")}
}

func bundleCacheRequests() []dataquery.BundleRequest {
	base := dataquery.Query{Surface: dataquery.SurfaceDashboard, Operation: dataquery.OperationDashboardAggregate, ModelID: "sales", Kind: dataquery.KindSemanticAggregate, Target: "orders"}
	first := base
	first.Measures = []dataquery.Field{{Field: "order_count", Alias: "value"}}
	second := base
	second.Measures = []dataquery.Field{{Field: "event_count", Alias: "value"}}
	return []dataquery.BundleRequest{{ID: "orders", Query: first}, {ID: "events", Query: second}}
}

type bundleCountingDatabase struct {
	cacheRuntimeDatabase
	queries atomic.Int32
}

func (d *bundleCountingDatabase) Query(_ context.Context, plan semanticquery.Plan) (semanticquery.Rows, error) {
	d.queries.Add(1)
	if len(plan.Columns) > 0 && plan.Columns[0] == semanticquery.BundleBranchColumn {
		rows := semanticquery.Rows{}
		for ordinal := int64(0); ordinal < 2; ordinal++ {
			row := semanticquery.Row{}
			for _, column := range plan.Columns {
				row[column] = int64(1)
			}
			row[semanticquery.BundleBranchColumn] = ordinal
			rows = append(rows, row)
		}
		return rows, nil
	}
	row := semanticquery.Row{}
	for _, column := range plan.Columns {
		row[column] = int64(1)
	}
	return semanticquery.Rows{row}, nil
}

func TestRuntimeDoesNotCacheNonDashboardQueries(t *testing.T) {
	database := &countingCacheRuntimeDatabase{}
	runtime := &Runtime{
		modelID: "sales",
		model: &semanticmodel.Model{Name: "sales", Tables: map[string]semanticmodel.Table{
			"orders": {
				Columns:    map[string]semanticmodel.ModelColumn{"id": {Name: "id"}},
				Dimensions: map[string]semanticmodel.MetricDimension{"id": {Label: "ID"}},
			},
		}},
		db:         database,
		queryCache: newQueryResultCache(256, ""),
	}
	request := dataquery.Query{
		Surface: dataquery.SurfaceAPI, Operation: dataquery.OperationAPIQuery,
		ModelID: "sales", Kind: dataquery.KindSemanticAggregate, Target: "orders",
		Fields: []dataquery.Field{{Field: "orders.id", Alias: "id"}}, Limit: 50,
	}
	for range 2 {
		if _, err := runtime.ExecuteDataQuery(context.Background(), request); err != nil {
			t.Fatal(err)
		}
	}
	if got := database.queries.Load(); got != 2 {
		t.Fatalf("non-dashboard physical executions = %d, want 2", got)
	}
}

func TestRuntimeCountFailsClosedForMaskedAuthorizationProjection(t *testing.T) {
	runtime := &Runtime{
		modelID: "sales",
		model: &semanticmodel.Model{Name: "sales", Tables: map[string]semanticmodel.Table{
			"orders": {Dimensions: map[string]semanticmodel.MetricDimension{"email": {Type: "string"}}},
		}},
		db: cacheRuntimeDatabase{}, queryCache: newQueryResultCache(256, ""),
	}
	_, err := runtime.ExecuteDataQuery(context.Background(), dataquery.Query{
		Surface: dataquery.SurfaceDashboard, Operation: dataquery.OperationDashboardCount,
		ModelID: "sales", Kind: dataquery.KindSemanticRows, Target: "orders", IncludeTotal: true,
		AuthorizationFields: []dataquery.Field{{Field: "orders.email"}},
		ColumnMasks:         []dataquery.ColumnMask{{Field: "orders.email", Mask: "null"}},
	})
	if err == nil || !strings.Contains(err.Error(), "masked fields") {
		t.Fatalf("count error = %v, want masked authorization projection rejection", err)
	}
}

func TestRuntimeDashboardCacheHitDoesNotConsumeReadPermit(t *testing.T) {
	database := &countingCacheRuntimeDatabase{}
	runtime := &Runtime{
		modelID: "sales",
		model: &semanticmodel.Model{Name: "sales", Tables: map[string]semanticmodel.Table{
			"orders": {
				Columns:    map[string]semanticmodel.ModelColumn{"id": {Name: "id"}},
				Dimensions: map[string]semanticmodel.MetricDimension{"id": {Label: "ID"}},
			},
		}},
		db:         database,
		queryCache: newQueryResultCache(256, ""),
	}
	request := dataquery.Query{
		Surface: dataquery.SurfaceDashboard, Operation: dataquery.OperationDashboardAggregate,
		ModelID: "sales", Kind: dataquery.KindSemanticAggregate, Target: "orders",
		Fields: []dataquery.Field{{Field: "orders.id", Alias: "id"}}, Limit: 50,
	}
	if _, err := runtime.ExecuteDataQuery(context.Background(), request); err != nil {
		t.Fatal(err)
	}

	admission := execution.New(execution.Config{MaxRunningReads: 1, MaxQueuedReads: -1})
	occupied := make(chan struct{})
	release := make(chan struct{})
	var occupying sync.WaitGroup
	occupying.Add(1)
	go func() {
		defer occupying.Done()
		_, _ = admission.SubmitRead(context.Background(), dataquery.Query{}, func(context.Context) (dataquery.Result, error) {
			close(occupied)
			<-release
			return dataquery.Result{}, nil
		})
	}()
	<-occupied
	ctx := execution.WithReadAdmission(context.Background(), admission)
	result, err := runtime.ExecuteDataQuery(ctx, request)
	close(release)
	occupying.Wait()
	if err != nil {
		t.Fatalf("cache hit attempted read admission: %v", err)
	}
	if result.CacheOutcome != dataquery.CacheHit {
		t.Fatalf("cache outcome = %q, want hit", result.CacheOutcome)
	}
	if got := database.queries.Load(); got != 1 {
		t.Fatalf("physical executions = %d, want one initial miss", got)
	}
}

func TestRuntimeRefreshInvalidatesCacheBeforeFailingSchemaDiscovery(t *testing.T) {
	runtime := &Runtime{
		modelID:    "sales",
		model:      &semanticmodel.Model{Name: "sales", Tables: map[string]semanticmodel.Table{}},
		db:         failingDiscoveryRuntimeDatabase{},
		sources:    cacheSourceRegistrar{},
		queryCache: newQueryResultCache(256, "mutable"),
	}
	request := dataquery.Query{ModelID: "sales", Kind: dataquery.KindSemanticAggregate}
	var executions atomic.Int32
	execute := func() (dataquery.Result, error) {
		executions.Add(1)
		return dataquery.Result{Rows: []dataquery.Row{{"value": 1}}}, nil
	}
	if _, err := runtime.queryCache.execute(context.Background(), request, execute); err != nil {
		t.Fatal(err)
	}
	if err := runtime.Refresh(context.Background()); err == nil {
		t.Fatal("refresh error = nil, want schema discovery failure")
	}
	if _, err := runtime.queryCache.execute(context.Background(), request, execute); err != nil {
		t.Fatal(err)
	}
	if got := executions.Load(); got != 2 {
		t.Fatalf("physical executions = %d, want cache invalidated after materialization mutation", got)
	}
}

type cacheRuntimeDatabase struct{}

type countingCacheRuntimeDatabase struct {
	cacheRuntimeDatabase
	queries atomic.Int32
}

type failingDiscoveryRuntimeDatabase struct{ cacheRuntimeDatabase }

func (failingDiscoveryRuntimeDatabase) DiscoverSchemas(context.Context, *semanticmodel.Model) error {
	return errors.New("discover schemas")
}

type cacheSourceRegistrar struct{}

func (cacheSourceRegistrar) PrepareSourceRuntime(context.Context, *semanticmodel.Model) error {
	return nil
}

func (cacheSourceRegistrar) PlanModelTable(context.Context, *semanticmodel.Model, string, semanticmodel.Table) (ModelTablePlan, error) {
	return ModelTablePlan{}, errors.New("unexpected model table")
}

func (d *countingCacheRuntimeDatabase) Query(ctx context.Context, plan semanticquery.Plan) (semanticquery.Rows, error) {
	d.queries.Add(1)
	return d.cacheRuntimeDatabase.Query(ctx, plan)
}

func TestRuntimeSeparatesConnectionWaitFromDatabaseExecution(t *testing.T) {
	runtime := &Runtime{
		modelID: "sales",
		model: &semanticmodel.Model{Name: "sales", Tables: map[string]semanticmodel.Table{
			"orders": {Columns: map[string]semanticmodel.ModelColumn{"id": {Name: "id"}}},
		}},
		db: timingRuntimeDatabase{},
	}
	result, err := runtime.ExecuteDataQuery(context.Background(), dataquery.Query{
		ModelID: "sales", Kind: dataquery.KindModelTableRows, Target: "orders",
		Operation: dataquery.OperationDashboardRows,
		Fields:    []dataquery.Field{{Field: "id"}},
		Limit:     1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.ConnectionWaitMS != 20 {
		t.Fatalf("connection wait = %dms, want 20ms", result.ConnectionWaitMS)
	}
	if result.DatabaseMS < 5 || result.DatabaseMS > 20 {
		t.Fatalf("database execution = %dms, want connection wait excluded", result.DatabaseMS)
	}
}

type timingRuntimeDatabase struct{ cacheRuntimeDatabase }

func (timingRuntimeDatabase) Query(ctx context.Context, _ semanticquery.Plan) (semanticquery.Rows, error) {
	dataquery.ObserveConnectionWait(ctx, 20*time.Millisecond)
	time.Sleep(30 * time.Millisecond)
	return semanticquery.Rows{{"id": 1}}, nil
}

func (cacheRuntimeDatabase) Exec(context.Context, string) error { return nil }
func (cacheRuntimeDatabase) Query(context.Context, semanticquery.Plan) (semanticquery.Rows, error) {
	return semanticquery.Rows{{"id": 1}}, nil
}
func (cacheRuntimeDatabase) Count(context.Context, semanticquery.Plan) (int, error) { return 1, nil }
func (cacheRuntimeDatabase) FloatBounds(context.Context, semanticquery.Plan, string) (semanticquery.FloatBounds, error) {
	return semanticquery.FloatBounds{}, nil
}
func (cacheRuntimeDatabase) Histogram(context.Context, semanticquery.Plan, semanticquery.HistogramSpec) ([]semanticquery.HistogramBin, error) {
	return nil, nil
}
func (cacheRuntimeDatabase) Distribution(context.Context, semanticquery.Plan, semanticquery.DistributionSpec) (semanticquery.Rows, error) {
	return nil, nil
}
func (cacheRuntimeDatabase) Close() error { return nil }
func (cacheRuntimeDatabase) Path() string { return "" }
