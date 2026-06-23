package app

import (
	"context"

	"github.com/Yacobolo/libredash/internal/agentapp"
	"github.com/Yacobolo/libredash/internal/api"
)

func chatSignalWithConversations(conversations []api.AgentConversationResponse, activeID string, transcript []api.AgentChatTranscriptItem, statusErr string, running, enabled bool) api.AgentChatSignal {
	if !enabled && statusErr == "" {
		statusErr = "Agent is not configured"
	}
	if conversations == nil {
		conversations = []api.AgentConversationResponse{}
	}
	return api.AgentChatSignal{
		Conversations:        conversations,
		ActiveConversationID: activeID,
		Transcript:           transcript,
		Status: api.AgentChatStatus{
			Enabled: enabled,
			Running: running,
			Error:   statusErr,
		},
		Composer: api.AgentComposerSignal{
			Value:       "",
			Disabled:    !enabled || running,
			Placeholder: chatPlaceholder(enabled, running),
		},
	}
}

func (s *Server) chatSignal(ctx context.Context, scope agentapp.Scope, activeID, statusErr string, running bool) api.AgentChatSignal {
	transcript := []api.AgentChatTranscriptItem{}
	if activeID != "" && s.agent != nil && scope.PrincipalID != "" {
		if loaded, err := s.agent.ConversationTranscript(ctx, scope, activeID); err == nil {
			transcript = loaded
		}
	}
	return s.chatSignalWith(ctx, scope, activeID, transcript, statusErr, running)
}

func (s *Server) chatSignalWith(ctx context.Context, scope agentapp.Scope, activeID string, transcript []api.AgentChatTranscriptItem, statusErr string, running bool) api.AgentChatSignal {
	conversations := s.chatConversations(ctx, scope)
	enabled := s.agent != nil && s.agent.Enabled()
	if !enabled && statusErr == "" {
		statusErr = "Agent is not configured"
	}
	return api.AgentChatSignal{
		Conversations:        conversations,
		ActiveConversationID: activeID,
		Transcript:           transcript,
		Status: api.AgentChatStatus{
			Enabled: enabled,
			Running: running,
			Error:   statusErr,
		},
		Composer: api.AgentComposerSignal{
			Value:       "",
			Disabled:    !enabled || running,
			Placeholder: chatPlaceholder(enabled, running),
		},
	}
}

func (s *Server) chatConversations(ctx context.Context, scope agentapp.Scope) []api.AgentConversationResponse {
	conversations := []api.AgentConversationResponse{}
	if s.agent == nil || scope.PrincipalID == "" {
		return conversations
	}
	rows, err := s.agent.ListConversations(ctx, scope)
	if err != nil {
		return conversations
	}
	for _, row := range rows {
		out := agentConversationDTO(row)
		out.TitlePending = s.isChatTitlePending(row.ID)
		conversations = append(conversations, out)
	}
	return conversations
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
