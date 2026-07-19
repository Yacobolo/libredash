package app

import (
	"context"
	"encoding/json"

	"github.com/Yacobolo/libredash/internal/agent"
	"github.com/Yacobolo/libredash/internal/dashboard"
	"github.com/Yacobolo/libredash/internal/ui"
	uisignals "github.com/Yacobolo/libredash/internal/ui/signals"
)

func chatSignalWithConversations(conversations []ui.ChatConversationSummary, activeID string, transcript []agent.ChatTranscriptItem, artifacts agent.ChatArtifactSignals, statusErr string, running, enabled bool) ui.ChatViewState {
	if !enabled && statusErr == "" {
		statusErr = "Agent is not configured"
	}
	if conversations == nil {
		conversations = []ui.ChatConversationSummary{}
	}
	artifacts = normalizeChatArtifacts(artifacts)
	return ui.ChatViewState{
		Visuals: typedChatArtifacts(artifacts),
		Agent: ui.ChatSignal{
			Conversations:        conversations,
			ActiveConversationID: activeID,
			Transcript:           ui.ChatTranscriptItems(transcript),
			Status: ui.ChatStatus{
				Enabled: enabled,
				Running: running,
				Error:   uisignals.Optional(statusErr),
			},
			Composer: ui.ComposerSignal{
				Value:       "",
				Disabled:    !enabled || running,
				Placeholder: chatPlaceholder(enabled, running),
			},
		},
	}
}

func (s *Server) chatSignal(ctx context.Context, scope agent.Scope, activeID, statusErr string, running bool) ui.ChatViewState {
	transcript := []agent.ChatTranscriptItem{}
	artifacts := agent.ChatArtifactSignals{}
	if activeID != "" && s.agent != nil && scope.PrincipalID != "" {
		if loaded, err := s.agent.ConversationTranscriptState(ctx, scope, activeID); err == nil {
			transcript = loaded.Transcript
			artifacts = loaded.Artifacts
		}
	}
	return s.chatSignalWith(ctx, scope, activeID, transcript, artifacts, statusErr, running)
}

func (s *Server) chatSignalWith(ctx context.Context, scope agent.Scope, activeID string, transcript []agent.ChatTranscriptItem, artifacts agent.ChatArtifactSignals, statusErr string, running bool) ui.ChatViewState {
	conversations := s.chatConversations(ctx, scope)
	enabled := s.agent != nil && s.agent.Enabled()
	if !enabled && statusErr == "" {
		statusErr = "Agent is not configured"
	}
	artifacts = normalizeChatArtifacts(artifacts)
	return ui.ChatViewState{
		Visuals: typedChatArtifacts(artifacts),
		Agent: ui.ChatSignal{
			Conversations:        conversations,
			ActiveConversationID: activeID,
			Transcript:           ui.ChatTranscriptItems(transcript),
			Status: ui.ChatStatus{
				Enabled: enabled,
				Running: running,
				Error:   uisignals.Optional(statusErr),
			},
			Composer: ui.ComposerSignal{
				Value:       "",
				Disabled:    !enabled || running,
				Placeholder: chatPlaceholder(enabled, running),
			},
		},
	}
}

func normalizeChatArtifacts(artifacts agent.ChatArtifactSignals) agent.ChatArtifactSignals {
	if artifacts.Visuals == nil {
		artifacts.Visuals = map[string]any{}
	}
	return artifacts
}

func typedChatArtifacts(artifacts agent.ChatArtifactSignals) map[string]uisignals.DashboardVisual {
	visuals := map[string]uisignals.DashboardVisual{}
	for key, value := range artifacts.Visuals {
		raw, err := json.Marshal(value)
		if err != nil {
			continue
		}
		var discriminator struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(raw, &discriminator); err != nil {
			continue
		}
		if !isChatVisualType(discriminator.Type) {
			continue
		}
		if discriminator.Type == "table" || discriminator.Type == "matrix" || discriminator.Type == "pivot" {
			var tabular dashboard.TabularVisual
			if err := json.Unmarshal(raw, &tabular); err == nil {
				tabular.Table.Kind = map[string]string{"table": "data_table", "matrix": "matrix_table", "pivot": "pivot_table"}[discriminator.Type]
				visuals[key] = uisignals.DashboardTabularVisualFromDashboard(key, tabular.Table)
			}
			continue
		}
		var visual dashboard.Visual
		if err := json.Unmarshal(raw, &visual); err == nil {
			visuals[key] = uisignals.DashboardVisualFromDashboard(visual)
		}
	}
	return visuals
}

func isChatVisualType(value string) bool {
	switch value {
	case "line", "area", "bar", "column", "pie", "donut", "scatter", "funnel", "treemap", "gauge", "heatmap", "sankey", "graph", "map", "candlestick", "boxplot", "combo", "waterfall", "histogram", "radar", "tree", "sunburst", "kpi", "table", "matrix", "pivot":
		return true
	default:
		return false
	}
}

func chatSignalPatch(signal ui.ChatViewState) map[string]any {
	return map[string]any{
		"agent":   signal.Agent,
		"visuals": signal.Visuals,
	}
}

func (s *Server) chatConversations(ctx context.Context, scope agent.Scope) []ui.ChatConversationSummary {
	conversations := []ui.ChatConversationSummary{}
	if s.agent == nil || scope.PrincipalID == "" {
		return conversations
	}
	rows, err := s.agent.ListConversations(ctx, scope)
	if err != nil {
		return conversations
	}
	for _, row := range rows {
		out := chatConversationSummary(row)
		out.TitlePending = uisignals.Optional(s.isChatTitlePending(row.ID))
		conversations = append(conversations, out)
	}
	return conversations
}

func chatConversationSummary(row agent.Conversation) ui.ChatConversationSummary {
	return ui.ChatConversationSummary{
		ID:          row.ID,
		WorkspaceID: row.WorkspaceID,
		PrincipalID: row.PrincipalID,
		Title:       row.Title,
		Status:      row.Status,
		CreatedAt:   row.CreatedAt,
		UpdatedAt:   row.UpdatedAt,
		ArchivedAt:  uisignals.Optional(row.ArchivedAt),
	}
}

func chatPlaceholder(enabled, running bool) string {
	if !enabled {
		return "Agent is not configured"
	}
	if running {
		return "Waiting for the current answer..."
	}
	return "Ask about dashboards, metrics, or models..."
}
