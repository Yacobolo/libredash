package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/Yacobolo/libredash/internal/api"
	"github.com/Yacobolo/libredash/internal/deployment"
	"github.com/Yacobolo/libredash/internal/deployment/activate"
	deploymentfs "github.com/Yacobolo/libredash/internal/deployment/filesystem"
	deploymenthttp "github.com/Yacobolo/libredash/internal/deployment/http"
	deploymentsqlite "github.com/Yacobolo/libredash/internal/deployment/sqlite"
	"github.com/Yacobolo/libredash/internal/deployment/validate"
)

type runtimeReloader interface {
	Reload(ctx context.Context) error
	PrepareDeployment(ctx context.Context, deploymentID string) (deployment.PreparedRuntime, error)
	CommitPrepared(prepared deployment.PreparedRuntime) error
}

type deploymentRepository interface {
	validate.Repository
	activate.Repository
	deploymentfs.ArtifactRepository
	ActiveArtifact(ctx context.Context, workspaceID deployment.WorkspaceID, environment deployment.Environment) (deployment.Deployment, deployment.Artifact, error)
	Create(ctx context.Context, input deployment.CreateInput) (deployment.Deployment, error)
	List(ctx context.Context, workspaceID deployment.WorkspaceID, environment deployment.Environment) ([]deployment.Deployment, error)
}

func (s *Server) deploymentHTTPHandler() *deploymenthttp.Handler {
	return deploymenthttp.NewHandler(deploymenthttp.Options{
		Repository: func() (deploymenthttp.Repository, error) {
			return s.deploymentRepository()
		},
		WorkspaceRepository: s.workspaceRepository,
		AccessRepository:    s.accessRepository,
		Runtime:             s.reloader,
		CurrentPrincipal: func(r *http.Request) (deploymenthttp.Principal, bool) {
			if s.auth == nil {
				return deploymenthttp.Principal{}, false
			}
			principal, ok := s.auth.Principal(r)
			return deploymenthttp.Principal{ID: principal.ID}, ok
		},
		ArtifactDir: s.artifactDir,
		DataDir: func() string {
			if s.metrics == nil {
				return ""
			}
			return s.metrics.DataDir()
		},
		DefaultEnvironment: s.defaultEnvironment,
		WorkspaceID:        s.workspaceID,
	})
}

func (s *Server) deploymentRepository() (deploymentRepository, error) {
	if s.deploymentRepo != nil {
		return s.deploymentRepo, nil
	}
	if s.store == nil {
		return nil, fmt.Errorf("deployment repository is not configured")
	}
	s.deploymentRepo = deploymentsqlite.NewRepository(s.store.SQLDB())
	return s.deploymentRepo, nil
}

func (s *Server) workspaceID(candidate string) string {
	return candidate
}

func requestDeploymentEnvironment(r *http.Request, fallback string) deployment.Environment {
	if query := r.URL.Query().Get("environment"); query != "" {
		fallback = query
	}
	return deployment.NormalizeEnvironment(deployment.Environment(fallback))
}

func (s *Server) defaultDeploymentEnvironment() deployment.Environment {
	return deployment.NormalizeEnvironment(deployment.Environment(s.defaultEnvironment))
}

func (s *Server) requestDeploymentEnvironment(r *http.Request) deployment.Environment {
	return requestDeploymentEnvironment(r, string(s.defaultDeploymentEnvironment()))
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeJSONError(w http.ResponseWriter, err error, status int) {
	writeJSON(w, status, api.ErrorResponse{
		Code:      status,
		Message:   err.Error(),
		Details:   map[string]any{},
		RequestID: "",
	})
}

func decodeOptionalJSONBody(r *http.Request, dst any) error {
	if r.Body == nil || r.Body == http.NoBody {
		return nil
	}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		return fmt.Errorf("malformed JSON: %w", err)
	}
	var extra struct{}
	if err := decoder.Decode(&extra); err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		return fmt.Errorf("malformed JSON: %w", err)
	}
	return fmt.Errorf("malformed JSON: multiple JSON values")
}

func statusForNotFound(err error) int {
	if errors.Is(err, deployment.ErrNotFound) {
		return http.StatusNotFound
	}
	return http.StatusInternalServerError
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
