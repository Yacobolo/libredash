package app

import (
	"context"
	"encoding/json"

	"github.com/Yacobolo/leapview/internal/agent"
	"github.com/Yacobolo/leapview/internal/ui"
	uisignals "github.com/Yacobolo/leapview/internal/ui/signals"
	visualizationir "github.com/Yacobolo/leapview/internal/visualization/ir"
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

func typedChatArtifacts(artifacts agent.ChatArtifactSignals) map[string]visualizationir.VisualizationEnvelope {
	visuals := map[string]visualizationir.VisualizationEnvelope{}
	for key, value := range artifacts.Visuals {
		raw, err := json.Marshal(value)
		if err != nil {
			continue
		}
		var envelope visualizationir.VisualizationEnvelope
		if err := json.Unmarshal(raw, &envelope); err != nil || envelope.VisualID != key || visualizationir.ValidateEnvelope(envelope) != nil {
			continue
		}
		visuals[key] = envelope
	}
	return visuals
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
		out.TitlePending = uisignals.Pointer(s.isChatTitlePending(row.ID))
		conversations = append(conversations, out)
	}
	return conversations
}

func chatConversationSummary(row agent.Conversation) ui.ChatConversationSummary {
	return ui.ChatConversationSummary{
		ID:          row.ID,
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
