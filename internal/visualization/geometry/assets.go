// Package geometry owns the immutable, content-addressed geographic assets
// available to compiled visualization specifications.
package geometry

import (
	"fmt"

	visualizationir "github.com/Yacobolo/libredash/internal/visualization/ir"
)

var assets = map[string]visualizationir.VisualizationGeometryAsset{
	"brazil_states": {
		ID:               "br-states-ibge",
		Digest:           "sha256:439a3603cf12f49707a34821c68a170f04de75dbe3e8dfcd1a8af7f85df86964",
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
