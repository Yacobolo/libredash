package app

import (
	"net/http"

	manageddatahttp "github.com/Yacobolo/libredash/internal/manageddata/http"
)

func (s *Server) managedDataHTTPHandler() *manageddatahttp.Handler {
	options := s.managedDataOptions
	options.CurrentPrincipal = func(r *http.Request) (manageddatahttp.Principal, bool) {
		if s.auth == nil {
			return manageddatahttp.Principal{}, false
		}
		principal, ok := s.auth.Principal(r)
		return manageddatahttp.Principal{ID: principal.ID}, ok
	}
	return manageddatahttp.NewHandler(options)
}
