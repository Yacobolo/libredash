package mapasset

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveReturnsImmutableSameOriginManifest(t *testing.T) {
	asset, err := Resolve("streets")
	if err != nil {
		t.Fatal(err)
	}
	if asset.ID != "libredash-streets" || asset.StyleURL[0] != '/' || asset.ArchiveURL[0] != '/' || len(asset.ArchiveDigest) != 71 {
		t.Fatalf("asset = %#v", asset)
	}
	styleDigest := strings.TrimPrefix(asset.StyleDigest, "sha256:")
	archiveDigest := strings.TrimPrefix(asset.ArchiveDigest, "sha256:")
	if !strings.Contains(asset.StyleURL, "/styles/"+styleDigest+"/") {
		t.Fatalf("style URL is not content addressed: %q", asset.StyleURL)
	}
	if !strings.Contains(asset.ArchiveURL, "/archives/"+archiveDigest+"/") {
		t.Fatalf("archive URL is not content addressed: %q", asset.ArchiveURL)
	}
	if !strings.Contains(asset.GlyphsURL, "/assets/"+BasemapAssetsRevision+"/") || !strings.Contains(asset.SpriteURL, "/assets/"+BasemapAssetsRevision+"/") {
		t.Fatalf("supporting asset URLs are not revision addressed: %#v", asset)
	}
	if _, err := Resolve("remote"); err == nil {
		t.Fatal("unknown map style asset accepted")
	}
}

func TestVerifyInstalledFailsClosedForMissingOrChangedAssets(t *testing.T) {
	root := t.TempDir()
	files := ExpectedFiles()
	if len(files) < 3 {
		t.Fatalf("expected complete basemap package, got %d files", len(files))
	}
	for _, file := range files {
		path := filepath.Join(root, filepath.FromSlash(file.Path))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("wrong"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := VerifyInstalled(root); err == nil || !strings.Contains(err.Error(), "digest mismatch") {
		t.Fatalf("VerifyInstalled() error = %v, want digest mismatch", err)
	}
	if err := os.Remove(filepath.Join(root, filepath.FromSlash(files[0].Path))); err != nil {
		t.Fatal(err)
	}
	if err := VerifyInstalled(root); err == nil || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("VerifyInstalled() error = %v, want missing asset", err)
	}
}

func TestContentAddressedURLPathRejectsLegacyAndForgedPaths(t *testing.T) {
	asset, err := Resolve("streets")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{asset.StyleURL, asset.ArchiveURL, strings.ReplaceAll(asset.GlyphsURL, "{fontstack}", "Noto%20Sans%20Regular"), asset.SpriteURL + ".png"} {
		path = strings.ReplaceAll(path, "{range}", "0-255")
		if !IsContentAddressedURLPath(path) {
			t.Errorf("trusted path rejected: %q", path)
		}
	}
	for _, path := range []string{"/map-assets/libredash-streets/style.json", "/map-assets/libredash-streets/basemap.pmtiles", "/map-assets/other/" + strings.Repeat("a", 64) + "/style.json", "/map-assets/libredash-streets/../../secret"} {
		if IsContentAddressedURLPath(path) {
			t.Errorf("untrusted path accepted: %q", path)
		}
	}
}
