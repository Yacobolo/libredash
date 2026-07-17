package cursorsigning

import (
	"bytes"
	"testing"
)

func TestKeyRingVerifiesExistingCursorsDuringRotation(t *testing.T) {
	v1 := bytes.Repeat([]byte{1}, 32)
	v2 := bytes.Repeat([]byte{2}, 32)
	if err := Configure("v1", map[string][]byte{"v1": v1}); err != nil {
		t.Fatal(err)
	}
	old := Sign("q1", []byte(`{"offset":1}`))
	if err := Configure("v2", map[string][]byte{"v1": v1, "v2": v2}); err != nil {
		t.Fatal(err)
	}
	if payload, err := Verify("q1", old); err != nil || string(payload) != `{"offset":1}` {
		t.Fatalf("verify old cursor payload=%s err=%v", payload, err)
	}
	next := Sign("q1", []byte(`{"offset":2}`))
	if !bytes.Contains([]byte(next), []byte("q1.v2.")) {
		t.Fatalf("new cursor does not use v2: %s", next)
	}
	if err := Configure("v2", map[string][]byte{"v2": v2}); err != nil {
		t.Fatal(err)
	}
	if _, err := Verify("q1", old); err == nil {
		t.Fatal("retired key unexpectedly verified an old cursor")
	}
}
