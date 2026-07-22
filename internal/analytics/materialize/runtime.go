package materialize

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Yacobolo/leapview/internal/analytics/arrowquery"
	semanticmodel "github.com/Yacobolo/leapview/internal/analytics/model"
	semanticquery "github.com/Yacobolo/leapview/internal/analytics/query"
	analyticsresource "github.com/Yacobolo/leapview/internal/analytics/resource"
	"github.com/Yacobolo/leapview/internal/analytics/resultcache"
	"github.com/Yacobolo/leapview/internal/dataquery"
	"github.com/Yacobolo/leapview/internal/workload"
)

type RuntimeConfig struct {
	ModelID string
	Model   *semanticmodel.Model
	// QueryCacheNamespace identifies the immutable serving snapshot and source
	// digests backing this runtime. Mutable refreshes additionally advance the
	// cache generation before any subsequent query can reuse results.
	QueryCacheNamespace string
	QueryCache          *resultcache.Scope
	ResultLimits        dataquery.ResultLimits
	TableRelation       semanticquery.TableRelation

	Database Database
	Sources  SourcePreparer
	Resolver SourcePathResolver
}

type ModelTableQuery struct {
	Table       string
	Columns     []string
	Sort        []semanticquery.Sort
	ColumnMasks []semanticquery.ColumnMask
	Limit       int
	Offset      int
}

type Runtime struct {
	modelID      string
	model        *semanticmodel.Model
	planner      *semanticquery.Planner
	db           Database
	sources      SourcePreparer
	queries      *semanticquery.Service
	queryCache   *queryResultCache
	resultLimits dataquery.ResultLimits
	lastRefresh  time.Time
}

type Database interface {
	Executor
	semanticquery.Executor
	Close() error
	Path() string
}

type arrowDatabase interface {
	QueryArrow(context.Context, semanticquery.Plan, arrowquery.Sink) error
}

type schemaDiscoverer interface {
	DiscoverSchemas(context.Context, *semanticmodel.Model) error
}

func OpenRuntime(ctx context.Context, config RuntimeConfig) (*Runtime, error) {
	runtime, err := NewRuntimeView(ctx, config)
	if err != nil {
		return nil, err
	}
	if err := runtime.Refresh(ctx); err != nil {
		config.Database.Close()
		return nil, err
	}
	return runtime, nil
}

func NewRuntimeView(ctx context.Context, config RuntimeConfig) (*Runtime, error) {
	if config.Model == nil {
		return nil, fmt.Errorf("semantic model is required")
	}
	if config.Database == nil {
		return nil, fmt.Errorf("materialization database is required")
	}
	if config.Sources == nil {
		return nil, fmt.Errorf("source preparer is required")
	}
	resolver := config.Resolver
	if resolver == nil {
		resolver = defaultSourcePathResolver{}
	}
	if err := ValidateFilesWithResolver(config.Model, resolver); err != nil {
		return nil, err
	}
	plannerOptions := []semanticquery.PlannerOption{}
	if config.TableRelation != nil {
		plannerOptions = append(plannerOptions, semanticquery.WithTableRelation(config.TableRelation))
	}
	planner, err := semanticquery.NewCompiledPlanner(config.Model, plannerOptions...)
	if err != nil {
		return nil, fmt.Errorf("compile semantic model: %w", err)
	}
	cache := newQueryResultCacheWithScope(config.QueryCache, config.QueryCacheNamespace)
	if config.QueryCache == nil {
		cache = newQueryResultCache(256, config.QueryCacheNamespace)
	}
	limits := config.ResultLimits
	if limits.MaxRows <= 0 {
		limits.MaxRows = 10000
	}
	if limits.MaxBytes <= 0 {
		limits.MaxBytes = 32 << 20
	}
	if err := limits.Validate(); err != nil {
		return nil, err
	}
	runtime := &Runtime{
		modelID: config.ModelID, model: config.Model, planner: planner, db: config.Database,
		sources: config.Sources, queries: semanticquery.NewService(planner, config.Database),
		queryCache: cache, resultLimits: limits,
	}
	return runtime, nil
}

func (r *Runtime) queryPlanner() *semanticquery.Planner {
	if r != nil && r.planner != nil {
		return r.planner
	}
	return semanticquery.NewPlanner(r.model)
}

func (r *Runtime) Close() error {
	if r == nil {
		return nil
	}
	return errors.Join(r.db.Close(), r.queryCache.close())
}

// CloseView releases generation-scoped cache state without closing the
// process-owned analytical database shared by other runtimes.
func (r *Runtime) CloseView() error {
	if r == nil || r.queryCache == nil {
		return nil
	}
	return r.queryCache.close()
}

func (r *Runtime) Refresh(ctx context.Context) error {
	prepared, err := r.sources.Prepare(ctx, r.model)
	if err != nil {
		return err
	}
	lastRefresh, err := Refresh(ctx, r.db, prepared, r.model)
	err = errors.Join(err, prepared.Close())
	if err != nil {
		return err
	}
	// The database has changed even if subsequent schema discovery fails. Advance
	// the generation immediately so no in-flight or later read can publish stale
	// results from the previous materialization.
	r.ClearQueryCache()
	if discoverer, ok := r.db.(schemaDiscoverer); ok {
		if err := discoverer.DiscoverSchemas(ctx, r.model); err != nil {
			return err
		}
	}
	r.lastRefresh = lastRefresh
	return nil
}

func (r *Runtime) RefreshModelTables(ctx context.Context, tableNames []string) error {
	prepared, err := r.sources.Prepare(ctx, r.model)
	if err != nil {
		return err
	}
	lastRefresh, err := RefreshModelTables(ctx, r.db, prepared, r.model, tableNames)
	err = errors.Join(err, prepared.Close())
	if err != nil {
		return err
	}
	r.ClearQueryCache()
	if discoverer, ok := r.db.(schemaDiscoverer); ok {
		if err := discoverer.DiscoverSchemas(ctx, r.model); err != nil {
			return err
		}
	}
	r.lastRefresh = lastRefresh
	return nil
}

func (r *Runtime) Queries() *semanticquery.Service {
	if r == nil {
		return nil
	}
	return r.queries
}

func (r *Runtime) queryResultLimits() dataquery.ResultLimits {
	limits := r.resultLimits
	if limits.MaxRows <= 0 {
		limits.MaxRows = 10000
	}
	if limits.MaxBytes <= 0 {
		limits.MaxBytes = 32 << 20
	}
	return limits
}

func (r *Runtime) ExecuteDataQuery(ctx context.Context, request dataquery.Query) (dataquery.Result, error) {
	if r == nil || r.db == nil {
		return dataquery.Result{}, fmt.Errorf("materialization runtime is not initialized")
	}
	ctx = dataquery.WithResultBudget(ctx, r.queryResultLimits())
	if request.ModelID == "" {
		request.ModelID = r.modelID
	}
	if r.modelID != "" && request.ModelID != "" && request.ModelID != r.modelID {
		return dataquery.Result{}, fmt.Errorf("semantic model %q is not available in runtime for %q", request.ModelID, r.modelID)
	}
	var transform dataquery.ResultTransformer
	if governor, ok := dataquery.GovernorFromContext(ctx); ok && !dataquery.GovernanceApplied(ctx) {
		governed, nextTransform, err := governor.GovernDataQuery(ctx, request)
		if err != nil {
			return dataquery.Result{Status: dataquery.StatusError, ExecutionState: dataquery.ExecutionRejected, Error: err.Error()}, err
		}
		request = governed
		transform = nextTransform
		ctx = dataquery.WithGovernanceApplied(ctx)
	}
	if err := request.Validate(); err != nil {
		return dataquery.Result{}, err
	}
	executePhysical := func(execCtx context.Context) (dataquery.Result, error) {
		lease, leasedCtx, err := acquireDatabaseLease(execCtx, r.db)
		if err != nil {
			return dataquery.Result{}, err
		}
		if lease != nil {
			defer lease.Release()
			execCtx = leasedCtx
		}
		execCtx, connectionWait := dataquery.WithConnectionWaitCounter(execCtx)
		var result dataquery.Result
		err = nil
		switch request.Kind {
		case dataquery.KindSemanticAggregate:
			result, err = r.executeSemanticAggregate(execCtx, request)
		case dataquery.KindSemanticRows:
			result, err = r.executeSemanticRows(execCtx, request)
		case dataquery.KindModelTableRows:
			result, err = r.executeModelTableRows(execCtx, request)
		case dataquery.KindSemanticHistogram:
			result, err = r.executeSemanticHistogram(execCtx, request)
		case dataquery.KindSemanticDistribution:
			result, err = r.executeSemanticDistribution(execCtx, request)
		default:
			return dataquery.Result{}, fmt.Errorf("unsupported data query kind %q", request.Kind)
		}
		waitMS := connectionWait.Duration().Milliseconds()
		result.ConnectionWaitMS += waitMS
		if waitMS >= result.DatabaseMS {
			result.DatabaseMS = 0
		} else {
			result.DatabaseMS -= waitMS
		}
		return result, err
	}
	execute := func() (dataquery.Result, error) {
		execCtx, statements := withPhysicalStatementCounter(dataquery.WithResultBudget(ctx, r.queryResultLimits()))
		result, err := admitPhysicalQuery(execCtx, request, executePhysical)
		if count := int(statements.Load()); count > 0 {
			dataquery.ObservePhysicalQuery(ctx, dataquery.PhysicalQueryObservation{Count: count, Result: result})
		}
		return result, err
	}
	var (
		result dataquery.Result
		err    error
	)
	if dashboardQueryResultCacheable(request) {
		result, err = r.queryCache.execute(ctx, request, execute)
		observeQueryCacheOutcome(ctx, result, err)
	} else {
		result, err = execute()
	}
	if err == nil && result.CacheOutcome == dataquery.CacheHit {
		if budget, ok := dataquery.ResultBudgetFromContext(ctx); ok {
			err = budget.ConsumeRows(result.Rows)
		}
	}
	if _, ok := dataquery.ResultLimitReasonOf(err); ok {
		return dataquery.Result{Status: dataquery.StatusError, ExecutionState: dataquery.ExecutionFailed, Error: err.Error()}, err
	}
	if err == nil {
		result.RowsReturned = len(result.Rows)
		result.BytesEstimate = resultcache.EstimateResultBytes(result)
	}
	if transform != nil {
		if transformErr := transform(&result, err); transformErr != nil {
			return dataquery.Result{Status: dataquery.StatusError, ExecutionState: dataquery.ExecutionRejected, Error: transformErr.Error()}, transformErr
		}
	}
	return result, err
}

// ExecuteDataQueryArrow is the native, streaming execution path for Arrow
// transports. It deliberately bypasses the row-oriented result cache: API
// queries are not cacheable, and converting cached maps back into Arrow would
// defeat the ownership and allocation guarantees of this contract.
func (r *Runtime) ExecuteDataQueryArrow(ctx context.Context, request dataquery.Query, sink arrowquery.Sink) (dataquery.Result, error) {
	if r == nil || r.db == nil {
		return dataquery.Result{}, fmt.Errorf("materialization runtime is not initialized")
	}
	if sink == nil {
		return dataquery.Result{}, fmt.Errorf("Arrow sink is required")
	}
	db, ok := r.db.(arrowDatabase)
	if !ok {
		return dataquery.Result{}, fmt.Errorf("analytical database does not support native Arrow execution")
	}
	ctx = dataquery.WithResultBudget(ctx, r.queryResultLimits())
	if request.ModelID == "" {
		request.ModelID = r.modelID
	}
	if r.modelID != "" && request.ModelID != "" && request.ModelID != r.modelID {
		return dataquery.Result{}, fmt.Errorf("semantic model %q is not available in runtime for %q", request.ModelID, r.modelID)
	}
	var transform dataquery.ResultTransformer
	if governor, found := dataquery.GovernorFromContext(ctx); found && !dataquery.GovernanceApplied(ctx) {
		governed, nextTransform, err := governor.GovernDataQuery(ctx, request)
		if err != nil {
			return dataquery.Result{Status: dataquery.StatusError, ExecutionState: dataquery.ExecutionRejected, Error: err.Error()}, err
		}
		request, transform = governed, nextTransform
		ctx = dataquery.WithGovernanceApplied(ctx)
	}
	if err := request.Validate(); err != nil {
		return dataquery.Result{}, err
	}

	executePhysical := func(execCtx context.Context) (dataquery.Result, error) {
		planningStarted := time.Now()
		plan, err := r.planArrowQuery(request)
		planningMS := elapsedStageMS(planningStarted)
		if err != nil {
			return dataquery.Result{PlanningMS: planningMS}, err
		}
		lease, leasedCtx, err := acquireDatabaseLease(execCtx, r.db)
		if err != nil {
			return dataquery.Result{}, err
		}
		if lease != nil {
			defer lease.Release()
			execCtx = leasedCtx
		}
		execCtx, connectionWait := dataquery.WithConnectionWaitCounter(execCtx)
		databaseStarted := time.Now()
		markPhysicalStatement(execCtx)
		err = db.QueryArrow(execCtx, plan, sink)
		databaseMS := elapsedStageMS(databaseStarted)
		rows, bytes := 0, int64(0)
		if budget, found := dataquery.ResultBudgetFromContext(execCtx); found {
			rows, bytes = budget.Usage()
		}
		if stats, found := sink.(arrowquery.SinkStats); found {
			rows = stats.RowsWritten()
		}
		result := dataquery.Result{
			Columns: dataquery.ColumnsFromNames(plan.Columns), SQL: plan.SQL,
			PlanningMS: planningMS, DatabaseMS: databaseMS,
			ConnectionWaitMS: connectionWait.Duration().Milliseconds(),
			RowsReturned:     rows, BytesEstimate: bytes,
		}
		if result.ConnectionWaitMS >= result.DatabaseMS {
			result.DatabaseMS = 0
		} else {
			result.DatabaseMS -= result.ConnectionWaitMS
		}
		return result, err
	}

	execCtx, statements := withPhysicalStatementCounter(ctx)
	result, err := admitPhysicalQuery(execCtx, request, executePhysical)
	if count := int(statements.Load()); count > 0 {
		dataquery.ObservePhysicalQuery(ctx, dataquery.PhysicalQueryObservation{Count: count, Result: result})
	}
	if _, found := dataquery.ResultLimitReasonOf(err); found {
		result.Status, result.ExecutionState, result.Error = dataquery.StatusError, dataquery.ExecutionFailed, err.Error()
	}
	if transform != nil {
		if transformErr := transform(&result, err); transformErr != nil {
			return dataquery.Result{Status: dataquery.StatusError, ExecutionState: dataquery.ExecutionRejected, Error: transformErr.Error()}, transformErr
		}
	}
	return result, err
}

func (r *Runtime) planArrowQuery(request dataquery.Query) (semanticquery.Plan, error) {
	switch request.Kind {
	case dataquery.KindSemanticAggregate:
		return r.queryPlanner().Plan(semanticquery.Request{
			Table: request.Target, Dimensions: dataQueryFields(request.Fields), Measures: dataQueryFields(request.Measures),
			Time:    semanticquery.Time{Field: request.Time.Field, Grain: request.Time.Grain, Alias: request.Time.Alias},
			Filters: dataQueryFilters(request.Filters), Sort: dataQuerySorts(request.Sort),
			ColumnMasks: dataQueryColumnMasks(request.ColumnMasks), Limit: request.Limit, Offset: request.Offset,
		})
	case dataquery.KindSemanticRows:
		if request.IncludeTotal {
			return semanticquery.Plan{}, fmt.Errorf("native Arrow row queries do not include an auxiliary total")
		}
		return r.queryPlanner().PlanRows(semanticquery.RowRequest{
			Table: request.Target, Dimensions: dataQueryFields(request.Fields), Measures: dataQueryFields(request.Measures),
			Filters: dataQueryFilters(request.Filters), Sort: dataQuerySorts(request.Sort),
			ColumnMasks: dataQueryColumnMasks(request.ColumnMasks), Limit: request.Limit, Offset: request.Offset,
		})
	default:
		return semanticquery.Plan{}, fmt.Errorf("data query kind %q does not support native Arrow streaming", request.Kind)
	}
}

// ExecuteDataQueryBundle authorizes every branch before compiling one
// single-fact GROUPING SETS statement. It is intentionally separate from the
// ordinary result cache: consumers fall back to the regular path on cache hits
// or incompatibility, while a bundle miss is admitted and observed as exactly
// one physical query.
func (r *Runtime) ExecuteDataQueryBundle(ctx context.Context, requests []dataquery.BundleRequest) (dataquery.BundleResult, error) {
	if r == nil || r.db == nil {
		return dataquery.BundleResult{}, fmt.Errorf("materialization runtime is not initialized")
	}
	if len(requests) < 2 {
		return dataquery.BundleResult{}, &dataquery.BundleIncompatibleError{Err: fmt.Errorf("bundle requires at least two branches")}
	}
	ctx = dataquery.WithResultBudget(ctx, r.queryResultLimits())
	governed := make([]dataquery.BundleRequest, 0, len(requests))
	transforms := make(map[string]dataquery.ResultTransformer, len(requests))
	result := dataquery.BundleResult{Results: make(map[string]dataquery.Result, len(requests))}
	type cacheSlot struct {
		key        string
		generation uint64
	}
	cacheSlots := map[string]cacheSlot{}
	flightSlots := map[string]cacheSlot{}
	for _, branch := range requests {
		request := branch.Query.WithMetadata(dataquery.MetadataFromContext(ctx))
		if request.ModelID == "" {
			request.ModelID = r.modelID
		}
		if request.ModelID != r.modelID || request.Kind != dataquery.KindSemanticAggregate {
			return dataquery.BundleResult{}, &dataquery.BundleIncompatibleError{Err: fmt.Errorf("branch %q is not an aggregate for model %q", branch.ID, r.modelID)}
		}
		if governor, ok := dataquery.GovernorFromContext(ctx); ok && !dataquery.GovernanceApplied(ctx) {
			var err error
			request, transforms[branch.ID], err = governor.GovernDataQuery(ctx, request)
			if err != nil {
				return dataquery.BundleResult{}, &dataquery.BundleBranchError{ID: branch.ID, Err: err}
			}
		}
		if err := request.Validate(); err != nil {
			return dataquery.BundleResult{}, &dataquery.BundleBranchError{ID: branch.ID, Err: err}
		}
		if !dashboardQueryResultCacheable(request) {
			return dataquery.BundleResult{}, &dataquery.BundleIncompatibleError{Err: fmt.Errorf("branch %q is not a cache-governed dashboard query", branch.ID)}
		}
		cached, key, generation, hit, err := r.queryCache.lookup(request)
		if err != nil {
			return dataquery.BundleResult{}, &dataquery.BundleBranchError{ID: branch.ID, Err: err}
		}
		if hit {
			if budget, ok := dataquery.ResultBudgetFromContext(ctx); ok {
				if err := budget.ConsumeRows(cached.Rows); err != nil {
					return dataquery.BundleResult{}, err
				}
			}
			dataquery.ObserveCacheOutcome(ctx, dataquery.CacheHit)
			result.Results[branch.ID] = cached
			continue
		}
		cacheSlots[branch.ID] = cacheSlot{key: key, generation: generation}
		flightSlots[branch.ID] = cacheSlot{key: key, generation: generation}
		governed = append(governed, dataquery.BundleRequest{ID: branch.ID, Query: request})
	}
	applyTransforms := func(executeErr error) error {
		for _, branch := range requests {
			transform := transforms[branch.ID]
			if transform == nil {
				continue
			}
			branchResult := result.Results[branch.ID]
			if err := transform(&branchResult, executeErr); err != nil {
				return &dataquery.BundleBranchError{ID: branch.ID, Err: err}
			}
			if executeErr == nil {
				result.Results[branch.ID] = branchResult
			}
		}
		return nil
	}
	if len(governed) == len(requests) {
		projection, handled, projectionErr := r.executeProjectionBundle(ctx, governed, transforms)
		if handled {
			if projectionErr != nil {
				_ = applyTransforms(projectionErr)
				return dataquery.BundleResult{}, projectionErr
			}
			result = projection
			if err := applyTransforms(nil); err != nil {
				return dataquery.BundleResult{}, err
			}
			return result, nil
		}
	}
	if len(governed) == 0 {
		if err := ctx.Err(); err != nil {
			_ = applyTransforms(err)
			return dataquery.BundleResult{}, err
		}
		if err := applyTransforms(nil); err != nil {
			return dataquery.BundleResult{}, err
		}
		return result, nil
	}
	if len(governed) == 1 {
		branch := governed[0]
		branchResult, err := r.ExecuteDataQuery(dataquery.WithGovernanceApplied(ctx), branch.Query)
		if err != nil {
			_ = applyTransforms(err)
			return dataquery.BundleResult{}, &dataquery.BundleBranchError{ID: branch.ID, Err: err}
		}
		if err := ctx.Err(); err != nil {
			_ = applyTransforms(err)
			return dataquery.BundleResult{}, err
		}
		result.Results[branch.ID] = branchResult
		result.SQL = branchResult.SQL
		if err := applyTransforms(nil); err != nil {
			return dataquery.BundleResult{}, err
		}
		return result, nil
	}
	semanticRequests := make([]semanticquery.BundleRequest, len(governed))
	for i, branch := range governed {
		request := branch.Query
		semanticRequests[i] = semanticquery.BundleRequest{ID: branch.ID, Request: semanticquery.Request{
			Table: request.Target, Dimensions: dataQueryFields(request.Fields), Measures: dataQueryFields(request.Measures), Time: semanticquery.Time{Field: request.Time.Field, Grain: request.Time.Grain, Alias: request.Time.Alias}, Filters: dataQueryFilters(request.Filters), Sort: dataQuerySorts(request.Sort), ColumnMasks: dataQueryColumnMasks(request.ColumnMasks), Limit: request.Limit, Offset: request.Offset,
		}}
	}
	planningStarted := time.Now()
	bundle, err := r.queryPlanner().PlanBundle(semanticRequests)
	planningMS := elapsedStageMS(planningStarted)
	if err != nil {
		return dataquery.BundleResult{}, &dataquery.BundleIncompatibleError{Err: err}
	}
	representative := governed[0].Query
	type bundleExecution struct {
		decoded map[string]semanticquery.Rows
		summary dataquery.Result
	}
	execute := func(execCtx context.Context) (dataquery.Result, map[string]semanticquery.Rows, error) {
		lease, leasedCtx, err := acquireDatabaseLease(execCtx, r.db)
		if err != nil {
			return dataquery.Result{}, nil, err
		}
		if lease != nil {
			defer lease.Release()
			execCtx = leasedCtx
		}
		execCtx, connectionWait := dataquery.WithConnectionWaitCounter(execCtx)
		databaseStarted := time.Now()
		markPhysicalStatement(execCtx)
		rows, queryErr := r.db.Query(execCtx, bundle.Plan)
		databaseMS := elapsedStageMS(databaseStarted)
		waitMS := connectionWait.Duration().Milliseconds()
		if waitMS >= databaseMS {
			databaseMS = 0
		} else {
			databaseMS -= waitMS
		}
		if queryErr != nil {
			return dataquery.Result{PlanningMS: planningMS, ConnectionWaitMS: waitMS, DatabaseMS: databaseMS, SQL: bundle.Plan.SQL}, nil, queryErr
		}
		decoded, decodeErr := bundle.Decode(rows)
		return dataquery.Result{PlanningMS: planningMS, ConnectionWaitMS: waitMS, DatabaseMS: databaseMS, SQL: bundle.Plan.SQL, ExecutionState: dataquery.ExecutionSucceeded}, decoded, decodeErr
	}
	flightIdentity := make([]struct {
		ID         string `json:"id"`
		Key        string `json:"key"`
		Generation uint64 `json:"generation"`
	}, 0, len(governed))
	for _, branch := range governed {
		slot := flightSlots[branch.ID]
		flightIdentity = append(flightIdentity, struct {
			ID         string `json:"id"`
			Key        string `json:"key"`
			Generation uint64 `json:"generation"`
		}{ID: branch.ID, Key: slot.key, Generation: slot.generation})
	}
	flightKey, err := json.Marshal(flightIdentity)
	if err != nil {
		return dataquery.BundleResult{}, fmt.Errorf("encode aggregate bundle flight identity: %w", err)
	}
	flight, shared, err := r.queryCache.coalesce(ctx, string(flightKey), func() (any, error) {
		execCtx, statements := withPhysicalStatementCounter(dataquery.WithIndependentResultBudget(ctx, r.queryResultLimits()))
		var decoded map[string]semanticquery.Rows
		summary, executeErr := admitPhysicalQuery(execCtx, representative, func(queryCtx context.Context) (dataquery.Result, error) {
			queryResult, rows, queryErr := execute(queryCtx)
			decoded = rows
			return queryResult, queryErr
		})
		if count := int(statements.Load()); count > 0 {
			dataquery.ObservePhysicalQuery(ctx, dataquery.PhysicalQueryObservation{Count: count, Result: summary})
		}
		return bundleExecution{decoded: decoded, summary: summary}, executeErr
	})
	if err != nil {
		_ = applyTransforms(err)
		return dataquery.BundleResult{}, err
	}
	if err := ctx.Err(); err != nil {
		_ = applyTransforms(err)
		return dataquery.BundleResult{}, err
	}
	executionResult := flight.(bundleExecution)
	result.SQL = bundle.Plan.SQL
	type pendingCacheStore struct {
		slot   cacheSlot
		result dataquery.Result
	}
	pendingStores := make([]pendingCacheStore, 0, len(governed))
	for _, branch := range governed {
		rows := dataQueryRows(executionResult.decoded[branch.ID])
		if budget, ok := dataquery.ResultBudgetFromContext(ctx); ok {
			if err := budget.ConsumeRows(rows); err != nil {
				return dataquery.BundleResult{}, err
			}
		}
		branchResult := dataquery.Result{Rows: rows, Columns: dataquery.ColumnsFromNames(bundleOutputColumns(bundle, branch.ID)), SQL: bundle.Plan.SQL, PlanningMS: executionResult.summary.PlanningMS, ConnectionWaitMS: executionResult.summary.ConnectionWaitMS, DatabaseMS: executionResult.summary.DatabaseMS, ExecutionState: dataquery.ExecutionSucceeded, Status: dataquery.StatusSuccess, RowsReturned: len(rows)}
		branchResult.BytesEstimate = resultcache.EstimateResultBytes(branchResult)
		if slot, ok := cacheSlots[branch.ID]; ok {
			if err := ctx.Err(); err != nil {
				_ = applyTransforms(err)
				return dataquery.BundleResult{}, err
			}
			branchResult.CacheOutcome = dataquery.CacheMiss
			if shared {
				branchResult.CacheOutcome = dataquery.CacheCoalesced
			}
			pendingStores = append(pendingStores, pendingCacheStore{slot: slot, result: branchResult})
		}
		result.Results[branch.ID] = branchResult
	}
	if err := ctx.Err(); err != nil {
		_ = applyTransforms(err)
		return dataquery.BundleResult{}, err
	}
	for _, pending := range pendingStores {
		r.queryCache.store(pending.slot.key, pending.slot.generation, pending.result)
		dataquery.ObserveCacheOutcome(ctx, pending.result.CacheOutcome)
	}
	if err := applyTransforms(nil); err != nil {
		return dataquery.BundleResult{}, err
	}
	return result, nil
}

// acquireDatabaseLease is mandatory for production analytical environments.
// Lightweight pure-Go test executors do not own physical connections and are
// intentionally allowed to omit the capability.
func acquireDatabaseLease(ctx context.Context, database Database) (analyticsresource.Lease, context.Context, error) {
	provider, ok := database.(analyticsresource.Provider)
	if !ok {
		return nil, ctx, nil
	}
	lease, err := provider.Acquire(ctx)
	if err != nil {
		return nil, ctx, err
	}
	return lease, lease.Context(), nil
}

func admitPhysicalQuery(ctx context.Context, request dataquery.Query, execute func(context.Context) (dataquery.Result, error)) (dataquery.Result, error) {
	admitter, ok := workload.FromContext(ctx)
	if !ok {
		return execute(ctx)
	}
	class := workload.Interactive
	workspaceID := request.WorkspaceID
	if request.Surface == dataquery.SurfaceAgent {
		class = workload.Background
		if activeClass, activeWorkspace, admitted := workload.Current(ctx); admitted && activeClass == workload.Background {
			workspaceID = activeWorkspace
		}
	}
	operation := request.Operation
	if operation == "" {
		operation = string(request.Kind)
	}
	lease, err := admitter.Acquire(ctx, workload.Request{Class: class, WorkspaceID: workspaceID, Operation: operation})
	if err != nil {
		state := dataquery.ExecutionRejected
		if reason, found := workload.ReasonOf(err); found && reason == workload.QueueTimeout {
			state = dataquery.ExecutionTimeout
		}
		result := dataquery.Result{ExecutionState: state}
		var rejection *workload.Rejection
		if errors.As(err, &rejection) {
			result.QueueWaitMS = durationMillis(rejection.QueueWait)
		}
		return result, err
	}
	defer lease.Release()
	started := time.Now()
	result, err := execute(lease.Context())
	if result.QueueWaitMS == 0 {
		result.QueueWaitMS = durationMillis(lease.QueueWait())
	}
	if result.ExecutionMS == 0 {
		result.ExecutionMS = durationMillis(time.Since(started))
	}
	if result.ExecutionState == "" {
		switch {
		case err == nil:
			result.ExecutionState = dataquery.ExecutionSucceeded
		case lease.Context().Err() == context.DeadlineExceeded:
			result.ExecutionState = dataquery.ExecutionTimeout
		case lease.Context().Err() == context.Canceled:
			result.ExecutionState = dataquery.ExecutionCanceled
		default:
			result.ExecutionState = dataquery.ExecutionFailed
		}
	}
	return result, err
}

func durationMillis(duration time.Duration) int64 {
	if duration <= 0 {
		return 0
	}
	if milliseconds := duration.Milliseconds(); milliseconds > 0 {
		return milliseconds
	}
	return 1
}

func bundleOutputColumns(bundle semanticquery.BundlePlan, id string) []string {
	for _, branch := range bundle.Branches {
		if branch.ID == id {
			columns := make([]string, len(branch.Columns))
			for i, column := range branch.Columns {
				columns[i] = column.Output
			}
			return columns
		}
	}
	return nil
}

type physicalStatementCounterContextKey struct{}

func withPhysicalStatementCounter(ctx context.Context) (context.Context, *atomic.Int64) {
	counter := &atomic.Int64{}
	return context.WithValue(ctx, physicalStatementCounterContextKey{}, counter), counter
}

func markPhysicalStatement(ctx context.Context) {
	if counter, ok := ctx.Value(physicalStatementCounterContextKey{}).(*atomic.Int64); ok && counter != nil {
		counter.Add(1)
	}
}

func observeQueryCacheOutcome(ctx context.Context, result dataquery.Result, err error) {
	outcome := result.CacheOutcome
	if err != nil {
		outcome = dataquery.CacheError
	}
	dataquery.ObserveCacheOutcome(ctx, outcome)
}

// dashboardQueryResultCacheable is deliberately explicit. API, CLI, agent,
// preview, and unclassified calls must not populate the dashboard result cache
// even if they happen to use an equivalent physical query shape.
func dashboardQueryResultCacheable(request dataquery.Query) bool {
	if request.Surface != dataquery.SurfaceDashboard {
		return false
	}
	switch request.Operation {
	case dataquery.OperationDashboardAggregate,
		dataquery.OperationDashboardRows,
		dataquery.OperationDashboardCount,
		dataquery.OperationDashboardHistogram,
		dataquery.OperationDashboardDistribution,
		dataquery.OperationDashboardFilterOptions:
		return true
	default:
		return false
	}
}

func (r *Runtime) ClearQueryCache() {
	if r != nil && r.queryCache != nil {
		r.queryCache.clear()
	}
}

func (r *Runtime) executeSemanticAggregate(ctx context.Context, request dataquery.Query) (dataquery.Result, error) {
	semanticRequest := semanticquery.Request{
		Table:       request.Target,
		Dimensions:  dataQueryFields(request.Fields),
		Measures:    dataQueryFields(request.Measures),
		Time:        semanticquery.Time{Field: request.Time.Field, Grain: request.Time.Grain, Alias: request.Time.Alias},
		Filters:     dataQueryFilters(request.Filters),
		Sort:        dataQuerySorts(request.Sort),
		ColumnMasks: dataQueryColumnMasks(request.ColumnMasks),
		Limit:       request.Limit,
		Offset:      request.Offset,
	}
	planningStarted := time.Now()
	plan, err := r.queryPlanner().Plan(semanticRequest)
	planningMS := elapsedStageMS(planningStarted)
	if err != nil {
		return dataquery.Result{}, err
	}
	databaseStarted := time.Now()
	markPhysicalStatement(ctx)
	rows, err := r.db.Query(ctx, plan)
	databaseMS := elapsedStageMS(databaseStarted)
	if err != nil {
		return dataquery.Result{}, err
	}
	return dataquery.Result{Columns: dataquery.ColumnsFromNames(plan.Columns), Rows: dataQueryRows(rows), SQL: plan.SQL, PlanningMS: planningMS, DatabaseMS: databaseMS}, nil
}

func (r *Runtime) executeSemanticRows(ctx context.Context, request dataquery.Query) (dataquery.Result, error) {
	planner := r.queryPlanner()
	if len(request.Fields) == 0 && len(request.Measures) == 0 && request.IncludeTotal {
		if len(request.ColumnMasks) > 0 {
			return dataquery.Result{}, fmt.Errorf("table count is unavailable because its authorization projection contains masked fields")
		}
		planningStarted := time.Now()
		countPlan, err := planner.PlanCount(semanticquery.CountRequest{Table: request.Target, Filters: dataQueryFilters(request.Filters)})
		planningMS := elapsedStageMS(planningStarted)
		if err != nil {
			return dataquery.Result{}, err
		}
		databaseStarted := time.Now()
		markPhysicalStatement(ctx)
		total, err := r.db.Count(ctx, countPlan)
		databaseMS := elapsedStageMS(databaseStarted)
		if err != nil {
			return dataquery.Result{}, err
		}
		return dataquery.Result{TotalRows: total, TotalRowsKnown: true, SQL: countPlan.SQL, PlanningMS: planningMS, DatabaseMS: databaseMS}, nil
	}
	semanticRequest := semanticquery.RowRequest{
		Table:       request.Target,
		Dimensions:  dataQueryFields(request.Fields),
		Measures:    dataQueryFields(request.Measures),
		Filters:     dataQueryFilters(request.Filters),
		Sort:        dataQuerySorts(request.Sort),
		ColumnMasks: dataQueryColumnMasks(request.ColumnMasks),
		Limit:       request.Limit,
		Offset:      request.Offset,
	}
	planningStarted := time.Now()
	plan, err := planner.PlanRows(semanticRequest)
	if err != nil {
		return dataquery.Result{}, err
	}
	if request.IncludeTotal {
		plan, err = rowPlanWithTotal(plan)
		if err != nil {
			return dataquery.Result{}, err
		}
	}
	planningMS := elapsedStageMS(planningStarted)
	databaseStarted := time.Now()
	markPhysicalStatement(ctx)
	rows, err := r.db.Query(ctx, plan)
	databaseMS := elapsedStageMS(databaseStarted)
	if err != nil {
		return dataquery.Result{}, err
	}
	result := dataquery.Result{Columns: dataquery.ColumnsFromNames(plan.Columns), Rows: dataQueryRows(rows), SQL: plan.SQL, PlanningMS: planningMS, DatabaseMS: databaseMS}
	if request.IncludeTotal {
		if len(result.Rows) > 0 {
			result.TotalRows = intFromDataQueryValue(result.Rows[0][totalRowsColumn])
			result.TotalRowsKnown = true
		} else if request.Offset == 0 {
			result.TotalRowsKnown = true
		}
		for _, row := range result.Rows {
			delete(row, totalRowsColumn)
		}
		result.Columns = dataquery.ColumnsFromNames(plan.Columns[:len(plan.Columns)-1])
		if !result.TotalRowsKnown {
			planningStarted = time.Now()
			countPlan, err := planner.PlanCount(semanticquery.CountRequest{Table: request.Target, Filters: dataQueryFilters(request.Filters)})
			result.PlanningMS += elapsedStageMS(planningStarted)
			if err != nil {
				return dataquery.Result{}, err
			}
			databaseStarted = time.Now()
			markPhysicalStatement(ctx)
			total, err := r.db.Count(ctx, countPlan)
			result.DatabaseMS += elapsedStageMS(databaseStarted)
			if err != nil {
				return dataquery.Result{}, err
			}
			result.TotalRows = total
			result.TotalRowsKnown = true
		}
	}
	return result, nil
}

const totalRowsColumn = "__leapview_total_rows"

func rowPlanWithTotal(plan semanticquery.Plan) (semanticquery.Plan, error) {
	from := strings.Index(plan.SQL, "\nFROM ")
	if from < 0 {
		return semanticquery.Plan{}, fmt.Errorf("row query plan has no FROM clause")
	}
	plan.SQL = plan.SQL[:from] + ", COUNT(*) OVER () AS " + totalRowsColumn + plan.SQL[from:]
	plan.Columns = append(append([]string{}, plan.Columns...), totalRowsColumn)
	return plan, nil
}

func intFromDataQueryValue(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int32:
		return int(typed)
	case int64:
		return int(typed)
	case uint:
		return int(typed)
	case uint32:
		return int(typed)
	case uint64:
		return int(typed)
	case float64:
		return int(typed)
	default:
		return 0
	}
}

func (r *Runtime) executeModelTableRows(ctx context.Context, request dataquery.Query) (dataquery.Result, error) {
	planningStarted := time.Now()
	plan, err := r.modelTableQueryPlan(ModelTableQuery{
		Table:       request.Target,
		Columns:     dataquery.FieldNames(request.Fields),
		Sort:        dataQuerySorts(request.Sort),
		ColumnMasks: dataQueryColumnMasks(request.ColumnMasks),
		Limit:       request.Limit,
		Offset:      request.Offset,
	})
	if err != nil {
		return dataquery.Result{}, err
	}
	planningMS := elapsedStageMS(planningStarted)
	databaseStarted := time.Now()
	markPhysicalStatement(ctx)
	rows, err := r.db.Query(ctx, plan)
	databaseMS := elapsedStageMS(databaseStarted)
	if err != nil {
		return dataquery.Result{}, err
	}
	result := dataquery.Result{Columns: dataquery.ColumnsFromNames(plan.Columns), Rows: dataQueryRows(rows), SQL: plan.SQL, PlanningMS: planningMS, DatabaseMS: databaseMS}
	if request.IncludeTotal {
		databaseStarted = time.Now()
		total, err := r.CountModelTable(ctx, request.Target)
		result.DatabaseMS += elapsedStageMS(databaseStarted)
		if err != nil {
			return dataquery.Result{}, err
		}
		result.TotalRows = total
		result.TotalRowsKnown = true
	}
	return result, nil
}

func (r *Runtime) executeSemanticHistogram(ctx context.Context, request dataquery.Query) (dataquery.Result, error) {
	rawRequest := semanticquery.RawValueRequest{
		Table:       request.Target,
		Dimensions:  dataQueryFields(request.Fields),
		Measure:     dataQueryFields([]dataquery.Field{request.Value})[0],
		Filters:     dataQueryFilters(request.Filters),
		ColumnMasks: dataQueryColumnMasks(request.ColumnMasks),
	}
	planningStarted := time.Now()
	plan, err := r.queryPlanner().PlanRawValues(rawRequest)
	planningMS := elapsedStageMS(planningStarted)
	if err != nil {
		return dataquery.Result{}, err
	}
	valueColumn := rawRequest.Measure.Alias
	if valueColumn == "" {
		valueColumn = "value"
	}
	databaseStarted := time.Now()
	markPhysicalStatement(ctx)
	bins, err := r.db.Histogram(ctx, plan, semanticquery.HistogramSpec{
		ValueColumn: valueColumn,
		BinCount:    request.BinCount,
	})
	databaseMS := elapsedStageMS(databaseStarted)
	if err != nil {
		return dataquery.Result{}, err
	}
	rows := make([]dataquery.Row, 0, len(bins))
	for _, bin := range bins {
		rows = append(rows, dataquery.Row{
			"bucket": bin.Bucket,
			"count":  bin.Count,
			"start":  bin.Start,
			"end":    bin.End,
		})
	}
	return dataquery.Result{
		Columns:    dataquery.ColumnsFromNames([]string{"bucket", "count", "start", "end"}),
		Rows:       rows,
		SQL:        plan.SQL,
		PlanningMS: planningMS,
		DatabaseMS: databaseMS,
	}, nil
}

func (r *Runtime) executeSemanticDistribution(ctx context.Context, request dataquery.Query) (dataquery.Result, error) {
	rawRequest := semanticquery.RawValueRequest{
		Table:       request.Target,
		Dimensions:  dataQueryFields(request.Fields),
		Measure:     dataQueryFields([]dataquery.Field{request.Value})[0],
		Filters:     dataQueryFilters(request.Filters),
		ColumnMasks: dataQueryColumnMasks(request.ColumnMasks),
	}
	planningStarted := time.Now()
	plan, err := r.queryPlanner().PlanRawValues(rawRequest)
	planningMS := elapsedStageMS(planningStarted)
	if err != nil {
		return dataquery.Result{}, err
	}
	valueColumn := rawRequest.Measure.Alias
	if valueColumn == "" {
		valueColumn = "value"
	}
	groupColumn := "label"
	if len(rawRequest.Dimensions) > 0 && rawRequest.Dimensions[0].Alias != "" {
		groupColumn = rawRequest.Dimensions[0].Alias
	}
	databaseStarted := time.Now()
	markPhysicalStatement(ctx)
	rows, err := r.db.Distribution(ctx, plan, semanticquery.DistributionSpec{
		GroupColumn: groupColumn,
		ValueColumn: valueColumn,
		Sort:        dataQuerySorts(request.Sort),
		Limit:       request.Limit,
	})
	databaseMS := elapsedStageMS(databaseStarted)
	if err != nil {
		return dataquery.Result{}, err
	}
	return dataquery.Result{
		Columns:    dataquery.ColumnsFromNames([]string{"label", "min", "q1", "median", "q3", "max"}),
		Rows:       dataQueryRows(rows),
		SQL:        plan.SQL,
		PlanningMS: planningMS,
		DatabaseMS: databaseMS,
	}, nil
}

func elapsedStageMS(started time.Time) int64 {
	elapsed := time.Since(started).Milliseconds()
	if elapsed <= 0 {
		return 1
	}
	return elapsed
}

func (r *Runtime) CountModelTable(ctx context.Context, tableName string) (int, error) {
	if r == nil || r.db == nil {
		return 0, fmt.Errorf("materialization runtime is not initialized")
	}
	if _, err := r.modelTable(tableName); err != nil {
		return 0, err
	}
	relation, err := r.physicalModelTable(tableName)
	if err != nil {
		return 0, err
	}
	markPhysicalStatement(ctx)
	return r.db.Count(ctx, semanticquery.Plan{
		SQL:     "SELECT count(*) FROM " + relation,
		Columns: []string{"count"},
	})
}

func (r *Runtime) ModelTableRows(ctx context.Context, request ModelTableQuery) (semanticquery.Rows, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("materialization runtime is not initialized")
	}
	plan, err := r.modelTableQueryPlan(request)
	if err != nil {
		return nil, err
	}
	markPhysicalStatement(ctx)
	return r.db.Query(ctx, plan)
}

func (r *Runtime) modelTableQueryPlan(request ModelTableQuery) (semanticquery.Plan, error) {
	table, err := r.modelTable(request.Table)
	if err != nil {
		return semanticquery.Plan{}, err
	}
	columns, err := modelTableQueryColumns(table, request.Columns)
	if err != nil {
		return semanticquery.Plan{}, err
	}
	relation, err := r.physicalModelTable(request.Table)
	if err != nil {
		return semanticquery.Plan{}, err
	}
	var sql strings.Builder
	sql.WriteString("SELECT ")
	maskSet, err := rawColumnMaskMap(request.ColumnMasks)
	if err != nil {
		return semanticquery.Plan{}, err
	}
	for index, column := range columns {
		if index > 0 {
			sql.WriteString(", ")
		}
		if mask, ok := maskSet[strings.ToLower(request.Table+"."+column)]; ok {
			sql.WriteString(mask)
			sql.WriteString(" AS ")
			sql.WriteString(quoteMaterializedIdentifier(column))
		} else if mask, ok := maskSet[strings.ToLower(column)]; ok {
			sql.WriteString(mask)
			sql.WriteString(" AS ")
			sql.WriteString(quoteMaterializedIdentifier(column))
		} else {
			sql.WriteString(quoteMaterializedIdentifier(column))
		}
	}
	sql.WriteString("\nFROM ")
	sql.WriteString(relation)
	if len(request.Sort) > 0 {
		orderParts := []string{}
		columnSet := modelTableColumnSet(table)
		for _, sortSpec := range request.Sort {
			if !columnSet[sortSpec.Field] {
				return semanticquery.Plan{}, fmt.Errorf("model table %q does not expose sort column %q", request.Table, sortSpec.Field)
			}
			direction := strings.ToUpper(strings.TrimSpace(sortSpec.Direction))
			if direction != "ASC" && direction != "DESC" {
				return semanticquery.Plan{}, fmt.Errorf("unsupported sort direction %q", sortSpec.Direction)
			}
			orderParts = append(orderParts, quoteMaterializedIdentifier(sortSpec.Field)+" "+direction)
		}
		if len(orderParts) > 0 {
			sql.WriteString("\nORDER BY ")
			sql.WriteString(strings.Join(orderParts, ", "))
		}
	}
	if request.Limit > 0 {
		sql.WriteString(fmt.Sprintf("\nLIMIT %d", request.Limit))
	}
	if request.Offset > 0 {
		if request.Limit <= 0 {
			return semanticquery.Plan{}, fmt.Errorf("offset requires limit")
		}
		sql.WriteString(fmt.Sprintf("\nOFFSET %d", request.Offset))
	}
	return semanticquery.Plan{SQL: sql.String(), Columns: columns}, nil
}

func (r *Runtime) physicalModelTable(tableName string) (string, error) {
	quoted, err := quotedModelTableName(tableName)
	if err != nil {
		return "", err
	}
	if r != nil && r.planner != nil && r.planner.TableRelation() != nil {
		return r.planner.TableRelation()(tableName)
	}
	return "model." + quoted, nil
}

func (r *Runtime) modelTable(tableName string) (semanticmodel.Table, error) {
	if r == nil || r.model == nil {
		return semanticmodel.Table{}, fmt.Errorf("semantic model is required")
	}
	tableName = strings.TrimSpace(tableName)
	table, ok := r.model.Tables[tableName]
	if !ok {
		return semanticmodel.Table{}, fmt.Errorf("model table %q is not available in semantic model %q", tableName, r.model.Name)
	}
	return table, nil
}

func modelTableQueryColumns(table semanticmodel.Table, requested []string) ([]string, error) {
	columnSet := modelTableColumnSet(table)
	if len(requested) > 0 {
		columns := []string{}
		for _, column := range requested {
			column = strings.TrimSpace(column)
			if column == "" {
				continue
			}
			if !columnSet[column] {
				return nil, fmt.Errorf("model table does not expose column %q", column)
			}
			columns = append(columns, column)
		}
		if len(columns) > 0 {
			return columns, nil
		}
	}
	if len(table.Schema.Columns) > 0 {
		schemaColumns := append([]semanticmodel.ColumnSchema{}, table.Schema.Columns...)
		sort.SliceStable(schemaColumns, func(i, j int) bool {
			if schemaColumns[i].Ordinal != schemaColumns[j].Ordinal {
				return schemaColumns[i].Ordinal < schemaColumns[j].Ordinal
			}
			return schemaColumns[i].Name < schemaColumns[j].Name
		})
		columns := make([]string, 0, len(schemaColumns))
		for _, column := range schemaColumns {
			if column.Name != "" {
				columns = append(columns, column.Name)
			}
		}
		if len(columns) > 0 {
			return columns, nil
		}
	}
	columns := make([]string, 0, len(table.Columns))
	for name := range table.Columns {
		columns = append(columns, name)
	}
	sort.Strings(columns)
	if len(columns) == 0 {
		return nil, fmt.Errorf("model table has no columns")
	}
	return columns, nil
}

func modelTableColumnSet(table semanticmodel.Table) map[string]bool {
	columns := map[string]bool{}
	for name := range table.Columns {
		columns[name] = true
	}
	for _, column := range table.Schema.Columns {
		if column.Name != "" {
			columns[column.Name] = true
		}
	}
	return columns
}

func quotedModelTableName(tableName string) (string, error) {
	if err := validateIdentifier(tableName); err != nil {
		return "", err
	}
	return quoteMaterializedIdentifier(tableName), nil
}

func rawRelationPlan(relation string, columns []string, sort []dataquery.Sort, masks []dataquery.ColumnMask, offset, limit int) (semanticquery.Plan, error) {
	columnSet := map[string]bool{}
	for _, column := range columns {
		if err := validateIdentifier(column); err != nil {
			return semanticquery.Plan{}, err
		}
		columnSet[column] = true
	}
	var sql strings.Builder
	sql.WriteString("WITH data AS (")
	sql.WriteString(relation)
	sql.WriteString(")\nSELECT ")
	maskSet, err := rawColumnMaskMap(dataQueryColumnMasks(masks))
	if err != nil {
		return semanticquery.Plan{}, err
	}
	for index, column := range columns {
		if index > 0 {
			sql.WriteString(", ")
		}
		if mask, ok := maskSet[strings.ToLower(column)]; ok {
			sql.WriteString(mask)
			sql.WriteString(" AS ")
			sql.WriteString(quoteMaterializedIdentifier(column))
		} else {
			sql.WriteString(quoteMaterializedIdentifier(column))
		}
	}
	sql.WriteString(" FROM data")
	if len(sort) > 0 {
		parts := []string{}
		for _, sortSpec := range sort {
			if !columnSet[sortSpec.Field] {
				return semanticquery.Plan{}, fmt.Errorf("raw data does not expose sort column %q", sortSpec.Field)
			}
			direction := strings.ToUpper(strings.TrimSpace(sortSpec.Direction))
			if direction != "ASC" && direction != "DESC" {
				return semanticquery.Plan{}, fmt.Errorf("unsupported sort direction %q", sortSpec.Direction)
			}
			parts = append(parts, quoteMaterializedIdentifier(sortSpec.Field)+" "+direction)
		}
		if len(parts) > 0 {
			sql.WriteString("\nORDER BY ")
			sql.WriteString(strings.Join(parts, ", "))
		}
	}
	if limit > 0 {
		sql.WriteString(fmt.Sprintf("\nLIMIT %d", limit))
	}
	if offset > 0 {
		if limit <= 0 {
			return semanticquery.Plan{}, fmt.Errorf("offset requires limit")
		}
		sql.WriteString(fmt.Sprintf("\nOFFSET %d", offset))
	}
	return semanticquery.Plan{SQL: sql.String(), Columns: columns}, nil
}

func sourceQueryColumns(source semanticmodel.Source, requested []string) ([]string, error) {
	columnSet := sourceColumnSet(source)
	if len(requested) > 0 {
		columns := []string{}
		for _, column := range requested {
			column = strings.TrimSpace(column)
			if column == "" {
				continue
			}
			if !columnSet[column] {
				return nil, fmt.Errorf("source does not expose column %q", column)
			}
			columns = append(columns, column)
		}
		if len(columns) > 0 {
			return columns, nil
		}
	}
	if len(source.Schema.Columns) > 0 {
		schemaColumns := append([]semanticmodel.ColumnSchema{}, source.Schema.Columns...)
		sort.SliceStable(schemaColumns, func(i, j int) bool {
			if schemaColumns[i].Ordinal != schemaColumns[j].Ordinal {
				return schemaColumns[i].Ordinal < schemaColumns[j].Ordinal
			}
			return schemaColumns[i].Name < schemaColumns[j].Name
		})
		columns := make([]string, 0, len(schemaColumns))
		for _, column := range schemaColumns {
			if column.Name != "" {
				columns = append(columns, column.Name)
			}
		}
		if len(columns) > 0 {
			return columns, nil
		}
	}
	columns := make([]string, 0, len(source.Fields))
	for name := range source.Fields {
		columns = append(columns, name)
	}
	sort.Strings(columns)
	if len(columns) == 0 {
		return nil, fmt.Errorf("source has no columns")
	}
	return columns, nil
}

func sourceColumnSet(source semanticmodel.Source) map[string]bool {
	columns := map[string]bool{}
	for name := range source.Fields {
		columns[name] = true
	}
	for _, column := range source.Schema.Columns {
		if column.Name != "" {
			columns[column.Name] = true
		}
	}
	return columns
}

func sourceInModel(model *semanticmodel.Model, key string) (semanticmodel.Source, bool) {
	if model == nil {
		return semanticmodel.Source{}, false
	}
	key = strings.TrimSpace(key)
	if source, ok := model.Sources[key]; ok {
		return source, true
	}
	localKey := strings.ReplaceAll(key, ".", "_")
	if source, ok := model.Sources[localKey]; ok {
		return source, true
	}
	return semanticmodel.Source{}, false
}

func dataQueryFields(fields []dataquery.Field) []semanticquery.Field {
	out := make([]semanticquery.Field, 0, len(fields))
	for _, field := range fields {
		out = append(out, semanticquery.Field{
			Field: field.Field,
			Alias: field.Alias,
		})
	}
	return out
}

func dataQueryFilters(filters []dataquery.Filter) []semanticquery.Filter {
	out := make([]semanticquery.Filter, 0, len(filters))
	for _, filter := range filters {
		groups := make([]semanticquery.FilterGroup, 0, len(filter.Groups))
		for _, group := range filter.Groups {
			groups = append(groups, semanticquery.FilterGroup{Filters: dataQueryFilters(group.Filters)})
		}
		out = append(out, semanticquery.Filter{
			Field:    filter.Field,
			Fact:     filter.Fact,
			Operator: filter.Operator,
			Values:   append([]any{}, filter.Values...),
			Groups:   groups,
		})
	}
	return out
}

func dataQuerySorts(sort []dataquery.Sort) []semanticquery.Sort {
	out := make([]semanticquery.Sort, 0, len(sort))
	for _, item := range sort {
		out = append(out, semanticquery.Sort{Field: item.Field, Direction: item.Direction})
	}
	return out
}

func dataQueryColumnMasks(masks []dataquery.ColumnMask) []semanticquery.ColumnMask {
	out := make([]semanticquery.ColumnMask, 0, len(masks))
	for _, mask := range masks {
		out = append(out, semanticquery.ColumnMask{Field: mask.Field, Mask: mask.Mask})
	}
	return out
}

func rawColumnMaskMap(masks []semanticquery.ColumnMask) (map[string]string, error) {
	out := map[string]string{}
	for _, mask := range masks {
		field := strings.ToLower(strings.TrimSpace(mask.Field))
		if field == "" {
			continue
		}
		expr, err := rawMaskSQLExpr(mask.Mask)
		if err != nil {
			return nil, err
		}
		out[field] = expr
	}
	return out, nil
}

func rawMaskSQLExpr(mask string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(mask)) {
	case "", "null":
		return "NULL", nil
	case "redact", "redacted":
		return "'REDACTED'", nil
	case "zero":
		return "0", nil
	default:
		return "", fmt.Errorf("unsupported column mask %q", mask)
	}
}

func dataQueryRows(rows semanticquery.Rows) []dataquery.Row {
	out := make([]dataquery.Row, 0, len(rows))
	for _, row := range rows {
		converted := dataquery.Row{}
		for key, value := range row {
			converted[key] = value
		}
		out = append(out, converted)
	}
	return out
}

func quoteMaterializedIdentifier(identifier string) string {
	return `"` + strings.ReplaceAll(identifier, `"`, `""`) + `"`
}

func (r *Runtime) LastRefresh() time.Time {
	if r == nil {
		return time.Time{}
	}
	return r.lastRefresh
}

func (r *Runtime) DBPath() string {
	if r == nil {
		return ""
	}
	return r.db.Path()
}
