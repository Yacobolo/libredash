package deploy

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Yacobolo/libredash/internal/platform"
	"github.com/Yacobolo/libredash/internal/semantic"
)

const (
	BundleFormat = "tar.gz"
	CatalogFile  = "catalog.yaml"
)

type Manifest struct {
	Version        int            `json:"version"`
	WorkspaceID    string         `json:"workspaceId"`
	WorkspaceTitle string         `json:"workspaceTitle"`
	CatalogPath    string         `json:"catalogPath"`
	Files          []ManifestFile `json:"files"`
	SemanticModels []string       `json:"semanticModels"`
	MetricViews    []string       `json:"metricViews"`
	Dashboards     []string       `json:"dashboards"`
}

type ManifestFile struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

type Validation struct {
	Manifest     Manifest
	ManifestJSON string
	Digest       string
	CatalogPath  string
	RootDir      string
	Assets       []platform.Asset
	Edges        []platform.AssetEdge
}

func PackCatalog(catalogPath string, out io.Writer) (Manifest, string, error) {
	catalogPath, err := filepath.Abs(catalogPath)
	if err != nil {
		return Manifest{}, "", err
	}
	workspace, err := semantic.LoadWorkspace(catalogPath)
	if err != nil {
		return Manifest{}, "", err
	}
	baseDir := filepath.Dir(catalogPath)
	relFiles := []string{CatalogFile}
	for _, model := range workspace.Catalog.SemanticModels {
		relFiles = append(relFiles, cleanBundlePath(model.Path))
	}
	for _, view := range workspace.Catalog.MetricViews {
		relFiles = append(relFiles, cleanBundlePath(view.Path))
	}
	for _, report := range workspace.Catalog.Dashboards {
		relFiles = append(relFiles, cleanBundlePath(report.Path))
	}
	sort.Strings(relFiles[1:])

	hash := sha256.New()
	mw := io.MultiWriter(out, hash)
	gz := gzip.NewWriter(mw)
	tw := tar.NewWriter(gz)
	manifest := Manifest{
		Version:        1,
		WorkspaceID:    workspaceID(workspace.Catalog.Workspace.ID),
		WorkspaceTitle: workspaceTitle(workspace.Catalog.Workspace.Title),
		CatalogPath:    CatalogFile,
		Files:          make([]ManifestFile, 0, len(relFiles)),
	}
	for _, model := range workspace.Catalog.SemanticModels {
		manifest.SemanticModels = append(manifest.SemanticModels, model.ID)
	}
	for _, view := range workspace.Catalog.MetricViews {
		manifest.MetricViews = append(manifest.MetricViews, view.ID)
	}
	for _, report := range workspace.Catalog.Dashboards {
		manifest.Dashboards = append(manifest.Dashboards, report.ID)
	}

	seen := map[string]struct{}{}
	for _, rel := range relFiles {
		if _, ok := seen[rel]; ok {
			continue
		}
		seen[rel] = struct{}{}
		sourcePath := filepath.Join(baseDir, rel)
		if rel == CatalogFile {
			sourcePath = catalogPath
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
		if err := tw.WriteHeader(&tar.Header{
			Name: rel,
			Mode: 0o644,
			Size: int64(len(bytes)),
		}); err != nil {
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

func workspaceID(value string) string {
	if strings.TrimSpace(value) != "" {
		return value
	}
	return platform.DefaultWorkspaceID
}

func workspaceIDOrDefault(value string) string {
	return workspaceID(value)
}

func workspaceTitle(value string) string {
	if strings.TrimSpace(value) != "" {
		return value
	}
	return "LibreDash Workspace"
}
