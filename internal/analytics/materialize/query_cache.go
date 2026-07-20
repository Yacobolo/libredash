package materialize

import (
	"container/list"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/Yacobolo/leapview/internal/dataquery"
	"golang.org/x/sync/singleflight"
)

type queryResultCache struct {
	mu           sync.Mutex
	capacity     int
	maxBytes     int64
	currentBytes int64
	namespace    string
	entries      map[string]*list.Element
	lru          *list.List
	group        singleflight.Group
	generation   uint64
}

type queryResultCacheEntry struct {
	key    string
	result dataquery.Result
	bytes  int64
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
	return &queryResultCache{capacity: capacity, maxBytes: maxBytes, namespace: namespace, entries: map[string]*list.Element{}, lru: list.New()}
}

func (c *queryResultCache) execute(ctx context.Context, request dataquery.Query, execute func() (dataquery.Result, error)) (dataquery.Result, error) {
	key, generation, err := c.cacheKey(request)
	if err != nil {
		return dataquery.Result{}, err
	}
	if cached, ok := c.get(key); ok {
		return cached, nil
	}
	flightKey := fmt.Sprintf("%d:%s", generation, key)
	for {
		resultChannel := c.group.DoChan(flightKey, func() (any, error) {
			if cached, ok := c.get(key); ok {
				return cached, nil
			}
			result, executeErr := execute()
			if ownerErr := ctx.Err(); ownerErr != nil {
				// The execution closure belongs to whichever caller created the
				// singleflight. Mark owner cancellation separately so live waiters
				// can replace this flight using their own execution context.
				return dataquery.Result{}, canceledQueryCacheFlightError{err: ownerErr}
			}
			if executeErr != nil {
				return result, executeErr
			}
			result.CacheOutcome = dataquery.CacheMiss
			c.put(key, generation, result)
			return cloneDataQueryResult(result), nil
		})
		select {
		case <-ctx.Done():
			return dataquery.Result{}, ctx.Err()
		case call := <-resultChannel:
			if call.Err != nil {
				var canceledFlight canceledQueryCacheFlightError
				if errors.As(call.Err, &canceledFlight) && ctx.Err() == nil {
					// singleflight removes a completed call before publishing its
					// result, so the next iteration safely starts or joins a live
					// replacement without inheriting the canceled owner's context.
					continue
				}
				return dataquery.Result{}, call.Err
			}
			result := cloneDataQueryResult(call.Val.(dataquery.Result))
			if call.Shared {
				result.CacheOutcome = dataquery.CacheCoalesced
			}
			return result, nil
		}
	}
}

// coalesce runs one exact multi-query flight at a time without caching the
// opaque result. Callers remain responsible for generation-safe per-query
// stores after they have checked cancellation and decoded every branch.
func (c *queryResultCache) coalesce(ctx context.Context, key string, execute func() (any, error)) (any, bool, error) {
	flightKey := "bundle:" + key
	for {
		resultChannel := c.group.DoChan(flightKey, func() (any, error) {
			result, err := execute()
			if ownerErr := ctx.Err(); ownerErr != nil {
				return nil, canceledQueryCacheFlightError{err: ownerErr}
			}
			return result, err
		})
		select {
		case <-ctx.Done():
			return nil, false, ctx.Err()
		case call := <-resultChannel:
			if err := ctx.Err(); err != nil {
				return nil, false, err
			}
			if call.Err != nil {
				var canceledFlight canceledQueryCacheFlightError
				if errors.As(call.Err, &canceledFlight) && ctx.Err() == nil {
					continue
				}
				return nil, call.Shared, call.Err
			}
			return call.Val, call.Shared, nil
		}
	}
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

func (c *queryResultCache) cacheKey(request dataquery.Query) (string, uint64, error) {
	keyBytes, err := json.Marshal(queryResultCacheKey{
		Namespace:           c.namespace,
		WorkspaceID:         request.WorkspaceID,
		Operation:           request.Operation,
		ModelID:             request.ModelID,
		Kind:                request.Kind,
		Target:              request.Target,
		Fields:              request.Fields,
		Measures:            request.Measures,
		AuthorizationFields: request.AuthorizationFields,
		Value:               request.Value,
		Time:                request.Time,
		Filters:             request.Filters,
		Sort:                request.Sort,
		ColumnMasks:         request.ColumnMasks,
		Offset:              request.Offset,
		Limit:               request.Limit,
		BinCount:            request.BinCount,
		IncludeTotal:        request.IncludeTotal,
	})
	if err != nil {
		return "", 0, fmt.Errorf("encode governed query cache key: %w", err)
	}
	key := string(keyBytes)
	c.mu.Lock()
	generation := c.generation
	c.mu.Unlock()
	return key, generation, nil
}

type canceledQueryCacheFlightError struct {
	err error
}

func (e canceledQueryCacheFlightError) Error() string { return e.err.Error() }
func (e canceledQueryCacheFlightError) Unwrap() error { return e.err }

// queryResultCacheKey intentionally excludes request, correlation, and principal IDs.
// Authorization has already rewritten the filters and masks before this key is built,
// so equivalent governed query shapes can share a result without observability IDs
// defeating warm-cache hits. Runtime replacement/generation provides snapshot safety.
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
	c.mu.Lock()
	defer c.mu.Unlock()
	element, ok := c.entries[key]
	if !ok {
		return dataquery.Result{}, false
	}
	c.lru.MoveToFront(element)
	result := cloneDataQueryResult(element.Value.(queryResultCacheEntry).result)
	result.CacheOutcome = dataquery.CacheHit
	result.QueueWaitMS = 0
	result.PlanningMS = 0
	result.ConnectionWaitMS = 0
	result.DatabaseMS = 0
	result.ExecutionMS = 0
	return result, true
}

func (c *queryResultCache) put(key string, generation uint64, result dataquery.Result) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if generation != c.generation {
		return
	}
	entryBytes := int64(len(key)) + estimateDataQueryResultBytes(result)
	if entryBytes > c.maxBytes {
		return
	}
	if element, ok := c.entries[key]; ok {
		previous := element.Value.(queryResultCacheEntry)
		c.currentBytes -= previous.bytes
		element.Value = queryResultCacheEntry{key: key, result: cloneDataQueryResult(result), bytes: entryBytes}
		c.currentBytes += entryBytes
		c.lru.MoveToFront(element)
	} else {
		element := c.lru.PushFront(queryResultCacheEntry{key: key, result: cloneDataQueryResult(result), bytes: entryBytes})
		c.entries[key] = element
		c.currentBytes += entryBytes
	}
	for c.lru.Len() > c.capacity || c.currentBytes > c.maxBytes {
		oldest := c.lru.Back()
		entry := oldest.Value.(queryResultCacheEntry)
		delete(c.entries, entry.key)
		c.currentBytes -= entry.bytes
		c.lru.Remove(oldest)
	}
}

func (c *queryResultCache) clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = map[string]*list.Element{}
	c.lru.Init()
	c.currentBytes = 0
	c.generation++
}

func estimateDataQueryResultBytes(result dataquery.Result) int64 {
	size := int64(len(result.SQL) + len(result.PlanText) + len(result.Error) + len(result.ExecutionState) + len(result.CacheOutcome) + len(result.Status))
	for _, column := range result.Columns {
		size += int64(len(column.Name)) + 16
	}
	for _, warning := range result.Warnings {
		size += int64(len(warning)) + 16
	}
	for _, row := range result.Rows {
		size += 48
		for key, value := range row {
			size += int64(len(key)) + estimateDataQueryValueBytes(value)
		}
	}
	return size + 128
}

func estimateDataQueryValueBytes(value any) int64 {
	switch typed := value.(type) {
	case nil:
		return 1
	case string:
		return int64(len(typed)) + 16
	case []byte:
		return int64(len(typed)) + 24
	case []string:
		size := int64(24)
		for _, item := range typed {
			size += int64(len(item)) + 16
		}
		return size
	case []any:
		size := int64(24)
		for _, item := range typed {
			size += estimateDataQueryValueBytes(item)
		}
		return size
	case map[string]any:
		size := int64(48)
		for key, item := range typed {
			size += int64(len(key)) + estimateDataQueryValueBytes(item)
		}
		return size
	case bool:
		return 1
	default:
		return 16
	}
}

func cloneDataQueryResult(result dataquery.Result) dataquery.Result {
	clone := result
	clone.Columns = append([]dataquery.Column{}, result.Columns...)
	clone.Rows = make([]dataquery.Row, len(result.Rows))
	for index, row := range result.Rows {
		clone.Rows[index] = make(dataquery.Row, len(row))
		for key, value := range row {
			clone.Rows[index][key] = cloneDataQueryValue(value)
		}
	}
	clone.Warnings = append([]string{}, result.Warnings...)
	return clone
}

func cloneDataQueryValue(value any) any {
	switch typed := value.(type) {
	case []byte:
		return append([]byte{}, typed...)
	case []string:
		return append([]string{}, typed...)
	case []any:
		out := make([]any, len(typed))
		for index, item := range typed {
			out[index] = cloneDataQueryValue(item)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, item := range typed {
			out[key] = cloneDataQueryValue(item)
		}
		return out
	default:
		return value
	}
}
