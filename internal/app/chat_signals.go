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
		Visuals: typedChatVisualArtifacts(artifacts.Visuals),
		Tables:  typedChatTableArtifacts(artifacts.Tables),
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
		Visuals: typedChatVisualArtifacts(artifacts.Visuals),
		Tables:  typedChatTableArtifacts(artifacts.Tables),
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
	if artifacts.Tables == nil {
		artifacts.Tables = map[string]any{}
	}
	return artifacts
}

func typedChatVisualArtifacts(values map[string]any) map[string]uisignals.DashboardVisual {
	visuals := map[string]uisignals.DashboardVisual{}
	for key, value := range values {
		raw, err := json.Marshal(value)
		if err != nil {
			continue
		}
		visual := dashboard.Visual{}
		if err := json.Unmarshal(raw, &visual); err != nil {
			continue
		}
		visuals[key] = uisignals.DashboardVisualFromDashboard(visual)
	}
	return visuals
}

func typedChatTableArtifacts(values map[string]any) map[string]uisignals.DashboardTable {
	tables := map[string]uisignals.DashboardTable{}
	for key, value := range values {
		raw, err := json.Marshal(value)
		if err != nil {
			continue
		}
		table := dashboard.Table{}
		if err := json.Unmarshal(raw, &table); err != nil {
			continue
		}
		tables[key] = uisignals.DashboardTableFromDashboard(table)
	}
	return tables
}

func chatSignalPatch(signal ui.ChatViewState) map[string]any {
	return map[string]any{
		"agent":   signal.Agent,
		"visuals": signal.Visuals,
		"tables":  signal.Tables,
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
