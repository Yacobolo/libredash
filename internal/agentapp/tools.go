package agentapp

import "github.com/Yacobolo/libredash/pkg/agent"

func (s *Service) toolDefinitions(scope Scope) []agent.ToolDefinition {
	var tools []agent.ToolDefinition
	for _, provider := range s.toolProviders {
		if provider == nil {
			continue
		}
		tools = append(tools, provider(scope)...)
	}
	return tools
}
