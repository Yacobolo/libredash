package materialize

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	semanticmodel "github.com/Yacobolo/libredash/internal/analytics/model"
	semanticquery "github.com/Yacobolo/libredash/internal/analytics/query"
	"github.com/Yacobolo/libredash/internal/dataquery"
)

func TestQueryResultCacheUsesGovernedRequestAndReturnsDeepCopies(t *testing.T) {
	cache := newQueryResultCache(256)
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

func TestQueryResultCacheDoesNotCacheErrorsAndInvalidatesGeneration(t *testing.T) {
	cache := newQueryResultCache(1)
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
		cache := newQueryResultCache(256)
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

func TestObserveFilterOptionCacheOutcomeReportsSuccessAndError(t *testing.T) {
	observed := []string{}
	ctx := dataquery.WithCacheOutcomeObserver(context.Background(), func(outcome string) {
		observed = append(observed, outcome)
	})

	for _, outcome := range []string{dataquery.CacheHit, dataquery.CacheMiss, dataquery.CacheCoalesced} {
		observeFilterOptionCacheOutcome(ctx, dataquery.Result{CacheOutcome: outcome}, nil)
	}
	observeFilterOptionCacheOutcome(ctx, dataquery.Result{}, errors.New("temporary"))

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
		db:          cacheRuntimeDatabase{},
		optionCache: newQueryResultCache(256),
	}
	physicalQueries := 0
	cacheOutcomes := []string{}
	ctx := dataquery.WithPhysicalQueryObserver(context.Background(), func(observation dataquery.PhysicalQueryObservation) {
		physicalQueries += observation.Count
	})
	ctx = dataquery.WithCacheOutcomeObserver(ctx, func(outcome string) { cacheOutcomes = append(cacheOutcomes, outcome) })
	request := dataquery.Query{
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

type cacheRuntimeDatabase struct{}

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
