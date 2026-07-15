package app

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/Yacobolo/libredash/internal/access"
	apigenapi "github.com/Yacobolo/libredash/internal/api/gen"
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

func TestProjectDeploymentGeneratedGenerationsAreInt64(t *testing.T) {
	typeOf := reflect.TypeOf(apigenapi.ProjectDeploymentConnectionResponse{})
	for _, fieldName := range []string{"PriorGeneration", "ActivatedGeneration"} {
		field, ok := typeOf.FieldByName(fieldName)
		if !ok {
			t.Fatalf("missing %s.%s", typeOf.Name(), fieldName)
		}
		kind := field.Type.Kind()
		if kind == reflect.Pointer {
			kind = field.Type.Elem().Kind()
		}
		if kind != reflect.Int64 {
			t.Fatalf("%s.%s type = %s, want int64", typeOf.Name(), fieldName, field.Type)
		}
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
		"getManagedDataEnvironmentRevision":    access.PrivilegeViewData,
		"listManagedDataRevisions":             access.PrivilegeViewData,
		"getManagedDataRevision":               access.PrivilegeViewData,
		"createManagedDataUploadSession":       access.PrivilegeIngestData,
		"getManagedDataUploadSession":          access.PrivilegeIngestData,
		"abortManagedDataUploadSession":        access.PrivilegeIngestData,
		"finalizeManagedDataUploadSession":     access.PrivilegeIngestData,
		"createManagedDataS3MultipartUpload":   access.PrivilegeIngestData,
		"signManagedDataS3MultipartPart":       access.PrivilegeIngestData,
		"completeManagedDataS3MultipartUpload": access.PrivilegeIngestData,
		"abortManagedDataS3MultipartUpload":    access.PrivilegeIngestData,
		"createProjectDeployment":              access.PrivilegeDeploy,
		"getProjectDeployment":                 access.PrivilegeViewItem,
		"activateProjectDeployment":            access.PrivilegeActivateDeployment,
	}
	for operation, privilege := range want {
		if got := apigenOperationPrivileges[operation]; got != privilege {
			t.Errorf("%s privilege = %q, want %q", operation, got, privilege)
		}
		if resolver, exists := apigenOperationObjectResolvers[operation]; exists || resolver != nil {
			t.Errorf("%s must not have a workspace object resolver", operation)
		}
	}
	for _, removed := range []string{"listManagedDataRollouts", "createManagedDataRollout", "getManagedDataRollout", "activateManagedDataRollout", "rollbackManagedDataRollout", "activatePublish"} {
		if _, exists := apigenOperationPrivileges[removed]; exists {
			t.Errorf("removed operation %s retains a privilege", removed)
		}
	}
}
