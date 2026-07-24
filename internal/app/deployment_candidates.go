package app

import (
	"context"
	"fmt"
	"net/http"

	refreshmodule "github.com/Yacobolo/leapview/internal/refresh/module"
	servingstatemodule "github.com/Yacobolo/leapview/internal/servingstate/module"
)

type runtimeReloader interface {
	PrepareServingState(ctx context.Context, servingStateID string) (servingstatemodule.PreparedRuntime, error)
	ActivatePrepared(prepared servingstatemodule.PreparedRuntime, activate func() error) error
}

type servingStateRepository interface {
	refreshmodule.ServingStateRepository
	ListActiveScopes(context.Context) ([]servingstatemodule.ActiveScope, error)
}

func resolveServingStateRepository(routes *capabilityRoutes, runtime *runtimeServices, platform *platformServices, policy *httpPolicy, persistence persistenceInputs) (servingStateRepository, error) {
	if persistence.servingStateRepo != nil {
		return persistence.servingStateRepo, nil
	}
	return nil, fmt.Errorf("serving state repository is not configured")
}

func workspaceID(routes *capabilityRoutes, runtime *runtimeServices, platform *platformServices, policy *httpPolicy, value string) string {
	return value
}

func defaultServingEnvironment(routes *capabilityRoutes, runtime *runtimeServices, platform *platformServices, policy *httpPolicy) servingstatemodule.Environment {
	return servingstatemodule.NormalizeEnvironment(servingstatemodule.Environment(policy.defaultEnvironment))
}

func requestServingEnvironment(routes *capabilityRoutes, runtime *runtimeServices, platform *platformServices, policy *httpPolicy, r *http.Request) servingstatemodule.Environment {
	return defaultServingEnvironment(routes, runtime, platform, policy)
}
