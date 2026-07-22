package materialize

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/Yacobolo/leapview/internal/analytics/resultcache"
	"github.com/Yacobolo/leapview/internal/dataquery"
)

var localCacheID atomic.Uint64

// queryResultCache retains the materialization-specific key contract while the
// resultcache scope owns retention, hierarchy, invalidation, and coalescing.
type queryResultCache struct {
	mu           sync.Mutex
	namespace    string
	pool         *resultcache.Pool
	scope        *resultcache.Scope
	owned        bool
	capacity     int
	maxBytes     int64
	currentBytes int64
	generation   uint64
}

func newQueryResultCache(capacity int, namespace string) *queryResultCache {
	return newQueryResultCacheWithLimits(capacity, 64<<20, namespace)
}

func newQueryResultCacheWithLimits(capacity int, maxBytes int64, namespace string) *queryResultCache {
	if capacity <= 0 {
		capacity = 1
	}
	if maxBytes <= 0 {
		maxBytes = 1
	}
	pool, err := resultcache.New(resultcache.Limits{RuntimeEntries: capacity, RuntimeBytes: maxBytes, WorkspaceEntries: capacity, WorkspaceBytes: maxBytes, NodeEntries: capacity, NodeBytes: maxBytes})
	if err != nil {
		panic(err)
	}
	id := fmt.Sprintf("local-%d", localCacheID.Add(1))
	scope, err := pool.OpenScope(resultcache.ScopeID{WorkspaceID: "_local", RuntimeID: id})
	if err != nil {
		panic(err)
	}
	return &queryResultCache{namespace: namespace, pool: pool, scope: scope, owned: true, capacity: capacity, maxBytes: maxBytes}
}

func newQueryResultCacheWithScope(scope *resultcache.Scope, namespace string) *queryResultCache {
	return &queryResultCache{namespace: namespace, scope: scope}
}

func (c *queryResultCache) execute(ctx context.Context, request dataquery.Query, execute func() (dataquery.Result, error)) (dataquery.Result, error) {
	key, generation, err := c.cacheKey(request)
	if err != nil {
		return dataquery.Result{}, err
	}
	if cached, ok := c.get(key); ok {
		return cached, nil
	}
	value, shared, err := c.scope.Coalesce(ctx, fmt.Sprintf("query:%d:%s", generation, key), func() (any, error) {
		if cached, ok := c.get(key); ok {
			return cached, nil
		}
		result, executeErr := execute()
		if ownerErr := ctx.Err(); ownerErr != nil {
			return dataquery.Result{}, canceledQueryCacheFlightError{err: ownerErr}
		}
		if executeErr != nil {
			return result, executeErr
		}
		result.CacheOutcome = dataquery.CacheMiss
		c.put(key, generation, result)
		return resultcache.CloneResult(result), nil
	})
	if err != nil {
		return dataquery.Result{}, err
	}
	result := resultcache.CloneResult(value.(dataquery.Result))
	if shared {
		result.CacheOutcome = dataquery.CacheCoalesced
	}
	return result, nil
}

func (c *queryResultCache) coalesce(ctx context.Context, key string, execute func() (any, error)) (any, bool, error) {
	return c.scope.Coalesce(ctx, "bundle:"+key, func() (any, error) {
		result, err := execute()
		if ownerErr := ctx.Err(); ownerErr != nil {
			return nil, canceledQueryCacheFlightError{err: ownerErr}
		}
		return result, err
	})
}

func (c *queryResultCache) lookup(request dataquery.Query) (dataquery.Result, string, uint64, bool, error) {
	key, generation, err := c.cacheKey(request)
	if err != nil {
		return dataquery.Result{}, "", 0, false, err
	}
	result, ok := c.get(key)
	return result, key, generation, ok, nil
}

func (c *queryResultCache) store(key string, generation uint64, result dataquery.Result) {
	result.CacheOutcome = dataquery.CacheMiss
	c.put(key, generation, result)
}

func (c *queryResultCache) remove(request dataquery.Query) {
	key, _, err := c.cacheKey(request)
	if err != nil {
		return
	}
	c.scope.Delete(key)
	c.syncStats()
}

func (c *queryResultCache) cacheKey(request dataquery.Query) (string, uint64, error) {
	keyBytes, err := json.Marshal(queryResultCacheKey{Namespace: c.namespace, WorkspaceID: request.WorkspaceID, Operation: request.Operation, ModelID: request.ModelID, Kind: request.Kind, Target: request.Target, Fields: request.Fields, Measures: request.Measures, AuthorizationFields: request.AuthorizationFields, Value: request.Value, Time: request.Time, Filters: request.Filters, Sort: request.Sort, ColumnMasks: request.ColumnMasks, Offset: request.Offset, Limit: request.Limit, BinCount: request.BinCount, IncludeTotal: request.IncludeTotal})
	if err != nil {
		return "", 0, fmt.Errorf("encode governed query cache key: %w", err)
	}
	generation := uint64(c.scope.Generation())
	c.mu.Lock()
	c.generation = generation
	c.mu.Unlock()
	return string(keyBytes), generation, nil
}

type canceledQueryCacheFlightError struct{ err error }

func (e canceledQueryCacheFlightError) Error() string { return e.err.Error() }
func (e canceledQueryCacheFlightError) Unwrap() error { return e.err }

type queryResultCacheKey struct {
	Namespace           string
	WorkspaceID         string
	Operation           string
	ModelID             string
	Kind                dataquery.Kind
	Target              string
	Fields              []dataquery.Field
	Measures            []dataquery.Field
	AuthorizationFields []dataquery.Field
	Value               dataquery.Field
	Time                dataquery.Time
	Filters             []dataquery.Filter
	Sort                []dataquery.Sort
	ColumnMasks         []dataquery.ColumnMask
	Offset              int
	Limit               int
	BinCount            int
	IncludeTotal        bool
}

func (c *queryResultCache) get(key string) (dataquery.Result, bool) {
	result, _, ok, err := c.scope.Lookup(key)
	if err != nil {
		return dataquery.Result{}, false
	}
	c.syncStats()
	return result, ok
}

func (c *queryResultCache) put(key string, generation uint64, result dataquery.Result) {
	c.scope.Store(key, resultcache.Token(generation), result)
	c.syncStats()
}

func (c *queryResultCache) clear() {
	c.scope.Invalidate()
	c.mu.Lock()
	c.generation = uint64(c.scope.Generation())
	c.mu.Unlock()
	c.syncStats()
}

func (c *queryResultCache) close() error {
	if c == nil || !c.owned {
		return nil
	}
	return errors.Join(c.scope.Close(), c.pool.Close())
}

func (c *queryResultCache) syncStats() {
	stats := c.scope.Stats()
	c.mu.Lock()
	c.currentBytes = stats.Bytes
	c.mu.Unlock()
}

func estimateDataQueryResultBytes(result dataquery.Result) int64 {
	return resultcache.EstimateResultBytes(result)
}
func cloneDataQueryResult(result dataquery.Result) dataquery.Result {
	return resultcache.CloneResult(result)
}
