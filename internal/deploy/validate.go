package deploy

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/Yacobolo/libredash/internal/semantic"
)

func ValidateArtifact(path, workspaceID, deploymentID string) (Validation, error) {
	digest, err := fileDigest(path)
	if err != nil {
		return Validation{}, err
	}
	root, err := os.MkdirTemp("", "libredash-deploy-*")
	if err != nil {
		return Validation{}, err
	}
	if err := ExtractArtifact(path, root); err != nil {
		os.RemoveAll(root)
		return Validation{}, err
	}
	manifest, err := readManifest(root)
	if err != nil {
		os.RemoveAll(root)
		return Validation{}, err
	}
	catalogRel, err := validateManifestFiles(root, manifest)
	if err != nil {
		os.RemoveAll(root)
		return Validation{}, err
	}
	if workspaceID == "" {
		workspaceID = workspaceIDOrDefault(manifest.WorkspaceID)
	}
	catalogPath := filepath.Join(root, catalogRel)
	workspace, err := semantic.LoadWorkspace(catalogPath)
	if err != nil {
		os.RemoveAll(root)
		return Validation{}, err
	}
	assets, edges, err := ExtractAssets(workspaceID, deploymentID, workspace)
	if err != nil {
		os.RemoveAll(root)
		return Validation{}, err
	}
	manifestJSON, err := json.Marshal(manifest)
	if err != nil {
		os.RemoveAll(root)
		return Validation{}, err
	}
	return Validation{
		Manifest:     manifest,
		ManifestJSON: string(manifestJSON),
		Digest:       digest,
		CatalogPath:  catalogPath,
		RootDir:      root,
		Assets:       assets,
		Edges:        edges,
	}, nil
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
		manifest.CatalogPath = CatalogFile
	}
	return manifest, nil
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
