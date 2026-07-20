package app

import (
	"encoding/json"
	"testing"
)

func TestKeysetPagePreservesEmptyArray(t *testing.T) {
	items, next, err := keysetPage([]string(nil), nil, nil, func(value string) string { return value })
	encoded, marshalErr := json.Marshal(items)
	if err != nil || marshalErr != nil || next != nil || string(encoded) != "[]" {
		t.Fatalf("empty page = %s, next=%v, error=%v/%v; want []", encoded, next, err, marshalErr)
	}
}
