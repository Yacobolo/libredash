package mapassethttp

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	visualizationmapasset "github.com/Yacobolo/leapview/internal/visualization/mapasset"
)

func TestVerifyDeliveryChecksFullAssetsAndBothArchiveRanges(t *testing.T) {
	root := t.TempDir()
	style := []byte(`{"version":8}`)
	archive := make([]byte, 192*1024)
	for index := range archive {
		archive[index] = byte(index % 251)
	}
	files := []visualizationmapasset.File{
		writeDeliveryFixture(t, root, "leapview-streets/styles/style/style.json", style),
		writeDeliveryFixture(t, root, "leapview-streets/archives/archive/basemap.pmtiles", archive),
	}
	handler := deliveryFixtureHandler(t, root, false)
	server := httptest.NewServer(handler)
	defer server.Close()

	summary, err := verifyDelivery(context.Background(), root, server.URL, files, server.Client(), deliveryLimits{fullFileMaximum: 128 * 1024, rangeSize: 64 * 1024})
	if err != nil {
		t.Fatal(err)
	}
	if summary.Files != 2 || summary.FullResponses != 1 || summary.RangeResponses != 2 {
		t.Fatalf("summary = %#v", summary)
	}
	wantBytes := int64(len(style) + 2*64*1024)
	if summary.Bytes != wantBytes {
		t.Fatalf("transferred bytes = %d, want %d", summary.Bytes, wantBytes)
	}
}

func TestVerifyDeliveryRejectsMutableOrCorruptResponses(t *testing.T) {
	root := t.TempDir()
	content := []byte("immutable style")
	file := writeDeliveryFixture(t, root, "leapview-streets/styles/style/style.json", content)

	for _, test := range []struct {
		name    string
		mutable bool
		corrupt bool
	}{
		{name: "mutable cache", mutable: true},
		{name: "corrupt body", corrupt: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			handler := deliveryFixtureHandler(t, root, test.mutable)
			if test.corrupt {
				handler = http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
					w.Header().Set("Accept-Ranges", "bytes")
					w.Header().Set("Cache-Control", visualizationmapasset.ImmutableCacheControl)
					w.Header().Set("Content-Type", visualizationmapasset.ContentType(file.Path))
					http.ServeContent(w, request, filepath.Base(file.Path), time.Unix(0, 0), strings.NewReader("forged response"))
				})
			}
			server := httptest.NewServer(handler)
			defer server.Close()
			if _, err := verifyDelivery(context.Background(), root, server.URL, []visualizationmapasset.File{file}, server.Client(), deliveryLimits{fullFileMaximum: 128 * 1024, rangeSize: 64 * 1024}); err == nil {
				t.Fatal("invalid delivery was accepted")
			}
		})
	}
}

func TestVerifyDeliveryRejectsCorruptArchiveRanges(t *testing.T) {
	root := t.TempDir()
	archive := make([]byte, 192*1024)
	for index := range archive {
		archive[index] = byte(index % 251)
	}
	file := writeDeliveryFixture(t, root, "leapview-streets/archives/archive/basemap.pmtiles", archive)
	remote := append([]byte(nil), archive...)
	remote[len(remote)-1] ^= 0xff
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		w.Header().Set("Accept-Ranges", "bytes")
		w.Header().Set("Cache-Control", visualizationmapasset.ImmutableCacheControl)
		w.Header().Set("Content-Type", visualizationmapasset.ContentType(file.Path))
		http.ServeContent(w, request, filepath.Base(file.Path), time.Unix(0, 0), bytes.NewReader(remote))
	}))
	defer server.Close()

	if _, err := verifyDelivery(context.Background(), root, server.URL, []visualizationmapasset.File{file}, server.Client(), deliveryLimits{fullFileMaximum: 128 * 1024, rangeSize: 64 * 1024}); err == nil || !strings.Contains(err.Error(), "differs from the verified package") {
		t.Fatalf("verifyDelivery() error = %v, want corrupt range rejection", err)
	}
}

func TestVerifyDeliveryBlocksCrossOriginRedirects(t *testing.T) {
	root := t.TempDir()
	content := []byte("style")
	file := writeDeliveryFixture(t, root, "leapview-streets/styles/style/style.json", content)
	var redirected atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { redirected.Add(1) }))
	defer target.Close()
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		http.Redirect(w, request, target.URL+request.URL.Path, http.StatusFound)
	}))
	defer origin.Close()

	if _, err := verifyDelivery(context.Background(), root, origin.URL, []visualizationmapasset.File{file}, origin.Client(), deliveryLimits{fullFileMaximum: 128 * 1024, rangeSize: 64 * 1024}); err == nil || !strings.Contains(err.Error(), "cross-origin") {
		t.Fatalf("verifyDelivery() error = %v, want cross-origin rejection", err)
	}
	if redirected.Load() != 0 {
		t.Fatal("cross-origin redirect was followed")
	}
}

func writeDeliveryFixture(t *testing.T, root, path string, content []byte) visualizationmapasset.File {
	t.Helper()
	name := filepath.Join(root, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, content, 0o644); err != nil {
		t.Fatal(err)
	}
	return visualizationmapasset.File{Path: path, Digest: fmt.Sprintf("%x", sha256.Sum256(content))}
}

func deliveryFixtureHandler(t *testing.T, root string, mutable bool) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		path := strings.TrimPrefix(request.URL.Path, "/map-assets/")
		name := filepath.Join(root, filepath.FromSlash(path))
		info, err := os.Stat(name)
		if err != nil {
			http.NotFound(w, request)
			return
		}
		file, err := os.Open(name)
		if err != nil {
			http.Error(w, "open fixture", http.StatusInternalServerError)
			return
		}
		defer file.Close()
		w.Header().Set("Accept-Ranges", "bytes")
		if mutable {
			w.Header().Set("Cache-Control", "no-cache")
		} else {
			w.Header().Set("Cache-Control", visualizationmapasset.ImmutableCacheControl)
		}
		w.Header().Set("Content-Type", visualizationmapasset.ContentType(path))
		http.ServeContent(w, request, filepath.Base(path), info.ModTime(), file)
	})
}
