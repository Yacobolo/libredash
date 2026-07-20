package agent

import (
	"sort"

	agentcore "github.com/Yacobolo/leapview/pkg/agent"
)

func (s *Service) toolDefinitions(scope Scope) []agentcore.ToolDefinition {
	var tools []agentcore.ToolDefinition
	for _, provider := range s.toolProviders {
		if provider == nil {
			continue
		}
		tools = append(tools, provider(scope)...)
	}
	sort.SliceStable(tools, func(i, j int) bool {
		return tools[i].Name < tools[j].Name
	})
	return tools
}

func (s *Service) ToolDefinitions(scope Scope) []agentcore.ToolDefinition {
	return s.toolDefinitions(scope)
}
