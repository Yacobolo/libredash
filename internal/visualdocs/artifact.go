// Package visualdocs defines the generated contract shared by the visual
// documentation generator and the static documentation site.
package visualdocs

import visualizationir "github.com/Yacobolo/leapview/internal/visualization/ir"

const ArtifactVersion = 6

type Artifact struct {
	Version    int                          `json:"version"`
	Documents  map[string][]Payload         `json:"documents"`
	References map[string]DocumentReference `json:"references"`
	Showcase   []Payload                    `json:"showcase"`
}

// Payload is the same validated envelope consumed by dashboards, APIs, agent
// artifacts, and the visualization host.
type Payload = visualizationir.VisualizationEnvelope

type DocumentReference struct {
	Kind          string                      `json:"kind"`
	Renderer      string                      `json:"renderer"`
	Shapes        []string                    `json:"shapes"`
	QueryFields   []string                    `json:"queryFields"`
	Presentation  []string                    `json:"presentation"`
	Fields        []FieldReference            `json:"fields"`
	Accessibility string                      `json:"accessibility"`
	Examples      map[string]ExampleReference `json:"examples"`
}

type FieldReference struct {
	Path          string   `json:"path"`
	Type          string   `json:"type"`
	Default       string   `json:"default,omitempty"`
	AllowedValues []string `json:"allowedValues,omitempty"`
	Description   string   `json:"description"`
}

type ExampleReference struct {
	KeyFields []string `json:"keyFields"`
}
