package filesystem

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/Yacobolo/libredash/internal/deployment"
)

func TestPackProjectValidatesSelectedWorkspace(t *testing.T) {
	projectPath := filepath.Join("..", "..", "..", "dashboards", ProjectFile)
	var bundle bytes.Buffer
	deploymentID := deployment.ID("dep_ops")
	manifest, _, err := PackProject(projectPath, "operations", deploymentID, &bundle)
	if err != nil {
		t.Fatalf("PackProject() error = %v", err)
	}
	if manifest.CatalogPath != ProjectFile {
		t.Fatalf("CatalogPath = %q, want %q", manifest.CatalogPath, ProjectFile)
	}
	if manifest.WorkspaceID != "operations" {
		t.Fatalf("WorkspaceID = %q, want operations", manifest.WorkspaceID)
	}

	path := filepath.Join(t.TempDir(), "artifact.tar.gz")
	if err := os.WriteFile(path, bundle.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	validation, err := ValidateArtifact(path, deployment.WorkspaceID("operations"), deploymentID)
	if err != nil {
		t.Fatalf("ValidateArtifact() error = %v", err)
	}
	if len(validation.Graph.Assets) == 0 {
		t.Fatal("validated graph has no assets")
	}
	for _, asset := range validation.Graph.Assets {
		if asset.WorkspaceID != "operations" {
			t.Fatalf("asset workspace = %q, want operations: %#v", asset.WorkspaceID, asset)
		}
	}
}

func TestValidateArtifactRejectsWrongDeploymentCompiledGraph(t *testing.T) {
	projectPath := filepath.Join("..", "..", "..", "dashboards", ProjectFile)
	var bundle bytes.Buffer
	if _, _, err := PackProject(projectPath, "operations", deployment.ID("dep_ops"), &bundle); err != nil {
		t.Fatalf("PackProject() error = %v", err)
	}
	path := filepath.Join(t.TempDir(), "artifact.tar.gz")
	if err := os.WriteFile(path, bundle.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := ValidateArtifact(path, deployment.WorkspaceID("operations"), deployment.ID("dep_other"))
	if err == nil {
		t.Fatal("ValidateArtifact() error = nil, want deployment mismatch")
	}
}

func TestPackProjectRejectsUnknownWorkspace(t *testing.T) {
	projectPath := filepath.Join("..", "..", "..", "dashboards", ProjectFile)
	var bundle bytes.Buffer
	_, _, err := PackProject(projectPath, "missing", deployment.ID("dep_missing"), &bundle)
	if err == nil {
		t.Fatal("PackProject() error = nil, want unknown workspace error")
	}
}
