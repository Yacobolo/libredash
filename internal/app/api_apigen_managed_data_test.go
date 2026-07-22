package app

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/Yacobolo/leapview/internal/access"
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
	var _ apigenapi.GenOperationDispatcher = apiGenAdapter{}

	server := New(nil)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	apiGenAdapter{server: server}.GetManagedDataRevision(recorder, request, "project-a", "orders", "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("unconfigured managed-data adapter status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestManagedDataAPIGenPrivilegesArePlatformGlobal(t *testing.T) {
	want := map[string]access.Privilege{
		"getActiveManagedDataRevision":         access.PrivilegeViewData,
		"listManagedConnections":               access.PrivilegeViewData,
		"getManagedConnection":                 access.PrivilegeViewData,
		"listManagedDataRevisions":             access.PrivilegeViewData,
		"getManagedDataRevision":               access.PrivilegeViewData,
		"createManagedDataUploadSession":       access.PrivilegeIngestData,
		"getManagedDataUploadSession":          access.PrivilegeIngestData,
		"listManagedDataUploadSessions":        access.PrivilegeIngestData,
		"cancelManagedDataUploadSession":       access.PrivilegeIngestData,
		"listManagedDataUploadSessionEvents":   access.PrivilegeIngestData,
		"finalizeManagedDataUploadSession":     access.PrivilegeIngestData,
		"createManagedDataS3MultipartUpload":   access.PrivilegeIngestData,
		"signManagedDataS3MultipartPart":       access.PrivilegeIngestData,
		"completeManagedDataS3MultipartUpload": access.PrivilegeIngestData,
		"abortManagedDataS3MultipartUpload":    access.PrivilegeIngestData,
		"createDeployment":                     access.PrivilegeActivateDeployment,
		"getDeployment":                        access.PrivilegeViewItem,
		"listDeployments":                      access.PrivilegeViewItem,
		"cancelDeployment":                     access.PrivilegeActivateDeployment,
		"rollbackDeployment":                   access.PrivilegeActivateDeployment,
	}
	for operation, privilege := range want {
		if got, ok := apigenOperationPrivilege(operation); !ok || got != privilege {
			t.Errorf("%s privilege = %q, want %q", operation, got, privilege)
		}
		if resolver, ok := apigenOperationObjectResolver(operation); !ok || resolver != nil {
			t.Errorf("%s must not have a workspace object resolver", operation)
		}
	}
	for _, removed := range []string{"listManagedDataRollouts", "createManagedDataRollout", "getManagedDataRollout", "activateManagedDataRollout", "rollbackManagedDataRollout", "activatePublish"} {
		if _, exists := apigenOperationPrivilege(removed); exists {
			t.Errorf("removed operation %s retains a privilege", removed)
		}
	}
}
