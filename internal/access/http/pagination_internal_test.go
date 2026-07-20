package http

import (
	"encoding/json"
	stdhttp "net/http"
	"net/http/httptest"
	"testing"
)

func TestPageSliceForRequestPreservesEmptyArray(t *testing.T) {
	response := httptest.NewRecorder()
	request := httptest.NewRequest(stdhttp.MethodGet, "/api/v1/principals", nil)
	items, _, ok := pageSliceForRequest[string](response, request, nil)
	encoded, err := json.Marshal(items)
	if err != nil || !ok || string(encoded) != "[]" {
		t.Fatalf("empty page = %s, ok=%v, error=%v; want []", encoded, ok, err)
	}
}
