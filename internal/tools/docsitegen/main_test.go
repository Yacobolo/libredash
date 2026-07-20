package main

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func TestGenerateBuildsUnifiedCatalogFromArticlesAndGeneratedCollections(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "navigation.yaml", `sections:
  - id: start
    title: Start here
    summary: Learn LeapView.
    documents:
      - slug: getting-started
        title: Getting started
        navigationTitle: Overview
        summary: Run the sample project.
        source: articles/getting-started.md
  - id: reference
    title: Reference
    summary: Exact generated contracts.
    groups:
      - id: cli
        title: CLI
        collection:
          kind: catalog
          catalog: reference/cli/catalog.json
          sourceDir: reference/cli
          slugPrefix: cli
          index:
            slug: cli
            title: CLI reference
            summary: Command index.
            source: reference/cli/index.md
`)
	writeFixture(t, root, "articles/getting-started.md", "# Getting started\n")
	writeFixture(t, root, "reference/cli/index.md", "# CLI reference\n")
	writeFixture(t, root, "reference/cli/deploy.md", "# Deploy\n")
	writeFixture(t, root, "reference/cli/catalog.json", `{"documents":[{"slug":"deploy","title":"leapview deploy","summary":"Deploy a project."}]}`)

	searchPath := filepath.Join(root, "search-index.sqlite3")
	if err := generate(filepath.Join(root, "navigation.yaml"), filepath.Join(root, "catalog.json"), searchPath); err != nil {
		t.Fatalf("generate documentation catalog: %v", err)
	}

	var catalog generatedCatalog
	decodeFixture(t, filepath.Join(root, "catalog.json"), &catalog)
	if got, want := len(catalog.Sections), 2; got != want {
		t.Fatalf("sections = %d, want %d", got, want)
	}
	if got, want := catalog.Sections[1].Groups[0].Documents[1].Slug, "cli/deploy"; got != want {
		t.Fatalf("generated CLI slug = %q, want %q", got, want)
	}
	if got, want := catalog.Sections[1].Groups[0].Documents[1].Source, "reference/cli/deploy.md"; got != want {
		t.Fatalf("generated CLI source = %q, want %q", got, want)
	}
	if got, want := catalog.Sections[0].Documents[0].NavigationTitle, "Overview"; got != want {
		t.Fatalf("navigation title = %q, want %q", got, want)
	}
	if catalog.Sections[1].Groups[0].Documents[0].Generated {
		t.Fatal("authored collection index marked as generated")
	}
	if !catalog.Sections[1].Groups[0].Documents[1].Generated {
		t.Fatal("generated collection document is not marked as generated")
	}
	llms := readFixture(t, filepath.Join(root, "llms.txt"))
	for _, want := range []string{"# LeapView", "[Documentation MCP](/mcp)", "[leapview deploy](/docs/cli/deploy)"} {
		if !strings.Contains(llms, want) {
			t.Errorf("llms.txt missing %q:\n%s", want, llms)
		}
	}

	database, err := sql.Open("sqlite", "file:"+searchPath+"?mode=ro")
	if err != nil {
		t.Fatalf("open generated search index: %v", err)
	}
	defer database.Close()

	var count int
	if err := database.QueryRow(`SELECT count(*) FROM search_documents`).Scan(&count); err != nil {
		t.Fatalf("count generated search documents: %v", err)
	}
	if got, want := count, 3; got != want {
		t.Fatalf("search documents = %d, want %d", got, want)
	}
	var slug string
	if err := database.QueryRow(`SELECT slug FROM search_documents WHERE search_documents MATCH ? ORDER BY bm25(search_documents, 0, 12, 5, 1, 1, 1, 0)`, `"deploy"*`).Scan(&slug); err != nil {
		t.Fatalf("query generated search index: %v", err)
	}
	if slug != "cli/deploy" {
		t.Fatalf("search result = %q, want %q", slug, "cli/deploy")
	}
}

func TestGenerateRejectsUnknownNavigationFields(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "navigation.yaml", `sections:
  - id: concepts
    title: Core concepts
    documents:
      - {slug: projects, title: Projects, workspaces, and environments, source: projects.md}
`)
	writeFixture(t, root, "projects.md", "# Projects, workspaces, and environments\n")

	err := generate(filepath.Join(root, "navigation.yaml"), filepath.Join(root, "catalog.json"), filepath.Join(root, "search-index.sqlite3"))
	if err == nil || !strings.Contains(err.Error(), "field workspaces not found") {
		t.Fatalf("generate error = %v, want strict unknown-field error", err)
	}
}

func TestGenerateRejectsDuplicateSlugs(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "navigation.yaml", `sections:
  - id: start
    title: Start
    documents:
      - {slug: duplicate, title: One, source: one.md}
      - {slug: duplicate, title: Two, source: two.md}
`)
	writeFixture(t, root, "one.md", "# One\n")
	writeFixture(t, root, "two.md", "# Two\n")

	err := generate(filepath.Join(root, "navigation.yaml"), filepath.Join(root, "catalog.json"), filepath.Join(root, "search-index.sqlite3"))
	if err == nil || !strings.Contains(err.Error(), `duplicate documentation slug "duplicate"`) {
		t.Fatalf("generate error = %v, want duplicate slug error", err)
	}
}

func TestGenerateRejectsMissingDocumentSource(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "navigation.yaml", `sections:
  - id: start
    title: Start
    documents:
      - {slug: missing, title: Missing, source: missing.md}
`)

	err := generate(filepath.Join(root, "navigation.yaml"), filepath.Join(root, "catalog.json"), filepath.Join(root, "search-index.sqlite3"))
	if err == nil || !strings.Contains(err.Error(), "missing.md") {
		t.Fatalf("generate error = %v, want missing source error", err)
	}
}

func TestGenerateRejectsOrphanedMarkdown(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "navigation.yaml", `sections:
  - id: start
    title: Start
    documents:
      - {slug: one, title: One, source: one.md}
`)
	writeFixture(t, root, "one.md", "# One\n")
	writeFixture(t, root, "orphan.md", "# Orphan\n")

	err := generate(filepath.Join(root, "navigation.yaml"), filepath.Join(root, "catalog.json"), filepath.Join(root, "search-index.sqlite3"))
	if err == nil || !strings.Contains(err.Error(), "orphaned documentation source orphan.md") {
		t.Fatalf("generate error = %v, want orphaned source error", err)
	}
}

func TestGenerateRejectsBrokenInternalLink(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "navigation.yaml", `sections:
  - id: start
    title: Start
    documents:
      - {slug: one, title: One, source: one.md}
`)
	writeFixture(t, root, "one.md", "# One\n\n[Missing](/docs/missing)\n")

	err := generate(filepath.Join(root, "navigation.yaml"), filepath.Join(root, "catalog.json"), filepath.Join(root, "search-index.sqlite3"))
	if err == nil || !strings.Contains(err.Error(), "broken documentation link /docs/missing") {
		t.Fatalf("generate error = %v, want broken link error", err)
	}
}

func TestGenerateRejectsInvalidYAMLExample(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "navigation.yaml", `sections:
  - id: start
    title: Start
    documents:
      - {slug: one, title: One, source: one.md}
`)
	writeFixture(t, root, "one.md", "# One\n\n```yaml\nitems: [unterminated\n```\n")

	err := generate(filepath.Join(root, "navigation.yaml"), filepath.Join(root, "catalog.json"), filepath.Join(root, "search-index.sqlite3"))
	if err == nil || !strings.Contains(err.Error(), "invalid YAML example") {
		t.Fatalf("generate error = %v, want invalid YAML example error", err)
	}
}

func TestCheckGeneratedRejectsOutdatedArtifacts(t *testing.T) {
	root := t.TempDir()
	navigation := filepath.Join(root, "navigation.yaml")
	catalog := filepath.Join(root, "catalog.json")
	search := filepath.Join(root, "search-index.sqlite3")
	writeFixture(t, root, "navigation.yaml", `sections:
  - id: start
    title: Start
    documents:
      - {slug: one, title: One, source: one.md}
`)
	writeFixture(t, root, "one.md", "# One\n")
	writeFixture(t, root, "catalog.json", "{}\n")
	writeFixture(t, root, "search-index.sqlite3", "[]\n")

	err := checkGenerated(navigation, catalog, search)
	if err == nil || !strings.Contains(err.Error(), "catalog.json is out of date") {
		t.Fatalf("check error = %v, want outdated catalog error", err)
	}

	if err := generate(navigation, catalog, search); err != nil {
		t.Fatalf("generate current documentation artifacts: %v", err)
	}
	if err := checkGenerated(navigation, catalog, search); err != nil {
		t.Fatalf("check current documentation artifacts: %v", err)
	}
}

func writeFixture(t *testing.T, root, name, contents string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create fixture directory: %v", err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write fixture %s: %v", name, err)
	}
}

func decodeFixture(t *testing.T, path string, value any) {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if err := json.Unmarshal(contents, value); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
}

func readFixture(t *testing.T, path string) string {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(contents)
}
