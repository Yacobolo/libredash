package app

import (
	"context"
	"strings"
	"time"

	"github.com/Yacobolo/leapview/internal/agent"
	"github.com/Yacobolo/leapview/pkg/pagestream"
)

func (s *Server) generateConversationTitleAsync(scope agent.Scope, conversationID, clientID string) {
	if s.agent == nil {
		s.clearChatTitlePending(conversationID)
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if _, err := s.agent.GenerateConversationTitle(ctx, scope, conversationID); err != nil && s.logger != nil {
			s.logger.DebugContext(ctx, "agent title generation failed", "conversation_id", conversationID, "error", err)
		}
		s.clearChatTitlePending(conversationID)
		s.broker.Publish(chatStreamID(scope, clientID), pagestream.SignalPatch{
			"agent": map[string]any{
				"conversations": s.chatConversations(ctx, scope),
			},
		})
	}()
}

// queueMissingChatTitle repairs old one-turn chats that missed the async title job.
func (s *Server) queueMissingChatTitle(ctx context.Context, scope agent.Scope, conversationID, clientID string) {
	if s.agent == nil || s.isChatTitlePending(conversationID) {
		return
	}
	ok, err := s.agent.ConversationNeedsGeneratedTitle(ctx, scope, conversationID)
	if err != nil || !ok {
		return
	}
	s.markChatTitlePending(conversationID)
	s.generateConversationTitleAsync(scope, conversationID, clientID)
}

func (s *Server) markChatTitlePending(conversationID string) {
	if conversationID == "" {
		return
	}
	s.chatTitleMu.Lock()
	defer s.chatTitleMu.Unlock()
	if s.pendingChatTitles == nil {
		s.pendingChatTitles = map[string]struct{}{}
	}
	s.pendingChatTitles[conversationID] = struct{}{}
}

func (s *Server) clearChatTitlePending(conversationID string) {
	if conversationID == "" {
		return
	}
	s.chatTitleMu.Lock()
	defer s.chatTitleMu.Unlock()
	delete(s.pendingChatTitles, conversationID)
}

func (s *Server) isChatTitlePending(conversationID string) bool {
	s.chatTitleMu.Lock()
	defer s.chatTitleMu.Unlock()
	_, ok := s.pendingChatTitles[conversationID]
	return ok
}

func chatStreamID(scope agent.Scope, clientID string) string {
	if strings.TrimSpace(clientID) == "" {
		clientID = "default"
	}
	return "chat:" + clientID + ":" + scope.PrincipalID
}
