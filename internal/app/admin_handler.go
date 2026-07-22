package app

import (
	"context"
	"fmt"
	"net/http"

	"github.com/Yacobolo/leapview/internal/access"
	adminhttp "github.com/Yacobolo/leapview/internal/admin/http"
	adminstorage "github.com/Yacobolo/leapview/internal/admin/storage"
	"github.com/Yacobolo/leapview/internal/api"
	"github.com/Yacobolo/leapview/internal/dashboard"
	"github.com/Yacobolo/leapview/internal/dashboard/publication"
	"github.com/Yacobolo/leapview/internal/ui"
	uisignals "github.com/Yacobolo/leapview/internal/ui/signals"
	"github.com/Yacobolo/leapview/pkg/pagestream"
)

func (s *Server) adminHTTPHandler() adminhttp.Handler {
	return adminhttp.Handler{
		Catalog: func() dashboard.Catalog {
			return s.metrics.Catalog()
		},
		ReadModel: adminhttp.ReadModel{
			AccessRepository:     s.accessRepository,
			AgentDetails:         s.adminAgentDetails,
			StorageService:       s.storageReadModel(),
			QueryAuditRepository: s.queryAuditRepository,
			CSRFToken:            func(r *http.Request) string { return csrfToken(r, s.auth) },
			CurrentPrincipal:     s.currentAdminPrincipal,
			Publications:         s.adminPublications,
			DefaultWorkspaceID:   s.defaultWorkspaceID,
			AuthConfigured:       s.auth != nil,
			AccessConfigured:     s.store != nil,
		},
		CurrentRoleLabel:    s.currentAdminRoleLabel,
		ChromeOption:        s.chatChromeOption,
		EnsureClientID:      func(w http.ResponseWriter, r *http.Request) { _ = pagestream.EnsureClientID(w, r) },
		Broker:              s.broker,
		PublicationMutation: s.mutateAdminPublication,
	}
}

func (s *Server) mutateAdminPublication(r *http.Request, command uisignals.AdminPublicationCommand) error {
	if s.publicationService == nil {
		return publication.ErrNotFound
	}
	principal, ok := currentPrincipal(s, r)
	if !ok {
		return publication.ErrConflict
	}
	if !principal.DevBypass {
		if s.auth != nil {
			if credential, ok := s.auth.APICredential(r); ok && !apiTokenAllows(credential.Token, command.WorkspaceID, access.PrivilegeManagePublications) {
				return publication.ErrNotFound
			}
		}
		repo, err := s.accessRepository()
		if err != nil {
			return err
		}
		if repo != nil {
			decision, err := repo.Authorize(r.Context(), principal.ID, access.PrivilegeManagePublications, access.WorkspaceObject(command.WorkspaceID))
			if err != nil {
				return err
			}
			if !decision.Allowed {
				return publication.ErrNotFound
			}
		}
	}
	_, err := s.publicationService.Mutate(r.Context(), command.WorkspaceID, command.Publication, principal.ID, publication.Action(command.Action))
	return err
}

func (s *Server) adminPublications(r *http.Request) ([]ui.AdminPublication, bool, error) {
	if s.publicationRepo == nil {
		return nil, false, nil
	}
	principal, ok := currentPrincipal(s, r)
	if !ok {
		return nil, false, nil
	}
	rows, err := s.publicationRepo.ListAll(r.Context())
	if err != nil {
		return nil, false, err
	}
	accessRepo, err := s.accessRepository()
	if err != nil {
		return nil, false, err
	}
	var credential *access.APICredential
	if s.auth != nil {
		if resolved, ok := s.auth.APICredential(r); ok {
			credential = &resolved
		}
	}
	canManage := principal.DevBypass || accessRepo == nil
	if !canManage {
		canManage, err = s.authorizeAnyWorkspacePrivilege(r.Context(), principal.ID, credential, access.PrivilegeManagePublications)
		if err != nil {
			return nil, false, err
		}
	}
	out := make([]ui.AdminPublication, 0, len(rows))
	for _, row := range rows {
		allowed := principal.DevBypass || accessRepo == nil
		if !allowed {
			if credential != nil && !apiTokenAllows(credential.Token, row.WorkspaceID, access.PrivilegeManagePublications) {
				continue
			}
			decision, err := accessRepo.Authorize(r.Context(), principal.ID, access.PrivilegeManagePublications, access.WorkspaceObject(row.WorkspaceID))
			if err != nil {
				return nil, false, err
			}
			allowed = decision.Allowed
		}
		if !allowed {
			continue
		}
		dto := s.dashboardPublicationDTO(row)
		events, err := s.publicationRepo.ListEvents(r.Context(), row.ID)
		if err != nil {
			return nil, false, err
		}
		history := make([]string, 0, len(events))
		for _, event := range events {
			actor := event.ActorID
			if actor == "" {
				actor = "system"
			}
			history = append(history, fmt.Sprintf("%s · %s · %s", event.CreatedAt, event.Type, actor))
		}
		out = append(out, ui.AdminPublication{
			WorkspaceID: row.WorkspaceID, Name: row.Name, Dashboard: row.Dashboard, DefaultPage: row.DefaultPage,
			Status: string(row.Status()), Origins: append([]string(nil), row.AllowedOrigins...), Generation: row.ServingStateID,
			PublicURL: dto.PublicUrl, EmbedURL: dto.EmbedUrl, IFrameSnippet: dto.IframeSnippet,
			ConfiguredAt: row.ConfiguredAt, SuspendedAt: row.SuspendedAt, DisabledAt: row.DisabledAt, RotatedAt: row.RotatedAt,
			History: history,
		})
	}
	return out, canManage, nil
}

func (s *Server) storageReadModel() adminstorage.Service {
	service := adminstorage.Service{
		CatalogPath: s.duckLakeCatalogPath,
		DataPath:    s.duckLakeDataPath,
		Environment: s.defaultEnvironment,
	}
	if s.store != nil {
		service.ControlPlane = s.store
	}
	if s.duckDBEnvironment != nil {
		service.Analytics = s.duckDBEnvironment
	}
	service.Admitter = s.workloadController()
	return service
}

func (s *Server) adminAgentDetails(ctx context.Context) (api.AdminAgentResponse, error) {
	return s.agentHTTPHandler().AdminDetails(ctx)
}

func (s *Server) currentAdminPrincipal(r *http.Request) (adminhttp.Principal, bool) {
	if s.auth == nil {
		principal := localDeveloperPrincipal()
		return adminhttp.Principal{
			ID:          principal.ID,
			Email:       principal.Email,
			DisplayName: principal.DisplayName,
			DevBypass:   principal.DevBypass,
		}, true
	}
	principal, ok := s.auth.Principal(r)
	if !ok {
		return adminhttp.Principal{}, false
	}
	return adminhttp.Principal{
		ID:          principal.ID,
		Email:       principal.Email,
		DisplayName: principal.DisplayName,
		DevBypass:   principal.DevBypass,
	}, true
}
