package filesystem

import (
	"os"

	servingstate "github.com/Yacobolo/libredash/internal/servingstate"
)

type Validator struct {
	DataDir   string
	DuckDBDir string
}

func (v Validator) ValidateArtifact(path string, workspaceID servingstate.WorkspaceID, environment servingstate.Environment, servingStateID servingstate.ID) (servingstate.Validation, error) {
	return ValidateArtifactWithOptions(path, workspaceID, servingStateID, ValidateOptions{
		DataDir:     v.DataDir,
		DuckDBDir:   v.DuckDBDir,
		Environment: environment,
	})
}

func (Validator) Cleanup(validation servingstate.Validation) error {
	if validation.RootDir == "" {
		return nil
	}
	return os.RemoveAll(validation.RootDir)
}
