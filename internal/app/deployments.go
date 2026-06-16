package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/Yacobolo/libredash/internal/api"
	"github.com/Yacobolo/libredash/internal/deploy"
	"github.com/Yacobolo/libredash/internal/platform"
	platformdb "github.com/Yacobolo/libredash/internal/platform/db"
	"github.com/Yacobolo/libredash/internal/runtime"
	"github.com/go-chi/chi/v5"
)

type runtimeReloader interface {
	Reload(ctx context.Context) error
	PrepareDeployment(ctx context.Context, deploymentID string) (*runtime.Prepared, error)
	CommitPrepared(prepared *runtime.Prepared) error
}

func (s *Server) createDeployment(w http.ResponseWriter, r *http.Request) {
	var input api.DeploymentCreateRequest
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&input)
	}
	workspaceID := s.workspaceID(input.WorkspaceID)
	if err := s.store.EnsureWorkspace(r.Context(), platform.WorkspaceInput{ID: workspaceID, Title: firstNonEmpty(input.Title, workspaceID), Description: input.Description}); err != nil {
		writeJSONError(w, err, http.StatusInternalServerError)
		return
	}
	createdBy := ""
	if s.auth != nil {
		if principal, ok := s.auth.Principal(r); ok {
			createdBy = principal.ID
		}
	}
	deployment, err := s.store.CreateDeployment(r.Context(), workspaceID, createdBy)
	if err != nil {
		writeJSONError(w, err, http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, deploymentDTO(deployment))
}

func (s *Server) uploadDeploymentArtifact(w http.ResponseWriter, r *http.Request) {
	deploymentID := chi.URLParam(r, "deployment")
	deployment, err := s.store.Queries().GetDeployment(r.Context(), deploymentID)
	if err != nil {
		writeJSONError(w, err, statusForNotFound(err))
		return
	}
	if err := os.MkdirAll(s.artifactDir, 0o755); err != nil {
		writeJSONError(w, err, http.StatusInternalServerError)
		return
	}
	path := filepath.Join(s.artifactDir, deployment.ID+".upload.tar.gz")
	out, err := os.Create(path)
	if err != nil {
		writeJSONError(w, err, http.StatusInternalServerError)
		return
	}
	size, copyErr := io.Copy(out, http.MaxBytesReader(w, r.Body, 128<<20))
	closeErr := out.Close()
	if copyErr != nil {
		writeJSONError(w, copyErr, http.StatusBadRequest)
		return
	}
	if closeErr != nil {
		writeJSONError(w, closeErr, http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deploymentId": deployment.ID, "sizeBytes": size})
}

func (s *Server) validateDeployment(w http.ResponseWriter, r *http.Request) {
	deploymentID := chi.URLParam(r, "deployment")
	deployment, err := s.store.Queries().GetDeployment(r.Context(), deploymentID)
	if err != nil {
		writeJSONError(w, err, statusForNotFound(err))
		return
	}
	uploadPath := filepath.Join(s.artifactDir, deployment.ID+".upload.tar.gz")
	validation, err := deploy.ValidateArtifact(uploadPath, deployment.WorkspaceID, deployment.ID)
	if err != nil {
		_ = s.store.MarkDeploymentFailed(r.Context(), deployment.ID, err)
		writeJSONError(w, err, http.StatusBadRequest)
		return
	}
	defer os.RemoveAll(validation.RootDir)
	finalPath := filepath.Join(s.artifactDir, validation.Digest+".tar.gz")
	if err := os.Rename(uploadPath, finalPath); err != nil {
		if copyErr := copyFile(uploadPath, finalPath); copyErr != nil {
			writeJSONError(w, copyErr, http.StatusInternalServerError)
			return
		}
		_ = os.Remove(uploadPath)
	}
	if err := s.store.ValidateDeployment(r.Context(), deployment.ID, validation.Digest, validation.ManifestJSON, platformdb.InsertDeploymentArtifactParams{
		ID:           "artifact_" + deployment.ID,
		DeploymentID: deployment.ID,
		WorkspaceID:  deployment.WorkspaceID,
		Digest:       validation.Digest,
		Format:       deploy.BundleFormat,
		Path:         finalPath,
		ManifestJson: validation.ManifestJSON,
		SizeBytes:    fileSize(finalPath),
	}, validation.Assets, validation.Edges); err != nil {
		writeJSONError(w, err, http.StatusInternalServerError)
		return
	}
	deployment, _ = s.store.Queries().GetDeployment(r.Context(), deployment.ID)
	writeJSON(w, http.StatusOK, deploymentDTO(deployment))
}

func (s *Server) activateDeployment(w http.ResponseWriter, r *http.Request) {
	deploymentID := chi.URLParam(r, "deployment")
	deployment, err := s.store.Queries().GetDeployment(r.Context(), deploymentID)
	if err != nil {
		writeJSONError(w, err, statusForNotFound(err))
		return
	}
	var prepared *runtime.Prepared
	if s.reloader != nil {
		prepared, err = s.reloader.PrepareDeployment(r.Context(), deployment.ID)
		if err != nil {
			writeJSONError(w, err, http.StatusInternalServerError)
			return
		}
	}
	if err := s.store.ActivateDeployment(r.Context(), deployment.WorkspaceID, deployment.ID); err != nil {
		if prepared != nil {
			_ = prepared.Close()
		}
		writeJSONError(w, err, http.StatusBadRequest)
		return
	}
	if prepared != nil {
		if err := s.reloader.CommitPrepared(prepared); err != nil {
			writeJSONError(w, err, http.StatusInternalServerError)
			return
		}
	}
	deployment, _ = s.store.Queries().GetDeployment(r.Context(), deployment.ID)
	writeJSON(w, http.StatusOK, deploymentDTO(deployment))
}

func (s *Server) listDeployments(w http.ResponseWriter, r *http.Request) {
	workspaceID := s.workspaceID(r.URL.Query().Get("workspace"))
	rows, err := s.store.Queries().ListDeployments(r.Context(), workspaceID)
	if err != nil {
		writeJSONError(w, err, http.StatusInternalServerError)
		return
	}
	response := make([]api.DeploymentResponse, 0, len(rows))
	for _, row := range rows {
		response = append(response, deploymentDTO(row))
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) getDeployment(w http.ResponseWriter, r *http.Request) {
	deployment, err := s.store.Queries().GetDeployment(r.Context(), chi.URLParam(r, "deployment"))
	if err != nil {
		writeJSONError(w, err, statusForNotFound(err))
		return
	}
	writeJSON(w, http.StatusOK, deploymentDTO(deployment))
}

func (s *Server) rollbackDeployment(w http.ResponseWriter, r *http.Request) {
	s.activateDeployment(w, r)
}

func (s *Server) workspaceID(candidate string) string {
	if candidate != "" {
		return candidate
	}
	if s.defaultWorkspaceID != "" {
		return s.defaultWorkspaceID
	}
	return platform.DefaultWorkspaceID
}

func deploymentDTO(row platformdb.Deployment) api.DeploymentResponse {
	out := api.DeploymentResponse{
		ID:          row.ID,
		WorkspaceID: row.WorkspaceID,
		Status:      row.Status,
		Digest:      row.Digest,
		CreatedAt:   row.CreatedAt,
		Error:       row.Error,
	}
	if row.ActivatedAt.Valid {
		out.ActivatedAt = row.ActivatedAt.String
	}
	return out
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeJSONError(w http.ResponseWriter, err error, status int) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

func statusForNotFound(err error) int {
	if err == sql.ErrNoRows {
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

func fileSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}

func copyFile(source, target string) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(target)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return fmt.Errorf("closing %s: %w", target, err)
	}
	return nil
}
