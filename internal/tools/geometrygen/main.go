// Command geometrygen vendors the pinned, authoritative geometry used by
// first-party geographic visualizations and their offline basemap.
package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"
)

const (
	brazilSourceURL = "https://servicodados.ibge.gov.br/api/v3/malhas/paises/BR?formato=application%2Fvnd.geo%2Bjson&qualidade=minima&intrarregiao=UF"
	worldSourceURL  = "https://raw.githubusercontent.com/nvkelso/natural-earth-vector/ca96624a56bd078437bca8184e78163e5039ad19/geojson/ne_110m_admin_0_countries.geojson"
)

var stateCodes = map[string]string{
	"11": "RO", "12": "AC", "13": "AM", "14": "RR", "15": "PA", "16": "AP", "17": "TO",
	"21": "MA", "22": "PI", "23": "CE", "24": "RN", "25": "PB", "26": "PE", "27": "AL", "28": "SE", "29": "BA",
	"31": "MG", "32": "ES", "33": "RJ", "35": "SP", "41": "PR", "42": "SC", "43": "RS",
	"50": "MS", "51": "MT", "52": "GO", "53": "DF",
}

type featureCollection struct {
	Type     string    `json:"type"`
	Features []feature `json:"features"`
	Metadata metadata  `json:"leapview"`
}
type feature struct {
	Type       string          `json:"type"`
	ID         string          `json:"id"`
	Geometry   json.RawMessage `json:"geometry"`
	Properties map[string]any  `json:"properties"`
}
type metadata struct {
	Source           string `json:"source"`
	Attribution      string `json:"attribution"`
	License          string `json:"license"`
	IdentifierSystem string `json:"identifierSystem"`
}

func main() {
	client := &http.Client{Timeout: 30 * time.Second}
	generateBrazilStates(client)
	generateWorldCountries(client)
}

func generateBrazilStates(client *http.Client) {
	response, err := client.Get(brazilSourceURL)
	if err != nil {
		panic(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		panic(fmt.Errorf("IBGE geometry returned %s", response.Status))
	}
	var collection featureCollection
	decoder := json.NewDecoder(response.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&collection); err != nil {
		panic(err)
	}
	if collection.Type != "FeatureCollection" || len(collection.Features) != 27 {
		panic(fmt.Errorf("unexpected IBGE state collection with %d features", len(collection.Features)))
	}
	for index := range collection.Features {
		code, _ := collection.Features[index].Properties["codarea"].(string)
		abbreviation := stateCodes[code]
		if abbreviation == "" {
			panic(fmt.Errorf("unknown IBGE state code %q", code))
		}
		collection.Features[index].ID = abbreviation
		collection.Features[index].Properties["id"] = abbreviation
	}
	collection.Metadata = metadata{Source: brazilSourceURL, Attribution: "Instituto Brasileiro de Geografia e Estatística (IBGE)", License: "IBGE data reuse terms", IdentifierSystem: "br-uf"}
	writeCollection("static/geometry/br-states-ibge.geojson", collection)
}

func generateWorldCountries(client *http.Client) {
	response, err := client.Get(worldSourceURL)
	if err != nil {
		panic(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		panic(fmt.Errorf("Natural Earth geometry returned %s", response.Status))
	}
	var source struct {
		Type     string `json:"type"`
		Features []struct {
			Type       string          `json:"type"`
			Geometry   json.RawMessage `json:"geometry"`
			Properties map[string]any  `json:"properties"`
		} `json:"features"`
	}
	if err := json.NewDecoder(response.Body).Decode(&source); err != nil {
		panic(err)
	}
	if source.Type != "FeatureCollection" || len(source.Features) != 177 {
		panic(fmt.Errorf("unexpected Natural Earth country collection with %d features", len(source.Features)))
	}
	collection := featureCollection{Type: "FeatureCollection", Features: make([]feature, len(source.Features))}
	seen := make(map[string]struct{}, len(source.Features))
	for index, sourceFeature := range source.Features {
		if sourceFeature.Type != "Feature" || len(sourceFeature.Geometry) == 0 || !json.Valid(sourceFeature.Geometry) {
			panic(fmt.Errorf("invalid Natural Earth feature %d", index))
		}
		code, _ := sourceFeature.Properties["ADM0_A3"].(string)
		if code == "" || code == "-99" {
			code = fmt.Sprintf("NE-%03d", index)
		}
		if _, exists := seen[code]; exists {
			panic(fmt.Errorf("duplicate Natural Earth country code %q", code))
		}
		seen[code] = struct{}{}
		name, _ := sourceFeature.Properties["ADMIN"].(string)
		if name == "" {
			name, _ = sourceFeature.Properties["NAME"].(string)
		}
		if name == "" {
			panic(fmt.Errorf("Natural Earth feature %d has no name", index))
		}
		collection.Features[index] = feature{
			Type: "Feature", ID: code, Geometry: sourceFeature.Geometry,
			Properties: map[string]any{"id": code, "name": name},
		}
	}
	collection.Metadata = metadata{Source: worldSourceURL, Attribution: "Natural Earth", License: "Public domain", IdentifierSystem: "natural-earth-adm0-a3"}
	writeCollection("static/geometry/world-countries-natural-earth-110m.geojson", collection)
}

func writeCollection(path string, collection featureCollection) {
	data, err := json.Marshal(collection)
	if err != nil {
		panic(err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		panic(err)
	}
}
