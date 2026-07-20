package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	apigenapi "github.com/Yacobolo/leapview/internal/api/gen"
)

func TestCapabilitiesReportOnlyEnabledUploadProtocols(t *testing.T) {
	server := NewWithOptions(fakeMetrics{}, Options{DefaultEnvironment: "prod"})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/capabilities", nil)
	rec := httptest.NewRecorder()
	(apiGenAdapter{server: server}).GetCapabilities(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var response apigenapi.CapabilitiesResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Environment != "prod" || len(response.UploadProtocols) != 0 {
		t.Fatalf("capabilities = %#v", response)
	}
	if len(response.VisualShapes) != 12 {
		t.Fatalf("visual shapes=%v", response.VisualShapes)
	}
}
