package app

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	apigenapi "github.com/Yacobolo/libredash/internal/api/gen"
	"github.com/Yacobolo/libredash/internal/asyncjob"
	"github.com/Yacobolo/libredash/internal/cursorsigning"
)

const asyncCursorLifetime = 15 * time.Minute
const asyncHeartbeatInterval = 15 * time.Second
const asyncEventPollInterval = time.Second
const asyncStreamAuthorizationLifetime = 5 * time.Minute

func writeStoredAsyncEventPage(w http.ResponseWriter, r *http.Request, repo asyncjob.Repository, resourceKind, resourceID string, limit *int32, token *string, scope string) {
	if acceptsEventStream(r.Header.Get("Accept")) {
		writeStoredAsyncEventStream(w, r, repo, resourceKind, resourceID)
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
		pageToken = strings.TrimSpace(*token)
	}
	after, err := asyncEventCursorAfter(pageToken, scope)
	if err != nil {
		writeAPIProblem(w, r, http.StatusBadRequest, "INVALID_CURSOR", "The event cursor is invalid or expired", nil)
		return
	}
	rows, err := repo.ListEvents(r.Context(), resourceKind, resourceID, after, pageLimit)
	if err != nil {
		writeAPIProblem(w, r, http.StatusInternalServerError, "ASYNC_EVENT_READ_FAILED", "Events could not be loaded", nil)
		return
	}
	items, err := asyncEventResponses(rows)
	if err != nil {
		writeAPIProblem(w, r, http.StatusInternalServerError, "ASYNC_EVENT_READ_FAILED", "Events could not be decoded", nil)
		return
	}
	next := ""
	if len(rows) == pageLimit {
		probe, probeErr := repo.ListEvents(r.Context(), resourceKind, resourceID, rows[len(rows)-1].ID, 1)
		if probeErr != nil {
			writeAPIProblem(w, r, http.StatusInternalServerError, "ASYNC_EVENT_READ_FAILED", "Events could not be loaded", nil)
			return
		}
		if len(probe) != 0 {
			next = encodeAsyncCursor(asyncCursor{Scope: scope, LastID: fmt.Sprintf("%020d", rows[len(rows)-1].ID), Expires: time.Now().Add(asyncCursorLifetime).Unix()})
		}
	}
	page := apigenapi.PageInfo{}
	if next != "" {
		page.NextCursor = &next
	}
	writeAPIJSON(w, http.StatusOK, apigenapi.AsyncEventListResponse{Items: items, Page: page})
}

func asyncEventCursorAfter(token, scope string) (int64, error) {
	if token == "" {
		return 0, nil
	}
	cursor, err := decodeAsyncCursor(token)
	if err != nil || cursor.Scope != scope || cursor.Expires < time.Now().Unix() || cursor.LastID == "" {
		return 0, fmt.Errorf("invalid cursor")
	}
	after, err := strconv.ParseInt(cursor.LastID, 10, 64)
	if err != nil || after < 1 {
		return 0, fmt.Errorf("invalid cursor")
	}
	return after, nil
}

func asyncEventResponses(rows []asyncjob.Event) ([]apigenapi.AsyncEventResponse, error) {
	events := make([]apigenapi.AsyncEventResponse, 0, len(rows))
	for _, row := range rows {
		data := map[string]any{}
		if err := json.Unmarshal(row.Data, &data); err != nil {
			return nil, err
		}
		response := apigenapi.AsyncEventResponse{
			Id: fmt.Sprintf("%020d", row.ID), Event: row.EventType,
			ResourceType: row.ResourceKind, ResourceId: row.ResourceID,
			Data: data, CreatedAt: row.CreatedAt,
		}
		if raw, ok := data["progress"].(map[string]any); ok {
			encoded, _ := json.Marshal(raw)
			var progress apigenapi.AsyncProgress
			if json.Unmarshal(encoded, &progress) == nil {
				response.Progress = &progress
				delete(data, "progress")
			}
		}
		if raw, ok := data["error"].(map[string]any); ok {
			encoded, _ := json.Marshal(raw)
			var problem apigenapi.AsyncStructuredError
			if json.Unmarshal(encoded, &problem) == nil && problem.Code != "" && problem.Detail != "" {
				response.Error = &problem
				delete(data, "error")
			}
		}
		events = append(events, response)
	}
	return events, nil
}

func writeStoredAsyncEventStream(w http.ResponseWriter, r *http.Request, repo asyncjob.Repository, resourceKind, resourceID string) {
	lastID := strings.TrimSpace(r.Header.Get("Last-Event-ID"))
	after := int64(0)
	if lastID != "" {
		parsed, err := strconv.ParseInt(lastID, 10, 64)
		if err != nil || parsed < 1 {
			writeAPIProblem(w, r, http.StatusBadRequest, "INVALID_LAST_EVENT_ID", "Last-Event-ID does not identify an event in this resource", nil)
			return
		}
		probe, err := repo.ListEvents(r.Context(), resourceKind, resourceID, parsed-1, 1)
		if err != nil || len(probe) != 1 || probe[0].ID != parsed {
			writeAPIProblem(w, r, http.StatusBadRequest, "INVALID_LAST_EVENT_ID", "Last-Event-ID does not identify an event in this resource", nil)
			return
		}
		after = parsed
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher, _ := w.(http.Flusher)
	heartbeat := time.NewTicker(asyncHeartbeatInterval)
	poll := time.NewTicker(asyncEventPollInterval)
	reauthorize := time.NewTimer(asyncStreamAuthorizationLifetime)
	defer heartbeat.Stop()
	defer poll.Stop()
	defer reauthorize.Stop()
	for {
		rows, err := repo.ListEvents(r.Context(), resourceKind, resourceID, after, 200)
		if err != nil {
			return
		}
		for _, row := range rows {
			responses, responseErr := asyncEventResponses([]asyncjob.Event{row})
			if responseErr != nil {
				return
			}
			payload, _ := json.Marshal(responses[0])
			_, _ = fmt.Fprintf(w, "id: %020d\nevent: %s\ndata: %s\n\n", row.ID, row.EventType, payload)
			after = row.ID
			if terminalAsyncEvent(row.EventType) {
				if flusher != nil {
					flusher.Flush()
				}
				return
			}
		}
		if flusher != nil && len(rows) != 0 {
			flusher.Flush()
		}
		if len(rows) == 200 {
			continue
		}
		select {
		case <-r.Context().Done():
			return
		case <-reauthorize.C:
			return
		case <-heartbeat.C:
			_, _ = fmt.Fprint(w, ": heartbeat\n\n")
			if flusher != nil {
				flusher.Flush()
			}
		case <-poll.C:
		}
	}
}

type asyncCursor struct {
	Scope   string `json:"scope"`
	LastID  string `json:"lastId"`
	Expires int64  `json:"expires"`
}

func acceptsEventStream(accept string) bool {
	for _, value := range strings.Split(accept, ",") {
		if strings.EqualFold(strings.TrimSpace(strings.SplitN(value, ";", 2)[0]), "text/event-stream") {
			return true
		}
	}
	return false
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

func encodeAsyncCursor(cursor asyncCursor) string {
	payload, _ := json.Marshal(cursor)
	return cursorsigning.Sign("e1", payload)
}

func decodeAsyncCursor(value string) (asyncCursor, error) {
	if !strings.HasPrefix(value, "e1.") {
		return asyncCursor{}, fmt.Errorf("invalid cursor")
	}
	payload, err := cursorsigning.Verify("e1", value)
	if err != nil {
		return asyncCursor{}, fmt.Errorf("invalid cursor")
	}
	var cursor asyncCursor
	if err := json.Unmarshal(payload, &cursor); err != nil {
		return asyncCursor{}, err
	}
	return cursor, nil
}
