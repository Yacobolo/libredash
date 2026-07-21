package app

import (
	"errors"
	"html"
	"net/http"
	"strings"

	apigenapi "github.com/Yacobolo/leapview/internal/api/gen"
	"github.com/Yacobolo/leapview/internal/dashboard/publication"
)

func (a apiGenAdapter) ListDashboardPublications(w http.ResponseWriter, r *http.Request, workspaceID string) {
	if a.server.publicationRepo == nil {
		writeAPIProblem(w, r, http.StatusNotFound, "PUBLICATIONS_NOT_AVAILABLE", "Dashboard publications are not available", nil)
		return
	}
	rows, err := a.server.publicationRepo.List(r.Context(), workspaceID)
	if err != nil {
		writeAPIProblem(w, r, http.StatusInternalServerError, "PUBLICATION_LIST_FAILED", "Dashboard publications could not be loaded", nil)
		return
	}
	items := make([]apigenapi.DashboardPublicationResponse, 0, len(rows))
	for _, row := range rows {
		items = append(items, a.server.dashboardPublicationDTO(row))
	}
	writeJSON(w, http.StatusOK, apigenapi.DashboardPublicationListResponse{Items: items})
}

func (a apiGenAdapter) GetDashboardPublication(w http.ResponseWriter, r *http.Request, workspaceID, name string) {
	row, ok := a.dashboardPublication(w, r, workspaceID, name)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, a.server.dashboardPublicationDTO(row))
}

func (a apiGenAdapter) SuspendDashboardPublication(w http.ResponseWriter, r *http.Request, workspaceID, name string, _ apigenapi.GenSuspendDashboardPublicationHeaders) {
	if !a.publicationMutationsAvailable(w, r) {
		return
	}
	actor := publicationActor(r)
	row, err := a.server.publicationService.Mutate(r.Context(), workspaceID, name, actor, publication.ActionSuspend)
	a.writePublicationMutation(w, r, row, err)
}

func (a apiGenAdapter) ResumeDashboardPublication(w http.ResponseWriter, r *http.Request, workspaceID, name string, _ apigenapi.GenResumeDashboardPublicationHeaders) {
	if !a.publicationMutationsAvailable(w, r) {
		return
	}
	row, err := a.server.publicationService.Mutate(r.Context(), workspaceID, name, publicationActor(r), publication.ActionResume)
	a.writePublicationMutation(w, r, row, err)
}

func (a apiGenAdapter) RotateDashboardPublication(w http.ResponseWriter, r *http.Request, workspaceID, name string, _ apigenapi.GenRotateDashboardPublicationHeaders) {
	if !a.publicationMutationsAvailable(w, r) {
		return
	}
	row, err := a.server.publicationService.Mutate(r.Context(), workspaceID, name, publicationActor(r), publication.ActionRotate)
	a.writePublicationMutation(w, r, row, err)
}

func (a apiGenAdapter) publicationMutationsAvailable(w http.ResponseWriter, r *http.Request) bool {
	if a.server.publicationService != nil {
		return true
	}
	writeAPIProblem(w, r, http.StatusNotFound, "PUBLICATION_NOT_FOUND", "Dashboard publication not found", nil)
	return false
}

func (a apiGenAdapter) dashboardPublication(w http.ResponseWriter, r *http.Request, workspaceID, name string) (publication.Publication, bool) {
	if a.server.publicationRepo == nil {
		writeAPIProblem(w, r, http.StatusNotFound, "PUBLICATION_NOT_FOUND", "Dashboard publication not found", nil)
		return publication.Publication{}, false
	}
	row, err := a.server.publicationRepo.Get(r.Context(), workspaceID, name)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, publication.ErrNotFound) {
			status = http.StatusNotFound
		}
		writeAPIProblem(w, r, status, "PUBLICATION_NOT_FOUND", "Dashboard publication not found", nil)
		return publication.Publication{}, false
	}
	return row, true
}

func (a apiGenAdapter) writePublicationMutation(w http.ResponseWriter, r *http.Request, row publication.Publication, err error) bool {
	if err != nil {
		status := http.StatusInternalServerError
		code := "PUBLICATION_MUTATION_FAILED"
		detail := "Dashboard publication could not be updated"
		switch {
		case errors.Is(err, publication.ErrNotFound):
			status, code, detail = http.StatusNotFound, "PUBLICATION_NOT_FOUND", "Dashboard publication not found"
		case errors.Is(err, publication.ErrConflict):
			status, code, detail = http.StatusConflict, "PUBLICATION_NOT_CONFIGURED", "Dashboard publication is not present in the active configuration"
		}
		writeAPIProblem(w, r, status, code, detail, nil)
		return false
	}
	writeJSON(w, http.StatusOK, a.server.dashboardPublicationDTO(row))
	return true
}

func (s *Server) dashboardPublicationDTO(row publication.Publication) apigenapi.DashboardPublicationResponse {
	publicPath := "/public/dashboards/" + row.PublicID
	embedPath := "/embed/dashboards/" + row.PublicID
	publicURL := s.absolutePublicURL(publicPath)
	embedURL := s.absolutePublicURL(embedPath)
	iframe := `<iframe src="` + html.EscapeString(embedURL) + `" title="` + html.EscapeString(row.Name) + `" loading="lazy" sandbox="allow-scripts allow-same-origin" referrerpolicy="no-referrer"></iframe>`
	dto := apigenapi.DashboardPublicationResponse{
		Name: row.Name, WorkspaceId: row.WorkspaceID, ProjectId: row.ProjectID, Dashboard: row.Dashboard,
		DefaultPage: row.DefaultPage, Status: apigenapi.DashboardPublicationStatus(row.Status()), Configured: row.Configured,
		AllowedOrigins: append([]string(nil), row.AllowedOrigins...), PublicUrl: publicURL, EmbedUrl: embedURL, IframeSnippet: iframe,
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
	optionalString := func(value string) *string {
		if strings.TrimSpace(value) == "" {
			return nil
		}
		copy := value
		return &copy
	}
	dto.ActiveServingStateId = optionalString(row.ServingStateID)
	dto.ConfiguredAt = optionalString(row.ConfiguredAt)
	dto.DisabledAt = optionalString(row.DisabledAt)
	dto.SuspendedAt = optionalString(row.SuspendedAt)
	dto.SuspendedBy = optionalString(row.SuspendedBy)
	dto.RotatedAt = optionalString(row.RotatedAt)
	return dto
}

func (s *Server) absolutePublicURL(path string) string {
	if s.publicURL == "" {
		return path
	}
	return s.publicURL + path
}

func publicationActor(r *http.Request) string {
	principal, ok := principalFromContext(r.Context())
	if !ok {
		return ""
	}
	return principal.ID
}
