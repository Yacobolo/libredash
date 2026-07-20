package cli

import (
	"context"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Yacobolo/leapview/internal/config"
	"github.com/Yacobolo/leapview/internal/manageddata/control"
	"github.com/Yacobolo/leapview/internal/manageddata/maintenance"
	"github.com/Yacobolo/leapview/internal/manageddata/storage"
)

func TestNewManagedDataStorageLocal(t *testing.T) {
	root := filepath.Join(t.TempDir(), "managed")
	services, err := newManagedDataStorage(context.Background(), config.Config{
		ManagedDataBackend:      "local",
		ManagedDataDir:          root,
		ManagedDataMaxFileBytes: 1024,
	})
	if err != nil {
		t.Fatal(err)
	}
	if services.blobs == nil || services.transport == nil || services.materializer == nil || services.tus == nil || services.s3 != nil {
		t.Fatalf("services = %#v", services)
	}
	if services.runtimeCache != nil {
		t.Fatal("local backend unexpectedly allocated a copying runtime cache")
	}
	collector, err := newManagedDataRuntimeCollector(services, config.Config{ManagedDataGCGracePeriod: time.Hour})
	if err != nil || collector != nil {
		t.Fatalf("local runtime collector = %#v, %v; want nil", collector, err)
	}
	if services.transport.Backend() != "local" {
		t.Fatalf("backend = %q", services.transport.Backend())
	}
	for _, relative := range []string{"objects", "uploads"} {
		info, statErr := os.Stat(filepath.Join(root, relative))
		if statErr != nil {
			t.Fatalf("stat %s: %v", relative, statErr)
		}
		if info.Mode().Perm()&0o077 != 0 {
			t.Fatalf("%s permissions = %o, want private", relative, info.Mode().Perm())
		}
	}
	if _, err := os.Stat(filepath.Join(root, "runtime")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("local runtime cache stat error = %v, want not exist", err)
	}
}

func TestCapacityProtectedTusRejectsChunkWithoutReserve(t *testing.T) {
	checker, err := maintenance.NewCapacityChecker(t.TempDir(), math.MaxInt64)
	if err != nil {
		t.Fatal(err)
	}
	called := false
	handler := capacityProtectedTus(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }), checker)
	request := httptest.NewRequest(http.MethodPatch, "/tus/upload", strings.NewReader("x"))
	request.ContentLength = 1
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusInsufficientStorage || called {
		t.Fatalf("status = %d, called = %v", recorder.Code, called)
	}
}

func TestNewManagedDataStorageRejectsUnknownBackend(t *testing.T) {
	_, err := newManagedDataStorage(context.Background(), config.Config{
		ManagedDataBackend: "shared-filesystem",
		ManagedDataDir:     t.TempDir(),
	})
	if err == nil || !errors.Is(err, storage.ErrInvalid) {
		t.Fatalf("error = %v, want storage.ErrInvalid", err)
	}
}

func TestNewManagedDataControlRequiresStorage(t *testing.T) {
	_, err := newManagedDataControl(nil, managedDataStorage{}, config.Config{})
	if err == nil || !errors.Is(err, control.ErrInvalid) {
		t.Fatalf("error = %v, want control.ErrInvalid", err)
	}
}
