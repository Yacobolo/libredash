package app

import (
	"context"

	"github.com/Yacobolo/libredash/internal/agentapp"
	"github.com/Yacobolo/libredash/internal/ui"
)

func chatSignalWithConversations(conversations []ui.ChatConversationSummary, activeID string, transcript []agentapp.ChatTranscriptItem, statusErr string, running, enabled bool) ui.ChatSignal {
	if !enabled && statusErr == "" {
		statusErr = "Agent is not configured"
	}
	if conversations == nil {
		conversations = []ui.ChatConversationSummary{}
	}
	return ui.ChatSignal{
		Conversations:        conversations,
		ActiveConversationID: activeID,
		Transcript:           transcript,
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
	if activeID != "" && s.agent != nil && scope.PrincipalID != "" {
		if loaded, err := s.agent.ConversationTranscript(ctx, scope, activeID); err == nil {
			transcript = loaded
		}
	}
	return s.chatSignalWith(ctx, scope, activeID, transcript, statusErr, running)
}

func (s *Server) chatSignalWith(ctx context.Context, scope agentapp.Scope, activeID string, transcript []agentapp.ChatTranscriptItem, statusErr string, running bool) ui.ChatSignal {
	conversations := s.chatConversations(ctx, scope)
	enabled := s.agent != nil && s.agent.Enabled()
	if !enabled && statusErr == "" {
		statusErr = "Agent is not configured"
	}
	return ui.ChatSignal{
		Conversations:        conversations,
		ActiveConversationID: activeID,
		Transcript:           transcript,
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
