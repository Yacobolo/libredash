package filesystem

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	servingstate "github.com/Yacobolo/libredash/internal/servingstate"
)

const (
	BundleFormat        = "tar.gz"
	ProjectFile         = "libredash.yaml"
	CompiledProjectFile = "compiled/workspace.json"
)

type ArtifactStore struct {
	dir string
}

func NewArtifactStore(dir string) *ArtifactStore {
	return &ArtifactStore{dir: dir}
}

func (s *ArtifactStore) UploadPath(servingStateID servingstate.ID) string {
	return filepath.Join(s.dir, string(servingStateID)+".upload.tar.gz")
}

func (s *ArtifactStore) PromoteUploaded(_ context.Context, servingStateID servingstate.ID, digest, manifestJSON string) (servingstate.Artifact, error) {
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
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
