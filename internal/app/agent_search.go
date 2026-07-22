package app

import (
	"errors"
	"net/http"
	"strings"

	"github.com/Yacobolo/leapview/internal/agent"
	productsearch "github.com/Yacobolo/leapview/internal/search"
	uisignals "github.com/Yacobolo/leapview/internal/ui/signals"
)

var agentReferenceTypes = []productsearch.Type{
	productsearch.TypeVisual,
	productsearch.TypeDashboard,
	productsearch.TypePage,
	productsearch.TypeMeasure,
	productsearch.TypeSemanticModel,
}

func isAgentReferenceType(typ productsearch.Type) bool {
	switch typ {
	case productsearch.TypeVisual, productsearch.TypeDashboard, productsearch.TypePage,
		productsearch.TypeMeasure, productsearch.TypeSemanticModel:
		return true
	default:
		return false
	}
}

func (s *Server) searchAgentReferences(r *http.Request, context agent.TurnContext, query string, limit int) ([]uisignals.AgentReferenceSignal, error) {
	if s.search == nil {
		return nil, errors.New("search is not configured")
	}
	subject, ok := s.searchSubject(r)
	if !ok {
		return nil, errors.New("search principal is unavailable")
	}
	page, err := s.search.Search(r.Context(), subject, productsearch.Query{
		Text: strings.TrimSpace(query), Environment: string(s.requestServingEnvironment(r)), Limit: limit,
		AllowedTypes: agentReferenceTypes,
		Context: productsearch.SearchContext{
			WorkspaceID: strings.TrimSpace(context.WorkspaceID),
			DashboardID: strings.TrimSpace(context.DashboardID),
			PageID:      strings.TrimSpace(context.PageID),
		},
	})
	if err != nil {
		return nil, err
	}
	out := make([]uisignals.AgentReferenceSignal, 0, len(page.Items))
	for _, result := range page.Items {
		out = append(out, agentReferenceSignal(result))
	}
	return out, nil
}

func agentReferenceSignal(result productsearch.Result) uisignals.AgentReferenceSignal {
	locations := make([]uisignals.AgentReferenceLocationSignal, 0, len(result.Locations))
	for _, location := range result.Locations {
		locations = append(locations, uisignals.AgentReferenceLocationSignal{
			DashboardID: uisignals.Optional(location.DashboardID), DashboardName: uisignals.Optional(location.DashboardName),
			PageID: uisignals.Optional(location.PageID), PageName: uisignals.Optional(location.PageName), Href: location.Href,
		})
	}
	contextTags := make([]string, 0, len(result.Context))
	for _, tag := range result.Context {
		contextTags = append(contextTags, string(tag))
	}
	return uisignals.AgentReferenceSignal{
		Reference: uisignals.AgentReferenceKeySignal{WorkspaceID: result.Reference.WorkspaceID, Type: string(result.Reference.Type), ID: result.Reference.ID},
		Name:      result.Name, Description: uisignals.Optional(result.Description),
		VisualType: uisignals.Optional(result.VisualType),
		Workspace:  uisignals.AgentReferenceWorkspaceSignal{ID: result.Workspace.ID, Name: result.Workspace.Name},
		Hierarchy:  agentReferenceHierarchy(result),
		Href:       result.Href, Locations: locations, Context: contextTags,
	}
}

func agentReferenceHierarchy(result productsearch.Result) []string {
	hierarchy := make([]string, 0, 3)
	if name := strings.TrimSpace(result.Workspace.Name); name != "" {
		hierarchy = append(hierarchy, name)
	}
	appendName := func(name string) {
		name = strings.TrimSpace(name)
		if name != "" && (len(hierarchy) == 0 || hierarchy[len(hierarchy)-1] != name) {
			hierarchy = append(hierarchy, name)
		}
	}
	switch result.Reference.Type {
	case productsearch.TypeVisual:
		if len(result.Locations) > 0 {
			appendName(result.Locations[0].DashboardName)
			appendName(result.Locations[0].PageName)
		}
	case productsearch.TypePage:
		if len(result.Locations) > 0 {
			appendName(result.Locations[0].DashboardName)
		}
	case productsearch.TypeMeasure:
		for _, ancestor := range result.Hierarchy {
			if ancestor.Type == productsearch.TypeSemanticModel {
				appendName(ancestor.Name)
			}
		}
	}
	return hierarchy
}

func agentTurnReference(result productsearch.Result) agent.TurnReference {
	locations := make([]agent.TurnReferenceLocation, 0, len(result.Locations))
	for _, location := range result.Locations {
		locations = append(locations, agent.TurnReferenceLocation{
			DashboardID: location.DashboardID, DashboardName: location.DashboardName,
			PageID: location.PageID, PageName: location.PageName, Href: location.Href,
		})
	}
	contextTags := make([]string, 0, len(result.Context))
	for _, tag := range result.Context {
		contextTags = append(contextTags, string(tag))
	}
	reference := agent.TurnReference{
		Reference: agent.TurnReferenceKey{WorkspaceID: result.Reference.WorkspaceID, Type: string(result.Reference.Type), ID: result.Reference.ID},
		Name:      result.Name, Description: result.Description,
		VisualType: result.VisualType,
		Workspace:  agent.TurnReferenceWorkspace{ID: result.Workspace.ID, Name: result.Workspace.Name},
		Hierarchy:  agentReferenceHierarchy(result),
		Href:       result.Href, Locations: locations, Context: contextTags,
	}
	parts := strings.Split(result.Reference.ID, ".")
	switch result.Reference.Type {
	case productsearch.TypeVisual:
		reference.VisualID = lastSearchReferencePart(result.Reference.ID)
	case productsearch.TypeFilter:
		reference.FilterID = lastSearchReferencePart(result.Reference.ID)
	case productsearch.TypeSemanticModel:
		reference.ModelID = result.Reference.ID
	case productsearch.TypeSemanticTable:
		if len(parts) > 0 {
			reference.ModelID = parts[0]
		}
		reference.DatasetID = lastSearchReferencePart(result.Reference.ID)
	case productsearch.TypeField:
		if len(parts) > 0 {
			reference.ModelID = parts[0]
		}
		if len(parts) > 1 {
			reference.DatasetID = parts[len(parts)-2]
		}
		reference.FieldID = lastSearchReferencePart(result.Reference.ID)
	case productsearch.TypeMeasure:
		if len(parts) > 0 {
			reference.ModelID = parts[0]
		}
		reference.FieldID = lastSearchReferencePart(result.Reference.ID)
	}
	return reference
}

func lastSearchReferencePart(value string) string {
	if index := strings.LastIndex(value, "."); index >= 0 {
		return value[index+1:]
	}
	return value
}
