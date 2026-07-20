package search

import (
	"container/heap"
	"fmt"
	"sort"
	"strings"
	"unicode"
)

var allowedTypes = map[string]struct{}{
	"asset":          {},
	"dashboard":      {},
	"dataset":        {},
	"field":          {},
	"filter":         {},
	"measure":        {},
	"page":           {},
	"semantic_model": {},
	"source":         {},
	"visual":         {},
}

var queryStopWords = map[string]struct{}{
	"a": {}, "an": {}, "and": {}, "by": {}, "for": {}, "in": {}, "of": {}, "on": {}, "the": {}, "to": {},
}

type TypeSet map[string]struct{}

type Refs struct {
	ComponentID string
	DashboardID string
	PageID      string
	VisualID    string
	TableID     string
	FilterID    string
	ModelID     string
	DatasetID   string
	FieldID     string
	AssetID     string
}

type Document struct {
	ID          string
	Type        string
	Name        string
	Description string
	Terms       []string
	Weight      int
	Refs        Refs
}

type Query struct {
	Text  string
	Types TypeSet
}

type Result struct {
	ID          string
	Type        string
	Name        string
	Description string
	Score       int
	Refs
}

type rankedDocument struct {
	document Document
	score    int
	order    int
}

func ParseTypes(raw string) (TypeSet, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	out := TypeSet{}
	for _, part := range strings.Split(raw, ",") {
		typ := strings.TrimSpace(part)
		if typ == "" {
			continue
		}
		if _, ok := allowedTypes[typ]; !ok {
			return nil, fmt.Errorf("unknown search type %q", typ)
		}
		out[typ] = struct{}{}
	}
	return out, nil
}

func Rank(documents []Document, query Query) []Result {
	return rank(documents, query, 0)
}

// RankLimit returns the same deterministic prefix as Rank without sorting or
// allocating results for every matching document.
func RankLimit(documents []Document, query Query, limit int) []Result {
	if limit <= 0 {
		return Rank(documents, query)
	}
	return rank(documents, query, limit)
}

func rank(documents []Document, query Query, limit int) []Result {
	tokens := tokens(query.Text)
	capacity := len(documents)
	if limit > 0 && capacity > limit {
		capacity = limit
	}
	ranked := make([]rankedDocument, 0, capacity)
	rankedHeap := (*rankedDocumentHeap)(&ranked)
	for order, document := range documents {
		if len(query.Types) > 0 {
			if _, ok := query.Types[document.Type]; !ok {
				continue
			}
		}
		score, ok := documentScore(document, tokens)
		if !ok {
			continue
		}
		candidate := rankedDocument{document: document, score: document.Weight + score, order: order}
		if limit <= 0 {
			ranked = append(ranked, candidate)
			continue
		}
		if len(ranked) < limit {
			heap.Push(rankedHeap, candidate)
			continue
		}
		if rankedDocumentBetter(candidate, ranked[0]) {
			ranked[0] = candidate
			heap.Fix(rankedHeap, 0)
		}
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		return rankedDocumentBetter(ranked[i], ranked[j])
	})
	out := make([]Result, 0, len(ranked))
	for _, item := range ranked {
		document := item.document
		out = append(out, Result{
			ID:          document.ID,
			Type:        document.Type,
			Name:        document.Name,
			Description: document.Description,
			Score:       item.score,
			Refs:        document.Refs,
		})
	}
	return out
}

func rankedDocumentBetter(left, right rankedDocument) bool {
	if left.score != right.score {
		return left.score > right.score
	}
	if left.document.Type != right.document.Type {
		return left.document.Type < right.document.Type
	}
	if left.document.Name != right.document.Name {
		return left.document.Name < right.document.Name
	}
	if left.document.ID != right.document.ID {
		return left.document.ID < right.document.ID
	}
	return left.order < right.order
}

type rankedDocumentHeap []rankedDocument

func (h rankedDocumentHeap) Len() int { return len(h) }
func (h rankedDocumentHeap) Less(i, j int) bool {
	return rankedDocumentBetter(h[j], h[i])
}
func (h rankedDocumentHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }
func (h *rankedDocumentHeap) Push(value any) {
	*h = append(*h, value.(rankedDocument))
}
func (h *rankedDocumentHeap) Pop() any {
	old := *h
	last := len(old) - 1
	value := old[last]
	*h = old[:last]
	return value
}

func documentScore(document Document, queryTokens []string) (int, bool) {
	if len(queryTokens) == 0 {
		return 0, true
	}
	name := strings.ToLower(document.Name)
	id := strings.ToLower(document.ID)
	description := strings.ToLower(document.Description)
	terms := strings.ToLower(strings.Join(document.Terms, " "))
	total := 0
	for _, token := range queryTokens {
		score := 0
		switch {
		case name == token:
			score = 120
		case wordHasPrefix(name, token):
			score = 90
		case strings.Contains(name, token):
			score = 75
		case id == token || wordHasPrefix(id, token):
			score = 70
		case strings.Contains(id, token):
			score = 55
		case strings.Contains(description, token):
			score = 35
		case strings.Contains(terms, token):
			score = 20
		}
		if score == 0 {
			return 0, false
		}
		total += score
	}
	return total, true
}

func tokens(query string) []string {
	fields := strings.FieldsFunc(strings.ToLower(query), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_'
	})
	out := make([]string, 0, len(fields))
	seen := map[string]struct{}{}
	for _, field := range fields {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}
		if _, ignored := queryStopWords[field]; ignored {
			continue
		}
		if _, ok := seen[field]; ok {
			continue
		}
		seen[field] = struct{}{}
		out = append(out, field)
	}
	return out
}

func wordHasPrefix(value, prefix string) bool {
	for _, word := range tokens(value) {
		if strings.HasPrefix(word, prefix) {
			return true
		}
	}
	return false
}
