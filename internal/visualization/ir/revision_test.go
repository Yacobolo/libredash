package ir

import (
	"encoding/json"
	"regexp"
	"testing"
)

func TestComputeSpecRevisionIsDeterministicAndContentAddressed(t *testing.T) {
	t.Parallel()

	spec := readCartesianSpecFixture(t)
	first, err := ComputeSpecRevision(spec)
	if err != nil {
		t.Fatalf("compute first revision: %v", err)
	}
	second, err := ComputeSpecRevision(spec)
	if err != nil {
		t.Fatalf("compute second revision: %v", err)
	}
	if first != second {
		t.Fatalf("revision is not deterministic: %q != %q", first, second)
	}
	if !regexp.MustCompile(`^sha256:[0-9a-f]{64}$`).MatchString(first.String()) {
		t.Fatalf("revision %q is not canonical", first)
	}

	var changed VisualizationSpec
	data, err := json.Marshal(spec)
	if err != nil {
		t.Fatalf("encode changed fixture source: %v", err)
	}
	var document map[string]any
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatalf("decode changed fixture document: %v", err)
	}
	document["title"] = "Net revenue"
	data, err = json.Marshal(document)
	if err != nil {
		t.Fatalf("encode changed fixture: %v", err)
	}
	if err := json.Unmarshal(data, &changed); err != nil {
		t.Fatalf("decode changed specification: %v", err)
	}
	changedRevision, err := ComputeSpecRevision(changed)
	if err != nil {
		t.Fatalf("compute changed revision: %v", err)
	}
	if first == changedRevision {
		t.Fatalf("semantic change retained revision %q", first)
	}
}

func TestParseSpecRevisionFailsClosed(t *testing.T) {
	t.Parallel()

	for _, value := range []string{"", "sha256:", "md5:00000000000000000000000000000000", "sha256:not-hex", "sha256:ABCDEF0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF0123456789"} {
		if _, err := ParseSpecRevision(value); err == nil {
			t.Errorf("ParseSpecRevision(%q) succeeded", value)
		}
	}

	computed, err := ComputeSpecRevision(readCartesianSpecFixture(t))
	if err != nil {
		t.Fatalf("compute revision: %v", err)
	}
	parsed, err := ParseSpecRevision(computed.String())
	if err != nil {
		t.Fatalf("parse computed revision: %v", err)
	}
	if parsed != computed {
		t.Fatalf("parsed revision %q != computed revision %q", parsed, computed)
	}
}

func TestComputeSpecRevisionRejectsMissingVariant(t *testing.T) {
	t.Parallel()

	if _, err := ComputeSpecRevision(VisualizationSpec{}); err == nil {
		t.Fatal("expected a missing specification variant to fail")
	}
}

func readCartesianSpecFixture(t *testing.T) VisualizationSpec {
	t.Helper()
	return readEnvelopeFixture(t, "cartesian-inline.json").Spec
}
