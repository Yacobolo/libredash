package app

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"

	accessmodule "github.com/Yacobolo/leapview/internal/access/module"
	refreshmodule "github.com/Yacobolo/leapview/internal/refresh/module"
)

func configureRefreshModule(routes *capabilityRoutes, runtime *runtimeServices, platform *platformServices, policy *httpPolicy, ctx context.Context, database *sql.DB, persistence persistenceInputs, workflow workflowInputs, storage storageInputs) error {
	if routes == nil || routes.refreshModule != nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	service, err := workspaceRefreshService(routes, runtime, platform, policy, persistence, workflow)
	if err != nil && database != nil {
		return fmt.Errorf("configure refresh service: %w", err)
	}
	config := refreshmodule.Config{
		Database: database, Service: service,
		Analytics: runtime.analyticsModule.WorkspaceMaterializer(), ManagedData: workflow.managedDataResolver,
		HTTP: refreshmodule.HTTPConfig{
			RunnerConfigured: func() bool { return runtime.metrics != nil },
			CurrentPrincipal: func(r *http.Request) (refreshmodule.HTTPPrincipal, bool) {
				principal, ok := routes.accessModule.CurrentPrincipal(r)
				return refreshmodule.HTTPPrincipal{ID: principal.ID}, ok
			},
			WorkspaceID: func(value string) string {
				return workspaceID(routes, runtime, platform, policy, value)
			},
			Environment: func(*http.Request) string {
				return string(defaultServingEnvironment(routes, runtime, platform, policy))
			},
		},
		Authorization: refreshmodule.AuthorizationConfig{
			CurrentPrincipal: func(r *http.Request) (refreshmodule.AuthorizationPrincipal, bool) {
				principal, ok := routes.accessModule.CurrentPrincipal(r)
				return refreshmodule.AuthorizationPrincipal{ID: principal.ID, DevBypass: principal.DevBypass}, ok
			},
			CurrentCredential: func(r *http.Request) (accessmodule.APICredential, bool) {
				return accessmodule.APICredentialFromContext(r.Context())
			},
			ResolvePipelineModel: refreshmodule.PipelineModelResolver(
				persistence.servingStateRepo,
				nil,
				defaultServingEnvironment(routes, runtime, platform, policy),
			),
			AuthorizeObject: routes.accessModule.AuthorizeObject,
		},
		ApplyAccessSnapshot: accessmodule.ApplySnapshot,
		Admission:           workloadController(routes, runtime, platform, policy), LeaseTimeout: storage.jobLeaseTimeout,
		Environment: string(defaultServingEnvironment(routes, runtime, platform, policy)), Clock: workflow.refreshPipelineClock,
		EnableDispatcher: database != nil && runtime.metrics != nil,
		EnableScheduler:  database != nil && persistence.servingStateRepo != nil,
		Logger:           platform.logger, Events: platform.asyncJobs,
		WorkloadStats: func() refreshmodule.WorkloadStats {
			return workloadController(routes, runtime, platform, policy).Stats()
		},
		RunFinished: func(ctx context.Context, run refreshmodule.RunRecord) {
			if run.Status == refreshmodule.RunStatusSucceeded && runtime.storageRetention != nil {
				_ = runtime.storageRetention.Run(ctx, false)
			}
		},
	}
	module, err := refreshmodule.Build(ctx, config)
	if err != nil {
		return fmt.Errorf("build refresh module: %w", err)
	}
	routes.refreshModule = module
	return nil
}
