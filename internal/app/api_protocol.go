package app

import (
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	apigenapi "github.com/Yacobolo/libredash/internal/api/gen"
)

type apiIdempotencyRecord struct {
	digest string
	ready  chan struct{}
	status int
	header http.Header
	body   []byte
}

const apiCursorLifetime = 15 * time.Minute

var apiCursorKey = func() [32]byte {
	var key [32]byte
	if _, err := rand.Read(key[:]); err != nil {
		panic(fmt.Sprintf("initialize API cursor key: %v", err))
	}
	return key
}()

type apiCursor struct {
	Value   string `json:"value"`
	Scope   string `json:"scope"`
	Expires int64  `json:"expires"`
}

func (s *Server) publicProtocolMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := strings.TrimSpace(r.Header.Get("X-Request-ID"))
		if requestID == "" {
			requestID = newAPIRequestID()
			r.Header.Set("X-Request-ID", requestID)
		}
		w.Header().Set("X-Request-ID", requestID)
		if s.auth != nil && !strings.HasPrefix(strings.ToLower(strings.TrimSpace(r.Header.Get("Authorization"))), "bearer ") {
			writeAPIProblem(w, r, http.StatusUnauthorized, "BEARER_REQUIRED", "The public API accepts bearer credentials only", nil)
			return
		}
		if !unwrapAPIPageCursor(w, r) {
			return
		}
		if !requiresAPIIdempotency(r) {
			next.ServeHTTP(w, r)
			return
		}
		s.serveIdempotent(w, r, next)
	})
}

func unwrapAPIPageCursor(w http.ResponseWriter, r *http.Request) bool {
	query := r.URL.Query()
	token := strings.TrimSpace(query.Get("pageToken"))
	if token == "" {
		return true
	}
	if strings.HasPrefix(token, "q1.") || strings.HasPrefix(token, "d1.") || strings.HasPrefix(token, "e1.") {
		return true
	}
	if !strings.HasPrefix(token, "g1.") {
		writeAPIProblem(w, r, http.StatusBadRequest, "INVALID_CURSOR", "The page cursor is invalid or expired", nil)
		return false
	}
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(token, "g1."))
	if err != nil || len(raw) <= sha256.Size {
		writeAPIProblem(w, r, http.StatusBadRequest, "INVALID_CURSOR", "The page cursor is invalid or expired", nil)
		return false
	}
	payload, signature := raw[:len(raw)-sha256.Size], raw[len(raw)-sha256.Size:]
	mac := hmac.New(sha256.New, apiCursorKey[:])
	_, _ = mac.Write(payload)
	var cursor apiCursor
	if !hmac.Equal(signature, mac.Sum(nil)) || json.Unmarshal(payload, &cursor) != nil || cursor.Expires < time.Now().Unix() || cursor.Scope != apiCursorScope(r) {
		writeAPIProblem(w, r, http.StatusBadRequest, "INVALID_CURSOR", "The page cursor is invalid or expired", nil)
		return false
	}
	query.Set("pageToken", cursor.Value)
	r.URL.RawQuery = query.Encode()
	return true
}

func signAPIPageCursor(r *http.Request, value string) string {
	value = strings.TrimSpace(value)
	if value == "" || strings.HasPrefix(value, "g1.") || strings.HasPrefix(value, "q1.") || strings.HasPrefix(value, "d1.") || strings.HasPrefix(value, "e1.") {
		return value
	}
	payload, _ := json.Marshal(apiCursor{Value: value, Scope: apiCursorScope(r), Expires: time.Now().Add(apiCursorLifetime).Unix()})
	mac := hmac.New(sha256.New, apiCursorKey[:])
	_, _ = mac.Write(payload)
	return "g1." + base64.RawURLEncoding.EncodeToString(append(payload, mac.Sum(nil)...))
}

func apiCursorScope(r *http.Request) string {
	query := r.URL.Query()
	query.Del("pageToken")
	digest := sha256.Sum256([]byte(r.Method + "\n" + r.URL.Path + "\n" + query.Encode()))
	return hex.EncodeToString(digest[:])
}

func signAPIResponseCursor(r *http.Request, body []byte) []byte {
	if r == nil || isQueryRequest(r) || len(body) == 0 {
		return body
	}
	var value map[string]any
	if json.Unmarshal(body, &value) != nil {
		return body
	}
	page, ok := value["page"].(map[string]any)
	if !ok {
		return body
	}
	next, _ := page["nextCursor"].(string)
	signed := signAPIPageCursor(r, next)
	if signed == next {
		return body
	}
	page["nextCursor"] = signed
	encoded, err := json.Marshal(value)
	if err != nil {
		return body
	}
	return append(encoded, '\n')
}

func requiresAPIIdempotency(r *http.Request) bool {
	if r == nil || r.Method != http.MethodPost || isQueryRequest(r) {
		return false
	}
	return !strings.HasPrefix(r.URL.Path, "/upload-protocols/tus")
}

func (s *Server) serveIdempotent(w http.ResponseWriter, r *http.Request, next http.Handler) {
	key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if key == "" || len(key) > 200 {
		writeAPIProblem(w, r, http.StatusBadRequest, "IDEMPOTENCY_KEY_REQUIRED", "Idempotency-Key must contain 1 to 200 characters", nil)
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeAPIProblem(w, r, http.StatusBadRequest, "INVALID_REQUEST_BODY", "The request body could not be read", nil)
		return
	}
	r.Body = io.NopCloser(bytes.NewReader(body))
	digest := apiRequestDigest(r, body)
	scopeHash := sha256.Sum256([]byte(strings.TrimSpace(r.Header.Get("Authorization"))))
	scope := hex.EncodeToString(scopeHash[:]) + ":" + key

	s.apiIdempotencyMu.Lock()
	if existing := s.apiIdempotency[scope]; existing != nil {
		if existing.digest != digest {
			s.apiIdempotencyMu.Unlock()
			writeAPIProblem(w, r, http.StatusConflict, "IDEMPOTENCY_KEY_REUSED", "Idempotency-Key was already used for a different request", nil)
			return
		}
		ready := existing.ready
		s.apiIdempotencyMu.Unlock()
		select {
		case <-ready:
			replayIdempotentResponse(w, existing)
		case <-r.Context().Done():
			writeAPIProblem(w, r, http.StatusRequestTimeout, "IDEMPOTENCY_WAIT_CANCELLED", "The original request did not finish before this request was cancelled", nil)
		}
		return
	}
	record := &apiIdempotencyRecord{digest: digest, ready: make(chan struct{})}
	s.apiIdempotency[scope] = record
	s.apiIdempotencyMu.Unlock()

	capture := newProtocolResponseCapture()
	next.ServeHTTP(capture, r)
	record.status = capture.statusCode()
	record.header = capture.header.Clone()
	record.body = append([]byte(nil), capture.body.Bytes()...)
	close(record.ready)
	capture.flush(w)
}

func apiRequestDigest(r *http.Request, body []byte) string {
	digest := sha256.New()
	_, _ = fmt.Fprintf(digest, "%s\n%s\n%s\n%s\n", r.Method, r.URL.EscapedPath(), r.URL.Query().Encode(), strings.ToLower(strings.TrimSpace(r.Header.Get("Content-Type"))))
	_, _ = digest.Write(body)
	return hex.EncodeToString(digest.Sum(nil))
}

func replayIdempotentResponse(w http.ResponseWriter, record *apiIdempotencyRecord) {
	for key, values := range record.header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	w.Header().Set("Idempotency-Replayed", "true")
	w.WriteHeader(record.status)
	if len(record.body) != 0 {
		_, _ = w.Write(record.body)
	}
}

type protocolResponseCapture struct {
	header http.Header
	body   bytes.Buffer
	status int
}

func newProtocolResponseCapture() *protocolResponseCapture {
	return &protocolResponseCapture{header: http.Header{}}
}

func (w *protocolResponseCapture) Header() http.Header { return w.header }

func (w *protocolResponseCapture) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
}

func (w *protocolResponseCapture) Write(value []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.body.Write(value)
}

func (w *protocolResponseCapture) statusCode() int {
	if w.status == 0 {
		return http.StatusOK
	}
	return w.status
}

func (w *protocolResponseCapture) flush(target http.ResponseWriter) {
	for key, values := range w.header {
		for _, value := range values {
			target.Header().Add(key, value)
		}
	}
	target.WriteHeader(w.statusCode())
	if w.body.Len() != 0 {
		_, _ = target.Write(w.body.Bytes())
	}
}

func newAPIRequestID() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "req_unavailable"
	}
	return "req_" + hex.EncodeToString(value[:])
}

func (s *Server) openAPIDescription(w http.ResponseWriter, r *http.Request) {
	spec, err := apigenapi.GetEmbeddedOpenAPISpec()
	if err != nil {
		writeAPIProblem(w, r, http.StatusInternalServerError, "OPENAPI_UNAVAILABLE", "The API description is unavailable", nil)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=300")
	_ = json.NewEncoder(w).Encode(spec)
}

func (s *Server) publicDocs(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=300")
	_, _ = w.Write([]byte(`<!doctype html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width"><title>LibreDash API</title></head><body><main><h1>LibreDash API v1</h1><p><a href="/api/openapi.json">OpenAPI description</a></p></main></body></html>`))
}
