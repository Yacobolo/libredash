package semantic

import (
	"fmt"
	"os"
	"sort"

	"gopkg.in/yaml.v3"
)

type Model struct {
	Name          string             `yaml:"name"`
	Title         string             `yaml:"title"`
	Description   string             `yaml:"description"`
	Sources       map[string]Source  `yaml:"sources"`
	Cache         Cache              `yaml:"cache"`
	Datasets      map[string]Dataset `yaml:"datasets"`
	Relationships []Relationship     `yaml:"relationships"`
}

type Source struct {
	File string `yaml:"file"`
}

type Cache struct {
	Tables map[string]CacheTable `yaml:"tables"`
}

type CacheTable struct {
	Description string `yaml:"description"`
	SQL         string `yaml:"sql"`
}

type Dataset struct {
	Source     string               `yaml:"source"`
	Dimensions map[string]Dimension `yaml:"dimensions"`
	Measures   map[string]Measure   `yaml:"measures"`
}

type Dimension struct {
	Label     string `yaml:"label"`
	Expr      string `yaml:"expr"`
	Where     string `yaml:"where"`
	OrderExpr string `yaml:"order_expr"`
}

type Measure struct {
	Label      string `yaml:"label"`
	Aggregate  string `yaml:"aggregate"`
	Column     string `yaml:"column"`
	Expression string `yaml:"expression"`
	Unit       string `yaml:"unit"`
	Format     string `yaml:"format"`
}

type Relationship struct {
	ID          string `yaml:"id"`
	From        string `yaml:"from"`
	To          string `yaml:"to"`
	Cardinality string `yaml:"cardinality"`
	Active      bool   `yaml:"active"`
}

func Load(path string) (*Model, error) {
	bytes, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var model Model
	if err := yaml.Unmarshal(bytes, &model); err != nil {
		return nil, err
	}
	if err := model.Validate(); err != nil {
		return nil, err
	}
	return &model, nil
}

func (m *Model) Validate() error {
	if m.Name == "" {
		return fmt.Errorf("semantic model name is required")
	}
	if len(m.Sources) == 0 {
		return fmt.Errorf("semantic model %q has no sources", m.Name)
	}
	if len(m.Cache.Tables) == 0 {
		return fmt.Errorf("semantic model %q has no cache tables", m.Name)
	}
	for name, source := range m.Sources {
		if source.File == "" {
			return fmt.Errorf("source %q is missing file", name)
		}
	}
	for name, table := range m.Cache.Tables {
		if table.SQL == "" {
			return fmt.Errorf("cache table %q is missing sql", name)
		}
	}
	if len(m.Datasets) == 0 {
		return fmt.Errorf("semantic model %q has no datasets", m.Name)
	}
	for name, dataset := range m.Datasets {
		if dataset.Source == "" {
			return fmt.Errorf("dataset %q requires source", name)
		}
		if _, ok := m.Cache.Tables[dataset.Source]; !ok {
			return fmt.Errorf("dataset %q references unknown cache table %q", name, dataset.Source)
		}
		if len(dataset.Dimensions) == 0 {
			return fmt.Errorf("dataset %q requires dimensions", name)
		}
		if len(dataset.Measures) == 0 {
			return fmt.Errorf("dataset %q requires measures", name)
		}
		for dimensionName, dimension := range dataset.Dimensions {
			if dimension.Expr == "" {
				return fmt.Errorf("dataset %q dimension %q requires expr", name, dimensionName)
			}
		}
		for measureName, measure := range dataset.Measures {
			if measure.Aggregate == "" {
				return fmt.Errorf("dataset %q measure %q requires aggregate", name, measureName)
			}
			if measure.Aggregate != "count" && measure.Aggregate != "expression" && measure.Column == "" {
				return fmt.Errorf("dataset %q measure %q requires column", name, measureName)
			}
			if measure.Aggregate == "expression" && measure.Expression == "" {
				return fmt.Errorf("dataset %q measure %q requires expression", name, measureName)
			}
		}
	}
	seenRelationships := map[string]struct{}{}
	for index, relationship := range m.Relationships {
		if relationship.ID == "" || relationship.From == "" || relationship.To == "" {
			return fmt.Errorf("relationship %d requires id, from, and to", index)
		}
		if _, exists := seenRelationships[relationship.ID]; exists {
			return fmt.Errorf("duplicate relationship id %q", relationship.ID)
		}
		seenRelationships[relationship.ID] = struct{}{}
	}
	return nil
}

func (m *Model) SourceFiles() map[string]string {
	files := make(map[string]string, len(m.Sources))
	for name, source := range m.Sources {
		files[name] = source.File
	}
	return files
}

func (m *Model) CacheTableNames() []string {
	names := make([]string, 0, len(m.Cache.Tables))
	for name := range m.Cache.Tables {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func supportsVisualKind(kind string) bool {
	return kind == "chart"
}

func supportsVisualShape(shape string) bool {
	switch shape {
	case "category_value", "category_series_value", "single_value", "matrix", "graph", "geo", "ohlc", "distribution":
		return true
	default:
		return false
	}
}

func supportsRenderer(renderer string) bool {
	return renderer == "echarts"
}

func rendererSupportsType(renderer, chartType string) bool {
	if renderer != "echarts" {
		return false
	}
	switch chartType {
	case "line", "area", "bar", "column", "pie", "donut", "scatter", "funnel", "treemap", "gauge", "heatmap", "sankey", "graph", "map", "candlestick", "boxplot":
		return true
	default:
		return false
	}
}

func supportsSeries(shape string) bool {
	return shape == "category_series_value"
}

func rendererSupportsShapeType(renderer, shape, chartType string) bool {
	if renderer != "echarts" {
		return false
	}
	switch shape {
	case "category_value":
		switch chartType {
		case "line", "area", "bar", "column", "pie", "donut", "scatter", "funnel", "treemap":
			return true
		}
	case "category_series_value":
		return rendererTypeSupportsSeries(renderer, chartType)
	case "single_value":
		return chartType == "gauge"
	case "matrix":
		return chartType == "heatmap"
	case "graph":
		return chartType == "sankey" || chartType == "graph"
	case "geo":
		return chartType == "map"
	case "ohlc":
		return chartType == "candlestick"
	case "distribution":
		return chartType == "boxplot"
	}
	return false
}

func rendererTypeSupportsSeries(renderer, chartType string) bool {
	if renderer != "echarts" {
		return false
	}
	switch chartType {
	case "line", "area", "bar", "column", "scatter":
		return true
	default:
		return false
	}
}
