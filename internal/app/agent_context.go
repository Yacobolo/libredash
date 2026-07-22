package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/Yacobolo/leapview/internal/access"
	"github.com/Yacobolo/leapview/internal/agent"
	"github.com/Yacobolo/leapview/internal/dashboard"
	reportdef "github.com/Yacobolo/leapview/internal/dashboard/report"
	productsearch "github.com/Yacobolo/leapview/internal/search"
)

func (s *Server) resolveAgentTurnContext(r *http.Request, scope agent.Scope, candidate agent.TurnContext) (agent.TurnContext, error) {
	if len(candidate.References) > agent.MaxTurnReferences {
		return agent.TurnContext{}, fmt.Errorf("at most %d references can be attached", agent.MaxTurnReferences)
	}
	switch strings.ToLower(strings.TrimSpace(candidate.Surface)) {
	case "dashboard":
		return s.resolveDashboardTurnContext(r.Context(), scope, candidate)
	case "chat":
		if s.search == nil {
			return agent.TurnContext{}, errors.New("search is not configured")
		}
		defaultWorkspaceID := firstNonEmpty(candidate.WorkspaceID, s.defaultWorkspaceID)
		references := make([]productsearch.Reference, 0, len(candidate.References))
		for _, reference := range candidate.References {
			typ := productsearch.Type(strings.ToLower(strings.TrimSpace(reference.Reference.Type)))
			if !isAgentReferenceType(typ) {
				continue
			}
			workspaceID := firstNonEmpty(reference.Reference.WorkspaceID, defaultWorkspaceID)
			if workspaceID == "" {
				continue
			}
			workspaceScope := scope
			workspaceScope.WorkspaceID = workspaceID
			if !agentCredentialAllowsPrivilege(workspaceScope, access.PrivilegeViewItem) {
				return agent.TurnContext{}, errors.New("credential cannot view referenced context")
			}
			references = append(references, productsearch.Reference{
				WorkspaceID: workspaceID,
				Type:        typ,
				ID:          reference.Reference.ID,
			})
		}
		subject, ok := s.searchSubject(r)
		if !ok {
			return agent.TurnContext{}, errors.New("search principal is unavailable")
		}
		rows, err := s.search.Resolve(r.Context(), subject, string(s.requestServingEnvironment(r)), references)
		if err != nil {
			return agent.TurnContext{}, err
		}
		resolved := make([]agent.TurnReference, 0, len(rows))
		resolvedWorkspaceID := ""
		for _, row := range rows {
			resolved = append(resolved, agentTurnReference(row))
			if len(resolved) == 1 {
				resolvedWorkspaceID = row.Reference.WorkspaceID
			} else if resolvedWorkspaceID != row.Reference.WorkspaceID {
				resolvedWorkspaceID = ""
			}
		}
		return agent.TurnContext{Surface: "chat", WorkspaceID: resolvedWorkspaceID, References: resolved}, nil
	default:
		return agent.TurnContext{}, errors.New("unsupported agent context surface")
	}
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
	catalog := metrics.Catalog()
	workspaceName := strings.TrimSpace(catalog.Workspace.Title)
	if workspaceName == "" {
		workspaceName = workspaceID
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
		References: resolveDashboardTurnReferences(candidate.References, dashboardTurnReferenceContext{
			Workspace:   agent.TurnReferenceWorkspace{ID: workspaceID, Name: workspaceName},
			DashboardID: report.ID, DashboardTitle: report.Title, Page: page,
		}, report.Visuals, report.Tables),
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

type dashboardTurnReferenceContext struct {
	Workspace      agent.TurnReferenceWorkspace
	DashboardID    string
	DashboardTitle string
	Page           dashboard.Page
}

func resolveDashboardTurnReferences(candidates []agent.TurnReference, context dashboardTurnReferenceContext, visuals map[string]reportdef.Visual, tables map[string]reportdef.TableVisual) []agent.TurnReference {
	resolved := make([]agent.TurnReference, 0, min(len(candidates), agent.MaxTurnReferences))
	seen := map[string]struct{}{}
	href := "/workspaces/" + url.PathEscape(context.Workspace.ID) + "/dashboards/" + url.PathEscape(context.DashboardID) + "/pages/" + url.PathEscape(context.Page.ID)
	location := agent.TurnReferenceLocation{
		DashboardID: context.DashboardID, DashboardName: context.DashboardTitle,
		PageID: context.Page.ID, PageName: context.Page.Title, Href: href,
	}
	for _, candidate := range candidates {
		if len(resolved) == agent.MaxTurnReferences {
			break
		}
		if strings.ToLower(strings.TrimSpace(candidate.Reference.Type)) != "visual" {
			continue
		}
		if strings.TrimSpace(candidate.Reference.WorkspaceID) != context.Workspace.ID {
			continue
		}
		visualID := lastSearchReferencePart(candidate.Reference.ID)
		if visualID == "" || candidate.Reference.ID != context.DashboardID+"."+visualID {
			continue
		}
		for _, component := range context.Page.Visuals {
			if component.Visual != visualID && component.Table != visualID {
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
				Reference:   candidate.Reference,
				Name:        title,
				Workspace:   context.Workspace,
				Hierarchy:   []string{context.Workspace.Name, context.DashboardTitle, context.Page.Title},
				Href:        href,
				Locations:   []agent.TurnReferenceLocation{location},
				Context:     []string{"current_page", "current_dashboard", "current_workspace"},
				ComponentID: component.ID,
				VisualID:    visualID,
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
