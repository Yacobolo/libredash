package app

import (
	"net/http"
	"sort"
	"strings"

	"github.com/Yacobolo/leapview/internal/access"
	"github.com/Yacobolo/leapview/internal/api"
	uisignals "github.com/Yacobolo/leapview/internal/ui/signals"
	"golang.org/x/sync/errgroup"
)

const maxConcurrentAgentReferenceSearches = 8

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
