// Package visualdocs defines the generated contract shared by the visual
// documentation generator and the static documentation site.
package visualdocs

import (
	"encoding/json"
	"fmt"

	"github.com/Yacobolo/leapview/internal/dashboard"
)

const ArtifactVersion = 4

type Artifact struct {
	Version    int                          `json:"version"`
	Documents  map[string][]Payload         `json:"documents"`
	References map[string]DocumentReference `json:"references"`
	Showcase   []Payload                    `json:"showcase"`
}

// Payload is the generated, type-discriminated visual documentation union.
// Its JSON representation is deliberately flat so the site consumes the same
// public payload shape as dashboards and agents.
type Payload struct {
	ID      string
	Type    string
	Chart   *dashboard.Visual
	Tabular *dashboard.TabularVisual
}

func ChartPayload(value dashboard.Visual) Payload {
	return Payload{ID: value.ID, Type: value.Type, Chart: &value}
}

func TabularPayload(value dashboard.TabularVisual) Payload {
	return Payload{ID: value.ID, Type: value.Type, Tabular: &value}
}

func (p Payload) MarshalJSON() ([]byte, error) {
	if p.Tabular != nil {
		return json.Marshal(p.Tabular)
	}
	if p.Chart != nil {
		return json.Marshal(p.Chart)
	}
	return nil, fmt.Errorf("visual documentation payload %q has no variant", p.ID)
}

func (p *Payload) UnmarshalJSON(data []byte) error {
	var tag struct {
		ID   string `json:"id"`
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &tag); err != nil {
		return err
	}
	p.ID, p.Type = tag.ID, tag.Type
	if tag.Type == "table" || tag.Type == "matrix" || tag.Type == "pivot" {
		var value dashboard.TabularVisual
		if err := json.Unmarshal(data, &value); err != nil {
			return err
		}
		p.Tabular = &value
		return nil
	}
	var value dashboard.Visual
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	p.Chart = &value
	return nil
}

type DocumentReference struct {
	Kind          string                      `json:"kind"`
	Renderer      string                      `json:"renderer"`
	Shapes        []string                    `json:"shapes"`
	QueryFields   []string                    `json:"queryFields"`
	Options       []string                    `json:"options"`
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
