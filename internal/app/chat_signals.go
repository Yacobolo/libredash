package app

import (
	"context"
	"encoding/json"

	"github.com/Yacobolo/libredash/internal/agentapp"
	"github.com/Yacobolo/libredash/internal/dashboard"
	"github.com/Yacobolo/libredash/internal/ui"
)

func chatSignalWithConversations(conversations []ui.ChatConversationSummary, activeID string, transcript []agentapp.ChatTranscriptItem, artifacts agentapp.ChatArtifactSignals, statusErr string, running, enabled bool) ui.ChatSignal {
	if !enabled && statusErr == "" {
		statusErr = "Agent is not configured"
	}
	if conversations == nil {
		conversations = []ui.ChatConversationSummary{}
	}
	artifacts = normalizeChatArtifacts(artifacts)
	return ui.ChatSignal{
		Conversations:        conversations,
		ActiveConversationID: activeID,
		Transcript:           ui.ChatTranscriptItems(transcript),
		Visuals:              typedChatVisualArtifacts(artifacts.Visuals),
		Tables:               typedChatTableArtifacts(artifacts.Tables),
		Status: ui.ChatStatus{
			Enabled: enabled,
			Running: running,
			Error:   statusErr,
		},
		Composer: ui.ComposerSignal{
			Value:       "",
			Disabled:    !enabled || running,
			Placeholder: chatPlaceholder(enabled, running),
		},
	}
}

func (s *Server) chatSignal(ctx context.Context, scope agentapp.Scope, activeID, statusErr string, running bool) ui.ChatSignal {
	transcript := []agentapp.ChatTranscriptItem{}
	artifacts := agentapp.ChatArtifactSignals{}
	if activeID != "" && s.agent != nil && scope.PrincipalID != "" {
		if loaded, err := s.agent.ConversationTranscriptState(ctx, scope, activeID); err == nil {
			transcript = loaded.Transcript
			artifacts = loaded.Artifacts
		}
	}
	return s.chatSignalWith(ctx, scope, activeID, transcript, artifacts, statusErr, running)
}

func (s *Server) chatSignalWith(ctx context.Context, scope agentapp.Scope, activeID string, transcript []agentapp.ChatTranscriptItem, artifacts agentapp.ChatArtifactSignals, statusErr string, running bool) ui.ChatSignal {
	conversations := s.chatConversations(ctx, scope)
	enabled := s.agent != nil && s.agent.Enabled()
	if !enabled && statusErr == "" {
		statusErr = "Agent is not configured"
	}
	artifacts = normalizeChatArtifacts(artifacts)
	return ui.ChatSignal{
		Conversations:        conversations,
		ActiveConversationID: activeID,
		Transcript:           ui.ChatTranscriptItems(transcript),
		Visuals:              typedChatVisualArtifacts(artifacts.Visuals),
		Tables:               typedChatTableArtifacts(artifacts.Tables),
		Status: ui.ChatStatus{
			Enabled: enabled,
			Running: running,
			Error:   statusErr,
		},
		Composer: ui.ComposerSignal{
			Value:       "",
			Disabled:    !enabled || running,
			Placeholder: chatPlaceholder(enabled, running),
		},
	}
}

func normalizeChatArtifacts(artifacts agentapp.ChatArtifactSignals) agentapp.ChatArtifactSignals {
	if artifacts.Visuals == nil {
		artifacts.Visuals = map[string]any{}
	}
	if artifacts.Tables == nil {
		artifacts.Tables = map[string]any{}
	}
	return artifacts
}

func typedChatVisualArtifacts(values map[string]any) map[string]dashboard.Visual {
	visuals := map[string]dashboard.Visual{}
	for key, value := range values {
		raw, err := json.Marshal(value)
		if err != nil {
			continue
		}
		visual := dashboard.Visual{}
		if err := json.Unmarshal(raw, &visual); err != nil {
			continue
		}
		visuals[key] = visual
	}
	return visuals
}

func typedChatTableArtifacts(values map[string]any) map[string]dashboard.Table {
	tables := map[string]dashboard.Table{}
	for key, value := range values {
		raw, err := json.Marshal(value)
		if err != nil {
			continue
		}
		table := dashboard.Table{}
		if err := json.Unmarshal(raw, &table); err != nil {
			continue
		}
		tables[key] = table
	}
	return tables
}

func chatSignalPatch(signal ui.ChatSignal) map[string]any {
	return map[string]any{
		"agent":   signal,
		"visuals": signal.Visuals,
		"tables":  signal.Tables,
	}
}

func (s *Server) chatConversations(ctx context.Context, scope agentapp.Scope) []ui.ChatConversationSummary {
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
		out.TitlePending = s.isChatTitlePending(row.ID)
		conversations = append(conversations, out)
	}
	return conversations
}

func chatConversationSummary(row agentapp.Conversation) ui.ChatConversationSummary {
	return ui.ChatConversationSummary{
		ID:          row.ID,
		WorkspaceID: row.WorkspaceID,
		PrincipalID: row.PrincipalID,
		Title:       row.Title,
		Status:      row.Status,
		CreatedAt:   row.CreatedAt,
		UpdatedAt:   row.UpdatedAt,
		ArchivedAt:  row.ArchivedAt,
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
