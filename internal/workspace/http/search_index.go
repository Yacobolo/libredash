package http

import (
	"strings"
	"sync"

	"github.com/Yacobolo/leapview/internal/workspace/search"
)

const maxCachedWorkspaceSearchIndexes = 64

type cachedSearchIndex struct {
	index    *search.Index
	lastUsed uint64
}

type pendingSearchIndex struct {
	done  chan struct{}
	index *search.Index
}

// SearchIndexCache keeps immutable indexes by serving-state revision. The
// bounded LRU permits old in-flight requests to finish without replacing a
// newer generation's index.
type SearchIndexCache struct {
	mu      sync.Mutex
	clock   uint64
	entries map[string]cachedSearchIndex
	pending map[string]*pendingSearchIndex
}

func (c *SearchIndexCache) index(workspaceID, environment, revision string, build func() *search.Index) *search.Index {
	if c == nil || strings.TrimSpace(revision) == "" {
		return build()
	}
	key := strings.Join([]string{workspaceID, environment, revision}, "\x00")
	c.mu.Lock()
	if c.entries == nil {
		c.entries = map[string]cachedSearchIndex{}
		c.pending = map[string]*pendingSearchIndex{}
	}
	if entry, ok := c.entries[key]; ok {
		c.clock++
		entry.lastUsed = c.clock
		c.entries[key] = entry
		c.mu.Unlock()
		return entry.index
	}
	if pending := c.pending[key]; pending != nil {
		c.mu.Unlock()
		<-pending.done
		return pending.index
	}
	pending := &pendingSearchIndex{done: make(chan struct{})}
	c.pending[key] = pending
	c.mu.Unlock()

	index := build()

	c.mu.Lock()
	c.clock++
	c.entries[key] = cachedSearchIndex{index: index, lastUsed: c.clock}
	pending.index = index
	delete(c.pending, key)
	close(pending.done)
	c.evictLocked()
	c.mu.Unlock()
	return index
}

func (c *SearchIndexCache) evictLocked() {
	for len(c.entries) > maxCachedWorkspaceSearchIndexes {
		oldestKey := ""
		oldestUse := ^uint64(0)
		for key, entry := range c.entries {
			if entry.lastUsed < oldestUse {
				oldestKey, oldestUse = key, entry.lastUsed
			}
		}
		delete(c.entries, oldestKey)
	}
}
