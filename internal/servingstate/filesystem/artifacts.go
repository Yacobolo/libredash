package filesystem

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/Yacobolo/leapview/internal/securefs"
	servingstate "github.com/Yacobolo/leapview/internal/servingstate"
)

const (
	BundleFormat        = "tar.gz"
	ProjectFile         = "leapview.yaml"
	CompiledProjectFile = "compiled/workspace.json"
	MaxUploadBytes      = 128 << 20
)

type ArtifactStore struct {
	dir string
}

func NewArtifactStore(dir string) *ArtifactStore {
	return &ArtifactStore{dir: dir}
}

func (s *ArtifactStore) UploadPath(servingStateID servingstate.ID) string {
	if err := validateArtifactPathComponent(string(servingStateID), "serving state id"); err != nil {
		return filepath.Join(s.dir, ".invalid.upload.tar.gz")
	}
	return filepath.Join(s.dir, string(servingStateID)+".upload.tar.gz")
}

func (s *ArtifactStore) SaveUpload(_ context.Context, servingStateID servingstate.ID, source io.Reader) (int64, error) {
	if err := validateArtifactPathComponent(string(servingStateID), "serving state id"); err != nil {
		return 0, err
	}
	if err := securefs.EnsurePrivateDir(s.dir); err != nil {
		return 0, err
	}
	tmp, err := os.CreateTemp(s.dir, string(servingStateID)+".upload-*.tmp")
	if err != nil {
		return 0, err
	}
	tmpPath := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpPath)
		}
	}()
	size, copyErr := io.Copy(tmp, source)
	closeErr := tmp.Close()
	if copyErr != nil {
		return size, copyErr
	}
	if closeErr != nil {
		return size, fmt.Errorf("closing uploaded artifact: %w", closeErr)
	}
	if err := os.Rename(tmpPath, s.UploadPath(servingStateID)); err != nil {
		return size, err
	}
	cleanup = false
	return size, nil
}

func (s *ArtifactStore) PromoteUploaded(_ context.Context, servingStateID servingstate.ID, digest, manifestJSON string) (servingstate.Artifact, error) {
	if err := validateArtifactPathComponent(string(servingStateID), "serving state id"); err != nil {
		return servingstate.Artifact{}, err
	}
	if err := validateArtifactPathComponent(digest, "artifact digest"); err != nil {
		return servingstate.Artifact{}, err
	}
	if err := securefs.EnsurePrivateDir(s.dir); err != nil {
		return servingstate.Artifact{}, err
	}
	uploadPath := s.UploadPath(servingStateID)
	finalPath := filepath.Join(s.dir, digest+".tar.gz")
	if err := os.Rename(uploadPath, finalPath); err != nil {
		if copyErr := copyFile(uploadPath, finalPath); copyErr != nil {
			return servingstate.Artifact{}, copyErr
		}
		_ = os.Remove(uploadPath)
	}
	return servingstate.Artifact{
		ID:             "artifact_" + string(servingStateID),
		ServingStateID: servingStateID,
		Digest:         digest,
		Format:         BundleFormat,
		Path:           finalPath,
		ManifestJSON:   manifestJSON,
		SizeBytes:      fileSize(finalPath),
	}, nil
}

func validateArtifactPathComponent(value, label string) error {
	value = strings.TrimSpace(value)
	if value == "" || value == "." || value == ".." || filepath.IsAbs(value) || filepath.Base(value) != value {
		return fmt.Errorf("%s must be a safe path component", label)
	}
	return nil
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
	out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, securefs.PrivateFileMode)
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
	if err := os.Chmod(target, securefs.PrivateFileMode); err != nil {
		return err
	}
	return nil
}
