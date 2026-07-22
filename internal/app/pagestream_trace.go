package app

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/Yacobolo/leapview/pkg/pagestream"
)

type pageStreamTraceResponse struct {
	Events    []pagestream.TraceEvent `json:"events"`
	NextAfter uint64                  `json:"nextAfter"`
}

type pageStreamSignalsResponse struct {
	StreamID  string                    `json:"streamId"`
	State     map[string]any            `json:"state"`
	Leaves    []pagestream.SignalLeaf   `json:"leaves"`
	History   []pagestream.SignalChange `json:"history"`
	NextAfter uint64                    `json:"nextAfter"`
}

func (s *Server) pageStreamTraces(w http.ResponseWriter, r *http.Request) {
	if s.pageStreamTrace == nil {
		http.NotFound(w, r)
		return
	}
	after, err := optionalUint64(r.URL.Query().Get("after"))
	if err != nil {
		http.Error(w, "after must be an unsigned integer", http.StatusBadRequest)
		return
	}
	limit, err := optionalInt(r.URL.Query().Get("limit"))
	if err != nil {
		http.Error(w, "limit must be an integer", http.StatusBadRequest)
		return
	}
	events := s.pageStreamTrace.Events(pagestream.TraceQuery{
		After: after, StreamID: strings.TrimSpace(r.URL.Query().Get("streamId")), Limit: limit,
	})
	nextAfter := after
	if len(events) > 0 {
		nextAfter = events[len(events)-1].ID
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(pageStreamTraceResponse{Events: events, NextAfter: nextAfter})
}

func (s *Server) pageStreamSignals(w http.ResponseWriter, r *http.Request) {
	if s.pageStreamTrace == nil {
		http.NotFound(w, r)
		return
	}
	after, err := optionalUint64(r.URL.Query().Get("after"))
	if err != nil {
		http.Error(w, "after must be an unsigned integer", http.StatusBadRequest)
		return
	}
	limit, err := optionalInt(r.URL.Query().Get("limit"))
	if err != nil {
		http.Error(w, "limit must be an integer", http.StatusBadRequest)
		return
	}
	path := strings.TrimSpace(r.URL.Query().Get("path"))
	if path != "" && !strings.HasPrefix(path, "/") {
		http.Error(w, "path must be a JSON Pointer", http.StatusBadRequest)
		return
	}
	requestedStreamID := strings.TrimSpace(r.URL.Query().Get("streamId"))
	snapshot, ok := s.pageStreamTrace.SignalSnapshot(requestedStreamID)
	if !ok {
		snapshot = pagestream.SignalSnapshot{StreamID: requestedStreamID, State: map[string]any{}, Leaves: []pagestream.SignalLeaf{}}
	}
	history := s.pageStreamTrace.SignalChanges(pagestream.SignalHistoryQuery{
		After: after, StreamID: snapshot.StreamID, Path: path, Limit: limit,
	})
	nextAfter := after
	if len(history) > 0 {
		nextAfter = history[len(history)-1].ID
	}
	response := pageStreamSignalsResponse{
		StreamID:  snapshot.StreamID,
		State:     snapshot.State,
		Leaves:    snapshot.Leaves,
		History:   history,
		NextAfter: nextAfter,
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(response)
}

func optionalUint64(value string) (uint64, error) {
	if strings.TrimSpace(value) == "" {
		return 0, nil
	}
	return strconv.ParseUint(value, 10, 64)
}

func optionalInt(value string) (int, error) {
	if strings.TrimSpace(value) == "" {
		return 0, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, err
	}
	return parsed, nil
}
