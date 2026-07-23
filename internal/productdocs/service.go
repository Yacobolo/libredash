// Package productdocs provides bounded search and read access to LeapView's
// generated, embedded documentation catalog.
package productdocs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"strconv"
	"strings"
	"unicode/utf8"
)

const (
	DefaultSearchLimit = 8
	MaxSearchLimit     = 25
	DefaultReadLimit   = 200
	MaxReadLimit       = 500
	MaxReadBytes       = 32 * 1024
	MaxLineLength      = 2000
)

var (
	ErrInvalid  = errors.New("invalid documentation request")
	ErrNotFound = errors.New("documentation not found")
)

type Document struct {
	Slug     string
	Title    string
	Summary  string
	Source   string
	Markdown string
}

type SearchMatch struct {
	Slug    string
	Excerpt string
}

type SearchIndex interface {
	Slugs(context.Context) ([]string, error)
	Search(context.Context, string, int) ([]SearchMatch, error)
	Close() error
}

type Reference struct {
	ID      string `json:"id"`
	Path    string `json:"path"`
	Title   string `json:"title"`
	Summary string `json:"summary"`
	URL     string `json:"url"`
	Excerpt string `json:"excerpt"`
}

type SearchRequest struct {
	Query string `json:"query"`
	Path  string `json:"path,omitempty"`
	Limit int    `json:"limit,omitempty"`
}

type SearchResult struct {
	Query     string      `json:"query"`
	Path      string      `json:"path,omitempty"`
	Matches   []Reference `json:"matches"`
	Truncated bool        `json:"truncated"`
}

type ReadRequest struct {
	ID     string `json:"id"`
	Offset int    `json:"offset,omitempty"`
	Limit  int    `json:"limit,omitempty"`
}

type ReadResult struct {
	ID         string `json:"id"`
	Path       string `json:"path"`
	Title      string `json:"title"`
	URL        string `json:"url"`
	Content    string `json:"content"`
	LineStart  int    `json:"lineStart"`
	LineEnd    int    `json:"lineEnd"`
	TotalLines int    `json:"totalLines"`
	NextOffset int    `json:"nextOffset,omitempty"`
	Truncated  bool   `json:"truncated"`
}

type Service struct {
	documents []Document
	bySlug    map[string]Document
	search    SearchIndex
}

func New(files fs.FS, index SearchIndex) (*Service, error) {
	documents, bySlug, err := loadDocuments(files)
	if err != nil {
		return nil, err
	}
	if index == nil {
		return nil, fmt.Errorf("documentation search index is not configured")
	}
	slugs, err := index.Slugs(context.Background())
	if err != nil {
		_ = index.Close()
		return nil, fmt.Errorf("read documentation search index: %w", err)
	}
	if len(slugs) != len(documents) {
		_ = index.Close()
		return nil, fmt.Errorf("documentation search index contains %d documents, catalog contains %d", len(slugs), len(documents))
	}
	for _, slug := range slugs {
		if _, ok := bySlug[slug]; !ok {
			_ = index.Close()
			return nil, fmt.Errorf("documentation search index contains unknown path %q", slug)
		}
	}
	return &Service{documents: documents, bySlug: bySlug, search: index}, nil
}

func (s *Service) Close() error {
	if s == nil || s.search == nil {
		return nil
	}
	return s.search.Close()
}

func (s *Service) Search(ctx context.Context, request SearchRequest) (SearchResult, error) {
	if s == nil || s.search == nil {
		return SearchResult{}, fmt.Errorf("documentation service is not configured")
	}
	query := strings.TrimSpace(request.Query)
	if query == "" {
		return SearchResult{}, fmt.Errorf("%w: query is required", ErrInvalid)
	}
	path := normalizePath(request.Path)
	limit, err := boundedLimit(request.Limit, DefaultSearchLimit, MaxSearchLimit)
	if err != nil {
		return SearchResult{}, err
	}
	matches, err := s.search.Search(ctx, query, len(s.documents))
	if err != nil {
		return SearchResult{}, err
	}
	result := SearchResult{Query: query, Path: path, Matches: make([]Reference, 0, limit)}
	for _, match := range matches {
		document := s.bySlug[match.Slug]
		if path != "" && document.Slug != path && !strings.HasPrefix(document.Slug, path+"/") {
			continue
		}
		if len(result.Matches) == limit {
			result.Truncated = true
			break
		}
		result.Matches = append(result.Matches, Reference{
			ID: "doc:" + document.Slug, Path: document.Slug,
			Title: document.Title, Summary: document.Summary,
			URL: "/docs/" + document.Slug, Excerpt: strings.TrimSpace(match.Excerpt),
		})
	}
	return result, nil
}

func (s *Service) Read(ctx context.Context, request ReadRequest) (ReadResult, error) {
	if s == nil {
		return ReadResult{}, fmt.Errorf("documentation service is not configured")
	}
	if err := ctx.Err(); err != nil {
		return ReadResult{}, err
	}
	slug, err := documentSlug(request.ID)
	if err != nil {
		return ReadResult{}, err
	}
	document, ok := s.bySlug[slug]
	if !ok {
		return ReadResult{}, fmt.Errorf("%w: %q", ErrNotFound, request.ID)
	}
	offset := request.Offset
	if offset == 0 {
		offset = 1
	}
	if offset < 1 {
		return ReadResult{}, fmt.Errorf("%w: offset must be at least 1", ErrInvalid)
	}
	limit, err := boundedLimit(request.Limit, DefaultReadLimit, MaxReadLimit)
	if err != nil {
		return ReadResult{}, err
	}
	lines := strings.Split(strings.TrimSuffix(strings.ReplaceAll(document.Markdown, "\r\n", "\n"), "\n"), "\n")
	if len(lines) == 1 && lines[0] == "" {
		lines = nil
	}
	if offset > len(lines) {
		return ReadResult{}, fmt.Errorf("%w: offset %d exceeds the document's %d lines", ErrInvalid, offset, len(lines))
	}
	start := offset - 1
	var content strings.Builder
	end := start
	for index := start; index < len(lines) && index < start+limit; index++ {
		line := lines[index]
		if utf8.RuneCountInString(line) > MaxLineLength {
			line = string([]rune(line)[:MaxLineLength]) + fmt.Sprintf("... (line truncated to %d characters)", MaxLineLength)
		}
		numbered := strconv.Itoa(index+1) + ": " + line
		addedBytes := len(numbered)
		if content.Len() > 0 {
			addedBytes++
		}
		if content.Len()+addedBytes > MaxReadBytes {
			break
		}
		if content.Len() > 0 {
			content.WriteByte('\n')
		}
		content.WriteString(numbered)
		end = index + 1
	}
	if err := ctx.Err(); err != nil {
		return ReadResult{}, err
	}
	if end < offset {
		return ReadResult{}, fmt.Errorf("documentation line exceeds the read output limit")
	}
	result := ReadResult{
		ID: "doc:" + slug, Path: slug, Title: document.Title, URL: "/docs/" + slug,
		Content: content.String(), LineStart: offset, LineEnd: end, TotalLines: len(lines),
		Truncated: end < len(lines),
	}
	if result.Truncated {
		result.NextOffset = end + 1
	}
	return result, nil
}

func loadDocuments(files fs.FS) ([]Document, map[string]Document, error) {
	contents, err := fs.ReadFile(files, "catalog.json")
	if err != nil {
		return nil, nil, fmt.Errorf("read documentation catalog: %w", err)
	}
	type catalogDocument struct {
		Slug    string `json:"slug"`
		Title   string `json:"title"`
		Summary string `json:"summary"`
		Source  string `json:"source"`
	}
	type catalogGroup struct {
		Documents []catalogDocument `json:"documents"`
	}
	var catalog struct {
		Sections []struct {
			Documents []catalogDocument `json:"documents"`
			Groups    []catalogGroup    `json:"groups"`
		} `json:"sections"`
	}
	if err := json.Unmarshal(contents, &catalog); err != nil {
		return nil, nil, fmt.Errorf("decode documentation catalog: %w", err)
	}
	documents := make([]Document, 0)
	bySlug := make(map[string]Document)
	add := func(item catalogDocument) error {
		item.Slug = normalizePath(item.Slug)
		if item.Slug == "" || strings.TrimSpace(item.Title) == "" || strings.TrimSpace(item.Source) == "" {
			return fmt.Errorf("documentation catalog entry requires slug, title, and source")
		}
		if _, exists := bySlug[item.Slug]; exists {
			return fmt.Errorf("duplicate documentation path %q", item.Slug)
		}
		markdown, err := fs.ReadFile(files, item.Source)
		if err != nil {
			return fmt.Errorf("read documentation source %q: %w", item.Source, err)
		}
		document := Document{
			Slug: item.Slug, Title: strings.TrimSpace(item.Title),
			Summary: strings.TrimSpace(item.Summary), Source: item.Source,
			Markdown: string(markdown),
		}
		documents = append(documents, document)
		bySlug[document.Slug] = document
		return nil
	}
	for _, section := range catalog.Sections {
		for _, document := range section.Documents {
			if err := add(document); err != nil {
				return nil, nil, err
			}
		}
		for _, group := range section.Groups {
			for _, document := range group.Documents {
				if err := add(document); err != nil {
					return nil, nil, err
				}
			}
		}
	}
	return documents, bySlug, nil
}

func documentSlug(id string) (string, error) {
	kind, slug, ok := strings.Cut(strings.TrimSpace(id), ":")
	slug = normalizePath(slug)
	if !ok || kind != "doc" || slug == "" {
		return "", fmt.Errorf("%w: id must use the doc:<path> returned by docs_search", ErrInvalid)
	}
	return slug, nil
}

func normalizePath(path string) string {
	path = strings.TrimSpace(path)
	path = strings.TrimPrefix(path, "/docs/")
	return strings.Trim(path, "/")
}

func boundedLimit(value, fallback, maximum int) (int, error) {
	if value == 0 {
		return fallback, nil
	}
	if value < 1 || value > maximum {
		return 0, fmt.Errorf("%w: limit must be between 1 and %d", ErrInvalid, maximum)
	}
	return value, nil
}
