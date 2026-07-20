package compiler

import (
	"errors"
	"fmt"

	"github.com/Yacobolo/leapview/internal/configschema"
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

func annotateSchemaError(err error, path, resourceID, fieldPath string) error {
	var schemaErr *configschema.Error
	if !errors.As(err, &schemaErr) {
		return err
	}
	diagnostics := append([]configschema.Diagnostic(nil), schemaErr.Diagnostics...)
	for index := range diagnostics {
		if diagnostics[index].File == "" {
			diagnostics[index].File = path
		}
		if diagnostics[index].ResourceID == "" {
			diagnostics[index].ResourceID = resourceID
		}
		if diagnostics[index].FieldPath == "" {
			diagnostics[index].FieldPath = fieldPath
		}
	}
	return &configschema.Error{Diagnostics: diagnostics}
}
