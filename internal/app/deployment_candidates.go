package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/Yacobolo/libredash/internal/api"
	manageddatabinding "github.com/Yacobolo/libredash/internal/manageddata/binding"
	servingstate "github.com/Yacobolo/libredash/internal/servingstate"
	servingstatehttp "github.com/Yacobolo/libredash/internal/servingstate/http"
	workspacerefresh "github.com/Yacobolo/libredash/internal/workspace/refresh"
)

type runtimeReloader interface {
	Reload(ctx context.Context) error
	PrepareServingState(ctx context.Context, servingStateID string) (servingstate.PreparedRuntime, error)
	CommitPrepared(prepared servingstate.PreparedRuntime) error
}

type servingStateRepository interface {
	servingstatehttp.Repository
	workspacerefresh.ServingStateRepository
}

func (s *Server) deploymentCandidateHTTPHandler() *servingstatehttp.Handler {
	return servingstatehttp.NewHandler(servingstatehttp.Options{
		Repository: func() (servingstatehttp.Repository, error) {
			return s.servingStateRepository()
		},
		BindingRepository: func() (manageddatabinding.Repository, error) {
			if s.managedDataBindingRepo == nil {
				return nil, fmt.Errorf("managed data binding repository is not configured")
			}
			return s.managedDataBindingRepo, nil
		},
		WorkspaceRepository: s.workspaceRepository,
		CurrentPrincipal: func(r *http.Request) (servingstatehttp.Principal, bool) {
			if s.auth == nil {
				return servingstatehttp.Principal{}, false
			}
			principal, ok := s.auth.Principal(r)
			return servingstatehttp.Principal{ID: principal.ID}, ok
		},
		ArtifactDir:        s.artifactDir,
		DefaultEnvironment: s.defaultEnvironment,
		WorkspaceID:        s.workspaceID,
	})
}

func (s *Server) servingStateRepository() (servingStateRepository, error) {
	if s.servingStateRepo != nil {
		return s.servingStateRepo, nil
	}
	return nil, fmt.Errorf("serving state repository is not configured")
}

func (s *Server) workspaceID(candidate string) string {
	return candidate
}

func (s *Server) defaultServingEnvironment() servingstate.Environment {
	return servingstate.NormalizeEnvironment(servingstate.Environment(s.defaultEnvironment))
}

func (s *Server) requestServingEnvironment(r *http.Request) servingstate.Environment {
	if query := r.URL.Query().Get("environment"); query != "" {
		return servingstate.NormalizeEnvironment(servingstate.Environment(query))
	}
	return s.defaultServingEnvironment()
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
