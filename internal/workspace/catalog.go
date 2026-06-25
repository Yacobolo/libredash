package workspace

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Yacobolo/libredash/internal/analytics/model"
	"github.com/Yacobolo/libredash/internal/configschema"
	"github.com/Yacobolo/libredash/internal/dashboard/report"
	"gopkg.in/yaml.v3"
)

type Catalog struct {
	Workspace      CatalogWorkspace   `yaml:"workspace"`
	SemanticModels []CatalogModel     `yaml:"semantic_models"`
	Dashboards     []CatalogDashboard `yaml:"dashboards"`
}

type CatalogWorkspace struct {
	ID          string `yaml:"id"`
	Title       string `yaml:"title"`
	Description string `yaml:"description"`
}

type CatalogModel struct {
	ID          string `yaml:"id"`
	Title       string `yaml:"title"`
	Path        string `yaml:"path"`
	Description string `yaml:"description"`
}

type CatalogDashboard struct {
	ID          string   `yaml:"id"`
	Title       string   `yaml:"title"`
	Path        string   `yaml:"path"`
	Description string   `yaml:"description"`
	Tags        []string `yaml:"tags"`
}

type Definition struct {
	Catalog    Catalog
	Models     map[string]*model.Model
	Dashboards map[string]*report.Dashboard
	BaseDir    string
}

func LoadCatalog(path string) (Catalog, string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return Catalog{}, "", err
	}
	if err := rejectLegacyCatalogContract(content); err != nil {
		return Catalog{}, "", err
	}
	if err := configschema.ValidateBytes(configschema.KindCatalog, path, content); err != nil {
		return Catalog{}, "", err
	}
	var catalog Catalog
	decoder := yaml.NewDecoder(bytes.NewReader(content))
	decoder.KnownFields(true)
	if err := decoder.Decode(&catalog); err != nil {
		return Catalog{}, "", err
	}
	baseDir := filepath.Dir(path)
	if err := catalog.Validate(baseDir); err != nil {
		return Catalog{}, "", err
	}
	return catalog, baseDir, nil
}

func (c Catalog) Validate(baseDir string) error {
	if len(c.SemanticModels) == 0 {
		return fmt.Errorf("catalog requires semantic_models")
	}
	if len(c.Dashboards) == 0 {
		return fmt.Errorf("catalog requires dashboards")
	}
	models := map[string]struct{}{}
	for index, model := range c.SemanticModels {
		if model.ID == "" || model.Title == "" || model.Path == "" {
			return fmt.Errorf("catalog semantic model %d requires id, title, and path", index)
		}
		if _, exists := models[model.ID]; exists {
			return fmt.Errorf("duplicate semantic model id %q", model.ID)
		}
		models[model.ID] = struct{}{}
		if _, err := os.Stat(filepath.Join(baseDir, model.Path)); err != nil {
			return fmt.Errorf("semantic model %q path %q: %w", model.ID, model.Path, err)
		}
	}

	dashboards := map[string]struct{}{}
	for index, report := range c.Dashboards {
		if report.ID == "" || report.Title == "" || report.Path == "" {
			return fmt.Errorf("catalog dashboard %d requires id, title, and path", index)
		}
		if _, exists := dashboards[report.ID]; exists {
			return fmt.Errorf("duplicate dashboard id %q", report.ID)
		}
		dashboards[report.ID] = struct{}{}
		if _, err := os.Stat(filepath.Join(baseDir, report.Path)); err != nil {
			return fmt.Errorf("dashboard %q path %q: %w", report.ID, report.Path, err)
		}
	}
	return nil
}

func rejectLegacyCatalogContract(content []byte) error {
	var node yaml.Node
	if err := yaml.Unmarshal(content, &node); err != nil {
		return err
	}
	root := catalogMappingNode(&node)
	if root == nil {
		return nil
	}
	if catalogMappingValue(root, "metric_views") != nil || catalogMappingValue(root, "metrics_views") != nil {
		return fmt.Errorf("catalog uses legacy metric views; use semantic_models and dashboards")
	}
	return nil
}

func catalogMappingNode(node *yaml.Node) *yaml.Node {
	if node.Kind == yaml.DocumentNode && len(node.Content) > 0 {
		return node.Content[0]
	}
	if node.Kind == yaml.MappingNode {
		return node
	}
	return nil
}

func catalogMappingValue(node *yaml.Node, key string) *yaml.Node {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}
	for index := 0; index+1 < len(node.Content); index += 2 {
		if node.Content[index].Value == key {
			return node.Content[index+1]
		}
	}
	return nil
}
