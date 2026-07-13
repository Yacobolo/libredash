package app

import (
	"context"

	"github.com/Yacobolo/libredash/internal/agent"
	agenthttp "github.com/Yacobolo/libredash/internal/agent/http"
	"github.com/Yacobolo/libredash/internal/dashboard"
	"github.com/Yacobolo/libredash/internal/ui"
)

func (s *Server) executeStartedChatTurn(ctx context.Context, service *agent.Service, scope agent.Scope, started *agent.StartedPrompt, execution agenthttp.ChatTurnExecution) (agent.PromptResult, error) {
	state, err := service.ConversationTranscriptState(ctx, scope, started.ConversationID)
	if err != nil {
		_ = started.Abort(ctx, err)
		return agent.PromptResult{}, err
	}
	transcript := state.Transcript
	streamArtifacts := state.Artifacts
	emit := func(signal ui.ChatViewState) {
		if execution.Emit != nil {
			_ = execution.Emit(signal)
		}
	}
	liveSignal := func(statusErr string, running bool) ui.ChatViewState {
		conversations := execution.LiveConversations
		if conversations == nil {
			conversations = s.chatConversations(ctx, scope)
		}
		return chatSignalWithConversations(conversations, started.ConversationID, transcript, streamArtifacts, statusErr, running, true)
	}
	finalSignal := func(statusErr string, running bool) ui.ChatViewState {
		return s.chatSignalWith(ctx, scope, started.ConversationID, transcript, streamArtifacts, statusErr, running)
	}
	if execution.EmitInitialRunning {
		emit(finalSignal("", true))
	}
	result, err := started.Complete(ctx, func(event agent.EventEnvelope) {
		transcript = applyLiveTranscriptEvent(transcript, started.ConversationID, event)
		emit(liveSignal("", true))
	})
	statusErr := chatTurnStatusError(err)
	if result.RunID != "" {
		if refreshed, refreshErr := service.ConversationTranscriptState(ctx, scope, started.ConversationID); refreshErr == nil {
			transcript = refreshed.Transcript
			streamArtifacts = refreshed.Artifacts
		}
	}
	shouldGenerateTitle := execution.GenerateTitle && err == nil && result.RunID != ""
	if shouldGenerateTitle {
		s.markChatTitlePending(started.ConversationID)
	}
	emit(finalSignal(statusErr, false))
	if shouldGenerateTitle {
		s.generateConversationTitleAsync(scope, started.ConversationID, execution.ClientID)
	}
	return result, err
}

func chatTurnStatusError(err error) string {
	if err == nil {
		return ""
	}
	if agent.IsBusy(err) {
		return "A turn is already running for this conversation."
	}
	return err.Error()
}

func (s *Server) catalogForWorkspace(workspaceID string) dashboard.Catalog {
	if metrics, ok := s.metricsForWorkspace(workspaceID); ok && metrics != nil {
		return metrics.Catalog()
	}
	if s.metrics == nil {
		return dashboard.Catalog{Workspace: dashboard.CatalogWorkspace{ID: workspaceID}}
	}
	return s.metrics.Catalog()
}
