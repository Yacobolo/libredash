package semantic

import (
	"bytes"
	"os"

	"github.com/Yacobolo/libredash/internal/configschema"
	"github.com/Yacobolo/libredash/internal/dashboard/report"
	"gopkg.in/yaml.v3"
)

func LoadDashboard(path string) (*report.Dashboard, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if err := rejectLegacyDashboardCollectionKeys(content); err != nil {
		return nil, err
	}
	if err := rejectLegacyVisualStacked(content); err != nil {
		return nil, err
	}
	if err := rejectLegacyKPIs(content); err != nil {
		return nil, err
	}
	if err := rejectLegacyDashboardQueryContract(content); err != nil {
		return nil, err
	}
	if err := configschema.ValidateBytes(configschema.KindDashboard, path, content); err != nil {
		return nil, err
	}
	var dashboard report.Dashboard
	decoder := yaml.NewDecoder(bytes.NewReader(content))
	decoder.KnownFields(true)
	if err := decoder.Decode(&dashboard); err != nil {
		return nil, err
	}
	if err := dashboard.ValidateContract(); err != nil {
		return nil, err
	}
	return &dashboard, nil
}

func mappingNode(node *yaml.Node) *yaml.Node {
	if node.Kind == yaml.DocumentNode && len(node.Content) > 0 {
		return node.Content[0]
	}
	if node.Kind == yaml.MappingNode {
		return node
	}
	return nil
}

func mappingValue(node *yaml.Node, key string) *yaml.Node {
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
