package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	apigenapi "github.com/Yacobolo/leapview/internal/api/gen"
	visualizationir "github.com/Yacobolo/leapview/internal/visualization/ir"
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
	if response.Visualization.SchemaVersion != visualizationir.CurrentSchemaVersion || len(response.Visualization.Renderers) != 5 {
		t.Fatalf("visualization capabilities=%#v", response.Visualization)
	}
	for _, renderer := range response.Visualization.Renderers {
		if renderer.SchemaVersion != response.Visualization.SchemaVersion {
			t.Fatalf("renderer schema version=%d, want %d: %#v", renderer.SchemaVersion, response.Visualization.SchemaVersion, renderer)
		}
	}
}
