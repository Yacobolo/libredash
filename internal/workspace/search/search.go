package search

import (
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

type TypeSet map[string]struct{}

type Refs struct {
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
	Refs
}

type rankedDocument struct {
	document Document
	score    int
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
	tokens := tokens(query.Text)
	ranked := make([]rankedDocument, 0, len(documents))
	for _, document := range documents {
		if len(query.Types) > 0 {
			if _, ok := query.Types[document.Type]; !ok {
				continue
			}
		}
		score, ok := documentScore(document, tokens)
		if !ok {
			continue
		}
		ranked = append(ranked, rankedDocument{document: document, score: document.Weight + score})
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].score != ranked[j].score {
			return ranked[i].score > ranked[j].score
		}
		if ranked[i].document.Type != ranked[j].document.Type {
			return ranked[i].document.Type < ranked[j].document.Type
		}
		if ranked[i].document.Name != ranked[j].document.Name {
			return ranked[i].document.Name < ranked[j].document.Name
		}
		return ranked[i].document.ID < ranked[j].document.ID
	})
	out := make([]Result, 0, len(ranked))
	for _, item := range ranked {
		document := item.document
		out = append(out, Result{
			ID:          document.ID,
			Type:        document.Type,
			Name:        document.Name,
			Description: document.Description,
			Refs:        document.Refs,
		})
	}
	return out
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
