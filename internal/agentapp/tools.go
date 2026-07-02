package agentapp

import (
	"sort"

	"github.com/Yacobolo/libredash/internal/workspace"
	"github.com/Yacobolo/libredash/pkg/agent"
)

func (s *Service) toolDefinitions(scope Scope) []agent.ToolDefinition {
	var tools []agent.ToolDefinition
	for _, provider := range s.toolProviders {
		if provider == nil {
			continue
		}
		tools = append(tools, provider(scope)...)
	}
	return s.filterToolDefinitions(scope, tools)
}

func (s *Service) ToolDefinitions(scope Scope) []agent.ToolDefinition {
	return s.toolDefinitions(scope)
}

func (s *Service) filterToolDefinitions(scope Scope, tools []agent.ToolDefinition) []agent.ToolDefinition {
	policy, _ := s.policyForScope(scope)
	if !policy.Enabled {
		return nil
	}
	allow := stringSet(policy.Tools.Allow)
	deny := stringSet(policy.Tools.Deny)
	filtered := make([]agent.ToolDefinition, 0, len(tools))
	for _, tool := range tools {
		if len(allow) > 0 {
			if _, ok := allow[tool.Name]; !ok {
				continue
			}
		}
		if _, ok := deny[tool.Name]; ok {
			continue
		}
		filtered = append(filtered, tool)
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		return filtered[i].Name < filtered[j].Name
	})
	return filtered
}

func (s *Service) policyForScope(scope Scope) (workspace.AgentPolicy, bool) {
	if s == nil || s.policyProvider == nil {
		return workspace.DefaultAgentPolicy(), false
	}
	policy, ok := s.policyProvider(scope)
	if !ok {
		return workspace.DefaultAgentPolicy(), false
	}
	return policy, true
}

func stringSet(values []string) map[string]struct{} {
	out := make(map[string]struct{}, len(values))
	for _, value := range values {
		out[value] = struct{}{}
	}
	return out
}
