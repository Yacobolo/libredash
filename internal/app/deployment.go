package app

import (
	"net/http"

	deploymenthttp "github.com/Yacobolo/libredash/internal/deployment/http"
)

func (s *Server) deploymentHTTPHandler() *deploymenthttp.Handler {
	options := s.deploymentOptions
	options.Logger = s.logger
	options.InstanceEnvironment = s.defaultEnvironment
	options.CurrentPrincipal = func(r *http.Request) (deploymenthttp.Principal, bool) {
		if s.auth == nil {
			return deploymenthttp.Principal{}, false
		}
		principal, ok := s.auth.Principal(r)
		return deploymenthttp.Principal{ID: principal.ID}, ok
	}
	return deploymenthttp.NewHandler(options)
}
