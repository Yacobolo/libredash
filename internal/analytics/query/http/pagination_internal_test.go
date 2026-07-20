package http

import (
	"encoding/json"
	nethttp "net/http"
	"net/http/httptest"
	"testing"
)

func TestPageSliceForRequestPreservesEmptyArray(t *testing.T) {
	response := httptest.NewRecorder()
	request := httptest.NewRequest(nethttp.MethodGet, "/api/v1/workspaces/sales/semantic-models", nil)
	items, _, ok := pageSliceForRequest[string](response, request, nil)
	encoded, err := json.Marshal(items)
	if err != nil || !ok || string(encoded) != "[]" {
		t.Fatalf("empty page = %s, ok=%v, error=%v; want []", encoded, ok, err)
	}
}
