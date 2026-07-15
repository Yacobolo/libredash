package filesystem

import (
	"os"

	servingstate "github.com/Yacobolo/libredash/internal/servingstate"
)

type Validator struct{}

func (v Validator) ValidateArtifact(path string, workspaceID servingstate.WorkspaceID, environment servingstate.Environment, servingStateID servingstate.ID) (servingstate.Validation, error) {
	return ValidateArtifactWithOptions(path, workspaceID, servingStateID, ValidateOptions{
		Environment: environment,
	})
}

func (Validator) Cleanup(validation servingstate.Validation) error {
	if validation.RootDir == "" {
		return nil
	}
	return os.RemoveAll(validation.RootDir)
}
