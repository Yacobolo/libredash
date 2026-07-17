package cli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	apigenapi "github.com/Yacobolo/libredash/internal/api/gen"
	"github.com/Yacobolo/libredash/internal/manageddata"
	"github.com/Yacobolo/libredash/internal/manageddata/localplan"
)

func TestDataSyncDeduplicatesAndUsesStableIdempotencyKey(t *testing.T) {
	root := t.TempDir()
	file := writeSyncFile(t, root, "orders.csv", []byte("order_id\n1\n"))
	plan := syncPlan(root, file)
	var keys []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, "/upload-sessions") {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		keys = append(keys, r.Header.Get("Idempotency-Key"))
		writeUploadSession(t, w, plan, "upload-1", apigenapi.ManagedDataUploadSessionStatusCompleted, []apigenapi.ManagedDataFileUploadResponse{{
			File: wireFile(t, file), Status: apigenapi.ManagedDataFileUploadStatusSkipped,
			Negotiation: apigenapi.ManagedDataUploadNegotiation{Protocol: apigenapi.ManagedDataUploadProtocolAlreadyPresent},
		}})
	}))
	defer server.Close()

	for range 2 {
		var out bytes.Buffer
		err := runDataSync(context.Background(), dataSyncRequest{
			ProjectPath: "/catalog/libredash.yaml", ProjectID: "demo", Connection: "orders", Root: root,
			Target: server.URL, Token: "secret-token", Plan: plan, Out: &out, HTTPClient: server.Client(),
		})
		if err != nil {
			t.Fatalf("runDataSync() error = %v", err)
		}
		if got, want := out.String(), "staged "+plan.Manifest.RevisionID()+"\n"; got != want {
			t.Fatalf("output = %q, want %q", got, want)
		}
	}
	if len(keys) != 2 || keys[0] == "" || keys[0] != keys[1] {
		t.Fatalf("idempotency keys = %#v", keys)
	}
}

func TestDataSyncResumesTusFromHEADOffset(t *testing.T) {
	root := t.TempDir()
	body := []byte("0123456789")
	file := writeSyncFile(t, root, "orders.csv", body)
	plan := syncPlan(root, file)
	var mu sync.Mutex
	offset := int64(4)
	var patched []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/upload-sessions"):
			writeUploadSession(t, w, plan, "upload-1", apigenapi.ManagedDataUploadSessionStatusOpen, []apigenapi.ManagedDataFileUploadResponse{{
				File: wireFile(t, file), Status: apigenapi.ManagedDataFileUploadStatusUploading,
				Negotiation: apigenapi.ManagedDataUploadNegotiation{Protocol: apigenapi.ManagedDataUploadProtocolTus, Tus: &apigenapi.ManagedDataTusUploadNegotiation{Endpoint: "/tus", UploadId: "blob-1", Offset: 0, ExpiresAt: "2030-01-01T00:00:00Z"}},
			}})
		case r.URL.Path == "/tus/blob-1" && r.Method == http.MethodHead:
			mu.Lock()
			current := offset
			mu.Unlock()
			w.Header().Set("Tus-Resumable", "1.0.0")
			w.Header().Set("Upload-Offset", strconv.FormatInt(current, 10))
			w.Header().Set("Upload-Length", strconv.Itoa(len(body)))
			w.WriteHeader(http.StatusNoContent)
		case r.URL.Path == "/tus/blob-1" && r.Method == http.MethodPatch:
			if got := r.Header.Get("Upload-Offset"); got != "4" {
				t.Fatalf("Upload-Offset = %q", got)
			}
			if got := r.Header.Get("Authorization"); got != "Bearer secret-token" {
				t.Fatalf("Authorization = %q", got)
			}
			chunk, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatal(err)
			}
			mu.Lock()
			patched = append(patched, chunk...)
			offset += int64(len(chunk))
			current := offset
			mu.Unlock()
			w.Header().Set("Tus-Resumable", "1.0.0")
			w.Header().Set("Upload-Offset", strconv.FormatInt(current, 10))
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/finalize"):
			writeUploadSession(t, w, plan, "upload-1", apigenapi.ManagedDataUploadSessionStatusCompleted, []apigenapi.ManagedDataFileUploadResponse{{
				File: wireFile(t, file), Status: apigenapi.ManagedDataFileUploadStatusVerified,
				Negotiation: apigenapi.ManagedDataUploadNegotiation{Protocol: apigenapi.ManagedDataUploadProtocolAlreadyPresent},
			}})
		default:
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	err := runDataSync(context.Background(), dataSyncRequest{ProjectID: "demo", Connection: "orders", Root: root, Target: server.URL, Token: "secret-token", Plan: plan, Out: io.Discard, HTTPClient: server.Client()})
	if err != nil {
		t.Fatalf("runDataSync() error = %v", err)
	}
	if got, want := string(patched), string(body[4:]); got != want {
		t.Fatalf("patched = %q, want %q", got, want)
	}
}

func TestDataSyncWaitsForAsynchronousFinalization(t *testing.T) {
	root := t.TempDir()
	file := writeSyncFile(t, root, "orders.csv", []byte("order_id\n1\n"))
	plan := syncPlan(root, file)
	getCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		files := []apigenapi.ManagedDataFileUploadResponse{{
			File: wireFile(t, file), Status: apigenapi.ManagedDataFileUploadStatusSkipped,
			Negotiation: apigenapi.ManagedDataUploadNegotiation{Protocol: apigenapi.ManagedDataUploadProtocolAlreadyPresent},
		}}
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/upload-sessions"):
			writeUploadSession(t, w, plan, "upload-async", apigenapi.ManagedDataUploadSessionStatusOpen, files)
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/finalize"):
			writeUploadSession(t, w, plan, "upload-async", apigenapi.ManagedDataUploadSessionStatusFinalizing, files)
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/upload-sessions/upload-async"):
			getCalls++
			status := apigenapi.ManagedDataUploadSessionStatusFinalizing
			if getCalls == 2 {
				status = apigenapi.ManagedDataUploadSessionStatusCompleted
			}
			writeUploadSession(t, w, plan, "upload-async", status, files)
		default:
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	err := runDataSync(context.Background(), dataSyncRequest{ProjectID: "demo", Connection: "orders", Root: root, Target: server.URL, Token: "secret-token", Plan: plan, Out: io.Discard, HTTPClient: server.Client()})
	if err != nil {
		t.Fatalf("runDataSync() error = %v", err)
	}
	if getCalls != 2 {
		t.Fatalf("upload status GET calls = %d, want 2", getCalls)
	}
}

func TestDataSyncRetriesTusCapacityFailureAndReportsHTTPStatus(t *testing.T) {
	root := t.TempDir()
	body := []byte("orders")
	file := writeSyncFile(t, root, "orders.csv", body)
	var patchAttempts int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodHead:
			w.Header().Set("Tus-Resumable", "1.0.0")
			w.Header().Set("Upload-Offset", "0")
			w.Header().Set("Upload-Length", strconv.Itoa(len(body)))
			w.WriteHeader(http.StatusNoContent)
		case http.MethodPatch:
			patchAttempts++
			http.Error(w, "capacity details must not be exposed", http.StatusInsufficientStorage)
		default:
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	client := newManagedDataCLIClient(server.Client(), server.URL, "secret-token")
	err := uploadManagedDataTus(context.Background(), client, root, file, apigenapi.ManagedDataTusUploadNegotiation{
		Endpoint: "/tus", UploadId: "blob-1", ExpiresAt: "2030-01-01T00:00:00Z",
	})
	if err == nil || !strings.Contains(err.Error(), `tus upload failed for "orders.csv" with HTTP 507`) {
		t.Fatalf("uploadManagedDataTus() error = %v", err)
	}
	if strings.Contains(err.Error(), "capacity details") || strings.Contains(err.Error(), "secret-token") {
		t.Fatalf("error exposed secret material: %v", err)
	}
	if patchAttempts != dataTransferAttempts {
		t.Fatalf("PATCH attempts = %d, want %d", patchAttempts, dataTransferAttempts)
	}
}

func TestDataSyncUploadsDeterministicS3PartsWithoutBearerToken(t *testing.T) {
	root := t.TempDir()
	body := []byte("abcdefghij")
	file := writeSyncFile(t, root, "orders.csv", body)
	plan := syncPlan(root, file)
	var uploaded [][]byte
	var signedSizes []int64
	var completed apigenapi.ManagedDataS3MultipartCompleteRequest
	var mutationKeys []string
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/upload-sessions"):
			mutationKeys = append(mutationKeys, r.Header.Get("Idempotency-Key"))
			writeUploadSession(t, w, plan, "upload-1", apigenapi.ManagedDataUploadSessionStatusOpen, []apigenapi.ManagedDataFileUploadResponse{{
				File: wireFile(t, file), Status: apigenapi.ManagedDataFileUploadStatusPending,
				Negotiation: apigenapi.ManagedDataUploadNegotiation{Protocol: apigenapi.ManagedDataUploadProtocolS3Multipart, S3Multipart: &apigenapi.ManagedDataS3MultipartNegotiation{CreateEndpoint: "/unused", MinimumPartSize: 4, MaximumPartSize: 6, MaximumParts: 3}},
			}})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/s3-multipart-uploads"):
			mutationKeys = append(mutationKeys, r.Header.Get("Idempotency-Key"))
			writeJSONTest(t, w, http.StatusCreated, apigenapi.ManagedDataS3MultipartUploadResponse{Id: "multipart-1", UploadSessionId: "upload-1", File: wireFile(t, file), Status: apigenapi.ManagedDataS3MultipartStatusOpen, CreatedAt: "2026-01-01T00:00:00Z"})
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/parts/") && strings.HasSuffix(r.URL.Path, "/sign"):
			var request apigenapi.ManagedDataS3MultipartSignPartRequest
			decodeJSONTest(t, r, &request)
			signedSizes = append(signedSizes, request.Size)
			partNumber, _ := strconv.Atoi(strings.Split(r.URL.Path, "/")[len(strings.Split(r.URL.Path, "/"))-2])
			writeJSONTest(t, w, http.StatusOK, apigenapi.ManagedDataS3MultipartSignedPartResponse{PartNumber: int32(partNumber), Url: fmt.Sprintf("%s/signed/%d?signature=must-not-leak", server.URL, partNumber), Headers: []apigenapi.ManagedDataHTTPHeader{{Name: "x-test", Value: "signed"}}, ExpiresAt: "2030-01-01T00:00:00Z"})
		case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/signed/"):
			if got := r.Header.Get("Authorization"); got != "" {
				t.Fatalf("signed PUT received Authorization = %q", got)
			}
			part, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatal(err)
			}
			uploaded = append(uploaded, part)
			w.Header().Set("ETag", fmt.Sprintf("etag-%d", len(uploaded)))
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/complete"):
			mutationKeys = append(mutationKeys, r.Header.Get("Idempotency-Key"))
			decodeJSONTest(t, r, &completed)
			writeJSONTest(t, w, http.StatusOK, apigenapi.ManagedDataS3MultipartUploadResponse{Id: "multipart-1", UploadSessionId: "upload-1", File: wireFile(t, file), Status: apigenapi.ManagedDataS3MultipartStatusCompleted, CreatedAt: "2026-01-01T00:00:00Z"})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/finalize"):
			mutationKeys = append(mutationKeys, r.Header.Get("Idempotency-Key"))
			writeUploadSession(t, w, plan, "upload-1", apigenapi.ManagedDataUploadSessionStatusCompleted, []apigenapi.ManagedDataFileUploadResponse{{File: wireFile(t, file), Status: apigenapi.ManagedDataFileUploadStatusVerified, Negotiation: apigenapi.ManagedDataUploadNegotiation{Protocol: apigenapi.ManagedDataUploadProtocolAlreadyPresent}}})
		default:
			t.Fatalf("request = %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	err := runDataSync(context.Background(), dataSyncRequest{ProjectID: "demo", Connection: "orders", Root: root, Target: server.URL, Token: "secret-token", Plan: plan, Out: io.Discard, HTTPClient: server.Client()})
	if err != nil {
		t.Fatalf("runDataSync() error = %v", err)
	}
	if got := strings.Join([]string{string(uploaded[0]), string(uploaded[1]), string(uploaded[2])}, ""); got != string(body) {
		t.Fatalf("uploaded = %q", got)
	}
	if fmt.Sprint(signedSizes) != "[4 4 2]" {
		t.Fatalf("signed sizes = %v", signedSizes)
	}
	if len(completed.Parts) != 3 || completed.Parts[0].Etag != "etag-1" || completed.Parts[2].PartNumber != 3 {
		t.Fatalf("completed parts = %#v", completed.Parts)
	}
	if len(mutationKeys) != 4 {
		t.Fatalf("mutation keys = %#v", mutationKeys)
	}
	for _, key := range mutationKeys {
		if key == "" {
			t.Fatalf("mutation keys = %#v", mutationKeys)
		}
	}
}

func TestDataSyncDetectsMutationAndSanitizesSignedURL(t *testing.T) {
	root := t.TempDir()
	file := writeSyncFile(t, root, "orders.csv", []byte("before"))
	plan := syncPlan(root, file)
	if err := os.WriteFile(filepath.Join(root, file.Path), []byte("after!"), 0o600); err != nil {
		t.Fatal(err)
	}
	var aborted bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/upload-sessions"):
			writeUploadSession(t, w, plan, "upload-1", apigenapi.ManagedDataUploadSessionStatusOpen, []apigenapi.ManagedDataFileUploadResponse{{File: wireFile(t, file), Status: apigenapi.ManagedDataFileUploadStatusPending, Negotiation: apigenapi.ManagedDataUploadNegotiation{Protocol: apigenapi.ManagedDataUploadProtocolS3Multipart, S3Multipart: &apigenapi.ManagedDataS3MultipartNegotiation{MinimumPartSize: 4, MaximumPartSize: 6, MaximumParts: 3}}}})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/cancel"):
			aborted = true
			writeUploadSession(t, w, plan, "upload-1", apigenapi.ManagedDataUploadSessionStatusCancelled, nil)
		default:
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	err := runDataSync(context.Background(), dataSyncRequest{ProjectID: "demo", Connection: "orders", Root: root, Target: server.URL, Token: "secret-token", Plan: plan, Out: io.Discard, HTTPClient: server.Client()})
	if err == nil || !strings.Contains(err.Error(), "changed since planning") {
		t.Fatalf("runDataSync() error = %v", err)
	}
	if strings.Contains(err.Error(), "secret-token") || strings.Contains(err.Error(), "signature=") {
		t.Fatalf("error exposed secret material: %v", err)
	}
	if !aborted {
		t.Fatal("upload session was not aborted")
	}
}

func TestSignedPartFailureDoesNotExposeSignedURL(t *testing.T) {
	root := t.TempDir()
	file := writeSyncFile(t, root, "orders.csv", []byte("part"))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "signature=also-secret", http.StatusForbidden)
	}))
	defer server.Close()

	signedURL := server.URL + "/part?signature=must-not-leak"
	_, err := putSignedPart(context.Background(), server.Client(), apigenapi.ManagedDataS3MultipartSignedPartResponse{PartNumber: 1, Url: signedURL}, root, file, 0, file.Size, file.SHA256)
	if err == nil {
		t.Fatal("putSignedPart() error = nil")
	}
	if strings.Contains(err.Error(), "signature=") || strings.Contains(err.Error(), signedURL) {
		t.Fatalf("error exposed signed URL: %v", err)
	}
}

func syncPlan(root string, file manageddata.File) localplan.Result {
	return localplan.Result{Connection: "orders", Root: root, Manifest: manageddata.Manifest{Files: []manageddata.File{file}}}
}

func writeSyncFile(t *testing.T, root, name string, body []byte) manageddata.File {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, name), body, 0o600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(body)
	return manageddata.File{Path: name, Size: int64(len(body)), SHA256: hex.EncodeToString(sum[:])}
}

func wireFile(t *testing.T, file manageddata.File) apigenapi.ManagedDataFileMetadata {
	t.Helper()
	return apigenapi.ManagedDataFileMetadata{Path: file.Path, Size: file.Size, Sha256: file.SHA256}
}

func writeUploadSession(t *testing.T, w http.ResponseWriter, plan localplan.Result, id string, status apigenapi.ManagedDataUploadSessionStatus, files []apigenapi.ManagedDataFileUploadResponse) {
	t.Helper()
	wFiles := make([]apigenapi.ManagedDataFileMetadata, len(plan.Manifest.Files))
	for i, file := range plan.Manifest.Files {
		wFiles[i] = wireFile(t, file)
	}
	writeJSONTest(t, w, map[apigenapi.ManagedDataUploadSessionStatus]int{apigenapi.ManagedDataUploadSessionStatusOpen: http.StatusCreated}[status], apigenapi.ManagedDataUploadSessionResponse{
		Id: id, Project: "demo", Connection: "orders", RevisionId: plan.Manifest.RevisionID(), Status: status,
		Manifest: apigenapi.ManagedDataManifest{Files: wFiles}, Files: files, CreatedAt: "2026-01-01T00:00:00Z", ExpiresAt: "2030-01-01T00:00:00Z",
	})
}

func writeJSONTest(t *testing.T, w http.ResponseWriter, status int, value any) {
	t.Helper()
	if status == 0 {
		status = http.StatusAccepted
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Fatal(err)
	}
}

func decodeJSONTest(t *testing.T, r *http.Request, value any) {
	t.Helper()
	if err := json.NewDecoder(r.Body).Decode(value); err != nil {
		t.Fatal(err)
	}
}
