package sqlite

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestSearchRanksExactTitlesThenAuthoredGuidance(t *testing.T) {
	documents := []Document{
		{Slug: "generated-exact", Title: "Access policy", Generated: true},
		{Slug: "authored-title", Title: "Access policy guide"},
		{Slug: "authored-body", Title: "Authorization guide", Body: "Choose a workspace access policy."},
		{Slug: "generated-title", Title: "Workspace access policy configuration", Generated: true},
		{Slug: "generated-body", Title: "Grant configuration", Body: "Exact workspace access policy fields.", Generated: true},
	}
	index := buildTestIndex(t, documents)

	results, err := index.Search(context.Background(), "access policy", 10)
	if err != nil {
		t.Fatalf("search documentation: %v", err)
	}
	want := []string{"generated-exact", "authored-title", "authored-body", "generated-title", "generated-body"}
	if len(results) != len(want) {
		t.Fatalf("search results = %d, want %d", len(results), len(want))
	}
	for position, slug := range want {
		if results[position].Slug != slug {
			t.Errorf("search result %d = %q, want %q", position, results[position].Slug, slug)
		}
	}
}

func TestSearchCompilesUserInputToSafePrefixTerms(t *testing.T) {
	index := buildTestIndex(t, []Document{{Slug: "semantic-models", Title: "Semantic models", Body: "Define relationships between datasets."}})

	results, err := index.Search(context.Background(), `semantic relat " *`, 10)
	if err != nil {
		t.Fatalf("search syntax-like input: %v", err)
	}
	if len(results) != 1 || results[0].Slug != "semantic-models" {
		t.Fatalf("prefix results = %#v, want semantic models", results)
	}
}

func TestSearchPreservesQuotedPhrases(t *testing.T) {
	index := buildTestIndex(t, []Document{
		{Slug: "exact", Title: "Semantic relationships"},
		{Slug: "separate", Title: "Semantic analytical relationships"},
	})

	results, err := index.Search(context.Background(), `"semantic relationships"`, 10)
	if err != nil {
		t.Fatalf("search quoted phrase: %v", err)
	}
	if len(results) != 1 || results[0].Slug != "exact" {
		t.Fatalf("phrase results = %#v, want exact phrase only", results)
	}
}

func buildTestIndex(t *testing.T, documents []Document) *Index {
	t.Helper()
	directory := t.TempDir()
	if err := Build(filepath.Join(directory, Filename), documents); err != nil {
		t.Fatalf("build documentation search index: %v", err)
	}
	index, err := Open(os.DirFS(directory), Filename)
	if err != nil {
		t.Fatalf("open documentation search index: %v", err)
	}
	t.Cleanup(func() {
		if err := index.Close(); err != nil {
			t.Errorf("close documentation search index: %v", err)
		}
	})
	return index
}
