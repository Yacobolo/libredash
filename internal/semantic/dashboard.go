package semantic

import (
	"fmt"

	"github.com/Yacobolo/libredash/internal/dashboard/report"
	"gopkg.in/yaml.v3"
)

func LoadDashboard(path string) (*report.Dashboard, error) {
	return nil, fmt.Errorf("legacy dashboard files are not supported; use libredash.dev/v1 Dashboard resources")
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
