package app

import (
	"net/http"

	adminhttp "github.com/Yacobolo/libredash/internal/admin/http"
	adminstorage "github.com/Yacobolo/libredash/internal/admin/storage"
	"github.com/Yacobolo/libredash/internal/dashboard"
	lddatastar "github.com/Yacobolo/libredash/internal/dashboard/datastar"
)

func (s *Server) adminHTTPHandler() adminhttp.Handler {
	return adminhttp.Handler{
		Catalog: func() dashboard.Catalog {
			return s.metrics.Catalog()
		},
		Data:                 s.adminData,
		CurrentRoleLabel:     s.currentAdminRoleLabel,
		ChromeOption:         s.chatChromeOption,
		EnsureClientID:       func(w http.ResponseWriter, r *http.Request) { _ = lddatastar.EnsureClientID(w, r) },
		Broker:               s.broker,
		StorageService:       s.storageReadModel(),
		QueryAuditRepository: s.queryAuditRepository,
		PrincipalLabels:      s.adminPrincipalLabels,
	}
}

func (s *Server) storageReadModel() adminstorage.Service {
	return adminstorage.Service{
		CatalogPath: s.duckLakeCatalogPath,
		DataPath:    s.duckLakeDataPath,
	}
}

func (s *Server) adminPrincipalLabels(r *http.Request, values []string) map[string]string {
	labels := map[string]string{}
	var current Principal
	var hasCurrent bool
	if s.auth != nil {
		current, hasCurrent = s.auth.Principal(r)
	}
	for _, value := range values {
		if value == "" {
			continue
		}
		if hasCurrent && value == current.ID {
			identity := firstNonEmpty(current.Email, current.DisplayName, current.ID)
			labels[value] = "Me (" + identity + ")"
			continue
		}
		labels[value] = value
	}
	return labels
}
