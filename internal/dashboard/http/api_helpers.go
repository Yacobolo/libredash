package http

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	nethttp "net/http"
	"strings"
	"time"
)

type pageResponse struct {
	NextCursor string `json:"nextCursor"`
}

func pagedResponseWithCursor(items any, nextCursor string) map[string]any {
	return map[string]any{"items": items, "page": pageResponse{NextCursor: nextCursor}}
}

func pageSliceForRequest[T any](w nethttp.ResponseWriter, r *nethttp.Request, items []T) ([]T, string, bool) {
	limit, ok := apiLimitForRequest(w, r)
	if !ok {
		return nil, "", false
	}
	scope, snapshot := dashboardRequestCursorScope(r, nil), dashboardServingSnapshot(r)
	start, ok := apiCursorOffsetForRequest(w, r, scope, snapshot)
	if !ok {
		return nil, "", false
	}
	if start > len(items) {
		start = len(items)
	}
	end := start + limit
	if end > len(items) {
		end = len(items)
	}
	nextCursor := ""
	if end < len(items) {
		nextCursor = encodeIndexCursor(end, scope, snapshot)
	}
	return append([]T(nil), items[start:end]...), nextCursor, true
}

const (
	defaultAPILimit = 50
	maxAPILimit     = 200
)

func apiLimitForRequest(w nethttp.ResponseWriter, r *nethttp.Request) (int, bool) {
	limit, err := parseAPILimit(r.URL.Query().Get("limit"))
	if err != nil {
		writeJSONError(w, err, nethttp.StatusBadRequest)
		return 0, false
	}
	return limit, true
}

func parseAPILimit(value string) (int, error) {
	if value == "" {
		return defaultAPILimit, nil
	}
	var limit int
	if _, err := fmt.Sscanf(value, "%d", &limit); err != nil {
		return 0, fmt.Errorf("limit must be an integer")
	}
	if limit < 1 {
		return 0, fmt.Errorf("limit must be at least 1")
	}
	if limit > maxAPILimit {
		return maxAPILimit, nil
	}
	return limit, nil
}

func apiCursorOffsetForRequest(w nethttp.ResponseWriter, r *nethttp.Request, scopes ...string) (int, bool) {
	offset, err := decodeIndexCursor(r.URL.Query().Get("pageToken"), scopes...)
	if err != nil {
		status := nethttp.StatusBadRequest
		if errors.Is(err, errDashboardCursorSnapshot) {
			status = nethttp.StatusConflict
		}
		writeJSONError(w, err, status)
		return 0, false
	}
	return offset, true
}

const dashboardCursorLifetime = 15 * time.Minute

var dashboardCursorKey = func() [32]byte {
	var key [32]byte
	if _, err := rand.Read(key[:]); err != nil {
		panic(fmt.Sprintf("initialize dashboard cursor key: %v", err))
	}
	return key
}()

var errDashboardCursorSnapshot = errors.New("cursor serving snapshot is unavailable")

type dashboardIndexCursor struct {
	Offset   int    `json:"offset"`
	Scope    string `json:"scope"`
	Snapshot string `json:"snapshot,omitempty"`
	Expires  int64  `json:"expires"`
}

func decodeIndexCursor(token string, scopes ...string) (int, error) {
	if token == "" {
		return 0, nil
	}
	if !strings.HasPrefix(token, "d1.") {
		return 0, fmt.Errorf("invalid page token")
	}
	token = strings.TrimPrefix(token, "d1.")
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(raw) <= sha256.Size {
		return 0, fmt.Errorf("invalid page token")
	}
	payload, signature := raw[:len(raw)-sha256.Size], raw[len(raw)-sha256.Size:]
	mac := hmac.New(sha256.New, dashboardCursorKey[:])
	_, _ = mac.Write(payload)
	if !hmac.Equal(signature, mac.Sum(nil)) {
		return 0, fmt.Errorf("invalid page token")
	}
	var cursor dashboardIndexCursor
	if json.Unmarshal(payload, &cursor) != nil || cursor.Offset < 0 || cursor.Expires < time.Now().Unix() {
		return 0, fmt.Errorf("invalid page token")
	}
	scope, snapshot := dashboardCursorScopeParts(scopes...)
	if cursor.Snapshot != snapshot {
		return 0, errDashboardCursorSnapshot
	}
	if cursor.Scope != scope {
		return 0, fmt.Errorf("invalid page token")
	}
	return cursor.Offset, nil
}

func encodeIndexCursor(offset int, scopes ...string) string {
	scope, snapshot := dashboardCursorScopeParts(scopes...)
	payload, _ := json.Marshal(dashboardIndexCursor{Offset: offset, Scope: scope, Snapshot: snapshot, Expires: time.Now().Add(dashboardCursorLifetime).Unix()})
	mac := hmac.New(sha256.New, dashboardCursorKey[:])
	_, _ = mac.Write(payload)
	return "d1." + base64.RawURLEncoding.EncodeToString(append(payload, mac.Sum(nil)...))
}

func dashboardCursorScopeParts(scopes ...string) (string, string) {
	if len(scopes) == 0 || strings.TrimSpace(scopes[0]) == "" {
		return "list", ""
	}
	snapshot := ""
	if len(scopes) > 1 {
		snapshot = scopes[1]
	}
	return scopes[0], snapshot
}

func dashboardRequestCursorScope(r *nethttp.Request, payload any) string {
	query := r.URL.Query()
	query.Del("pageToken")
	body, _ := json.Marshal(payload)
	digest := sha256.Sum256([]byte(r.Method + "\n" + r.URL.Path + "\n" + query.Encode() + "\n" + string(body)))
	return hex.EncodeToString(digest[:])
}

func dashboardServingSnapshot(r *nethttp.Request) string {
	if value := strings.TrimSpace(r.Header.Get("X-Serving-Snapshot")); value != "" {
		return value
	}
	return "unversioned"
}

func writeJSON(w nethttp.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeJSONError(w nethttp.ResponseWriter, err error, status int) {
	writeJSON(w, status, map[string]any{
		"code":      status,
		"message":   err.Error(),
		"details":   map[string]any{},
		"requestId": "",
	})
}

func decodeOptionalJSONBody(r *nethttp.Request, dst any) error {
	if r.Body == nil || r.Body == nethttp.NoBody {
		return nil
	}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		return fmt.Errorf("malformed JSON: %w", err)
	}
	var extra struct{}
	if err := decoder.Decode(&extra); err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		return fmt.Errorf("malformed JSON: %w", err)
	}
	return fmt.Errorf("malformed JSON: multiple JSON values")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
