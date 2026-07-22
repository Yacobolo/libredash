package app

import (
	"context"
	"net/http"

	"github.com/Yacobolo/leapview/internal/access"
	"github.com/Yacobolo/leapview/internal/agent"
	agenthttp "github.com/Yacobolo/leapview/internal/agent/http"
)

func (s *Server) agentHTTPHandler() *agenthttp.Handler {
	var settings agenthttp.Settings
	if s.store != nil {
		settings = s.store
	}
	return agenthttp.NewHandler(agenthttp.Options{
		Service:  s.agent,
		Settings: settings,
		Broker:   s.broker,
		CSRFToken: func(r *http.Request) string {
			return csrfToken(r, s.auth)
		},
		CurrentRoleLabel:       s.currentRoleLabel,
		ChatSignal:             s.chatSignal,
		ChatSignalWith:         s.chatSignalWith,
		QueueMissingTitle:      s.queueMissingChatTitle,
		ExecuteStartedChatTurn: s.executeStartedChatTurn,
		EnqueueRun: func(ctx context.Context, scope agent.Scope, started *agent.StartedPrompt) error {
			if err := s.appendAsyncEvent(ctx, "agent_run", started.RunID, "agent_run.queued", map[string]any{"runId": started.RunID, "conversationId": started.ConversationID, "status": "running"}); err != nil {
				return err
			}
			return s.enqueueAsyncJobPayload(ctx, "agent:"+started.RunID+":run", apiJobAgentRun, "agent_run", started.RunID, agentRunJob{Scope: scope, Conversation: started.ConversationID, Run: started.RunID, CorrelationID: started.CorrelationID})
		},
		CancelQueuedRun: func(ctx context.Context, scope agent.Scope, conversationID, runID string) (bool, error) {
			cancelled, err := s.cancelQueuedAsyncJob(ctx, "agent:"+runID+":run")
			if err != nil || !cancelled {
				return cancelled, err
			}
			if err := s.agent.CancelPersistedRun(ctx, scope, conversationID, runID); err != nil {
				return false, err
			}
			_ = s.appendAsyncEvent(ctx, "agent_run", runID, "agent_run.cancelled", map[string]any{"runId": runID, "conversationId": conversationID})
			return true, nil
		},
		CurrentPrincipal: func(r *http.Request) (agenthttp.Principal, bool) {
			if s.auth == nil {
				return agenthttp.Principal{}, false
			}
			principal, ok := s.auth.Principal(r)
			return agenthttp.Principal{ID: principal.ID, DevAuthBypass: principal.DevBypass}, ok
		},
		CurrentCredential: func(r *http.Request) (access.APICredential, bool) {
			if s.auth == nil {
				return access.APICredential{}, false
			}
			return s.auth.APICredential(r)
		},
	})
}

func (s *Server) agentSystemPrompt(ctx context.Context) (string, error) {
	return s.agentHTTPHandler().SystemPrompt(ctx)
}
