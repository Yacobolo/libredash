// Package geometry owns the immutable, content-addressed geographic assets
// available to compiled visualization specifications.
package geometry

import (
	"fmt"

	visualizationir "github.com/Yacobolo/leapview/internal/visualization/ir"
)

var assets = map[string]visualizationir.VisualizationGeometryAsset{
	"world_countries": {
		ID:               "world-countries-natural-earth-110m",
		Digest:           "sha256:49a9d42f383d3d567bf1a19189c6f8e14f14f880dc6d972bab577d13ceebb2e7",
		Source:           "Natural Earth 1:110m Admin 0 Countries, pinned at ca96624a56bd078437bca8184e78163e5039ad19",
		License:          "Public domain",
		Attribution:      "Natural Earth",
		IdentifierSystem: "natural-earth-adm0-a3",
		URL:              "/static/geometry/world-countries-natural-earth-110m.geojson",
	},
	"brazil_states": {
		ID:               "br-states-ibge",
		Digest:           "sha256:48af4bcdf332d11e5ad250f954ffead366791b3738376bafd34f4298c4337b92",
		Source:           "IBGE API de Malhas",
		License:          "IBGE data reuse terms",
		Attribution:      "Instituto Brasileiro de Geografia e Estatística (IBGE)",
		IdentifierSystem: "br-uf",
		URL:              "/static/geometry/br-states-ibge.geojson",
	},
}

// Resolve returns the complete provenance record for a public authoring asset.
func Resolve(id string) (visualizationir.VisualizationGeometryAsset, error) {
	asset, ok := assets[id]
	if !ok {
		return visualizationir.VisualizationGeometryAsset{}, fmt.Errorf("unknown geometry asset %q", id)
	}
	return asset, nil
}
