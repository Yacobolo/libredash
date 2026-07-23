// Package mapassethttp serves the immutable cartographic inventory through an
// HTTP transport without moving transport dependencies into the domain.
package mapassethttp

import (
	"net/http"

	visualizationmapasset "github.com/Yacobolo/leapview/internal/visualization/mapasset"
)

// CacheHandler is the single HTTP boundary for installed or edge-backed map
// assets. Only exact files in the compiled inventory are readable, and their
// content type and cache policy match the object-publication contract.
func CacheHandler(next http.Handler) http.Handler {
	if next == nil {
		next = http.NotFoundHandler()
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !visualizationmapasset.IsContentAddressedURLPath(r.URL.Path) {
			w.Header().Set("Cache-Control", "no-store")
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			w.Header().Set("Cache-Control", "no-store")
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Cache-Control", visualizationmapasset.ImmutableCacheControl)
		w.Header().Set("Accept-Ranges", "bytes")
		w.Header().Set("Content-Type", visualizationmapasset.ContentType(r.URL.Path))
		next.ServeHTTP(w, r)
	})
}
