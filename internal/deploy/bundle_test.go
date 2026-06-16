package deploy

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
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
	for _, typ := range []string{"catalog", "semantic_model", "dashboard", "page", "visual", "kpi", "filter", "table", "dataset", "measure", "dimension", "cache_table", "source"} {
		if !types[typ] {
			t.Fatalf("missing asset type %q", typ)
		}
	}
	if len(validation.Edges) == 0 {
		t.Fatal("expected lineage edges")
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
