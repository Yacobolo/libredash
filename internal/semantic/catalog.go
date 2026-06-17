package semantic

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Catalog struct {
	Workspace      CatalogWorkspace    `yaml:"workspace"`
	SemanticModels []CatalogModel      `yaml:"semantic_models"`
	MetricViews    []CatalogMetricView `yaml:"metrics_views"`
	Dashboards     []CatalogDashboard  `yaml:"dashboards"`
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

type CatalogMetricView struct {
	ID            string `yaml:"id"`
	Title         string `yaml:"title"`
	Path          string `yaml:"path"`
	Description   string `yaml:"description"`
	SemanticModel string `yaml:"semantic_model"`
}

type CatalogDashboard struct {
	ID          string   `yaml:"id"`
	Title       string   `yaml:"title"`
	Path        string   `yaml:"path"`
	Description string   `yaml:"description"`
	Tags        []string `yaml:"tags"`
}

type Workspace struct {
	Catalog     Catalog
	Models      map[string]*Model
	MetricViews map[string]*MetricView
	Dashboards  map[string]*Dashboard
	BaseDir     string
}

func LoadWorkspace(path string) (*Workspace, error) {
	bytes, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var catalog Catalog
	if err := yaml.Unmarshal(bytes, &catalog); err != nil {
		return nil, err
	}
	baseDir := filepath.Dir(path)
	if err := catalog.Validate(baseDir); err != nil {
		return nil, err
	}

	workspace := &Workspace{
		Catalog:     catalog,
		Models:      map[string]*Model{},
		MetricViews: map[string]*MetricView{},
		Dashboards:  map[string]*Dashboard{},
		BaseDir:     baseDir,
	}

	for _, entry := range catalog.SemanticModels {
		model, err := Load(filepath.Join(baseDir, entry.Path))
		if err != nil {
			return nil, fmt.Errorf("loading semantic model %q: %w", entry.ID, err)
		}
		if model.Name != entry.ID {
			return nil, fmt.Errorf("catalog model %q path loads model %q", entry.ID, model.Name)
		}
		workspace.Models[entry.ID] = model
	}

	for _, entry := range catalog.MetricViews {
		model := workspace.Models[entry.SemanticModel]
		view, err := LoadMetricView(filepath.Join(baseDir, entry.Path), model)
		if err != nil {
			return nil, fmt.Errorf("loading metrics view %q: %w", entry.ID, err)
		}
		if view.ID != entry.ID {
			return nil, fmt.Errorf("catalog metrics view %q path loads metrics view %q", entry.ID, view.ID)
		}
		if view.SemanticModel != entry.SemanticModel {
			return nil, fmt.Errorf("catalog metrics view %q references model %q but file references %q", entry.ID, entry.SemanticModel, view.SemanticModel)
		}
		workspace.MetricViews[entry.ID] = view
	}

	for _, entry := range catalog.Dashboards {
		report, err := LoadDashboard(filepath.Join(baseDir, entry.Path), workspace.MetricViews)
		if err != nil {
			return nil, fmt.Errorf("loading dashboard %q: %w", entry.ID, err)
		}
		if report.ID != entry.ID {
			return nil, fmt.Errorf("catalog dashboard %q path loads dashboard %q", entry.ID, report.ID)
		}
		workspace.Dashboards[entry.ID] = report
	}

	return workspace, nil
}

func (c Catalog) Validate(baseDir string) error {
	if len(c.SemanticModels) == 0 {
		return fmt.Errorf("catalog requires semantic_models")
	}
	if len(c.MetricViews) == 0 {
		return fmt.Errorf("catalog requires metrics_views")
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

	metricViews := map[string]struct{}{}
	for index, view := range c.MetricViews {
		if view.ID == "" || view.Title == "" || view.Path == "" || view.SemanticModel == "" {
			return fmt.Errorf("catalog metrics view %d requires id, title, path, and semantic_model", index)
		}
		if _, exists := metricViews[view.ID]; exists {
			return fmt.Errorf("duplicate metrics view id %q", view.ID)
		}
		metricViews[view.ID] = struct{}{}
		if _, ok := models[view.SemanticModel]; !ok {
			return fmt.Errorf("metrics view %q references unknown semantic model %q", view.ID, view.SemanticModel)
		}
		if _, err := os.Stat(filepath.Join(baseDir, view.Path)); err != nil {
			return fmt.Errorf("metrics view %q path %q: %w", view.ID, view.Path, err)
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
