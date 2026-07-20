package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/Yacobolo/leapview/internal/access"
	"github.com/Yacobolo/leapview/internal/agent"
	"github.com/Yacobolo/leapview/internal/api"
	"github.com/Yacobolo/leapview/internal/dashboard"
	reportdef "github.com/Yacobolo/leapview/internal/dashboard/report"
)

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

func (s *Server) resolveDashboardTurnContext(ctx context.Context, scope agent.Scope, candidate agent.TurnContext) (agent.TurnContext, error) {
	workspaceID := strings.TrimSpace(candidate.WorkspaceID)
	dashboardID := strings.TrimSpace(candidate.DashboardID)
	pageID := strings.TrimSpace(candidate.PageID)
	if workspaceID == "" || dashboardID == "" || pageID == "" {
		return agent.TurnContext{}, errors.New("dashboard context requires workspace, dashboard, and page")
	}
	scope.WorkspaceID = workspaceID
	if !agentCredentialAllowsPrivilege(scope, access.PrivilegeViewItem) {
		return agent.TurnContext{}, errors.New("credential cannot view this dashboard")
	}
	object := access.ItemObjectWithParent(access.SecurableDashboard, workspaceID, dashboardID, access.WorkspaceObject(workspaceID))
	if !scope.DevAuthBypass {
		allowed, err := s.authorizeDashboardTurnContext(ctx, scope.PrincipalID, object, access.WorkspaceObject(workspaceID))
		if err != nil {
			return agent.TurnContext{}, fmt.Errorf("authorize dashboard context: %w", err)
		}
		if !allowed {
			return agent.TurnContext{}, errors.New("dashboard context is not accessible")
		}
	}
	metrics, ok := s.metricsForWorkspace(workspaceID)
	if !ok || metrics == nil {
		return agent.TurnContext{}, fmt.Errorf("unknown workspace %q", workspaceID)
	}
	report, _, ok := metrics.Report(dashboardID)
	if !ok {
		return agent.TurnContext{}, fmt.Errorf("unknown dashboard %q", dashboardID)
	}
	var page dashboard.Page
	for _, current := range metrics.Pages(dashboardID) {
		if current.ID == pageID {
			page = current
			break
		}
	}
	if page.ID == "" {
		return agent.TurnContext{}, fmt.Errorf("unknown dashboard page %q", pageID)
	}
	filters, err := dashboardFiltersFromTurnContext(candidate.Filters)
	if err != nil {
		return agent.TurnContext{}, err
	}
	filters = report.NormalizeFiltersForPage(page.ID, filters).WithDefaults()
	filterMap, err := turnContextFilters(filters)
	if err != nil {
		return agent.TurnContext{}, err
	}
	return agent.TurnContext{
		Surface:        "dashboard",
		WorkspaceID:    workspaceID,
		DashboardID:    report.ID,
		DashboardTitle: report.Title,
		PageID:         page.ID,
		PageTitle:      page.Title,
		ModelID:        metrics.ModelIDForDashboard(report.ID),
		Generation:     candidate.Generation,
		Filters:        filterMap,
		References:     resolveDashboardTurnReferences(candidate.References, page, report.Visuals, report.Tables),
	}, nil
}

func (s *Server) authorizeDashboardTurnContext(ctx context.Context, principalID string, objects ...access.ObjectRef) (bool, error) {
	if s.auth == nil {
		return true, nil
	}
	repo, err := s.accessRepository()
	if err != nil {
		return false, err
	}
	if repo == nil || strings.TrimSpace(principalID) == "" {
		return false, nil
	}
	decision, err := repo.AuthorizeAny(ctx, principalID, access.PrivilegeViewItem, objects)
	if err != nil {
		return false, err
	}
	return decision.Allowed, nil
}

func dashboardFiltersFromTurnContext(raw map[string]any) (dashboard.Filters, error) {
	if raw == nil {
		return dashboard.Filters{}.WithDefaults(), nil
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return dashboard.Filters{}, fmt.Errorf("encode dashboard filters: %w", err)
	}
	var filters dashboard.Filters
	if err := json.Unmarshal(encoded, &filters); err != nil {
		return dashboard.Filters{}, fmt.Errorf("invalid dashboard filters: %w", err)
	}
	return filters.WithDefaults(), nil
}

func turnContextFilters(filters dashboard.Filters) (map[string]any, error) {
	encoded, err := json.Marshal(filters)
	if err != nil {
		return nil, fmt.Errorf("encode normalized dashboard filters: %w", err)
	}
	var out map[string]any
	if err := json.Unmarshal(encoded, &out); err != nil {
		return nil, fmt.Errorf("decode normalized dashboard filters: %w", err)
	}
	return out, nil
}

func resolveDashboardTurnReferences(candidates []agent.TurnReference, page dashboard.Page, visuals map[string]reportdef.Visual, tables map[string]reportdef.TableVisual) []agent.TurnReference {
	resolved := make([]agent.TurnReference, 0, min(len(candidates), agent.MaxTurnReferences))
	seen := map[string]struct{}{}
	for _, candidate := range candidates {
		if len(resolved) == agent.MaxTurnReferences {
			break
		}
		if strings.ToLower(strings.TrimSpace(candidate.Kind)) != "visual" {
			continue
		}
		componentID := strings.TrimSpace(candidate.ComponentID)
		visualID := strings.TrimSpace(candidate.VisualID)
		if componentID == "" || visualID == "" {
			continue
		}
		for _, component := range page.Visuals {
			if component.ID != componentID {
				continue
			}
			title, visualType, ok := resolvedVisualMetadata(component, visualID, visuals, tables)
			if !ok {
				break
			}
			if _, exists := seen[component.ID]; exists {
				break
			}
			seen[component.ID] = struct{}{}
			resolved = append(resolved, agent.TurnReference{
				Kind:        "visual",
				ID:          candidate.ID,
				ComponentID: component.ID,
				VisualID:    visualID,
				Title:       title,
				VisualType:  visualType,
			})
			break
		}
	}
	return resolved
}

func resolvedVisualMetadata(component dashboard.PageVisual, visualID string, visuals map[string]reportdef.Visual, tables map[string]reportdef.TableVisual) (string, string, bool) {
	if component.Visual == visualID {
		visual, ok := visuals[visualID]
		if !ok {
			return "", "", false
		}
		title := strings.TrimSpace(component.Title)
		if title == "" {
			title = strings.TrimSpace(visual.Title)
		}
		if title == "" {
			title = visualID
		}
		visualType := strings.TrimSpace(visual.Type)
		if visualType == "" {
			visualType = strings.TrimSpace(visual.Kind)
		}
		return title, visualType, true
	}
	if component.Table == visualID {
		table, ok := tables[visualID]
		if !ok {
			return "", "", false
		}
		title := strings.TrimSpace(component.Title)
		if title == "" {
			title = strings.TrimSpace(table.Title)
		}
		if title == "" {
			title = visualID
		}
		visualType := strings.TrimSpace(table.Kind)
		if visualType == "" {
			visualType = "table"
		}
		return title, visualType, true
	}
	return "", "", false
}
