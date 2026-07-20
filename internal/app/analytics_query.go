package app

import (
	"context"
	"net/http"

	"github.com/Yacobolo/leapview/internal/access"
	queryhttp "github.com/Yacobolo/leapview/internal/analytics/query/http"
)

func (s *Server) semanticQueryHTTP() queryhttp.Handler {
	return queryhttp.Handler{
		Metrics: s.metrics,
		MetricsForWorkspace: func(workspaceID string) (queryhttp.Metrics, bool) {
			return s.metricsForWorkspace(workspaceID)
		},
		CurrentPrincipalID: func(r *http.Request) string {
			principal, ok := principalFromContext(r.Context())
			if !ok {
				return ""
			}
			return principal.ID
		},
		AuthorizeListObject: func(ctx context.Context, principalID string, object access.ObjectRef) (bool, error) {
			return s.authorizeListObject(ctx, principalID, object)
		},
	}
}
