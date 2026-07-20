package http

import (
	"encoding/json"
	"testing"
)

func TestPageSliceForRequestPreservesEmptyArray(t *testing.T) {
	items, _, pageErr := pageSlice([]string(nil), nil, nil, "test", func(value string) string { return value })
	encoded, err := json.Marshal(items)
	if err != nil || pageErr != nil || string(encoded) != "[]" {
		t.Fatalf("empty page = %s, error=%v/%v; want []", encoded, pageErr, err)
	}
}
