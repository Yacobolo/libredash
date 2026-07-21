package app

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/Yacobolo/leapview/internal/access"
	apigenapi "github.com/Yacobolo/leapview/internal/api/gen"
	productsearch "github.com/Yacobolo/leapview/internal/search"
)

type appSearchAuthorizer struct{ server *Server }

func (a appSearchAuthorizer) CanView(ctx context.Context, subject productsearch.Subject, object access.ObjectRef) (bool, error) {
	if subject.CredentialRestricted && !containsSearchPrivilege(subject.Privileges, access.PrivilegeViewItem) {
		return false, nil
	}
	if subject.DevBypass {
		return true, nil
	}
	if subject.Restricted {
		allowedWorkspace := false
		for _, workspaceID := range subject.WorkspaceIDs {
			if strings.TrimSpace(workspaceID) == object.WorkspaceID {
				allowedWorkspace = true
				break
			}
		}
		if !allowedWorkspace {
			return false, nil
		}
	}
	repository, err := a.server.accessRepository()
	if err != nil {
		return false, err
	}
	decision, err := repository.Authorize(ctx, subject.ID, access.PrivilegeViewItem, object)
	return err == nil && decision.Allowed, err
}

func (s *Server) searchSubject(r *http.Request) (productsearch.Subject, bool) {
	principal, ok := currentPrincipal(s, r)
	if !ok {
		return productsearch.Subject{}, false
	}
	subject := productsearch.Subject{ID: principal.ID, DevBypass: principal.DevBypass}
	if credential, ok := apiCredentialFromContext(r.Context()); ok {
		subject.ID = credential.Principal.ID
		subject.DevBypass = false
		subject.CredentialRestricted = credential.Token.Privileges != nil
		if credential.Token.ID != "" {
			subject.CredentialID = "token:" + credential.Token.ID
		}
		for _, privilege := range credential.Token.Privileges {
			subject.Privileges = append(subject.Privileges, string(privilege))
		}
		if workspaceID := strings.TrimSpace(credential.Token.WorkspaceID); workspaceID != "" {
			subject.Restricted = true
			subject.WorkspaceIDs = []string{workspaceID}
		}
	}
	return subject, true
}

func containsSearchPrivilege(privileges []string, wanted access.Privilege) bool {
	for _, privilege := range privileges {
		if strings.EqualFold(strings.TrimSpace(privilege), string(wanted)) {
			return true
		}
	}
	return false
}

func (s *Server) searchAPI(w http.ResponseWriter, r *http.Request, params apigenapi.GenSearchParams) {
	if s.search == nil {
		writeJSONError(w, errors.New("search is not configured"), http.StatusServiceUnavailable)
		return
	}
	subject, ok := s.searchSubject(r)
	if !ok {
		writeJSONError(w, errors.New("search principal is unavailable"), http.StatusUnauthorized)
		return
	}
	query := productsearch.Query{Environment: string(s.requestServingEnvironment(r))}
	if params.Q != nil {
		query.Text = *params.Q
	}
	if params.Workspace != nil {
		query.Workspaces = append([]string(nil), (*params.Workspace)...)
	}
	if params.Type != nil {
		query.Types = make([]productsearch.Type, 0, len(*params.Type))
		for _, typ := range *params.Type {
			query.Types = append(query.Types, productsearch.Type(typ))
		}
	}
	if params.ContextWorkspace != nil {
		query.Context.WorkspaceID = *params.ContextWorkspace
	}
	if params.ContextDashboard != nil {
		query.Context.DashboardID = *params.ContextDashboard
	}
	if params.ContextPage != nil {
		query.Context.PageID = *params.ContextPage
	}
	if params.Limit != nil {
		query.Limit = int(*params.Limit)
	}
	if params.PageToken != nil {
		query.Cursor = *params.PageToken
	}
	page, err := s.search.Search(r.Context(), subject, query)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, productsearch.ErrInvalidCursor) {
			status = http.StatusBadRequest
		} else if errors.Is(err, productsearch.ErrSnapshotChanged) {
			status = http.StatusConflict
		} else if strings.Contains(err.Error(), "unknown search type") || strings.Contains(err.Error(), "search limit") {
			status = http.StatusBadRequest
		}
		writeJSONError(w, err, status)
		return
	}
	writeJSON(w, http.StatusOK, apigenapi.SearchResponse{Items: searchAPIResults(page.Items), Page: apigenapi.PageInfo{NextCursor: stringPointer(page.NextCursor)}})
}

func searchAPIResults(items []productsearch.Result) []apigenapi.SearchResult {
	out := make([]apigenapi.SearchResult, 0, len(items))
	for _, item := range items {
		locations := make([]apigenapi.SearchLocation, 0, len(item.Locations))
		for _, location := range item.Locations {
			locations = append(locations, apigenapi.SearchLocation{
				DashboardId: optionalString(location.DashboardID), DashboardName: optionalString(location.DashboardName),
				PageId: optionalString(location.PageID), PageName: optionalString(location.PageName), Href: location.Href,
			})
		}
		contextTags := make([]apigenapi.SearchContextTag, 0, len(item.Context))
		for _, tag := range item.Context {
			contextTags = append(contextTags, apigenapi.SearchContextTag(tag))
		}
		out = append(out, apigenapi.SearchResult{
			Reference: apigenapi.SearchReference{WorkspaceId: item.Reference.WorkspaceID, Type: apigenapi.SearchResultType(item.Reference.Type), Id: item.Reference.ID},
			Name:      item.Name, Description: optionalString(item.Description),
			VisualType: optionalString(item.VisualType),
			Workspace:  apigenapi.SearchWorkspaceSummary{Id: item.Workspace.ID, Name: item.Workspace.Name},
			Href:       item.Href, Locations: locations, Context: contextTags,
		})
	}
	return out
}

func optionalString(value string) *string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return &value
}

func stringPointer(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
