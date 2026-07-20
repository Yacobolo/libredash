package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/Yacobolo/libredash/internal/access"
	"github.com/Yacobolo/libredash/internal/agent"
	agenthttp "github.com/Yacobolo/libredash/internal/agent/http"
	"github.com/Yacobolo/libredash/internal/api"
	uisignals "github.com/Yacobolo/libredash/internal/ui/signals"
	"golang.org/x/sync/errgroup"
)

const maxConcurrentAgentReferenceSearches = 8

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
		SearchReferences:       s.searchAgentReferences,
		ResolveTurnContext:     s.resolveAgentTurnContext,
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

func (s *Server) searchAgentReferences(r *http.Request, workspaceID, query string, limit int) ([]uisignals.AgentReferenceSignal, error) {
	handler := s.workspaceHTTPHandler()
	workspaceIDs := []string{strings.TrimSpace(workspaceID)}
	global := workspaceIDs[0] == ""
	if global {
		var err error
		workspaceIDs, err = handler.VisibleWorkspaceIDs(r)
		if err != nil {
			return nil, err
		}
	}
	if credential, ok := apiCredentialFromContext(r.Context()); ok {
		allowedWorkspaceIDs := workspaceIDs[:0]
		for _, currentWorkspaceID := range workspaceIDs {
			if apiTokenAllows(credential.Token, currentWorkspaceID, access.PrivilegeViewItem) {
				allowedWorkspaceIDs = append(allowedWorkspaceIDs, currentWorkspaceID)
			}
		}
		workspaceIDs = allowedWorkspaceIDs
	}
	type rankedReference struct {
		workspaceID string
		row         api.SearchResult
	}
	groups := make([][]api.SearchResult, len(workspaceIDs))
	group, groupContext := errgroup.WithContext(r.Context())
	group.SetLimit(maxConcurrentAgentReferenceSearches)
	for index, currentWorkspaceID := range workspaceIDs {
		group.Go(func() error {
			rows, err := handler.SearchResults(r.Clone(groupContext), currentWorkspaceID, query, nil, limit)
			if err != nil {
				return err
			}
			groups[index] = rows
			return nil
		})
	}
	if err := group.Wait(); err != nil {
		return nil, err
	}
	capacity := 0
	if limit > 0 {
		capacity = limit
	}
	ranked := make([]rankedReference, 0, capacity)
	for index, rows := range groups {
		currentWorkspaceID := workspaceIDs[index]
		for _, row := range rows {
			ranked = append(ranked, rankedReference{workspaceID: currentWorkspaceID, row: row})
		}
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		left, right := ranked[i], ranked[j]
		if left.row.Score != right.row.Score {
			return left.row.Score > right.row.Score
		}
		if left.row.Type != right.row.Type {
			return left.row.Type < right.row.Type
		}
		if left.row.Name != right.row.Name {
			return left.row.Name < right.row.Name
		}
		if left.row.ID != right.row.ID {
			return left.row.ID < right.row.ID
		}
		return left.workspaceID < right.workspaceID
	})
	if limit > 0 && len(ranked) > limit {
		ranked = ranked[:limit]
	}
	out := make([]uisignals.AgentReferenceSignal, 0, len(ranked))
	for _, item := range ranked {
		reference := agentReferenceSignal(item.workspaceID, item.row)
		if global {
			description := item.workspaceID
			if strings.TrimSpace(item.row.Description) != "" {
				description += " · " + item.row.Description
			}
			reference.Description = uisignals.Optional(description)
		}
		out = append(out, reference)
	}
	return out, nil
}

func agentReferenceSignal(workspaceID string, row api.SearchResult) uisignals.AgentReferenceSignal {
	return uisignals.AgentReferenceSignal{
		Kind:        row.Type,
		ID:          row.ID,
		Title:       row.Name,
		Description: uisignals.Optional(row.Description),
		WorkspaceID: workspaceID,
		ComponentID: uisignals.Optional(row.ComponentID),
		DashboardID: uisignals.Optional(row.DashboardID),
		PageID:      uisignals.Optional(row.PageID),
		VisualID:    uisignals.Optional(row.VisualID),
		TableID:     uisignals.Optional(row.TableID),
		FilterID:    uisignals.Optional(row.FilterID),
		ModelID:     uisignals.Optional(row.ModelID),
		DatasetID:   uisignals.Optional(row.DatasetID),
		FieldID:     uisignals.Optional(row.FieldID),
		AssetID:     uisignals.Optional(row.AssetID),
	}
}

func (s *Server) resolveAgentTurnContext(r *http.Request, scope agent.Scope, candidate agent.TurnContext) (agent.TurnContext, error) {
	if len(candidate.References) > agent.MaxTurnReferences {
		return agent.TurnContext{}, fmt.Errorf("at most %d references can be attached", agent.MaxTurnReferences)
	}
	switch strings.ToLower(strings.TrimSpace(candidate.Surface)) {
	case "dashboard":
		return s.resolveDashboardTurnContext(r.Context(), scope, candidate)
	case "chat":
		defaultWorkspaceID := firstNonEmpty(candidate.WorkspaceID, s.defaultWorkspaceID)
		workspaceKeys := map[string]map[string]struct{}{}
		workspaceOrder := []string{}
		for _, reference := range candidate.References {
			workspaceID := firstNonEmpty(reference.WorkspaceID, defaultWorkspaceID)
			if workspaceID == "" {
				continue
			}
			workspaceScope := scope
			workspaceScope.WorkspaceID = workspaceID
			if !agentCredentialAllowsPrivilege(workspaceScope, access.PrivilegeViewItem) {
				return agent.TurnContext{}, errors.New("credential cannot view referenced context")
			}
			if _, exists := workspaceKeys[workspaceID]; !exists {
				workspaceKeys[workspaceID] = map[string]struct{}{}
				workspaceOrder = append(workspaceOrder, workspaceID)
			}
			workspaceKeys[workspaceID][agentReferenceLookupKey(reference.Kind, reference.ID)] = struct{}{}
		}
		workspaceRows := map[string]map[string]api.SearchResult{}
		for _, workspaceID := range workspaceOrder {
			rows, err := s.workspaceHTTPHandler().SearchResultsByKeys(r, workspaceID, workspaceKeys[workspaceID])
			if err != nil {
				return agent.TurnContext{}, err
			}
			byKey := make(map[string]api.SearchResult, len(rows))
			for _, row := range rows {
				byKey[agentReferenceLookupKey(row.Type, row.ID)] = row
			}
			workspaceRows[workspaceID] = byKey
		}
		resolved := make([]agent.TurnReference, 0, min(len(candidate.References), agent.MaxTurnReferences))
		seen := map[string]struct{}{}
		resolvedWorkspaceID := ""
		for _, reference := range candidate.References {
			if len(resolved) == agent.MaxTurnReferences {
				break
			}
			workspaceID := firstNonEmpty(reference.WorkspaceID, defaultWorkspaceID)
			key := workspaceID + ":" + strings.ToLower(strings.TrimSpace(reference.Kind)) + ":" + strings.TrimSpace(reference.ID)
			row, ok := workspaceRows[workspaceID][agentReferenceLookupKey(reference.Kind, reference.ID)]
			if !ok {
				continue
			}
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			resolved = append(resolved, agent.TurnReference{
				Kind: row.Type, ID: row.ID, Title: row.Name, WorkspaceID: workspaceID, ComponentID: row.ComponentID,
				DashboardID: row.DashboardID, PageID: row.PageID, VisualID: row.VisualID, TableID: row.TableID,
				FilterID: row.FilterID, ModelID: row.ModelID, DatasetID: row.DatasetID, FieldID: row.FieldID, AssetID: row.AssetID,
			})
			if len(resolved) == 1 {
				resolvedWorkspaceID = workspaceID
			} else if resolvedWorkspaceID != workspaceID {
				resolvedWorkspaceID = ""
			}
		}
		return agent.TurnContext{Surface: "chat", WorkspaceID: resolvedWorkspaceID, References: resolved}, nil
	default:
		return agent.TurnContext{}, errors.New("unsupported agent context surface")
	}
}

func agentReferenceLookupKey(kind, id string) string {
	return strings.ToLower(strings.TrimSpace(kind)) + ":" + strings.TrimSpace(id)
}

func (s *Server) agentSystemPrompt(ctx context.Context) (string, error) {
	return s.agentHTTPHandler().SystemPrompt(ctx)
}
