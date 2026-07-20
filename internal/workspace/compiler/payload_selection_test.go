package compiler

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/Yacobolo/leapview/internal/dashboard/report"
)

func TestVisualPayloadIncludesPointSelectionContract(t *testing.T) {
	visual := report.Visual{Interaction: report.Interaction{PointSelection: report.SelectionInteraction{
		Toggle: true,
		Mappings: []report.SelectionMapping{{
			Field: "activity_date",
			Grain: "month",
			Value: "label",
			Label: "label",
		}},
		Targets: []string{"tags_per_rating"},
	}}}

	payload, err := json.Marshal(visualPayload(visual))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"Interaction"`, `"activity_date"`, `"month"`, `"tags_per_rating"`} {
		if !bytes.Contains(payload, []byte(want)) {
			t.Fatalf("visual payload = %s, want %s", payload, want)
		}
	}
}

func TestTablePayloadIncludesFactLocalRowSelectionContract(t *testing.T) {
	table := report.TableVisual{Interaction: report.Interaction{RowSelection: report.SelectionInteraction{
		Mappings: []report.SelectionMapping{{
			Field: "ratings.rating_bucket",
			Fact:  "ratings",
			Value: "rating_bucket",
		}},
		Targets: []string{"tags_per_rating"},
	}}}

	payload, err := json.Marshal(tableVisualPayload(table))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"Interaction"`, `"ratings.rating_bucket"`, `"ratings"`, `"tags_per_rating"`} {
		if !bytes.Contains(payload, []byte(want)) {
			t.Fatalf("table payload = %s, want %s", payload, want)
		}
	}
}
