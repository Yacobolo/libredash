package compiler

import (
	"fmt"
	"strings"

	"github.com/Yacobolo/libredash/internal/workspace"
)

type Options struct {
	WorkspaceID  workspace.WorkspaceID
	DeploymentID workspace.DeploymentID
}

type CompiledWorkspace struct {
	Workspace  workspace.Workspace
	Definition *workspace.Definition
}

func Compile(projectPath string, opts Options) (CompiledWorkspace, error) {
	compiled, err := CompileProject(projectPath, opts)
	if err != nil {
		return CompiledWorkspace{}, err
	}
	workspaceID := opts.WorkspaceID
	if workspaceID == "" {
		return CompiledWorkspace{}, fmt.Errorf("workspace id is required")
	}
	selected, ok := compiled.Workspaces[string(workspaceID)]
	if !ok {
		return CompiledWorkspace{}, fmt.Errorf("project %q has no workspace %q", projectPath, workspaceID)
	}
	return selected, nil
}

func workspaceTitle(value, workspaceID string) string {
	if strings.TrimSpace(value) != "" {
		return value
	}
	if strings.TrimSpace(workspaceID) != "" {
		return workspaceID
	}
	return "LibreDash"
}
