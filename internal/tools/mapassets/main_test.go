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
