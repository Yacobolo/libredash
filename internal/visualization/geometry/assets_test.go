package geometry

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"strings"
	"testing"
)

func TestWorldBasemapHasPinnedPublicDomainProvenance(t *testing.T) {
	asset, err := Resolve("world_countries")
	if err != nil {
		t.Fatal(err)
	}
	if asset.ID != "world-countries-natural-earth-110m" || asset.License != "Public domain" || asset.Attribution != "Natural Earth" {
		t.Fatalf("world basemap provenance = %#v", asset)
	}
	if asset.Digest == "" || asset.URL != "/static/geometry/world-countries-natural-earth-110m.geojson" {
		t.Fatalf("world basemap identity = %#v", asset)
	}
	if !strings.Contains(asset.Source, "ca96624a56bd078437bca8184e78163e5039ad19") {
		t.Fatalf("world basemap source is not pinned: %q", asset.Source)
	}
	data, err := os.ReadFile("../../../static/geometry/world-countries-natural-earth-110m.geojson")
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(data)
	if got := "sha256:" + hex.EncodeToString(digest[:]); got != asset.Digest {
		t.Fatalf("world basemap digest = %q, want %q", got, asset.Digest)
	}
}
