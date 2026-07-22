package app

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	apigenapi "github.com/Yacobolo/leapview/internal/api/gen"
	apiidempotencysqlite "github.com/Yacobolo/leapview/internal/apiidempotency/sqlite"
	"github.com/Yacobolo/leapview/internal/brand"
	"github.com/Yacobolo/leapview/internal/cursorsigning"
	"github.com/Yacobolo/leapview/internal/workspace"
)

type apiIdempotencyRecord struct {
	digest string
	ready  chan struct{}
	status int
	header http.Header
	body   []byte
}

const apiCursorLifetime = 15 * time.Minute
const apiIdempotencyLifetime = 24 * time.Hour
const apiIdempotencyLease = 30 * time.Second

type apiCursor struct {
	Value    string `json:"value"`
	Scope    string `json:"scope"`
	Snapshot string `json:"snapshot"`
	Expires  int64  `json:"expires"`
}

const apiCursorSnapshotHeader = "X-LeapView-Cursor-Snapshot"

func (s *Server) publicProtocolMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.authenticatePublicAPIRequest(w, r) {
			return
		}
		r.Header.Set(apiCursorSnapshotHeader, s.cursorSnapshot(r))
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

func (s *Server) authenticatePublicAPIRequest(w http.ResponseWriter, r *http.Request) bool {
	preparePublicAPIRequest(w, r)
	if bearerToken(r) == "" {
		writeAPIProblem(w, r, http.StatusUnauthorized, "BEARER_REQUIRED", "The public API accepts bearer credentials only", nil)
		return false
	}
	if s.auth != nil && !s.auth.acceptsPublicBearer(r) {
		writeAPIProblem(w, r, http.StatusUnauthorized, "INVALID_BEARER", "The bearer credential is invalid", nil)
		return false
	}
	return true
}

func preparePublicAPIRequest(w http.ResponseWriter, r *http.Request) {
	requestID := strings.TrimSpace(r.Header.Get("X-Request-ID"))
	if requestID == "" {
		requestID = newAPIRequestID()
		r.Header.Set("X-Request-ID", requestID)
	}
	w.Header().Set("X-Request-ID", requestID)
}

func unwrapAPIPageCursor(w http.ResponseWriter, r *http.Request) bool {
	query := r.URL.Query()
	token := strings.TrimSpace(query.Get("pageToken"))
	if token == "" {
		return true
	}
	if hasNativeCursorPrefix(token) {
		return true
	}
	if !strings.HasPrefix(token, "g1.") {
		writeAPIProblem(w, r, http.StatusBadRequest, "INVALID_CURSOR", "The page cursor is invalid or expired", nil)
		return false
	}
	payload, err := cursorsigning.Verify("g1", token)
	if err != nil {
		writeAPIProblem(w, r, http.StatusBadRequest, "INVALID_CURSOR", "The page cursor is invalid or expired", nil)
		return false
	}
	var cursor apiCursor
	if json.Unmarshal(payload, &cursor) != nil || cursor.Expires < time.Now().Unix() || cursor.Scope != apiCursorScope(r) {
		writeAPIProblem(w, r, http.StatusBadRequest, "INVALID_CURSOR", "The page cursor is invalid or expired", nil)
		return false
	}
	if cursor.Snapshot != apiCursorSnapshotForRequest(r) {
		writeAPIProblem(w, r, http.StatusConflict, "SNAPSHOT_UNAVAILABLE", "The serving snapshot bound to this cursor is no longer available", nil)
		return false
	}
	query.Set("pageToken", cursor.Value)
	r.URL.RawQuery = query.Encode()
	return true
}

func signAPIPageCursor(r *http.Request, value string) string {
	value = strings.TrimSpace(value)
	if value == "" || strings.HasPrefix(value, "g1.") || hasNativeCursorPrefix(value) {
		return value
	}
	payload, _ := json.Marshal(apiCursor{Value: value, Scope: apiCursorScope(r), Snapshot: apiCursorSnapshotForRequest(r), Expires: time.Now().Add(apiCursorLifetime).Unix()})
	return cursorsigning.Sign("g1", payload)
}

func hasNativeCursorPrefix(value string) bool {
	for _, prefix := range []string{"q1.", "q2.", "d1.", "d2.", "e1.", "s1."} {
		if strings.HasPrefix(value, prefix) {
			return true
		}
	}
	return false
}

func apiCursorSnapshotForRequest(r *http.Request) string {
	if r != nil {
		if snapshot := strings.TrimSpace(r.Header.Get(apiCursorSnapshotHeader)); snapshot != "" {
			return snapshot
		}
		segments := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		for index, segment := range segments {
			if segment == "workspaces" && index+1 < len(segments) {
				return "workspace:" + segments[index+1] + ":unversioned"
			}
			if segment == "projects" && index+1 < len(segments) {
				return "project:" + segments[index+1] + ":unversioned"
			}
		}
	}
	return "instance"
}

func (s *Server) cursorSnapshot(r *http.Request) string {
	fallback := apiCursorSnapshotForRequest(r)
	segments := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	for index, segment := range segments {
		if index+1 >= len(segments) {
			continue
		}
		switch segment {
		case "workspaces":
			repo, err := s.workspaceRepository()
			if err == nil && repo != nil {
				row, rowErr := repo.ByID(r.Context(), workspace.WorkspaceID(s.workspaceID(segments[index+1])))
				if rowErr == nil && row.ActiveServingStateID != "" {
					return string(row.ActiveServingStateID)
				}
			}
		case "projects":
			if repo := s.releaseRepository(); repo != nil {
				row, rowErr := repo.GetProject(r.Context(), segments[index+1])
				if rowErr == nil {
					if row.ActiveDeploymentID != "" {
						return "deployment:" + row.ActiveDeploymentID
					}
					if row.LatestReleaseID != "" {
						return "release:" + row.LatestReleaseID
					}
				}
			}
		}
	}
	return fallback
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
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			writeAPIProblem(w, r, http.StatusRequestEntityTooLarge, "CONTENT_TOO_LARGE", "The request body exceeds the configured size limit", nil)
			return
		}
		writeAPIProblem(w, r, http.StatusBadRequest, "INVALID_REQUEST_BODY", "The request body could not be read", nil)
		return
	}
	r.Body = io.NopCloser(bytes.NewReader(body))
	digest := apiRequestDigest(r, body)
	callerScope := ""
	if s.auth != nil {
		if principal, _, ok := s.auth.authenticate(r); ok {
			callerScope = principal.ID
		}
	}
	if callerScope == "" {
		scopeHash := sha256.Sum256([]byte(strings.TrimSpace(r.Header.Get("Authorization"))))
		callerScope = hex.EncodeToString(scopeHash[:])
	}
	scope := callerScope + ":" + r.Method + ":" + r.URL.EscapedPath() + ":" + key
	if s.apiIdempotencyStore != nil {
		s.serveDurableIdempotent(w, r, next, scope, digest)
		return
	}

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

func (s *Server) serveDurableIdempotent(w http.ResponseWriter, r *http.Request, next http.Handler, scope, digest string) {
	owner := newAPIRequestID()
	record, execute, err := s.apiIdempotencyStore.Claim(r.Context(), scope, digest, owner, apiIdempotencyLease, apiIdempotencyLifetime)
	if err != nil {
		writeAPIProblem(w, r, http.StatusInternalServerError, "IDEMPOTENCY_UNAVAILABLE", "Idempotency state is unavailable", nil)
		return
	}
	if record.Digest != digest {
		writeAPIProblem(w, r, http.StatusConflict, "IDEMPOTENCY_KEY_REUSED", "Idempotency-Key was already used for a different request", nil)
		return
	}
	if !execute {
		if record.Status == 0 {
			record, execute, err = waitForAPIIdempotency(r, s.apiIdempotencyStore, scope, digest, owner)
			if err != nil {
				if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
					writeAPIProblem(w, r, http.StatusRequestTimeout, "IDEMPOTENCY_WAIT_CANCELLED", "The original request did not finish before this request was cancelled", nil)
					return
				}
				writeAPIProblem(w, r, http.StatusInternalServerError, "IDEMPOTENCY_UNAVAILABLE", "Idempotency state is unavailable", nil)
				return
			}
		}
		if record.Digest != digest {
			writeAPIProblem(w, r, http.StatusConflict, "IDEMPOTENCY_KEY_REUSED", "Idempotency-Key was already used for a different request", nil)
			return
		}
		if !execute {
			replayStoredIdempotentResponse(w, record.Status, record.Header, record.Body)
			return
		}
	}

	leaseCtx, stopLease := context.WithCancel(context.Background())
	defer stopLease()
	go renewAPIIdempotencyLease(leaseCtx, s.apiIdempotencyStore, scope, digest, owner)
	capture := newProtocolResponseCapture()
	next.ServeHTTP(capture, r)
	record.Status = capture.statusCode()
	record.Header = capture.header.Clone()
	record.Body = append([]byte(nil), capture.body.Bytes()...)
	persistCtx, cancelPersist := context.WithTimeout(context.WithoutCancel(r.Context()), 5*time.Second)
	defer cancelPersist()
	if record.Status >= http.StatusInternalServerError {
		if err := s.apiIdempotencyStore.Abandon(persistCtx, scope, digest, owner); err != nil {
			writeAPIProblem(w, r, http.StatusInternalServerError, "IDEMPOTENCY_UNAVAILABLE", "The failed request lease could not be released", nil)
			return
		}
		capture.flush(w)
		return
	}
	if err := s.apiIdempotencyStore.Complete(persistCtx, scope, digest, owner, record.Status, record.Header, record.Body); err != nil {
		writeAPIProblem(w, r, http.StatusInternalServerError, "IDEMPOTENCY_UNAVAILABLE", "The response could not be committed to durable idempotency state", nil)
		return
	}
	capture.flush(w)
}

func waitForAPIIdempotency(r *http.Request, store *apiidempotencysqlite.Store, scope, digest, owner string) (apiidempotencysqlite.Record, bool, error) {
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		record, err := store.Load(r.Context(), scope)
		if err != nil {
			return apiidempotencysqlite.Record{}, false, err
		}
		if record.Digest != digest || record.Status != 0 {
			return record, false, nil
		}
		if !record.LeaseExpires.After(time.Now().UTC()) {
			record, execute, claimErr := store.Claim(r.Context(), scope, digest, owner, apiIdempotencyLease, apiIdempotencyLifetime)
			if claimErr != nil {
				return apiidempotencysqlite.Record{}, false, claimErr
			}
			if execute || record.Digest != digest || record.Status != 0 {
				return record, execute, nil
			}
		}
		select {
		case <-r.Context().Done():
			return apiidempotencysqlite.Record{}, false, r.Context().Err()
		case <-ticker.C:
		}
	}
}

func renewAPIIdempotencyLease(ctx context.Context, store *apiidempotencysqlite.Store, scope, digest, owner string) {
	ticker := time.NewTicker(apiIdempotencyLease / 3)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = store.Renew(ctx, scope, digest, owner, apiIdempotencyLease)
		}
	}
}

func apiRequestDigest(r *http.Request, body []byte) string {
	digest := sha256.New()
	_, _ = fmt.Fprintf(digest, "%s\n%s\n%s\n%s\n", r.Method, r.URL.EscapedPath(), r.URL.Query().Encode(), strings.ToLower(strings.TrimSpace(r.Header.Get("Content-Type"))))
	_, _ = digest.Write(body)
	return hex.EncodeToString(digest.Sum(nil))
}

func replayIdempotentResponse(w http.ResponseWriter, record *apiIdempotencyRecord) {
	replayStoredIdempotentResponse(w, record.status, record.header, record.body)
}

func replayStoredIdempotentResponse(w http.ResponseWriter, status int, header http.Header, body []byte) {
	for key, values := range header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	w.Header().Set("Idempotency-Replayed", "true")
	w.WriteHeader(status)
	if len(body) != 0 {
		_, _ = w.Write(body)
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
	_, _ = fmt.Fprintf(w, `<!doctype html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width"><title>%s API</title></head><body><main><h1>%s API v1</h1><p><a href="/api/openapi.json">OpenAPI description</a></p></main></body></html>`, brand.Name, brand.Name)
}
