package deploy

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestPackValidateAndExtractAssets(t *testing.T) {
	var buf bytes.Buffer
	manifest, digest, err := PackCatalog(filepath.Join("..", "..", "dashboards", "catalog.yaml"), &buf)
	if err != nil {
		t.Fatalf("pack catalog: %v", err)
	}
	if manifest.WorkspaceID != "libredash" {
		t.Fatalf("workspace id = %q, want libredash", manifest.WorkspaceID)
	}
	if digest == "" {
		t.Fatal("empty bundle digest")
	}
	path := filepath.Join(t.TempDir(), "bundle.tar.gz")
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("write bundle: %v", err)
	}
	validation, err := ValidateArtifact(path, "libredash", "dep_test")
	if err != nil {
		t.Fatalf("validate artifact: %v", err)
	}
	defer os.RemoveAll(validation.RootDir)

	types := map[string]bool{}
	for _, asset := range validation.Assets {
		types[asset.Type] = true
	}
	for _, typ := range []string{"catalog", "semantic_model", "metric_view", "dashboard", "page", "visual", "filter", "table", "model_table", "measure", "dimension", "source", "connection"} {
		if !types[typ] {
			t.Fatalf("missing asset type %q", typ)
		}
	}
	if len(validation.Edges) == 0 {
		t.Fatal("expected lineage edges")
	}
	hasSourceConnectionEdge := false
	for _, edge := range validation.Edges {
		if edge.Type == "uses_connection" {
			hasSourceConnectionEdge = true
			break
		}
	}
	if !hasSourceConnectionEdge {
		t.Fatal("expected source to connection lineage edge")
	}
}

func TestExtractArtifactRejectsTraversal(t *testing.T) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	payload := []byte("bad")
	if err := tw.WriteHeader(&tar.Header{Name: "../bad.yaml", Mode: 0o644, Size: int64(len(payload))}); err != nil {
		t.Fatalf("write header: %v", err)
	}
	if _, err := tw.Write(payload); err != nil {
		t.Fatalf("write payload: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("close gzip: %v", err)
	}
	path := filepath.Join(t.TempDir(), "bad.tar.gz")
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("write bundle: %v", err)
	}
	if err := ExtractArtifact(path, t.TempDir()); err == nil {
		t.Fatal("expected traversal bundle to be rejected")
	}
}

func TestValidateArtifactRejectsUnsafeManifestCatalogPath(t *testing.T) {
	manifest := validManifest(t)
	manifest.CatalogPath = "../catalog.yaml"
	path := writeManifestBundle(t, manifest)
	if _, err := ValidateArtifact(path, "libredash", "dep_test"); err == nil {
		t.Fatal("expected unsafe catalog path to be rejected")
	}
}

func TestValidateArtifactRejectsUnsafeManifestFilePath(t *testing.T) {
	manifest := validManifest(t)
	manifest.Files = append(manifest.Files, ManifestFile{Path: "../bad.yaml", SHA256: "bad"})
	path := writeManifestBundle(t, manifest)
	if _, err := ValidateArtifact(path, "libredash", "dep_test"); err == nil {
		t.Fatal("expected unsafe manifest file path to be rejected")
	}
}

func TestValidateArtifactRejectsDuplicateManifestFilePath(t *testing.T) {
	manifest := validManifest(t)
	manifest.Files = append(manifest.Files, manifest.Files[0])
	path := writeManifestBundle(t, manifest)
	if _, err := ValidateArtifact(path, "libredash", "dep_test"); err == nil {
		t.Fatal("expected duplicate manifest file path to be rejected")
	}
}

func TestValidateArtifactRejectsMissingManifestCatalogFile(t *testing.T) {
	manifest := validManifest(t)
	manifest.Files = manifest.Files[1:]
	path := writeManifestBundle(t, manifest)
	if _, err := ValidateArtifact(path, "libredash", "dep_test"); err == nil {
		t.Fatal("expected missing catalog manifest entry to be rejected")
	}
}

func TestValidateArtifactRejectsManifestDigestMismatch(t *testing.T) {
	manifest := validManifest(t)
	manifest.Files[0].SHA256 = "not-a-real-digest"
	path := writeManifestBundle(t, manifest)
	if _, err := ValidateArtifact(path, "libredash", "dep_test"); err == nil {
		t.Fatal("expected digest mismatch to be rejected")
	}
}

func validManifest(t *testing.T) Manifest {
	t.Helper()
	var buf bytes.Buffer
	manifest, _, err := PackCatalog(filepath.Join("..", "..", "dashboards", "catalog.yaml"), &buf)
	if err != nil {
		t.Fatalf("pack catalog: %v", err)
	}
	return manifest
}

func writeManifestBundle(t *testing.T, manifest Manifest) string {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	baseDir := filepath.Join("..", "..", "dashboards")
	written := map[string]struct{}{}
	for _, file := range manifest.Files {
		rel, err := safeBundlePath(file.Path)
		if err != nil {
			continue
		}
		if _, ok := written[rel]; ok {
			continue
		}
		written[rel] = struct{}{}
		bytes, err := os.ReadFile(filepath.Join(baseDir, rel))
		if err != nil {
			continue
		}
		if err := tw.WriteHeader(&tar.Header{Name: rel, Mode: 0o644, Size: int64(len(bytes))}); err != nil {
			t.Fatalf("write header: %v", err)
		}
		if _, err := tw.Write(bytes); err != nil {
			t.Fatalf("write file: %v", err)
		}
	}
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	if err := tw.WriteHeader(&tar.Header{Name: "manifest.json", Mode: 0o644, Size: int64(len(manifestBytes))}); err != nil {
		t.Fatalf("write manifest header: %v", err)
	}
	if _, err := tw.Write(manifestBytes); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("close gzip: %v", err)
	}
	path := filepath.Join(t.TempDir(), "bundle.tar.gz")
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("write bundle: %v", err)
	}
	return path
}
