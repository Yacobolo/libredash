package app

import (
	"context"
	"net/http"

	adminhttp "github.com/Yacobolo/libredash/internal/admin/http"
	adminstorage "github.com/Yacobolo/libredash/internal/admin/storage"
	"github.com/Yacobolo/libredash/internal/api"
	"github.com/Yacobolo/libredash/internal/dashboard"
	lddatastar "github.com/Yacobolo/libredash/internal/dashboard/datastar"
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
			DefaultWorkspaceID:   s.defaultWorkspaceID,
			AuthConfigured:       s.auth != nil,
			AccessConfigured:     s.store != nil,
		},
		CurrentRoleLabel: s.currentAdminRoleLabel,
		ChromeOption:     s.chatChromeOption,
		EnsureClientID:   func(w http.ResponseWriter, r *http.Request) { _ = lddatastar.EnsureClientID(w, r) },
		Broker:           s.broker,
	}
}

func (s *Server) storageReadModel() adminstorage.Service {
	return adminstorage.Service{
		CatalogPath: s.duckLakeCatalogPath,
		DataPath:    s.duckLakeDataPath,
	}
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
