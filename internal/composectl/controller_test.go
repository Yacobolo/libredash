package composectl

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestControllerLockRejectsConcurrentOperationAndRecoversAfterRelease(t *testing.T) {
	path := filepath.Join(t.TempDir(), controllerLockName)
	first, err := acquireControllerLock(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := acquireControllerLock(path); err == nil || !strings.Contains(err.Error(), "another LeapView operation") {
		t.Fatalf("concurrent lock error = %v", err)
	}
	if err := first.Release(); err != nil {
		t.Fatal(err)
	}
	second, err := acquireControllerLock(path)
	if err != nil {
		t.Fatalf("reacquire released lock: %v", err)
	}
	defer second.Release()
}

func TestFirstLoginRetainsCredentialsUntilOutputSucceeds(t *testing.T) {
	root := t.TempDir()
	credentialsPath := filepath.Join(root, credentialsName)
	credentials := []byte("{\"temporaryPassword\":\"temporary\"}\n")
	if err := os.WriteFile(credentialsPath, credentials, 0o600); err != nil {
		t.Fatal(err)
	}
	controller, err := New(Options{Root: root, Stdout: failingWriter{}})
	if err != nil {
		t.Fatal(err)
	}
	if err := controller.FirstLogin(); err == nil {
		t.Fatal("first-login output failure = nil")
	}
	if contents, err := os.ReadFile(credentialsPath); err != nil || !bytes.Equal(contents, credentials) {
		t.Fatalf("credentials after output failure = %q, %v", contents, err)
	}

	var output bytes.Buffer
	controller, err = New(Options{Root: root, Stdout: &output})
	if err != nil {
		t.Fatal(err)
	}
	if err := controller.FirstLogin(); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(output.Bytes(), credentials) {
		t.Fatalf("first-login output = %q", output.Bytes())
	}
	if _, err := os.Stat(credentialsPath); !os.IsNotExist(err) {
		t.Fatalf("credentials remain after successful output: %v", err)
	}
}

func TestUpdateEnvFileIsPrivateAndRejectsMissingContractKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "deployment.env")
	if err := os.WriteFile(path, []byte("LEAPVIEW_IMAGE=old\nCOMPOSE_HTTPS=1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := updateEnvFile(path, map[string]string{"LEAPVIEW_IMAGE": "new"}); err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(path)
	if err != nil || string(contents) != "LEAPVIEW_IMAGE=new\nCOMPOSE_HTTPS=1\n" {
		t.Fatalf("updated environment = %q, %v", contents, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("environment permissions = %v", info.Mode().Perm())
	}
	if err := updateEnvFile(path, map[string]string{"CADDY_DOMAIN": "dash.example.com"}); err == nil {
		t.Fatal("missing environment key update succeeded")
	}
}

func TestEnvironmentLineValuesRejectConfigurationInjection(t *testing.T) {
	for _, value := range []string{"prod\nLEAPVIEW_CSRF_KEY=forged", "dash.example.com\rCOMPOSE_HTTPS=0", "admin@example.com\x00suffix"} {
		if err := validateEnvLineValue("test value", value); err == nil {
			t.Fatalf("configuration injection value %q was accepted", value)
		}
	}
	if err := validateEnvLineValue("domain", "dash.example.com"); err != nil {
		t.Fatalf("ordinary value rejected: %v", err)
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) {
	return 0, errors.New("output failed")
}
