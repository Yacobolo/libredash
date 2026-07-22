package app

import (
	"net/http"

	accessdomain "github.com/Yacobolo/leapview/internal/access"
	accesshttp "github.com/Yacobolo/leapview/internal/access/http"
)

func (s *Server) accessHTTPHandler() accesshttp.Handler {
	return accesshttp.Handler{
		Repository:           s.accessRepository,
		QueryAuditRepository: s.queryAuditRepository,
		CurrentPrincipal: func(r *http.Request) (accesshttp.Principal, bool) {
			if s.auth == nil {
				principal := localDeveloperPrincipal()
				return accesshttp.Principal{ID: principal.ID, Email: principal.Email, DisplayName: principal.DisplayName}, true
			}
			principal, ok := s.auth.Principal(r)
			if !ok {
				return accesshttp.Principal{}, false
			}
			return accesshttp.Principal{ID: principal.ID, Email: principal.Email, DisplayName: principal.DisplayName}, true
		},
		CurrentCredential: func(r *http.Request) (accessdomain.APICredential, bool) {
			if s.auth == nil {
				return accessdomain.APICredential{}, false
			}
			return s.auth.APICredential(r)
		},
		WorkspaceID: s.workspaceID,
	}
}
