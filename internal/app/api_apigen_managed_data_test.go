package app

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	apigenapi "github.com/Yacobolo/leapview/internal/api/gen"
)

func TestManagedDataGeneratedByteCountsAreInt64(t *testing.T) {
	for _, value := range []any{
		apigenapi.ManagedDataFileMetadata{},
		apigenapi.ManagedDataRevisionSummaryResponse{},
		apigenapi.ManagedDataS3MultipartNegotiation{},
		apigenapi.ManagedDataS3MultipartSignPartRequest{},
		apigenapi.ManagedDataTusUploadNegotiation{},
	} {
		typeOf := reflect.TypeOf(value)
		for _, fieldName := range []string{"Size", "Offset", "MinimumPartSize", "MaximumPartSize"} {
			field, ok := typeOf.FieldByName(fieldName)
			if ok && field.Type.Kind() != reflect.Int64 {
				t.Fatalf("%s.%s type = %s, want int64", typeOf.Name(), fieldName, field.Type)
			}
		}
	}
}

func TestReleaseArtifactGeneratedSizeIsInt64(t *testing.T) {
	typeOf := reflect.TypeOf(apigenapi.ReleaseArtifactResponse{})
	field, ok := typeOf.FieldByName("SizeBytes")
	if !ok || field.Type.Kind() != reflect.Int64 {
		t.Fatalf("%s.SizeBytes type = %v, want int64", typeOf.Name(), field.Type)
	}
}

func TestManagedDataAPIGenAdapterImplementsEveryGeneratedOperation(t *testing.T) {
	var _ apigenapi.GenOperationDispatcher = apiGenDispatcher{}

	server := newAppTestHarness(nil)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	apiGenDispatcherForTest(server).GetManagedDataRevision(recorder, request, "project-a", "orders", "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("unconfigured managed-data adapter status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
}
