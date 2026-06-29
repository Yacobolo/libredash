package compiler

import (
	"fmt"

	"github.com/Yacobolo/libredash/internal/configschema"
)

type ResourceError struct {
	Path       string
	ResourceID string
	FieldPath  string
	Message    string
}

func (e ResourceError) Error() string {
	return e.Message
}

func (e ResourceError) Diagnostic() configschema.Diagnostic {
	return configschema.Diagnostic{
		File:       e.Path,
		ResourceID: e.ResourceID,
		FieldPath:  e.FieldPath,
		Severity:   configschema.SeverityError,
		Code:       "compiler.resource",
		Message:    e.Message,
	}
}

func resourceError(path, resourceID, fieldPath, format string, args ...any) error {
	return ResourceError{
		Path:       path,
		ResourceID: resourceID,
		FieldPath:  fieldPath,
		Message:    fmt.Sprintf(format, args...),
	}
}
