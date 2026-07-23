package ir

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestVisualizationEnvelopeConformanceFixtures(t *testing.T) {
	t.Parallel()

	paths, err := filepath.Glob("../../../api/visualization/conformance/*.json")
	if err != nil {
		t.Fatalf("find conformance fixtures: %v", err)
	}
	if len(paths) == 0 {
		t.Fatal("no visualization conformance fixtures found")
	}
	for _, path := range paths {
		if filepath.Base(path) == "formatting.json" {
			continue
		}
		path := path
		t.Run(filepath.Base(path), func(t *testing.T) {
			t.Parallel()
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read fixture: %v", err)
			}
			var envelope VisualizationEnvelope
			if err := json.Unmarshal(data, &envelope); err != nil {
				t.Fatalf("decode fixture: %v", err)
			}
			computed, err := ComputeSpecRevision(envelope.Spec)
			if err != nil {
				t.Fatalf("compute fixture specification revision: %v", err)
			}
			if envelope.SpecRevision != computed.String() {
				t.Fatalf("fixture specification revision = %q, want %q", envelope.SpecRevision, computed)
			}
			if err := ValidateEnvelopeRevisions(envelope); err != nil {
				t.Fatalf("validate fixture revisions: %v", err)
			}

			encoded, err := json.Marshal(envelope)
			if err != nil {
				t.Fatalf("encode fixture: %v", err)
			}
			var roundTrip VisualizationEnvelope
			if err := json.Unmarshal(encoded, &roundTrip); err != nil {
				t.Fatalf("decode round trip: %v", err)
			}
			if err := ValidateEnvelopeRevisions(roundTrip); err != nil {
				t.Fatalf("validate round trip: %v", err)
			}
		})
	}
}

func TestValidateEnvelopeRevisionsRejectsStaleData(t *testing.T) {
	t.Parallel()

	envelope := readEnvelopeFixture(t, "cartesian-inline.json")
	state := envelope.DataState.Value.(*InlineVisualizationDataState)
	state.DataRevision--
	if err := ValidateEnvelopeRevisions(envelope); err == nil {
		t.Fatal("expected stale data state to fail")
	}
}

func TestWithStreamRevisionRevisesTheWholeEnvelopeAtomically(t *testing.T) {
	envelope := readEnvelopeFixture(t, "cartesian-inline.json")
	state := envelope.DataState.Value.(*InlineVisualizationDataState)
	envelope.Selection = []VisualizationSelectionEntry{{Datum: VisualizationDatumRef{
		Dataset: state.Datasets[0].ID, DataRevision: envelope.DataRevision,
		Identity: map[string]any{"order_month": "2026-01"},
	}}}

	revised, err := WithStreamRevision(envelope, 11, 7)
	if err != nil {
		t.Fatal(err)
	}
	state = revised.DataState.Value.(*InlineVisualizationDataState)
	if revised.DataRevision != 11 || state.DataRevision != 11 || state.Generation != 7 || state.Datasets[0].DataRevision != 11 || state.Datasets[0].Generation != 7 || revised.Selection[0].Datum.DataRevision != 11 {
		t.Fatalf("revision was not applied atomically: %#v", revised)
	}
}

func TestVisualizationEnvelopeFailsClosed(t *testing.T) {
	t.Parallel()

	fixture, err := os.ReadFile("../../../api/visualization/conformance/cartesian-inline.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var document map[string]any
	if err := json.Unmarshal(fixture, &document); err != nil {
		t.Fatalf("decode fixture document: %v", err)
	}

	t.Run("unknown property", func(t *testing.T) {
		document["legacyOptions"] = map[string]any{"animation": true}
		data, err := json.Marshal(document)
		if err != nil {
			t.Fatalf("encode fixture: %v", err)
		}
		var envelope VisualizationEnvelope
		if err := json.Unmarshal(data, &envelope); err == nil {
			t.Fatal("expected unknown envelope property to fail")
		}
	})

	t.Run("unknown schema version", func(t *testing.T) {
		envelope := readEnvelopeFixture(t, "cartesian-inline.json")
		envelope.SchemaVersion = CurrentSchemaVersion + 1
		if err := ValidateEnvelope(envelope); err == nil {
			t.Fatal("expected unknown schema version to fail")
		}
	})
}

func readEnvelopeFixture(t *testing.T, name string) VisualizationEnvelope {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("../../../api/visualization/conformance", name))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var envelope VisualizationEnvelope
	if err := json.Unmarshal(data, &envelope); err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	return envelope
}
