package filesystem

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	semanticmodel "github.com/Yacobolo/libredash/internal/analytics/model"
	servingstate "github.com/Yacobolo/libredash/internal/servingstate"
	"github.com/Yacobolo/libredash/internal/workspace"
	workspacecompiler "github.com/Yacobolo/libredash/internal/workspace/compiler"
)

const olistManagedDataRevision = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func olistManagedDataRevisions() map[string]string {
	return map[string]string{"olist": olistManagedDataRevision}
}

func TestPackProjectValidatesSelectedWorkspace(t *testing.T) {
	projectPath := filepath.Join("..", "..", "..", "dashboards", ProjectFile)
	var bundle bytes.Buffer
	servingStateID := servingstate.ID("dep_ops")
	manifest, _, err := PackProject(projectPath, PackProjectOptions{WorkspaceID: "operations", ServingStateID: servingStateID, ManagedDataRevisions: olistManagedDataRevisions()}, &bundle)
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
	validation, err := ValidateArtifact(path, servingstate.WorkspaceID("operations"), servingStateID)
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
	root := t.TempDir()
	if err := ExtractArtifact(path, root); err != nil {
		t.Fatalf("ExtractArtifact() error = %v", err)
	}
	compiled, _, err := LoadCompiledWorkspaceArtifact(root)
	if err != nil {
		t.Fatalf("LoadCompiledWorkspaceArtifact() error = %v", err)
	}
	if compiled.ProjectID != "libredash-showcase" {
		t.Fatalf("ProjectID = %q, want libredash-showcase", compiled.ProjectID)
	}
	if compiled.ProjectDigest == "" {
		t.Fatal("ProjectDigest is empty")
	}
	wantWorkspaces := []string{"operations", "sales", "visuals"}
	if !reflect.DeepEqual(compiled.ProjectWorkspaces, wantWorkspaces) {
		t.Fatalf("ProjectWorkspaces = %#v, want %#v", compiled.ProjectWorkspaces, wantWorkspaces)
	}
	if validation.ProjectDigest != compiled.ProjectDigest || !reflect.DeepEqual(validation.ProjectWorkspaces, wantWorkspaces) {
		t.Fatalf("validation project identity = (%q, %#v), want (%q, %#v)", validation.ProjectDigest, validation.ProjectWorkspaces, compiled.ProjectDigest, wantWorkspaces)
	}
	if !reflect.DeepEqual(validation.AccessPolicy, compiled.Definition.Access) {
		t.Fatalf("validation access policy does not match compiled artifact")
	}
	if compiled.Validation.Status != "passed" || compiled.Validation.SchemaVersion != "libredash.dev/v1" {
		t.Fatalf("compiled validation = %#v, want passed libredash.dev/v1", compiled.Validation)
	}
	if compiled.Validation.GraphHash == "" || compiled.Validation.GraphHash != graphHash(compiled.Graph) {
		t.Fatalf("compiled validation graph hash = %q, want %q", compiled.Validation.GraphHash, graphHash(compiled.Graph))
	}
}

func TestPackProjectEmbedsCanonicalManagedDataRevisionPins(t *testing.T) {
	projectPath := writeManagedBundleProject(t)
	digest := "sha256:" + strings.Repeat("a", 64)
	var bundle bytes.Buffer
	_, _, err := PackProject(projectPath, PackProjectOptions{
		WorkspaceID:          "sales",
		ServingStateID:       "dep_sales",
		ManagedDataRevisions: map[string]string{"orders": digest},
	}, &bundle)
	if err != nil {
		t.Fatalf("PackProject() error = %v", err)
	}
	path := filepath.Join(t.TempDir(), "artifact.tar.gz")
	if err := os.WriteFile(path, bundle.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := ExtractArtifact(path, root); err != nil {
		t.Fatal(err)
	}
	compiled, _, err := LoadCompiledWorkspaceArtifact(root)
	if err != nil {
		t.Fatal(err)
	}
	if got := compiled.ManagedDataRevisions["orders"]; got != digest {
		t.Fatalf("managedDataRevisions[orders] = %q, want %q", got, digest)
	}
}

func TestPackProjectRequiresExactManagedDataRevisionPins(t *testing.T) {
	projectPath := writeManagedBundleProject(t)
	digest := "sha256:" + strings.Repeat("a", 64)
	tests := []struct {
		name string
		pins map[string]string
	}{
		{name: "missing", pins: map[string]string{}},
		{name: "extra", pins: map[string]string{"orders": digest, "other": digest}},
		{name: "whitespace key", pins: map[string]string{"orders ": digest}},
		{name: "internal revision id", pins: map[string]string{"orders": "revision_1"}},
		{name: "uppercase digest", pins: map[string]string{"orders": "sha256:" + strings.Repeat("A", 64)}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var bundle bytes.Buffer
			_, _, err := PackProject(projectPath, PackProjectOptions{
				WorkspaceID:          "sales",
				ServingStateID:       "dep_sales",
				ManagedDataRevisions: test.pins,
			}, &bundle)
			if err == nil {
				t.Fatal("PackProject() error = nil, want pin validation error")
			}
		})
	}
}

func TestValidateCompiledWorkspaceArtifactRejectsConflictingManagedConnectionDefinitions(t *testing.T) {
	digest := "sha256:" + strings.Repeat("a", 64)
	compiled := CompiledWorkspaceArtifact{
		Version:   compiledWorkspaceArtifactVersion,
		ProjectID: "project-a",
		Definition: &workspace.Definition{Models: map[string]*semanticmodel.Model{
			"first":  {Connections: map[string]semanticmodel.Connection{"orders": {Kind: "managed"}}},
			"second": {Connections: map[string]semanticmodel.Connection{"orders": {Kind: "managed", Description: "different"}}},
		}},
		ManagedDataRevisions: map[string]string{"orders": digest},
	}
	compiled.Validation = CompiledArtifactValidation{Status: "passed", SchemaVersion: projectAPIVersion, GraphHash: graphHash(compiled.Graph)}
	if err := ValidateCompiledWorkspaceArtifact(compiled); err == nil {
		t.Fatal("ValidateCompiledWorkspaceArtifact() error = nil, want conflicting connection rejection")
	}
}

func TestExtractArtifactRejectsSymlinkEscape(t *testing.T) {
	artifactPath := writeTestBundle(t, map[string]string{"link/escape.txt": "owned"})
	dest := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(dest, "link")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	err := ExtractArtifact(artifactPath, dest)

	if err == nil {
		t.Fatal("ExtractArtifact() error = nil, want symlink escape rejection")
	}
	if _, statErr := os.Stat(filepath.Join(outside, "escape.txt")); !os.IsNotExist(statErr) {
		t.Fatalf("outside file stat err = %v, want not exist", statErr)
	}
}

func TestValidateArtifactRejectsUnlistedBundleFile(t *testing.T) {
	path := packedProjectArtifact(t, "sales", "dev", "dep_extra")
	addUnlistedArtifactFileForTest(t, path, "unexpected/extra.yaml", "owned")

	_, err := ValidateArtifact(path, servingstate.WorkspaceID("sales"), servingstate.ID("dep_extra"))
	if err == nil {
		t.Fatal("ValidateArtifact() error = nil, want unlisted file rejection")
	}
	if !strings.Contains(err.Error(), "not listed in manifest") {
		t.Fatalf("ValidateArtifact() error = %v, want unlisted manifest file error", err)
	}
}

func TestValidateArtifactIsDataLocationIndependent(t *testing.T) {
	projectPath := filepath.Join("..", "..", "..", "dashboards", ProjectFile)
	var bundle bytes.Buffer
	servingStateID := servingstate.ID("dep_discovered")
	if _, _, err := PackProject(projectPath, PackProjectOptions{WorkspaceID: "sales", ServingStateID: servingStateID, ManagedDataRevisions: olistManagedDataRevisions()}, &bundle); err != nil {
		t.Fatalf("PackProject() error = %v", err)
	}
	path := filepath.Join(t.TempDir(), "artifact.tar.gz")
	if err := os.WriteFile(path, bundle.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	validation, err := ValidateArtifact(path, "sales", servingStateID)
	if err != nil {
		t.Fatalf("ValidateArtifact() error = %v", err)
	}
	if len(validation.Graph.Assets) == 0 {
		t.Fatal("validated graph has no assets")
	}
}

func TestValidateArtifactRetargetsPortableCompiledGraph(t *testing.T) {
	projectPath := filepath.Join("..", "..", "..", "dashboards", ProjectFile)
	var bundle bytes.Buffer
	if _, _, err := PackProject(projectPath, PackProjectOptions{WorkspaceID: "operations", ServingStateID: "dep_ops", ManagedDataRevisions: olistManagedDataRevisions()}, &bundle); err != nil {
		t.Fatalf("PackProject() error = %v", err)
	}
	path := filepath.Join(t.TempDir(), "artifact.tar.gz")
	if err := os.WriteFile(path, bundle.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	validation, err := ValidateArtifact(path, servingstate.WorkspaceID("operations"), servingstate.ID("dep_other"))
	if err != nil {
		t.Fatalf("ValidateArtifact() error = %v", err)
	}
	for _, asset := range validation.Graph.Assets {
		if asset.ServingStateID != "dep_other" {
			t.Fatalf("asset serving state = %q, want dep_other", asset.ServingStateID)
		}
	}
}

func TestValidateArtifactRejectsMissingOrMismatchedEnvironment(t *testing.T) {
	tests := []struct {
		name        string
		environment servingstate.Environment
		mutate      func(*CompiledWorkspaceArtifact, *Manifest)
	}{
		{
			name:        "missing compiled environment",
			environment: "dev",
			mutate: func(compiled *CompiledWorkspaceArtifact, manifest *Manifest) {
				compiled.Environment = ""
			},
		},
		{
			name:        "missing manifest environment",
			environment: "dev",
			mutate: func(compiled *CompiledWorkspaceArtifact, manifest *Manifest) {
				manifest.Environment = ""
			},
		},
		{
			name:        "manifest compiled mismatch",
			environment: "dev",
			mutate: func(compiled *CompiledWorkspaceArtifact, manifest *Manifest) {
				compiled.Environment = "prod"
				manifest.Environment = "dev"
			},
		},
		{
			name:        "target mismatch",
			environment: "prod",
			mutate: func(compiled *CompiledWorkspaceArtifact, manifest *Manifest) {
				compiled.Environment = "dev"
				manifest.Environment = "dev"
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := packedProjectArtifact(t, "sales", "dev", "dep_env")
			mutateArtifactForTest(t, path, tt.mutate)

			_, err := ValidateArtifactWithOptions(path, "sales", "dep_env", ValidateOptions{Environment: tt.environment})
			if err == nil {
				t.Fatal("ValidateArtifactWithOptions() error = nil, want environment mismatch")
			}
		})
	}
}

func digestCompiledForTest(t *testing.T, compiled CompiledWorkspaceArtifact) string {
	t.Helper()
	raw, err := json.MarshalIndent(compiled, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return digestBytes(raw)
}

func packedProjectArtifact(t *testing.T, workspaceID string, environment servingstate.Environment, servingStateID servingstate.ID) string {
	t.Helper()
	projectPath := filepath.Join("..", "..", "..", "dashboards", ProjectFile)
	var bundle bytes.Buffer
	if _, _, err := PackProject(projectPath, PackProjectOptions{WorkspaceID: workspaceID, Environment: environment, ServingStateID: servingStateID, ManagedDataRevisions: olistManagedDataRevisions()}, &bundle); err != nil {
		t.Fatalf("PackProject() error = %v", err)
	}
	path := filepath.Join(t.TempDir(), "artifact.tar.gz")
	if err := os.WriteFile(path, bundle.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func mutateArtifactForTest(t *testing.T, path string, mutate func(*CompiledWorkspaceArtifact, *Manifest)) {
	t.Helper()
	root := t.TempDir()
	if err := ExtractArtifact(path, root); err != nil {
		t.Fatalf("ExtractArtifact() error = %v", err)
	}
	compiled, manifest, err := LoadCompiledWorkspaceArtifact(root)
	if err != nil {
		t.Fatalf("LoadCompiledWorkspaceArtifact() error = %v", err)
	}
	mutate(&compiled, &manifest)
	compiledRel, err := safeBundlePath(manifest.CompiledPath)
	if err != nil {
		t.Fatal(err)
	}
	compiledBytes, err := json.MarshalIndent(compiled, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, compiledRel), compiledBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	manifestBytes, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "manifest.json"), manifestBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".libredash-test-artifact-*.tar.gz")
	if err != nil {
		t.Fatal(err)
	}
	tmpPath := tmp.Name()
	if err := writeExtractedRoot(root, tmp); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		t.Fatal(err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		t.Fatal(err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		t.Fatal(err)
	}
}

func addUnlistedArtifactFileForTest(t *testing.T, path, name, content string) {
	t.Helper()
	root := t.TempDir()
	if err := ExtractArtifact(path, root); err != nil {
		t.Fatalf("ExtractArtifact() error = %v", err)
	}
	target := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".libredash-test-artifact-*.tar.gz")
	if err != nil {
		t.Fatal(err)
	}
	tmpPath := tmp.Name()
	if err := writeExtractedRoot(root, tmp); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		t.Fatal(err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		t.Fatal(err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		t.Fatal(err)
	}
}

func TestPackProjectRejectsUnknownWorkspace(t *testing.T) {
	projectPath := filepath.Join("..", "..", "..", "dashboards", ProjectFile)
	var bundle bytes.Buffer
	_, _, err := PackProject(projectPath, PackProjectOptions{WorkspaceID: "missing", ServingStateID: "dep_missing"}, &bundle)
	if err == nil {
		t.Fatal("PackProject() error = nil, want unknown workspace error")
	}
}

func TestPackProjectStoresActiveDeploymentPlanDiff(t *testing.T) {
	projectPath := filepath.Join("..", "..", "..", "dashboards", ProjectFile)
	active, err := workspacecompiler.CompileProject(projectPath, workspacecompiler.Options{ServingStateID: workspace.ServingStateID("dep_active")})
	if err != nil {
		t.Fatalf("CompileProject() error = %v", err)
	}
	activeGraph := active.Workspaces["operations"].Workspace.Graph
	for index := range activeGraph.Assets {
		if activeGraph.Assets[index].ID == "model_table:operations.orders" {
			var payload map[string]any
			if err := json.Unmarshal([]byte(activeGraph.Assets[index].PayloadJSON), &payload); err != nil {
				t.Fatalf("unmarshal model table payload: %v", err)
			}
			payload["SQL"] = "SELECT *, 'changed' AS changed FROM source.\"olist.orders\""
			payloadBytes, err := json.Marshal(payload)
			if err != nil {
				t.Fatalf("marshal model table payload: %v", err)
			}
			activeGraph.Assets[index].PayloadJSON = string(payloadBytes)
		}
	}
	activeGraph.Assets = append(activeGraph.Assets, workspace.Asset{
		ID:             "dashboard:operations.removed",
		WorkspaceID:    "operations",
		ServingStateID: "dep_active",
		Type:           workspace.AssetTypeDashboard,
		Key:            "operations.removed",
		PayloadSchema:  workspace.PayloadSchemaForAssetType(workspace.AssetTypeDashboard),
		ContentHash:    "removed",
	})

	var bundle bytes.Buffer
	if _, _, err := PackProject(projectPath, PackProjectOptions{WorkspaceID: "operations", ServingStateID: "dep_ops", ActiveGraph: activeGraph, ManagedDataRevisions: olistManagedDataRevisions()}, &bundle); err != nil {
		t.Fatalf("PackProject() error = %v", err)
	}
	path := filepath.Join(t.TempDir(), "artifact.tar.gz")
	if err := os.WriteFile(path, bundle.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := ExtractArtifact(path, root); err != nil {
		t.Fatalf("ExtractArtifact() error = %v", err)
	}
	compiled, _, err := LoadCompiledWorkspaceArtifact(root)
	if err != nil {
		t.Fatalf("LoadCompiledWorkspaceArtifact() error = %v", err)
	}
	if compiled.Plan.Summary.Changed != 1 || compiled.Plan.Summary.Removed != 1 {
		t.Fatalf("plan summary = %#v, want one changed and one removed", compiled.Plan.Summary)
	}
}

func TestPackProjectDoesNotSerializeResolvedConnectionCredentials(t *testing.T) {
	t.Setenv("LIBREDASH_TEST_CRM_URL", "postgres://secret-host/sales")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "ambient-secret-must-not-be-serialized")
	projectPath := writeBundleProjectFixture(t, map[string]string{
		"libredash.yaml": `
apiVersion: libredash.dev/v1
kind: Project
metadata:
  name: test
spec:
  connections:
    include:
      - connections/*.yaml
  sources:
    include:
      - sources/*.yaml
  workspaces:
    include:
      - workspaces/*/workspace.yaml
`,
		"connections/crm.yaml": `
apiVersion: libredash.dev/v1
kind: Connection
metadata:
  name: crm
spec:
  kind: postgres
  credentials:
    provider: env
    secret: LIBREDASH_TEST_CRM_URL
`,
		"connections/lake.yaml": `
apiVersion: libredash.dev/v1
kind: Connection
metadata:
  name: lake
spec:
  kind: s3
  scope: s3://company-analytics/sales/
  credentials:
    provider: ambient
    region: eu-west-1
`,
		"sources/crm.orders.yaml": `
apiVersion: libredash.dev/v1
kind: Source
metadata:
  name: crm.orders
spec:
  connection: crm
  object: public.orders
  fields:
    order_id:
      type: string
`,
		"sources/lake.events.yaml": `
apiVersion: libredash.dev/v1
kind: Source
metadata:
  name: lake.events
spec:
  connection: lake
  path: s3://company-analytics/sales/events/*.parquet
  format: parquet
  fields:
    event_id:
      type: string
`,
		"workspaces/sales/workspace.yaml": `
apiVersion: libredash.dev/v1
kind: Workspace
metadata:
  name: sales
spec:
  uses:
    sources:
      - crm.orders
      - lake.events
  models:
    include:
      - models/*.yaml
  semanticModels:
    include:
      - semantic-models/*.yaml
  dashboards:
    include:
      - dashboards/*.yaml
  access:
    include: []
  agentPolicy:
    include: []
`,
		"workspaces/sales/models/orders.yaml": `
apiVersion: libredash.dev/v1
kind: ModelTable
metadata:
  workspace: sales
  name: orders
spec:
  primaryKey: order_id
  sources:
    - crm.orders
  fields:
    order_id:
      label: ID
  transform:
    sql: |
      SELECT order_id FROM source."crm.orders"
`,
		"workspaces/sales/semantic-models/sales.yaml": `
apiVersion: libredash.dev/v1
kind: SemanticModel
metadata:
  workspace: sales
  name: sales
spec:
  tables:
    - orders
  measures:
    order_count:
      fact: orders
      aggregation: count
      empty: zero
`,
		"workspaces/sales/dashboards/sales.yaml": `
apiVersion: libredash.dev/v1
kind: Dashboard
metadata:
  workspace: sales
  name: sales
  title: Sales
spec:
  semanticModel: sales
  visuals:
    total:
      type: kpi
      query:
        measures:
          order_count:
  pages:
    - id: overview
      title: Overview
      components:
        - id: total
          kind: visual
          visual: total
          placement:
            col: 1
            row: 1
            col_span: 3
            row_span: 2
`,
	})

	var bundle bytes.Buffer
	if _, _, err := PackProject(projectPath, PackProjectOptions{WorkspaceID: "sales", ServingStateID: "dep_sales"}, &bundle); err != nil {
		t.Fatalf("PackProject() error = %v", err)
	}
	path := filepath.Join(t.TempDir(), "artifact.tar.gz")
	if err := os.WriteFile(path, bundle.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := ExtractArtifact(path, root); err != nil {
		t.Fatalf("ExtractArtifact() error = %v", err)
	}
	compiledBytes, err := os.ReadFile(filepath.Join(root, CompiledProjectFile))
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"postgres://secret-host/sales", "ambient-secret-must-not-be-serialized"} {
		if strings.Contains(string(compiledBytes), secret) {
			t.Fatalf("compiled artifact serialized resolved credential")
		}
	}
}

func TestReadCompiledWorkspaceArtifactRejectsStaleContractVersion(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, CompiledProjectFile)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	stale := CompiledWorkspaceArtifact{Version: compiledWorkspaceArtifactVersion - 1}
	data, err := json.Marshal(stale)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}

	_, err = readCompiledWorkspaceArtifact(root, Manifest{CompiledPath: CompiledProjectFile})
	if err == nil || !strings.Contains(err.Error(), "redeploy the workspace") {
		t.Fatalf("stale artifact error = %v, want explicit redeployment requirement", err)
	}
}

func writeBundleProjectFixture(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(strings.TrimSpace(content)+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return filepath.Join(dir, ProjectFile)
}

func writeManagedBundleProject(t *testing.T) string {
	t.Helper()
	return writeBundleProjectFixture(t, map[string]string{
		"libredash.yaml": `
apiVersion: libredash.dev/v1
kind: Project
metadata:
  name: project-a
spec:
  connections:
    include: [connections/*.yaml]
  sources:
    include: [sources/*.yaml]
  workspaces:
    include: [workspaces/*/workspace.yaml]
`,
		"connections/orders.yaml": `
apiVersion: libredash.dev/v1
kind: Connection
metadata:
  name: orders
spec:
  kind: managed
  credentials:
    provider: none
`,
		"sources/orders.orders.yaml": `
apiVersion: libredash.dev/v1
kind: Source
metadata:
  name: orders.orders
spec:
  connection: orders
  path: orders.csv
  format: csv
  fields:
    order_id:
      type: string
`,
		"workspaces/sales/workspace.yaml": `
apiVersion: libredash.dev/v1
kind: Workspace
metadata:
  name: sales
spec:
  uses:
    sources: [orders.orders]
  models:
    include: [models/*.yaml]
  semanticModels:
    include: [semantic-models/*.yaml]
  dashboards:
    include: []
  access:
    include: []
  agentPolicy:
    include: []
`,
		"workspaces/sales/models/orders.yaml": `
apiVersion: libredash.dev/v1
kind: ModelTable
metadata:
  workspace: sales
  name: orders
spec:
  source: orders.orders
  primaryKey: order_id
  fields:
    order_id:
      label: Order ID
`,
		"workspaces/sales/semantic-models/sales.yaml": `
apiVersion: libredash.dev/v1
kind: SemanticModel
metadata:
  workspace: sales
  name: sales
spec:
  tables: [orders]
  measures: {}
`,
	})
}

func writeTestBundle(t *testing.T, files map[string]string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "artifact.tar.gz")
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("create test bundle: %v", err)
	}
	gz := gzip.NewWriter(file)
	tw := tar.NewWriter(gz)
	for name, content := range files {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(content)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatalf("write tar header: %v", err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatalf("write tar content: %v", err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar writer: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("close gzip writer: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close test bundle: %v", err)
	}
	return path
}
