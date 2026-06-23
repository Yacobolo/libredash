package semantic

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

func rejectLegacyDashboardCollectionKeys(bytes []byte) error {
	var node yaml.Node
	if err := yaml.Unmarshal(bytes, &node); err != nil {
		return err
	}
	root := mappingNode(&node)
	if root == nil {
		return nil
	}
	if mappingValue(root, "metrics_views") != nil {
		return fmt.Errorf("dashboard uses legacy metrics_views; use semantic_model")
	}
	return nil
}

func rejectLegacyDashboardQueryContract(bytes []byte) error {
	var node yaml.Node
	if err := yaml.Unmarshal(bytes, &node); err != nil {
		return err
	}
	root := mappingNode(&node)
	if root == nil {
		return nil
	}
	semanticFirst := mappingValue(root, "semantic_model") != nil
	for _, section := range []string{"filters", "visuals", "tables"} {
		items := mappingValue(root, section)
		if items == nil || items.Kind != yaml.MappingNode {
			continue
		}
		for index := 0; index+1 < len(items.Content); index += 2 {
			name := items.Content[index].Value
			item := items.Content[index+1]
			if item.Kind != yaml.MappingNode {
				continue
			}
			if mappingValue(item, "metrics_view") != nil {
				return fmt.Errorf("%s %q uses legacy metrics_view; use dashboard semantic_model", strings.TrimSuffix(section, "s"), name)
			}
			if section == "visuals" && mappingValue(item, "metric_view") != nil {
				return fmt.Errorf("visual %q uses top-level metric_view; use dashboard semantic_model", name)
			}
			if section == "tables" {
				if mappingValue(item, "rows") != nil || mappingValue(item, "measures") != nil {
					return fmt.Errorf("table %q uses legacy rows/measures; use query.rows/query.measures", name)
				}
			}
			queryNode := mappingValue(item, "query")
			if queryNode == nil {
				continue
			}
			if rawSQL := mappingValue(queryNode, "sql"); rawSQL != nil {
				return fmt.Errorf("%s %q uses raw SQL; dashboards must query semantic models", strings.TrimSuffix(section, "s"), name)
			}
			if !semanticFirst {
				for _, key := range []string{"dimensions", "measures", "rows", "columns"} {
					if err := rejectScalarFieldRefs(section, name, queryNode, key); err != nil {
						return err
					}
				}
			}
			if series := mappingValue(queryNode, "series"); series != nil && series.Kind == yaml.ScalarNode {
				return fmt.Errorf("%s %q query.series must be a field object", strings.TrimSuffix(section, "s"), name)
			}
		}
	}
	return nil
}

func rejectScalarFieldRefs(section, name string, queryNode *yaml.Node, key string) error {
	node := mappingValue(queryNode, key)
	if node == nil {
		return nil
	}
	if node.Kind != yaml.SequenceNode {
		return fmt.Errorf("%s %q query.%s must be a sequence", strings.TrimSuffix(section, "s"), name, key)
	}
	for _, item := range node.Content {
		if item.Kind != yaml.MappingNode {
			return fmt.Errorf("%s %q query.%s must contain field objects", strings.TrimSuffix(section, "s"), name, key)
		}
	}
	return nil
}

func rejectLegacyVisualStacked(bytes []byte) error {
	var node yaml.Node
	if err := yaml.Unmarshal(bytes, &node); err != nil {
		return err
	}
	root := mappingNode(&node)
	if root == nil {
		return nil
	}
	visuals := mappingValue(root, "visuals")
	if visuals == nil || visuals.Kind != yaml.MappingNode {
		return nil
	}
	for index := 0; index+1 < len(visuals.Content); index += 2 {
		name := visuals.Content[index].Value
		visualNode := visuals.Content[index+1]
		if visualNode.Kind != yaml.MappingNode {
			continue
		}
		if mappingValue(visualNode, "stacked") != nil {
			return fmt.Errorf("visual %q uses legacy top-level stacked; use options.stacked", name)
		}
	}
	return nil
}

func rejectLegacyKPIs(bytes []byte) error {
	var node yaml.Node
	if err := yaml.Unmarshal(bytes, &node); err != nil {
		return err
	}
	root := mappingNode(&node)
	if root == nil {
		return nil
	}
	if mappingValue(root, "kpis") != nil {
		return fmt.Errorf("dashboard uses legacy kpis; define KPI cards as visuals with kind kpi")
	}
	return nil
}
