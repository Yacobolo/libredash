package filesystem

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	analyticsduckdb "github.com/Yacobolo/libredash/internal/analytics/duckdb"
	servingstate "github.com/Yacobolo/libredash/internal/servingstate"
	"github.com/Yacobolo/libredash/internal/workspace"
	workspacecompiler "github.com/Yacobolo/libredash/internal/workspace/compiler"
)

type Manifest struct {
	Version        int            `json:"version"`
	WorkspaceID    string         `json:"workspaceId"`
	WorkspaceTitle string         `json:"workspaceTitle"`
	Environment    string         `json:"environment"`
	CatalogPath    string         `json:"catalogPath"`
	CompiledPath   string         `json:"compiledPath"`
	GraphHash      string         `json:"graphHash"`
	Files          []ManifestFile `json:"files"`
	SemanticModels []string       `json:"semanticModels"`
	Dashboards     []string       `json:"dashboards"`
}

type ManifestFile struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

type ValidateOptions struct {
	DataDir     string
	DuckDBDir   string
	Environment servingstate.Environment
}

type CompiledWorkspaceArtifact struct {
	Version        int                                    `json:"version"`
	WorkspaceID    string                                 `json:"workspaceId"`
	WorkspaceTitle string                                 `json:"workspaceTitle"`
	Environment    string                                 `json:"environment"`
	ServingStateID string                                 `json:"servingStateId"`
	Validation     CompiledArtifactValidation             `json:"validation"`
	Definition     *workspace.Definition                  `json:"definition"`
	Graph          workspace.AssetGraph                   `json:"graph"`
	Plan           workspacecompiler.ProjectPlanWorkspace `json:"plan"`
}

type CompiledArtifactValidation struct {
	Status        string                       `json:"status"`
	Diagnostics   []CompiledArtifactDiagnostic `json:"diagnostics,omitempty"`
	GraphHash     string                       `json:"graphHash"`
	SchemaVersion string                       `json:"schemaVersion"`
}

type CompiledArtifactDiagnostic struct {
	Severity string `json:"severity"`
	Code     string `json:"code"`
	Message  string `json:"message"`
}

func PackProject(projectPath, workspaceID string, servingStateID servingstate.ID, out io.Writer) (Manifest, string, error) {
	return PackProjectAgainstGraphForEnvironment(projectPath, workspaceID, servingstate.DefaultEnvironment, servingStateID, workspace.AssetGraph{}, out)
}

func PackProjectAgainstGraph(projectPath, workspaceID string, servingStateID servingstate.ID, active workspace.AssetGraph, out io.Writer) (Manifest, string, error) {
	return PackProjectAgainstGraphForEnvironment(projectPath, workspaceID, servingstate.DefaultEnvironment, servingStateID, active, out)
}

func PackProjectAgainstGraphForEnvironment(projectPath, workspaceID string, environment servingstate.Environment, servingStateID servingstate.ID, active workspace.AssetGraph, out io.Writer) (Manifest, string, error) {
	projectPath, err := filepath.Abs(projectPath)
	if err != nil {
		return Manifest{}, "", err
	}
	environment = servingstate.NormalizeEnvironment(environment)
	if workspaceID == "" {
		return Manifest{}, "", fmt.Errorf("project publish requires explicit workspace")
	}
	if servingStateID == "" {
		return Manifest{}, "", fmt.Errorf("project publish requires serving state id")
	}
	compiled, err := workspacecompiler.CompileProject(projectPath, workspacecompiler.Options{ServingStateID: workspace.ServingStateID(servingStateID)})
	if err != nil {
		return Manifest{}, "", err
	}
	compiledWorkspace, ok := compiled.Workspaces[workspaceID]
	if !ok {
		return Manifest{}, "", fmt.Errorf("project %q has no workspace %q", projectPath, workspaceID)
	}
	if err := workspace.ValidateAssetGraphForServingState(compiledWorkspace.Workspace.Graph, workspace.WorkspaceID(workspaceID), workspace.ServingStateID(servingStateID)); err != nil {
		return Manifest{}, "", err
	}
	plan, err := workspacecompiler.PlanProjectAgainstGraph(projectPath, workspaceID, active)
	if err != nil {
		return Manifest{}, "", err
	}
	workspacePlan, ok := projectPlanWorkspace(plan, workspaceID)
	if !ok {
		return Manifest{}, "", fmt.Errorf("project %q has no workspace %q in plan", projectPath, workspaceID)
	}
	compiledArtifact := CompiledWorkspaceArtifact{
		Version:        1,
		WorkspaceID:    workspaceID,
		WorkspaceTitle: compiledWorkspace.Workspace.Title,
		Environment:    string(environment),
		ServingStateID: string(servingStateID),
		Validation: CompiledArtifactValidation{
			Status:        "passed",
			GraphHash:     graphHash(compiledWorkspace.Workspace.Graph),
			SchemaVersion: "libredash.dev/v1",
		},
		Definition: compiledWorkspace.Definition,
		Graph:      compiledWorkspace.Workspace.Graph,
		Plan:       workspacePlan,
	}
	compiledBytes, err := json.MarshalIndent(compiledArtifact, "", "  ")
	if err != nil {
		return Manifest{}, "", err
	}
	baseDir := filepath.Dir(projectPath)
	relFiles, err := collectProjectBundleFiles(baseDir, projectPath)
	if err != nil {
		return Manifest{}, "", err
	}
	manifest := Manifest{
		Version:        1,
		WorkspaceID:    workspaceID,
		WorkspaceTitle: compiledWorkspace.Workspace.Title,
		Environment:    string(environment),
		CatalogPath:    ProjectFile,
		CompiledPath:   CompiledProjectFile,
		GraphHash:      digestBytes(compiledBytes),
		Files:          make([]ManifestFile, 0, len(relFiles)),
	}
	for _, model := range compiledWorkspace.Definition.Catalog.SemanticModels {
		manifest.SemanticModels = append(manifest.SemanticModels, model.ID)
	}
	for _, report := range compiledWorkspace.Definition.Catalog.Dashboards {
		manifest.Dashboards = append(manifest.Dashboards, report.ID)
	}
	return writeBundle(baseDir, relFiles, ProjectFile, projectPath, map[string][]byte{CompiledProjectFile: compiledBytes}, manifest, out)
}

func collectProjectBundleFiles(baseDir, projectPath string) ([]string, error) {
	relProject, err := filepath.Rel(baseDir, projectPath)
	if err != nil {
		return nil, err
	}
	relFiles := []string{cleanBundlePath(relProject)}
	for _, root := range []string{"connections", "sources", "workspaces"} {
		dir := filepath.Join(baseDir, root)
		if _, err := os.Stat(dir); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() {
				return nil
			}
			if filepath.Ext(path) != ".yaml" && filepath.Ext(path) != ".yml" {
				return nil
			}
			rel, err := filepath.Rel(baseDir, path)
			if err != nil {
				return err
			}
			relFiles = append(relFiles, cleanBundlePath(rel))
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	sort.Strings(relFiles[1:])
	return relFiles, nil
}

func writeBundle(baseDir string, relFiles []string, rootRel string, rootPath string, generatedFiles map[string][]byte, manifest Manifest, out io.Writer) (Manifest, string, error) {
	hash := sha256.New()
	mw := io.MultiWriter(out, hash)
	gz := gzip.NewWriter(mw)
	tw := tar.NewWriter(gz)
	seen := map[string]struct{}{}
	for _, rel := range relFiles {
		if _, ok := seen[rel]; ok {
			continue
		}
		seen[rel] = struct{}{}
		sourcePath := filepath.Join(baseDir, rel)
		if rel == rootRel {
			sourcePath = rootPath
		}
		info, err := os.Stat(sourcePath)
		if err != nil {
			return Manifest{}, "", err
		}
		if info.IsDir() {
			return Manifest{}, "", fmt.Errorf("bundle path %s is a directory", rel)
		}
		bytes, err := os.ReadFile(sourcePath)
		if err != nil {
			return Manifest{}, "", err
		}
		fileHash := sha256.Sum256(bytes)
		manifest.Files = append(manifest.Files, ManifestFile{
			Path:   rel,
			SHA256: hex.EncodeToString(fileHash[:]),
			Size:   info.Size(),
		})
		if err := tw.WriteHeader(&tar.Header{Name: rel, Mode: 0o644, Size: int64(len(bytes))}); err != nil {
			return Manifest{}, "", err
		}
		if _, err := tw.Write(bytes); err != nil {
			return Manifest{}, "", err
		}
	}
	generatedPaths := make([]string, 0, len(generatedFiles))
	for rel := range generatedFiles {
		generatedPaths = append(generatedPaths, rel)
	}
	sort.Strings(generatedPaths)
	for _, rel := range generatedPaths {
		cleanRel, err := safeBundlePath(rel)
		if err != nil {
			return Manifest{}, "", err
		}
		if _, ok := seen[cleanRel]; ok {
			return Manifest{}, "", fmt.Errorf("bundle generated path %s duplicates source file", cleanRel)
		}
		bytes := generatedFiles[rel]
		if err := tw.WriteHeader(&tar.Header{Name: cleanRel, Mode: 0o644, Size: int64(len(bytes))}); err != nil {
			return Manifest{}, "", err
		}
		if _, err := tw.Write(bytes); err != nil {
			return Manifest{}, "", err
		}
	}
	manifestBytes, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return Manifest{}, "", err
	}
	if err := tw.WriteHeader(&tar.Header{Name: "manifest.json", Mode: 0o644, Size: int64(len(manifestBytes))}); err != nil {
		return Manifest{}, "", err
	}
	if _, err := tw.Write(manifestBytes); err != nil {
		return Manifest{}, "", err
	}
	if err := tw.Close(); err != nil {
		return Manifest{}, "", err
	}
	if err := gz.Close(); err != nil {
		return Manifest{}, "", err
	}
	return manifest, hex.EncodeToString(hash.Sum(nil)), nil
}

func ValidateArtifact(path string, workspaceID servingstate.WorkspaceID, servingStateID servingstate.ID) (servingstate.Validation, error) {
	return ValidateArtifactWithOptions(path, workspaceID, servingStateID, ValidateOptions{})
}

func ValidateArtifactWithOptions(path string, workspaceID servingstate.WorkspaceID, servingStateID servingstate.ID, options ValidateOptions) (servingstate.Validation, error) {
	digest, err := fileDigest(path)
	if err != nil {
		return servingstate.Validation{}, err
	}
	root, err := os.MkdirTemp("", "libredash-deploy-*")
	if err != nil {
		return servingstate.Validation{}, err
	}
	if err := ExtractArtifact(path, root); err != nil {
		os.RemoveAll(root)
		return servingstate.Validation{}, err
	}
	manifest, err := readManifest(root)
	if err != nil {
		os.RemoveAll(root)
		return servingstate.Validation{}, err
	}
	if _, err := validateManifestFiles(root, manifest); err != nil {
		os.RemoveAll(root)
		return servingstate.Validation{}, err
	}
	compiled, err := readCompiledWorkspaceArtifact(root, manifest)
	if err != nil {
		os.RemoveAll(root)
		return servingstate.Validation{}, err
	}
	if workspaceID == "" {
		if strings.TrimSpace(manifest.WorkspaceID) == "" {
			os.RemoveAll(root)
			return servingstate.Validation{}, fmt.Errorf("project artifact manifest requires workspaceId")
		}
		workspaceID = servingstate.WorkspaceID(manifest.WorkspaceID)
	}
	if compiled.WorkspaceID != string(workspaceID) {
		os.RemoveAll(root)
		return servingstate.Validation{}, fmt.Errorf("compiled artifact workspace = %q, want %q", compiled.WorkspaceID, workspaceID)
	}
	if strings.TrimSpace(compiled.Environment) == "" {
		os.RemoveAll(root)
		return servingstate.Validation{}, fmt.Errorf("compiled artifact requires environment")
	}
	if strings.TrimSpace(manifest.Environment) == "" {
		os.RemoveAll(root)
		return servingstate.Validation{}, fmt.Errorf("project artifact manifest requires environment")
	}
	if compiled.Environment != manifest.Environment {
		os.RemoveAll(root)
		return servingstate.Validation{}, fmt.Errorf("compiled artifact environment = %q, manifest environment = %q", compiled.Environment, manifest.Environment)
	}
	if options.Environment != "" {
		expectedEnvironment := servingstate.NormalizeEnvironment(options.Environment)
		if servingstate.Environment(compiled.Environment) != expectedEnvironment {
			os.RemoveAll(root)
			return servingstate.Validation{}, fmt.Errorf("compiled artifact environment = %q, want %q", compiled.Environment, expectedEnvironment)
		}
	}
	if compiled.ServingStateID != string(servingStateID) {
		os.RemoveAll(root)
		return servingstate.Validation{}, fmt.Errorf("compiled artifact serving state = %q, want %q", compiled.ServingStateID, servingStateID)
	}
	if err := workspace.ValidateAssetGraphForServingState(compiled.Graph, workspace.WorkspaceID(workspaceID), workspace.ServingStateID(servingStateID)); err != nil {
		os.RemoveAll(root)
		return servingstate.Validation{}, err
	}
	if err := validateCompiledArtifactValidation(compiled); err != nil {
		os.RemoveAll(root)
		return servingstate.Validation{}, err
	}
	if options.DataDir != "" {
		if err := discoverSchemasForDefinition(context.Background(), compiled.Definition, options); err != nil {
			os.RemoveAll(root)
			return servingstate.Validation{}, err
		}
		graph, err := workspacecompiler.ExtractLineage(workspace.WorkspaceID(workspaceID), workspace.ServingStateID(servingStateID), compiled.Definition)
		if err != nil {
			os.RemoveAll(root)
			return servingstate.Validation{}, err
		}
		compiled.Graph = graph
		if err := workspace.ValidateAssetGraphForServingState(compiled.Graph, workspace.WorkspaceID(workspaceID), workspace.ServingStateID(servingStateID)); err != nil {
			os.RemoveAll(root)
			return servingstate.Validation{}, err
		}
		compiled.Validation = CompiledArtifactValidation{
			Status:        "passed",
			GraphHash:     graphHash(compiled.Graph),
			SchemaVersion: projectAPIVersion,
		}
		manifest, digest, err = persistValidatedArtifact(path, root, manifest, compiled)
		if err != nil {
			os.RemoveAll(root)
			return servingstate.Validation{}, err
		}
	}
	manifestJSON, err := json.Marshal(manifest)
	if err != nil {
		os.RemoveAll(root)
		return servingstate.Validation{}, err
	}
	return servingstate.Validation{
		Digest:       digest,
		ManifestJSON: string(manifestJSON),
		RootDir:      root,
		DataRoot:     options.DataDir,
		Graph:        compiled.Graph,
	}, nil
}

func persistValidatedArtifact(path, root string, manifest Manifest, compiled CompiledWorkspaceArtifact) (Manifest, string, error) {
	compiledRel, err := safeBundlePath(manifest.CompiledPath)
	if err != nil {
		return Manifest{}, "", err
	}
	compiledBytes, err := json.MarshalIndent(compiled, "", "  ")
	if err != nil {
		return Manifest{}, "", err
	}
	manifest.GraphHash = digestBytes(compiledBytes)
	if err := os.WriteFile(filepath.Join(root, compiledRel), compiledBytes, 0o644); err != nil {
		return Manifest{}, "", err
	}
	manifestBytes, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return Manifest{}, "", err
	}
	if err := os.WriteFile(filepath.Join(root, "manifest.json"), manifestBytes, 0o644); err != nil {
		return Manifest{}, "", err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".libredash-validated-*.tar.gz")
	if err != nil {
		return Manifest{}, "", err
	}
	tmpPath := tmp.Name()
	if err := writeExtractedRoot(root, tmp); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return Manifest{}, "", err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return Manifest{}, "", err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return Manifest{}, "", err
	}
	digest, err := fileDigest(path)
	if err != nil {
		return Manifest{}, "", err
	}
	return manifest, digest, nil
}

func writeExtractedRoot(root string, out io.Writer) error {
	hash := sha256.New()
	gz := gzip.NewWriter(io.MultiWriter(out, hash))
	tw := tar.NewWriter(gz)
	paths := []string{}
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		paths = append(paths, filepath.ToSlash(rel))
		return nil
	}); err != nil {
		return err
	}
	sort.Strings(paths)
	for _, rel := range paths {
		bytes, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			return err
		}
		if err := tw.WriteHeader(&tar.Header{Name: rel, Mode: 0o644, Size: int64(len(bytes))}); err != nil {
			return err
		}
		if _, err := tw.Write(bytes); err != nil {
			return err
		}
	}
	if err := tw.Close(); err != nil {
		return err
	}
	return gz.Close()
}

func validateCompiledArtifactValidation(compiled CompiledWorkspaceArtifact) error {
	if compiled.Validation.Status != "passed" {
		return fmt.Errorf("compiled artifact validation status = %q, want passed", compiled.Validation.Status)
	}
	if compiled.Validation.SchemaVersion != projectAPIVersion {
		return fmt.Errorf("compiled artifact validation schemaVersion = %q, want %q", compiled.Validation.SchemaVersion, projectAPIVersion)
	}
	if want := graphHash(compiled.Graph); compiled.Validation.GraphHash != want {
		return fmt.Errorf("compiled artifact validation graphHash = %q, want %q", compiled.Validation.GraphHash, want)
	}
	return nil
}

func ValidateCompiledWorkspaceArtifact(compiled CompiledWorkspaceArtifact) error {
	return validateCompiledArtifactValidation(compiled)
}

const projectAPIVersion = "libredash.dev/v1"

func discoverSchemasForDefinition(ctx context.Context, definition *workspace.Definition, options ValidateOptions) error {
	duckDBRoot := options.DuckDBDir
	removeDuckDBRoot := false
	if duckDBRoot == "" {
		var err error
		duckDBRoot, err = os.MkdirTemp("", "libredash-schema-*")
		if err != nil {
			return err
		}
		removeDuckDBRoot = true
	}
	if removeDuckDBRoot {
		defer os.RemoveAll(duckDBRoot)
	}
	runtime, err := analyticsduckdb.OpenWorkspaceMaterializeRuntime(ctx, analyticsduckdb.WorkspaceRuntimeConfig{
		Models:  definition.Models,
		DataDir: options.DataDir,
		DBDir:   duckDBRoot,
	})
	if err != nil {
		return err
	}
	if err := runtime.Close(); err != nil {
		return err
	}
	return nil
}

func ExtractArtifact(path, dest string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	gz, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		rel, err := safeBundlePath(header.Name)
		if err != nil {
			return err
		}
		target := filepath.Join(dest, rel)
		if !strings.HasPrefix(target, filepath.Clean(dest)+string(os.PathSeparator)) && filepath.Clean(target) != filepath.Clean(dest) {
			return fmt.Errorf("bundle path %q escapes destination", header.Name)
		}
		switch header.Typeflag {
		case tar.TypeReg, tar.TypeRegA:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return err
			}
			if err := out.Close(); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unsupported bundle entry %q", header.Name)
		}
	}
}

func readManifest(root string) (Manifest, error) {
	bytes, err := os.ReadFile(filepath.Join(root, "manifest.json"))
	if err != nil {
		return Manifest{}, err
	}
	var manifest Manifest
	if err := json.Unmarshal(bytes, &manifest); err != nil {
		return Manifest{}, err
	}
	if manifest.CatalogPath == "" {
		manifest.CatalogPath = ProjectFile
	}
	if manifest.CompiledPath == "" {
		manifest.CompiledPath = CompiledProjectFile
	}
	return manifest, nil
}

func LoadCompiledWorkspaceArtifact(root string) (CompiledWorkspaceArtifact, Manifest, error) {
	manifest, err := readManifest(root)
	if err != nil {
		return CompiledWorkspaceArtifact{}, Manifest{}, err
	}
	compiled, err := readCompiledWorkspaceArtifact(root, manifest)
	if err != nil {
		return CompiledWorkspaceArtifact{}, Manifest{}, err
	}
	return compiled, manifest, nil
}

func readCompiledWorkspaceArtifact(root string, manifest Manifest) (CompiledWorkspaceArtifact, error) {
	compiledRel, err := safeBundlePath(manifest.CompiledPath)
	if err != nil {
		return CompiledWorkspaceArtifact{}, fmt.Errorf("invalid compiled path: %w", err)
	}
	bytes, err := os.ReadFile(filepath.Join(root, compiledRel))
	if err != nil {
		return CompiledWorkspaceArtifact{}, err
	}
	if manifest.GraphHash != "" && digestBytes(bytes) != manifest.GraphHash {
		return CompiledWorkspaceArtifact{}, fmt.Errorf("compiled artifact digest mismatch")
	}
	var compiled CompiledWorkspaceArtifact
	if err := json.Unmarshal(bytes, &compiled); err != nil {
		return CompiledWorkspaceArtifact{}, err
	}
	if compiled.Version != 1 {
		return CompiledWorkspaceArtifact{}, fmt.Errorf("compiled artifact version = %d, want 1", compiled.Version)
	}
	if compiled.Definition == nil {
		return CompiledWorkspaceArtifact{}, fmt.Errorf("compiled artifact definition is required")
	}
	return compiled, nil
}

func validateManifestFiles(root string, manifest Manifest) (string, error) {
	catalogRel, err := safeBundlePath(manifest.CatalogPath)
	if err != nil {
		return "", fmt.Errorf("invalid catalog path: %w", err)
	}
	seen := map[string]struct{}{}
	hasCatalog := false
	for _, file := range manifest.Files {
		rel, err := safeBundlePath(file.Path)
		if err != nil {
			return "", fmt.Errorf("invalid manifest file path %q: %w", file.Path, err)
		}
		if _, ok := seen[rel]; ok {
			return "", fmt.Errorf("duplicate manifest file path %q", rel)
		}
		seen[rel] = struct{}{}
		if rel == catalogRel {
			hasCatalog = true
		}
		path := filepath.Join(root, rel)
		bytes, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		sum := sha256.Sum256(bytes)
		if got := hex.EncodeToString(sum[:]); got != file.SHA256 {
			return "", fmt.Errorf("file %s digest mismatch", file.Path)
		}
	}
	if !hasCatalog {
		return "", fmt.Errorf("catalog path %q is not listed in manifest files", manifest.CatalogPath)
	}
	return catalogRel, nil
}

func fileDigest(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func digestBytes(bytes []byte) string {
	sum := sha256.Sum256(bytes)
	return hex.EncodeToString(sum[:])
}

func graphHash(graph workspace.AssetGraph) string {
	bytes, err := json.Marshal(graph)
	if err != nil {
		return ""
	}
	return digestBytes(bytes)
}

func projectPlanWorkspace(plan workspacecompiler.ProjectPlan, workspaceID string) (workspacecompiler.ProjectPlanWorkspace, bool) {
	for _, workspacePlan := range plan.Workspaces {
		if workspacePlan.ID == workspaceID {
			return workspacePlan, true
		}
	}
	return workspacecompiler.ProjectPlanWorkspace{}, false
}

func cleanBundlePath(path string) string {
	path = filepath.ToSlash(filepath.Clean(path))
	path = strings.TrimPrefix(path, "/")
	path = strings.TrimPrefix(path, "../")
	return path
}

func safeBundlePath(path string) (string, error) {
	if filepath.IsAbs(path) {
		return "", fmt.Errorf("bundle path %q must be relative", path)
	}
	clean := filepath.ToSlash(filepath.Clean(path))
	if clean == "." || clean == "" {
		return "", fmt.Errorf("bundle path %q is empty", path)
	}
	for _, part := range strings.Split(clean, "/") {
		if part == ".." {
			return "", fmt.Errorf("bundle path %q escapes bundle root", path)
		}
	}
	return clean, nil
}
