package app

import (
	"context"
	"net/http"

	"github.com/Yacobolo/leapview/internal/manageddata/control"
	manageddatahttp "github.com/Yacobolo/leapview/internal/manageddata/http"
)

func (s *Server) managedDataHTTPHandler() *manageddatahttp.Handler {
	options := s.managedDataOptions
	options.Environment = s.defaultEnvironment
	options.EnqueueFinalize = func(ctx context.Context, request control.UploadRequest) error {
		if err := s.appendAsyncEvent(ctx, "upload", request.UploadID, "upload_session.finalizing", map[string]any{"uploadSessionId": request.UploadID, "status": "finalizing"}); err != nil {
			return err
		}
		return s.enqueueAsyncJobPayload(ctx, "upload:"+request.UploadID+":finalize", apiJobUploadFinalize, "upload", request.UploadID, uploadFinalizeJob{Project: request.Project, Connection: request.Connection, UploadSession: request.UploadID})
	}
	options.RecordUploadCreated = func(ctx context.Context, result control.UploadResult) error {
		return s.appendAsyncEvent(ctx, "upload", result.ID, "upload_session.created", map[string]any{"uploadSessionId": result.ID, "projectId": result.Collection.Project, "connectionId": result.Collection.Connection, "status": result.Status})
	}
	options.CurrentPrincipal = func(r *http.Request) (manageddatahttp.Principal, bool) {
		if s.auth == nil {
			return manageddatahttp.Principal{}, false
		}
		principal, ok := s.auth.Principal(r)
		return manageddatahttp.Principal{ID: principal.ID}, ok
	}
	return manageddatahttp.NewHandler(options)
}
