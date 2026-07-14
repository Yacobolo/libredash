package materialize

import (
	"container/list"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/Yacobolo/libredash/internal/dataquery"
	"golang.org/x/sync/singleflight"
)

type queryResultCache struct {
	mu         sync.Mutex
	capacity   int
	entries    map[string]*list.Element
	lru        *list.List
	group      singleflight.Group
	generation uint64
}

type queryResultCacheEntry struct {
	key    string
	result dataquery.Result
}

func newQueryResultCache(capacity int) *queryResultCache {
	return &queryResultCache{capacity: capacity, entries: map[string]*list.Element{}, lru: list.New()}
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

func (c *queryResultCache) cacheKey(request dataquery.Query) (string, uint64, error) {
	keyBytes, err := json.Marshal(queryResultCacheKey{
		WorkspaceID:  request.WorkspaceID,
		Operation:    request.Operation,
		ModelID:      request.ModelID,
		Kind:         request.Kind,
		Target:       request.Target,
		Fields:       request.Fields,
		Measures:     request.Measures,
		Time:         request.Time,
		Filters:      request.Filters,
		Sort:         request.Sort,
		ColumnMasks:  request.ColumnMasks,
		Offset:       request.Offset,
		Limit:        request.Limit,
		BinCount:     request.BinCount,
		IncludeTotal: request.IncludeTotal,
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
	WorkspaceID  string
	Operation    string
	ModelID      string
	Kind         dataquery.Kind
	Target       string
	Fields       []dataquery.Field
	Measures     []dataquery.Field
	Time         dataquery.Time
	Filters      []dataquery.Filter
	Sort         []dataquery.Sort
	ColumnMasks  []dataquery.ColumnMask
	Offset       int
	Limit        int
	BinCount     int
	IncludeTotal bool
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
	if element, ok := c.entries[key]; ok {
		element.Value = queryResultCacheEntry{key: key, result: cloneDataQueryResult(result)}
		c.lru.MoveToFront(element)
		return
	}
	element := c.lru.PushFront(queryResultCacheEntry{key: key, result: cloneDataQueryResult(result)})
	c.entries[key] = element
	for c.lru.Len() > c.capacity {
		oldest := c.lru.Back()
		delete(c.entries, oldest.Value.(queryResultCacheEntry).key)
		c.lru.Remove(oldest)
	}
}

func (c *queryResultCache) clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = map[string]*list.Element{}
	c.lru.Init()
	c.generation++
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
