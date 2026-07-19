package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Yacobolo/libredash/internal/access"
	apigenapi "github.com/Yacobolo/libredash/internal/api/gen"
)

func TestGetInstanceReturnsConfiguredEnvironment(t *testing.T) {
	store := testStore(t)
	principal := testPrincipal(t, context.Background(), store, "publisher@example.com", "Publisher", access.RoleOwner)
	token, _ := testScopedAPIToken(t, context.Background(), store, access.APITokenInput{PrincipalID: principal.ID, Name: "publisher", Privileges: []access.Privilege{access.PrivilegeIngestData}})
	auth := testAuth(store, "test", AuthConfig{APITokenOnly: true})
	server := NewWithOptions(nil, Options{Store: store, Auth: auth, DefaultEnvironment: "prod"})
	unauthenticated := httptest.NewRecorder()
	server.Routes().ServeHTTP(unauthenticated, httptest.NewRequest(http.MethodGet, "/api/v1/instance", nil))
	if unauthenticated.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status = %d", unauthenticated.Code)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/instance", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	server.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var response apigenapi.InstanceResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	if response.Environment != "prod" {
		t.Fatalf("environment = %q", response.Environment)
	}
}
