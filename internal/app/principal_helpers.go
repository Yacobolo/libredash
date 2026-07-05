package app

import (
	"net/http"
)

func currentPrincipal(s *Server, r *http.Request) (Principal, bool) {
	if s.auth == nil {
		return localDeveloperPrincipal(), true
	}
	return s.auth.Principal(r)
}
