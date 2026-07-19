// Command docsitegen composes authored navigation with generated reference
// catalogs into the runtime catalog and FTS5 search index used by the public site.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	docsearch "github.com/Yacobolo/libredash/internal/site/search/sqlite"
	"gopkg.in/yaml.v3"
)

func main() {
	navigation := flag.String("navigation", "docs/navigation.yaml", "authored documentation navigation manifest")
	catalog := flag.String("catalog", "docs/catalog.json", "generated runtime catalog")
	search := flag.String("search", "docs/"+docsearch.Filename, "generated FTS5 search index")
	check := flag.Bool("check", false, "verify generated artifacts are current without changing them")
	flag.Parse()
	var err error
	if *check {
		err = checkGenerated(*navigation, *catalog, *search)
	} else {
		err = generate(*navigation, *catalog, *search)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "generate documentation site catalog: %v\n", err)
		os.Exit(1)
	}
}

type navigationManifest struct {
	Sections []sectionSpec `yaml:"sections"`
}

type sectionSpec struct {
	ID        string         `yaml:"id"`
	Title     string         `yaml:"title"`
	Summary   string         `yaml:"summary"`
	Documents []documentSpec `yaml:"documents"`
	Groups    []groupSpec    `yaml:"groups"`
}

type groupSpec struct {
	ID         string          `yaml:"id"`
	Title      string          `yaml:"title"`
	Summary    string          `yaml:"summary"`
	Documents  []documentSpec  `yaml:"documents"`
	Collection *collectionSpec `yaml:"collection"`
}

type collectionSpec struct {
	Kind       string        `yaml:"kind"`
	Catalog    string        `yaml:"catalog"`
	SourceDir  string        `yaml:"sourceDir"`
	SlugPrefix string        `yaml:"slugPrefix"`
	Index      *documentSpec `yaml:"index"`
}

type documentSpec struct {
	Slug            string `yaml:"slug" json:"slug"`
	Title           string `yaml:"title" json:"title"`
	NavigationTitle string `yaml:"navigationTitle" json:"navigationTitle,omitempty"`
	Summary         string `yaml:"summary" json:"summary"`
	Source          string `yaml:"source" json:"source"`
	Breadcrumb      string `yaml:"breadcrumb" json:"breadcrumb,omitempty"`
	Generated       bool   `yaml:"generated,omitempty" json:"generated,omitempty"`
}

type generatedCatalog struct {
	Sections []generatedSection `json:"sections"`
}

type generatedSection struct {
	ID        string           `json:"id"`
	Title     string           `json:"title"`
	Summary   string           `json:"summary"`
	Href      string           `json:"href"`
	Documents []documentSpec   `json:"documents,omitempty"`
	Groups    []generatedGroup `json:"groups,omitempty"`
}

type generatedGroup struct {
	ID        string         `json:"id"`
	Title     string         `json:"title"`
	Summary   string         `json:"summary,omitempty"`
	Href      string         `json:"href"`
	Documents []documentSpec `json:"documents"`
}

type referenceCatalog struct {
	Documents []struct {
		Slug    string `json:"slug"`
		Title   string `json:"title"`
		Summary string `json:"summary"`
	} `json:"documents"`
}

type visualCatalog struct {
	Overview struct {
		Source     string `json:"source"`
		Title      string `json:"title"`
		Breadcrumb string `json:"breadcrumb"`
	} `json:"overview"`
	Documents []struct {
		Source     string `json:"source"`
		Title      string `json:"title"`
		Breadcrumb string `json:"breadcrumb"`
	} `json:"documents"`
}

func generate(navigationPath, catalogPath, searchPath string) error {
	contents, err := os.ReadFile(navigationPath)
	if err != nil {
		return fmt.Errorf("read navigation: %w", err)
	}
	var manifest navigationManifest
	decoder := yaml.NewDecoder(bytes.NewReader(contents))
	decoder.KnownFields(true)
	if err := decoder.Decode(&manifest); err != nil {
		return fmt.Errorf("decode navigation: %w", err)
	}
	root := filepath.Dir(navigationPath)
	catalog := generatedCatalog{Sections: make([]generatedSection, 0, len(manifest.Sections))}
	search := make([]docsearch.Document, 0)
	seenSlugs := map[string]struct{}{}
	seenSources := map[string]struct{}{}

	for _, section := range manifest.Sections {
		if section.ID == "" || section.Title == "" {
			return fmt.Errorf("documentation section requires id and title")
		}
		generated := generatedSection{ID: section.ID, Title: section.Title, Summary: section.Summary}
		for _, document := range section.Documents {
			if err := addDocument(root, section.Title, "", document, seenSlugs, seenSources, &generated.Documents, &search); err != nil {
				return err
			}
		}
		for _, group := range section.Groups {
			if group.ID == "" || group.Title == "" {
				return fmt.Errorf("documentation group in %s requires id and title", section.ID)
			}
			documents := append([]documentSpec(nil), group.Documents...)
			if group.Collection != nil {
				collectionDocuments, err := loadCollection(root, *group.Collection)
				if err != nil {
					return fmt.Errorf("load %s collection: %w", group.ID, err)
				}
				documents = append(documents, collectionDocuments...)
			}
			generatedGroup := generatedGroup{ID: group.ID, Title: group.Title, Summary: group.Summary}
			for _, document := range documents {
				if err := addDocument(root, section.Title, group.Title, document, seenSlugs, seenSources, &generatedGroup.Documents, &search); err != nil {
					return err
				}
			}
			generatedGroup.Href = firstDocumentHref(generatedGroup.Documents)
			generated.Groups = append(generated.Groups, generatedGroup)
		}
		generated.Href = firstDocumentHref(generated.Documents)
		if generated.Href == "" {
			for _, group := range generated.Groups {
				if group.Href != "" {
					generated.Href = group.Href
					break
				}
			}
		}
		if generated.Href == "" {
			return fmt.Errorf("documentation section %s has no documents", section.ID)
		}
		catalog.Sections = append(catalog.Sections, generated)
	}
	if err := validateNoOrphanMarkdown(root, seenSources); err != nil {
		return err
	}
	if err := validateInternalLinks(root, seenSources, seenSlugs); err != nil {
		return err
	}
	if err := validateYAMLExamples(root, seenSources); err != nil {
		return err
	}
	if err := writeJSON(catalogPath, catalog); err != nil {
		return err
	}
	return docsearch.Build(searchPath, search)
}

func checkGenerated(navigationPath, catalogPath, searchPath string) error {
	temporary, err := os.MkdirTemp("", "libredash-docsite-check-*")
	if err != nil {
		return fmt.Errorf("create temporary directory: %w", err)
	}
	defer os.RemoveAll(temporary)

	generatedCatalog := filepath.Join(temporary, "catalog.json")
	generatedSearch := filepath.Join(temporary, docsearch.Filename)
	if err := generate(navigationPath, generatedCatalog, generatedSearch); err != nil {
		return err
	}
	for _, artifact := range []struct {
		current   string
		generated string
	}{
		{current: catalogPath, generated: generatedCatalog},
		{current: searchPath, generated: generatedSearch},
	} {
		current, err := os.ReadFile(artifact.current)
		if err != nil {
			return fmt.Errorf("read generated artifact %s: %w", artifact.current, err)
		}
		expected, err := os.ReadFile(artifact.generated)
		if err != nil {
			return fmt.Errorf("read expected artifact %s: %w", artifact.generated, err)
		}
		if !bytes.Equal(current, expected) {
			return fmt.Errorf("%s is out of date; run task docs:generate", artifact.current)
		}
	}
	return nil
}

func addDocument(root, section, group string, document documentSpec, seenSlugs, seenSources map[string]struct{}, output *[]documentSpec, search *[]docsearch.Document) error {
	document.Slug = strings.Trim(document.Slug, "/")
	if document.Slug == "" || document.Title == "" || document.Source == "" {
		return fmt.Errorf("documentation entry requires slug, title, and source")
	}
	if _, ok := seenSlugs[document.Slug]; ok {
		return fmt.Errorf("duplicate documentation slug %q", document.Slug)
	}
	seenSlugs[document.Slug] = struct{}{}
	path := filepath.Join(root, filepath.FromSlash(document.Source))
	contents, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read documentation source %s: %w", document.Source, err)
	}
	if document.Breadcrumb == "" {
		document.Breadcrumb = document.Title
	}
	seenSources[filepath.ToSlash(document.Source)] = struct{}{}
	*output = append(*output, document)
	*search = append(*search, docsearch.Document{Slug: document.Slug, Title: document.Title, Summary: document.Summary, Section: section, Category: group, Body: string(contents), Generated: document.Generated})
	return nil
}

func validateNoOrphanMarkdown(root string, seenSources map[string]struct{}) error {
	return filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".md" {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		if _, ok := seenSources[relative]; !ok {
			return fmt.Errorf("orphaned documentation source %s", relative)
		}
		return nil
	})
}

var internalDocumentationLink = regexp.MustCompile(`\]\((/docs(?:/[^\s)#?]+)?)`)
var yamlCodeFence = regexp.MustCompile("(?ms)```ya?ml(?:[ \\t]+[^\\n]*)?\\n(.*?)\\n```")

func validateInternalLinks(root string, seenSources, seenSlugs map[string]struct{}) error {
	for source := range seenSources {
		contents, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(source)))
		if err != nil {
			return err
		}
		for _, match := range internalDocumentationLink.FindAllSubmatch(contents, -1) {
			target := string(match[1])
			if target == "/docs" || target == "/docs/search" || target == "/docs/openapi.yaml" {
				continue
			}
			if strings.HasPrefix(target, "/docs/schemas/") {
				name := strings.TrimPrefix(target, "/docs/schemas/")
				if _, err := os.Stat(filepath.Join(root, "reference", "config", "schemas", filepath.Base(name))); err == nil && name == filepath.Base(name) {
					continue
				}
			}
			slug := strings.TrimPrefix(target, "/docs/")
			if _, ok := seenSlugs[slug]; ok {
				continue
			}
			return fmt.Errorf("broken documentation link %s in %s", target, source)
		}
	}
	return nil
}

func validateYAMLExamples(root string, seenSources map[string]struct{}) error {
	for source := range seenSources {
		contents, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(source)))
		if err != nil {
			return err
		}
		for index, match := range yamlCodeFence.FindAllSubmatch(contents, -1) {
			var value any
			if err := yaml.Unmarshal(match[1], &value); err != nil {
				return fmt.Errorf("invalid YAML example %d in %s: %w", index+1, source, err)
			}
		}
	}
	return nil
}

func loadCollection(root string, collection collectionSpec) ([]documentSpec, error) {
	documents := make([]documentSpec, 0)
	if collection.Index != nil {
		documents = append(documents, *collection.Index)
	}
	contents, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(collection.Catalog)))
	if err != nil {
		return nil, err
	}
	switch collection.Kind {
	case "catalog":
		var catalog referenceCatalog
		if err := json.Unmarshal(contents, &catalog); err != nil {
			return nil, err
		}
		for _, document := range catalog.Documents {
			documents = append(documents, documentSpec{
				Slug:      joinSlug(collection.SlugPrefix, document.Slug),
				Title:     document.Title,
				Summary:   document.Summary,
				Source:    filepath.ToSlash(filepath.Join(collection.SourceDir, document.Slug+".md")),
				Generated: true,
			})
		}
	case "visual":
		var catalog visualCatalog
		if err := json.Unmarshal(contents, &catalog); err != nil {
			return nil, err
		}
		if collection.Index == nil {
			documents = append(documents, visualDocument(collection, catalog.Overview.Source, catalog.Overview.Title, catalog.Overview.Breadcrumb))
		}
		for _, document := range catalog.Documents {
			documents = append(documents, visualDocument(collection, document.Source, document.Title, document.Breadcrumb))
		}
	default:
		return nil, fmt.Errorf("unsupported collection kind %q", collection.Kind)
	}
	return documents, nil
}

func visualDocument(collection collectionSpec, source, title, breadcrumb string) documentSpec {
	return documentSpec{
		Slug:       joinSlug(collection.SlugPrefix, source),
		Title:      title,
		Summary:    "Configuration and query shape for the " + title + " visual.",
		Source:     filepath.ToSlash(filepath.Join(collection.SourceDir, source+".md")),
		Breadcrumb: breadcrumb,
		Generated:  true,
	}
}

func joinSlug(prefix, slug string) string {
	return strings.Trim(strings.Trim(prefix, "/")+"/"+strings.Trim(slug, "/"), "/")
}

func firstDocumentHref(documents []documentSpec) string {
	if len(documents) == 0 {
		return ""
	}
	return "/docs/" + documents[0].Slug
}

func writeJSON(path string, value any) error {
	contents, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("encode %s: %w", path, err)
	}
	contents = append(contents, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create %s: %w", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, contents, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
