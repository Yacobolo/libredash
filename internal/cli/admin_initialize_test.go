package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Yacobolo/leapview/internal/access"
	accesssqlite "github.com/Yacobolo/leapview/internal/access/sqlite"
	"github.com/Yacobolo/leapview/internal/platform"
)

func TestAdminInitializeCreatesOneTimeCredentialBundle(t *testing.T) {
	home := t.TempDir()
	t.Setenv("LEAPVIEW_HOME", home)
	t.Setenv("LEAPVIEW_PRODUCTION", "1")
	t.Setenv("LEAPVIEW_ENVIRONMENT", "prod")
	t.Setenv("LEAPVIEW_BOOTSTRAP_ADMIN_EMAIL", "owner@example.com")
	var out bytes.Buffer
	if err := runAdminInitialize(context.Background(), "json", &out); err != nil {
		t.Fatal(err)
	}
	var credentials initialInstanceCredentials
	if err := json.Unmarshal(out.Bytes(), &credentials); err != nil {
		t.Fatal(err)
	}
	if credentials.Email != "owner@example.com" || credentials.TemporaryPassword == "" || credentials.PublisherToken == "" || credentials.PublisherTokenExpiresAt == "" {
		t.Fatalf("credentials = %#v", credentials)
	}
	expires, err := time.Parse(time.RFC3339, credentials.PublisherTokenExpiresAt)
	if err != nil || time.Until(expires) > 24*time.Hour || time.Until(expires) < 23*time.Hour {
		t.Fatalf("publisher expiry = %q, %v", credentials.PublisherTokenExpiresAt, err)
	}
	store, err := platform.Open(context.Background(), filepath.Join(home, "leapview.db"))
	if err != nil {
		t.Fatal(err)
	}
	repo := accesssqlite.NewRepository(store.SQLDB())
	principal, local, err := repo.VerifyLocalPassword(context.Background(), credentials.Email, credentials.TemporaryPassword)
	if err != nil || !local.MustChangePassword {
		t.Fatalf("initialized administrator = %#v credential=%#v err=%v", principal, local, err)
	}
	decision, err := repo.Authorize(context.Background(), principal.ID, access.PrivilegeManagePlatform, access.PlatformObject())
	if err != nil || !decision.Allowed {
		t.Fatalf("initialized administrator authorization = %#v err=%v", decision, err)
	}
	apiCredential, err := repo.CredentialForAPIToken(context.Background(), credentials.PublisherToken)
	if err != nil || len(apiCredential.Token.Privileges) == 0 {
		t.Fatalf("publisher credential = %#v err=%v", apiCredential, err)
	}
	for _, privilege := range apiCredential.Token.Privileges {
		if privilege == access.PrivilegeManagePlatform || privilege == access.PrivilegeManageGrants {
			t.Fatalf("publisher token contains administrative privilege %q", privilege)
		}
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if err := acknowledgeInitialCredentials(context.Background()); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := runAdminInitialize(context.Background(), "json", &out); err == nil || !strings.Contains(err.Error(), "already initialized") {
		t.Fatalf("second initialize error = %v", err)
	}
}

func TestAdminInitializeReplaysCredentialsAfterDeliveryFailure(t *testing.T) {
	home := t.TempDir()
	t.Setenv("LEAPVIEW_HOME", home)
	t.Setenv("LEAPVIEW_PRODUCTION", "1")
	t.Setenv("LEAPVIEW_ENVIRONMENT", "prod")
	t.Setenv("LEAPVIEW_BOOTSTRAP_ADMIN_EMAIL", "owner@example.com")

	if err := runAdminInitialize(context.Background(), "json", errorWriter{}); err == nil {
		t.Fatal("initialize output failure = nil")
	}
	var recovered bytes.Buffer
	if err := runAdminInitialize(context.Background(), "json", &recovered); err != nil {
		t.Fatalf("recover initialization credentials: %v", err)
	}
	var credentials initialInstanceCredentials
	if err := json.Unmarshal(recovered.Bytes(), &credentials); err != nil || credentials.TemporaryPassword == "" || credentials.PublisherToken == "" {
		t.Fatalf("recovered credentials = %#v, %v", credentials, err)
	}
	if err := acknowledgeInitialCredentials(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(initialCredentialRecoveryPath(home)); !os.IsNotExist(err) {
		t.Fatalf("credential recovery file remains after acknowledgement: %v", err)
	}
}

type errorWriter struct{}

func (errorWriter) Write([]byte) (int, error) {
	return 0, errors.New("credential destination failed")
}
