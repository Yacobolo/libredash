package app

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/Yacobolo/libredash/internal/analytics/materialize"
	apigenapi "github.com/Yacobolo/libredash/internal/api/gen"
	"github.com/Yacobolo/libredash/internal/deployment/apiadapter"
	"github.com/Yacobolo/libredash/internal/manageddata/control"
	"github.com/Yacobolo/libredash/internal/release"
)

const asyncCursorLifetime = 15 * time.Minute
const asyncHeartbeatInterval = 15 * time.Second
const asyncEventPollInterval = time.Second

type asyncEventLoader func(context.Context) ([]apigenapi.AsyncEventResponse, error)

var asyncCursorKey = func() [32]byte {
	var key [32]byte
	if _, err := rand.Read(key[:]); err != nil {
		panic(fmt.Sprintf("initialize async event cursor key: %v", err))
	}
	return key
}()

type asyncCursor struct {
	Scope   string `json:"scope"`
	Offset  int    `json:"offset"`
	Expires int64  `json:"expires"`
}

func releaseEvents(row release.Release) []apigenapi.AsyncEventResponse {
	events := []apigenapi.AsyncEventResponse{newAsyncEvent(1, "release.created", row.CreatedAt, map[string]any{
		"releaseId": row.ID, "projectId": row.ProjectID, "status": release.StatusDraft,
	})}
	artifacts := append([]release.Artifact(nil), row.Artifacts...)
	sort.Slice(artifacts, func(i, j int) bool {
		if artifacts[i].UploadedAt == artifacts[j].UploadedAt {
			return artifacts[i].WorkspaceID < artifacts[j].WorkspaceID
		}
		return artifacts[i].UploadedAt < artifacts[j].UploadedAt
	})
	for _, artifact := range artifacts {
		if artifact.UploadedAt == "" {
			continue
		}
		events = append(events, newAsyncEvent(len(events)+1, "release.artifact_uploaded", artifact.UploadedAt, map[string]any{
			"releaseId": row.ID, "workspaceId": artifact.WorkspaceID, "digest": artifact.ActualDigest,
		}))
	}
	if row.Status != release.StatusDraft {
		createdAt := row.FinalizedAt
		if createdAt == "" {
			createdAt = row.CreatedAt
		}
		data := map[string]any{"releaseId": row.ID, "status": row.Status}
		if row.Error != "" {
			data["error"] = row.Error
		}
		events = append(events, newAsyncEvent(len(events)+1, "release."+string(row.Status), createdAt, data))
	}
	return events
}

func deploymentEvents(row apiadapter.Deployment, releaseID string) []apigenapi.AsyncEventResponse {
	events := []apigenapi.AsyncEventResponse{newAsyncEvent(1, "deployment.created", row.CreatedAt, map[string]any{
		"deploymentId": row.ID, "projectId": row.Project, "releaseId": releaseID, "status": "queued",
	})}
	if row.Status != apiadapter.StatusPending {
		createdAt := row.ActivatedAt
		if createdAt == "" {
			createdAt = row.CreatedAt
		}
		data := map[string]any{"deploymentId": row.ID, "releaseId": releaseID, "status": row.Status}
		if row.Error != "" {
			data["error"] = row.Error
		}
		events = append(events, newAsyncEvent(2, "deployment."+string(row.Status), createdAt, data))
	}
	return events
}

func uploadSessionEvents(row control.UploadResult) []apigenapi.AsyncEventResponse {
	events := []apigenapi.AsyncEventResponse{newAsyncEvent(1, "upload_session.created", row.CreatedAt, map[string]any{
		"uploadSessionId": row.ID, "projectId": row.Collection.Project, "connectionId": row.Collection.Connection, "status": "open",
	})}
	if string(row.Status) != "open" {
		createdAt := row.CompletedAt
		if createdAt == "" {
			createdAt = row.CreatedAt
		}
		events = append(events, newAsyncEvent(2, "upload_session."+string(row.Status), createdAt, map[string]any{
			"uploadSessionId": row.ID, "status": row.Status,
		}))
	}
	return events
}

func refreshRunEvents(row materialize.RunRecord) []apigenapi.AsyncEventResponse {
	events := []apigenapi.AsyncEventResponse{newAsyncEvent(1, "refresh.queued", row.CreatedAt, map[string]any{
		"runId": row.ID, "workspaceId": row.WorkspaceID, "status": materialize.RunStatusQueued,
	})}
	if row.StartedAt != "" && row.Status != materialize.RunStatusQueued && row.Status != materialize.RunStatusCancelled {
		events = append(events, newAsyncEvent(len(events)+1, "refresh.running", row.StartedAt, map[string]any{"runId": row.ID, "status": materialize.RunStatusRunning}))
	}
	if row.FinishedAt != "" {
		data := map[string]any{"runId": row.ID, "status": row.Status}
		if row.Error != "" {
			data["error"] = row.Error
		}
		events = append(events, newAsyncEvent(len(events)+1, "refresh."+row.Status, row.FinishedAt, data))
	}
	return events
}

func newAsyncEvent(sequence int, event, createdAt string, data map[string]any) apigenapi.AsyncEventResponse {
	return apigenapi.AsyncEventResponse{Id: fmt.Sprintf("%020d", sequence), Event: event, Data: data, CreatedAt: createdAt}
}

func writeAsyncEventPage(w http.ResponseWriter, r *http.Request, events []apigenapi.AsyncEventResponse, limit *int32, token *string, scope string, loaders ...asyncEventLoader) {
	if acceptsEventStream(r.Header.Get("Accept")) {
		var loader asyncEventLoader
		if len(loaders) > 0 {
			loader = loaders[0]
		}
		writeAsyncEventStream(w, r, events, loader)
		return
	}
	pageLimit := 50
	if limit != nil {
		pageLimit = int(*limit)
	}
	if pageLimit < 1 || pageLimit > 200 {
		writeAPIProblem(w, r, http.StatusBadRequest, "INVALID_PAGE_LIMIT", "limit must be between 1 and 200", nil)
		return
	}
	pageToken := ""
	if token != nil {
		pageToken = *token
	}
	items, next, err := pageAsyncEvents(events, pageLimit, pageToken, scope)
	if err != nil {
		writeAPIProblem(w, r, http.StatusBadRequest, "INVALID_CURSOR", "The event cursor is invalid or expired", nil)
		return
	}
	page := apigenapi.PageInfo{}
	if next != "" {
		page.NextCursor = &next
	}
	writeAPIJSON(w, http.StatusOK, apigenapi.AsyncEventListResponse{Items: items, Page: page})
}

func acceptsEventStream(accept string) bool {
	for _, value := range strings.Split(accept, ",") {
		if strings.EqualFold(strings.TrimSpace(strings.SplitN(value, ";", 2)[0]), "text/event-stream") {
			return true
		}
	}
	return false
}

func writeAsyncEventStream(w http.ResponseWriter, r *http.Request, events []apigenapi.AsyncEventResponse, loader asyncEventLoader) {
	lastID := strings.TrimSpace(r.Header.Get("Last-Event-ID"))
	if _, ok := asyncEventsAfter(events, lastID); !ok {
		writeAPIProblem(w, r, http.StatusBadRequest, "INVALID_LAST_EVENT_ID", "Last-Event-ID does not identify an event in this resource", nil)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher, _ := w.(http.Flusher)
	heartbeat := time.NewTicker(asyncHeartbeatInterval)
	poll := time.NewTicker(asyncEventPollInterval)
	defer heartbeat.Stop()
	defer poll.Stop()

	for {
		pending, ok := asyncEventsAfter(events, lastID)
		if !ok {
			return
		}
		for _, event := range pending {
			payload, _ := json.Marshal(event.Data)
			_, _ = fmt.Fprintf(w, "id: %s\nevent: %s\ndata: %s\n\n", event.Id, event.Event, payload)
			lastID = event.Id
		}
		if flusher != nil {
			flusher.Flush()
		}
		if len(events) > 0 && terminalAsyncEvent(events[len(events)-1].Event) {
			return
		}

		select {
		case <-r.Context().Done():
			return
		case <-heartbeat.C:
			_, _ = fmt.Fprint(w, ": heartbeat\n\n")
			if flusher != nil {
				flusher.Flush()
			}
		case <-poll.C:
			if loader != nil {
				loaded, err := loader(r.Context())
				if err != nil {
					return
				}
				events = loaded
			}
		}
	}
}

func asyncEventsAfter(events []apigenapi.AsyncEventResponse, lastID string) ([]apigenapi.AsyncEventResponse, bool) {
	if lastID == "" {
		return events, true
	}
	for index, event := range events {
		if event.Id == lastID {
			return events[index+1:], true
		}
	}
	return nil, false
}

func terminalAsyncEvent(event string) bool {
	suffix := event
	if index := strings.LastIndexByte(event, '.'); index >= 0 {
		suffix = event[index+1:]
	}
	switch suffix {
	case "ready", "failed", "active", "succeeded", "complete", "completed", "cancelled", "canceled", "rolled_back":
		return true
	default:
		return false
	}
}

func pageAsyncEvents(events []apigenapi.AsyncEventResponse, limit int, token, scope string) ([]apigenapi.AsyncEventResponse, string, error) {
	offset := 0
	if token != "" {
		cursor, err := decodeAsyncCursor(token)
		if err != nil || cursor.Scope != scope || cursor.Expires < time.Now().Unix() || cursor.Offset < 0 || cursor.Offset > len(events) {
			return nil, "", fmt.Errorf("invalid cursor")
		}
		offset = cursor.Offset
	}
	end := offset + limit
	if end > len(events) {
		end = len(events)
	}
	next := ""
	if end < len(events) {
		next = encodeAsyncCursor(asyncCursor{Scope: scope, Offset: end, Expires: time.Now().Add(asyncCursorLifetime).Unix()})
	}
	return append([]apigenapi.AsyncEventResponse(nil), events[offset:end]...), next, nil
}

func encodeAsyncCursor(cursor asyncCursor) string {
	payload, _ := json.Marshal(cursor)
	mac := hmac.New(sha256.New, asyncCursorKey[:])
	_, _ = mac.Write(payload)
	value := append(payload, mac.Sum(nil)...)
	return "e1." + base64.RawURLEncoding.EncodeToString(value)
}

func decodeAsyncCursor(value string) (asyncCursor, error) {
	if !strings.HasPrefix(value, "e1.") {
		return asyncCursor{}, fmt.Errorf("invalid cursor")
	}
	value = strings.TrimPrefix(value, "e1.")
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil || len(raw) <= sha256.Size {
		return asyncCursor{}, fmt.Errorf("invalid cursor")
	}
	payload, signature := raw[:len(raw)-sha256.Size], raw[len(raw)-sha256.Size:]
	mac := hmac.New(sha256.New, asyncCursorKey[:])
	_, _ = mac.Write(payload)
	if !hmac.Equal(signature, mac.Sum(nil)) {
		return asyncCursor{}, fmt.Errorf("invalid cursor signature")
	}
	var cursor asyncCursor
	if err := json.Unmarshal(payload, &cursor); err != nil {
		return asyncCursor{}, err
	}
	return cursor, nil
}
