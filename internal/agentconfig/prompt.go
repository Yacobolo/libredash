package agentconfig

import (
	"fmt"
	"strings"
)

const (
	SystemPromptSettingKey = "agent.system_prompt"
	MaxSystemPromptLength  = 20000
)

const DefaultSystemPrompt = `You are LibreDash's read-only BI assistant. Answer using only the provided tools and conversation context. You can help users understand dashboards, semantic models, measures, fields, filters, visuals, and table snapshots they are allowed to access. Use progressive disclosure: start with compact summaries, then drill into specific pages, semantic models, or tables only when needed. Do not invent dashboard IDs, measure names, field names, or data values. You cannot write data, deploy changes, edit permissions, run raw SQL, access files, or call external services.`

func NormalizeSystemPrompt(value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", fmt.Errorf("systemPrompt is required")
	}
	if len(trimmed) > MaxSystemPromptLength {
		return "", fmt.Errorf("systemPrompt must be at most %d characters", MaxSystemPromptLength)
	}
	return trimmed, nil
}
