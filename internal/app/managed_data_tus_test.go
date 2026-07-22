package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Yacobolo/leapview/internal/manageddata/control"
	"github.com/Yacobolo/leapview/internal/workload"
)

func TestManagedDataTusRouteRejectsClientCreatedUploads(t *testing.T) {
	called := false
	server := NewWithOptions(fakeMetrics{}, Options{
		Store: testStore(t),
		ManagedDataTus: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			called = true
			w.WriteHeader(http.StatusNoContent)
		}),
	})

	request := httptest.NewRequest(http.MethodPost, "/upload-protocols/tus", nil)
	request.Header.Set("Authorization", "Bearer dev")
	recorder := httptest.NewRecorder()
	server.Routes().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusMethodNotAllowed)
	}
	if called {
		t.Fatal("tus backend received a client-created upload request")
	}
}

type testManagedDataExpirer struct{ called chan struct{} }

func (e testManagedDataExpirer) ExpireUploads(context.Context) (control.ExpireResult, error) {
	select {
	case e.called <- struct{}{}:
	default:
	}
	return control.ExpireResult{Expired: 1}, nil
}

func TestManagedDataUploadExpirationUsesBackgroundLifecycle(t *testing.T) {
	called := make(chan struct{}, 1)
	server := NewWithOptions(fakeMetrics{}, Options{
		ManagedDataExpirer:        testManagedDataExpirer{called: called},
		ManagedDataExpireInterval: time.Millisecond,
	})
	ctx, cancel := context.WithCancel(context.Background())
	server.StartBackgroundJobs(ctx)
	select {
	case <-called:
	case <-time.After(time.Second):
		t.Fatal("managed-data upload expiration did not run")
	}
	cancel()
	if err := server.StopBackgroundJobs(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestManagedDataMaintenanceSkipsSaturatedPassWithoutQueueing(t *testing.T) {
	controller, err := workload.New(workload.Config{MaxRunning: 1, Classes: map[workload.Class]workload.Policy{
		workload.Interactive: {MaximumRunning: 1}, workload.Maintenance: {MaximumRunning: 1},
	}})
	if err != nil {
		t.Fatal(err)
	}
	held, err := controller.Acquire(context.Background(), workload.Request{Class: workload.Interactive, WorkspaceID: "sales", Operation: "hold"})
	if err != nil {
		t.Fatal(err)
	}
	called := make(chan struct{}, 1)
	server := NewWithOptions(fakeMetrics{}, Options{Workload: controller, ManagedDataExpirer: testManagedDataExpirer{called: called}, ManagedDataExpireInterval: 10 * time.Millisecond})
	ctx, cancel := context.WithCancel(context.Background())
	server.StartBackgroundJobs(ctx)
	select {
	case <-called:
		t.Fatal("maintenance ran while node capacity was saturated")
	case <-time.After(40 * time.Millisecond):
	}
	if stats := controller.Stats(); stats.Queued != 0 {
		t.Fatalf("maintenance queued: %#v", stats)
	}
	held.Release()
	select {
	case <-called:
	case <-time.After(time.Second):
		t.Fatal("maintenance did not retry on its normal schedule")
	}
	cancel()
	if err := server.StopBackgroundJobs(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestManagedDataTusRouteForwardsResumableOperations(t *testing.T) {
	var method, path string
	server := NewWithOptions(fakeMetrics{}, Options{
		Store: testStore(t),
		ManagedDataTus: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			method, path = r.Method, r.URL.Path
			w.WriteHeader(http.StatusNoContent)
		}),
	})

	request := httptest.NewRequest(http.MethodPatch, "/upload-protocols/tus/tus_abc", nil)
	request.Header.Set("Authorization", "Bearer dev")
	recorder := httptest.NewRecorder()
	server.Routes().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNoContent || method != http.MethodPatch || path != "/upload-protocols/tus/tus_abc" {
		t.Fatalf("status = %d, method = %q, path = %q", recorder.Code, method, path)
	}
}

func TestManagedDataTusMethodsAreClosedByDefault(t *testing.T) {
	handler := managedDataTusHandler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodPost, http.MethodConnect, http.MethodTrace} {
		t.Run(method, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, httptest.NewRequest(method, "/upload-protocols/tus/tus_abc", nil))
			if recorder.Code != http.StatusMethodNotAllowed {
				t.Fatalf("status = %d, want %d", recorder.Code, http.StatusMethodNotAllowed)
			}
		})
	}
}
