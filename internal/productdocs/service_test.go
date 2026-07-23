package productdocs

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"testing/fstest"
)

func TestServiceSearchReturnsRankedBoundedReferences(t *testing.T) {
	service := testService(t)

	result, err := service.Search(context.Background(), SearchRequest{
		Query: "semantic models",
		Limit: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	want := SearchResult{
		Query: "semantic models",
		Matches: []Reference{{
			ID:      "doc:concepts/semantic-models",
			Path:    "concepts/semantic-models",
			Title:   "Semantic models",
			Summary: "Define governed analytical relationships.",
			URL:     "/docs/concepts/semantic-models",
			Excerpt: "# Semantic models\n\nSemantic models define governed dimensions, measures, and relationships.\nContinue reading.",
		}},
		Truncated: true,
	}
	if !equalJSON(result, want) {
		t.Fatalf("Search() = %#v, want %#v", result, want)
	}
}

func TestServiceSearchCanNarrowByDocumentationPath(t *testing.T) {
	service := testService(t)

	result, err := service.Search(context.Background(), SearchRequest{
		Query: "relationships",
		Path:  "guides/",
	})
	if err != nil {
		t.Fatal(err)
	}
	want := SearchResult{
		Query: "relationships",
		Path:  "guides",
		Matches: []Reference{{
			ID:      "doc:guides/build/semantic-model",
			Path:    "guides/build/semantic-model",
			Title:   "Build a semantic model",
			Summary: "Configure reusable relationships.",
			URL:     "/docs/guides/build/semantic-model",
			Excerpt: "# Build a semantic model\n\nConfigure reusable relationships in semantic models.",
		}},
	}
	if !equalJSON(result, want) {
		t.Fatalf("Search() = %#v, want %#v", result, want)
	}
}

func TestServiceReadReturnsNumberedBoundedWindow(t *testing.T) {
	service := testService(t)

	result, err := service.Read(context.Background(), ReadRequest{
		ID:     "doc:concepts/semantic-models",
		Offset: 2,
		Limit:  2,
	})
	if err != nil {
		t.Fatal(err)
	}
	want := ReadResult{
		ID:         "doc:concepts/semantic-models",
		Path:       "concepts/semantic-models",
		Title:      "Semantic models",
		URL:        "/docs/concepts/semantic-models",
		Content:    "2: \n3: Semantic models define governed dimensions, measures, and relationships.",
		LineStart:  2,
		LineEnd:    3,
		TotalLines: 4,
		NextOffset: 4,
		Truncated:  true,
	}
	if !equalJSON(result, want) {
		t.Fatalf("Read() = %#v, want %#v", result, want)
	}
}

func TestServiceReadRejectsUnknownDocument(t *testing.T) {
	service := testService(t)

	_, err := service.Read(context.Background(), ReadRequest{ID: "doc:missing"})
	if err == nil {
		t.Fatal("Read() accepted an unknown document")
	}
}

func TestServiceReadHasAHardModelOutputByteLimit(t *testing.T) {
	line := strings.Repeat("é", 1000)
	service := &Service{bySlug: map[string]Document{
		"large": {Slug: "large", Title: "Large", Markdown: strings.Repeat(line+"\n", 50)},
	}}

	result, err := service.Read(context.Background(), ReadRequest{ID: "doc:large", Limit: MaxReadLimit})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Content) > MaxReadBytes {
		t.Fatalf("Read() content = %d bytes, want at most %d", len(result.Content), MaxReadBytes)
	}
	if !result.Truncated || result.NextOffset <= 1 {
		t.Fatalf("Read() pagination = %#v, want a continuation", result)
	}
}

func testService(t *testing.T) *Service {
	t.Helper()
	documents := []Document{
		{
			Slug: "concepts/semantic-models", Title: "Semantic models",
			Summary:  "Define governed analytical relationships.",
			Source:   "articles/concepts/semantic-models.md",
			Markdown: "# Semantic models\n\nSemantic models define governed dimensions, measures, and relationships.\nContinue reading.\n",
		},
		{
			Slug: "guides/build/semantic-model", Title: "Build a semantic model",
			Summary:  "Configure reusable relationships.",
			Source:   "articles/build/semantic-model.md",
			Markdown: "# Build a semantic model\n\nConfigure reusable relationships in semantic models.\n",
		},
	}
	catalog := map[string]any{"sections": []any{
		map[string]any{"documents": []any{
			map[string]any{
				"slug": documents[0].Slug, "title": documents[0].Title,
				"summary": documents[0].Summary, "source": documents[0].Source,
			},
		}},
		map[string]any{"groups": []any{
			map[string]any{"documents": []any{
				map[string]any{
					"slug": documents[1].Slug, "title": documents[1].Title,
					"summary": documents[1].Summary, "source": documents[1].Source,
				},
			}},
		}},
	}}
	catalogJSON, err := json.Marshal(catalog)
	if err != nil {
		t.Fatal(err)
	}
	files := fstest.MapFS{"catalog.json": &fstest.MapFile{Data: catalogJSON}}
	for _, document := range documents {
		files[document.Source] = &fstest.MapFile{Data: []byte(document.Markdown)}
	}
	index := &fakeSearchIndex{
		slugs: []string{documents[0].Slug, documents[1].Slug},
		matches: []SearchMatch{
			{Slug: documents[0].Slug, Excerpt: documents[0].Markdown},
			{Slug: documents[1].Slug, Excerpt: documents[1].Markdown},
		},
	}
	service, err := New(files, index)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := service.Close(); err != nil {
			t.Errorf("close documentation service: %v", err)
		}
	})
	return service
}

type fakeSearchIndex struct {
	slugs   []string
	matches []SearchMatch
}

func (f *fakeSearchIndex) Slugs(context.Context) ([]string, error) {
	return append([]string(nil), f.slugs...), nil
}

func (f *fakeSearchIndex) Search(context.Context, string, int) ([]SearchMatch, error) {
	return append([]SearchMatch(nil), f.matches...), nil
}

func (f *fakeSearchIndex) Close() error {
	return nil
}

func equalJSON(left, right any) bool {
	leftJSON, _ := json.Marshal(left)
	rightJSON, _ := json.Marshal(right)
	return string(leftJSON) == string(rightJSON)
}
