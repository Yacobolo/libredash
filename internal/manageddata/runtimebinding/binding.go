// Package runtimebinding applies trusted managed-data runtime roots to a
// compiled workspace definition.
package runtimebinding

import (
	"fmt"
	"path/filepath"

	"github.com/Yacobolo/leapview/internal/runtimehost"
	"github.com/Yacobolo/leapview/internal/workspace"
)

func BindRoots(definition *workspace.Definition, resolution runtimehost.ManagedDataResolution) error {
	if definition == nil {
		return fmt.Errorf("workspace definition is required")
	}
	for modelID, model := range definition.Models {
		if model == nil {
			continue
		}
		for connectionName, connection := range model.Connections {
			if connection.Kind != "managed" {
				continue
			}
			resolvedRoot := resolution.Roots[connectionName]
			if resolvedRoot == "" {
				return fmt.Errorf("semantic model %q managed connection %q has no bound revision", modelID, connectionName)
			}
			root := filepath.Clean(resolvedRoot)
			if !filepath.IsAbs(root) {
				return fmt.Errorf("semantic model %q managed connection %q revision root must be absolute", modelID, connectionName)
			}
			connection.Root = root
			connection.Scope = ""
			model.Connections[connectionName] = connection
		}
	}
	return nil
}
