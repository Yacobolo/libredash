package app

import (
	"context"
	"database/sql"
	"net/http"
	"strings"

	accessmodule "github.com/Yacobolo/leapview/internal/access/module"
	apiprotocol "github.com/Yacobolo/leapview/internal/api/protocol"
)

func configureAPIProtocol(routes *capabilityRoutes, runtime *runtimeServices, platform *platformServices, policy *httpPolicy, ctx context.Context, database *sql.DB) error {
	if ctx == nil {
		ctx = context.Background()
	}
	protocol, err := apiprotocol.Build(ctx, apiprotocol.Config{
		Database:    database,
		BearerToken: accessmodule.BearerToken,
		AcceptsBearer: func(r *http.Request) bool {
			return platform.auth == nil || platform.auth.AcceptsPublicBearer(r)
		},
		PrincipalID: func(r *http.Request) (string, bool) {
			if platform.auth == nil {
				return "", false
			}
			principal, _, ok := platform.auth.Authenticate(r)
			return principal.ID, ok
		},
		CursorSnapshot: func(r *http.Request) string {
			return cursorSnapshot(routes, runtime, platform, policy, r)
		},
	})
	if err != nil {
		return err
	}
	platform.apiProtocol = protocol
	return nil
}

func publicProtocolMiddleware(routes *capabilityRoutes, runtime *runtimeServices, platform *platformServices, policy *httpPolicy, next http.Handler) http.Handler {
	return platform.apiProtocol.Middleware(next)
}

func openAPIDescription(routes *capabilityRoutes, runtime *runtimeServices, platform *platformServices, policy *httpPolicy, w http.ResponseWriter, r *http.Request) {
	platform.apiProtocol.OpenAPIDescription(w, r)
}

func publicDocs(routes *capabilityRoutes, runtime *runtimeServices, platform *platformServices, policy *httpPolicy, w http.ResponseWriter, r *http.Request) {
	platform.apiProtocol.PublicDocs(w, r)
}

func cursorSnapshot(routes *capabilityRoutes, runtime *runtimeServices, platform *platformServices, policy *httpPolicy, r *http.Request) string {
	segments := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	for index, segment := range segments {
		if index+1 >= len(segments) {
			continue
		}
		switch segment {
		case "workspaces":
			if routes.workspaceModule != nil {
				snapshot, err := routes.workspaceModule.ActiveServingStateID(r.Context(), workspaceID(routes, runtime, platform, policy, segments[index+1]))
				if err == nil && snapshot != "" {
					return snapshot
				}
			}
		case "projects":
			if snapshot := routes.releaseModule.ProjectCursorSnapshot(r, segments[index+1]); snapshot != "" {
				return snapshot
			}
		}
	}
	return ""
}
