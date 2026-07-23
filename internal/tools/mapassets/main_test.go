package main

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestVerifyFileFailsClosedOnDigestMismatch(t *testing.T) {
	name := filepath.Join(t.TempDir(), "asset")
	if err := os.WriteFile(name, []byte("map"), 0o644); err != nil {
		t.Fatal(err)
	}
	digest := fmt.Sprintf("%x", sha256.Sum256([]byte("map")))
	if err := verifyFile(name, digest); err != nil {
		t.Fatal(err)
	}
	if err := verifyFile(name, archiveDigest); err == nil {
		t.Fatal("digest mismatch accepted")
	}
}

func TestGlyphPackageCoversBasemapScriptsObservedAtRuntime(t *testing.T) {
	want := []string{"0-255", "256-511", "512-767", "768-1023", "1024-1279", "1280-1535", "1536-1791", "4096-4351", "11520-11775"}
	present := make(map[string]bool, len(glyphRanges))
	for _, value := range glyphRanges {
		present[value] = true
	}
	for _, value := range want {
		if !present[value] {
			t.Errorf("glyph package missing Unicode range %s", value)
		}
	}
}

func TestInstallTargetsContentAddressedPackagePaths(t *testing.T) {
	root := t.TempDir()
	archive, err := assetTarget(root, "/map-assets/leapview-streets/archives/"+archiveDigest+"/basemap.pmtiles")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(root, "leapview-streets", "archives", archiveDigest, "basemap.pmtiles")
	if archive != want {
		t.Fatalf("asset target = %q, want %q", archive, want)
	}
	if _, err := assetTarget(root, "/map-assets/leapview-streets/../../secret"); err == nil {
		t.Fatal("assetTarget accepted traversal")
	}
}

func TestRegionalExtractionProfileExtendsTheGlobalArchive(t *testing.T) {
	if regionalBounds != "-82,-56,-30,14" || regionalMinimumZoom != "7" || regionalMaximumZoom != "10" {
		t.Fatalf("regional profile = bounds %q zoom %s..%s", regionalBounds, regionalMinimumZoom, regionalMaximumZoom)
	}
	if archiveDigest == globalArchiveDigest {
		t.Fatal("regional detail was not merged into a distinct immutable archive")
	}
}
