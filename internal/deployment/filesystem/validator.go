package filesystem

import (
	"os"

	"github.com/Yacobolo/libredash/internal/deployment"
)

type Validator struct {
	DataDir   string
	DuckDBDir string
}

func (v Validator) ValidateArtifact(path string, workspaceID deployment.WorkspaceID, environment deployment.Environment, deploymentID deployment.ID) (deployment.Validation, error) {
	return ValidateArtifactWithOptions(path, workspaceID, deploymentID, ValidateOptions{
		DataDir:     v.DataDir,
		DuckDBDir:   v.DuckDBDir,
		Environment: environment,
	})
}

func (Validator) Cleanup(validation deployment.Validation) error {
	if validation.RootDir == "" {
		return nil
	}
	return os.RemoveAll(validation.RootDir)
}
