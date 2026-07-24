package app

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Yacobolo/leapview/pkg/pagestream"
)

func TestDevelopmentPageStreamTraceEndpointReturnsSanitizedEvents(t *testing.T) {
	t.Setenv("LEAPVIEW_PRODUCTION", "")
	var logs bytes.Buffer
	server := assembleRuntime(fakeMetrics{}, assemblyConfig{
		Logger: slog.New(slog.NewJSONHandler(&logs, nil)),
	})
	server.runtime.broker.PublishEnvelope("trace:test", pagestream.Envelope{
		Signals: pagestream.SignalPatch{"status": "ready", "token": "private"},
		Trace:   pagestream.TraceMetadata{Origin: "test.publisher"},
	})

	req := httptest.NewRequest(http.MethodGet, "/__dev/pagestream/traces?streamId=trace%3Atest", nil)
	rec := httptest.NewRecorder()
	server.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("trace response = %d headers=%v body=%s", rec.Code, rec.Header(), rec.Body.String())
	}
	var response struct {
		Events    []pagestream.TraceEvent `json:"events"`
		NextAfter uint64                  `json:"nextAfter"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode trace response: %v", err)
	}
	if len(response.Events) != 1 || response.NextAfter != response.Events[0].ID || response.Events[0].Payload["token"] != "[REDACTED]" {
		t.Fatalf("trace response = %#v", response)
	}
	if strings.Contains(logs.String(), "private") || !strings.Contains(logs.String(), "pagestream signal") {
		t.Fatalf("trace logs = %s", logs.String())
	}
}

func TestProductionOmitsPageStreamTraceEndpoint(t *testing.T) {
	t.Setenv("LEAPVIEW_PRODUCTION", "1")
	server := newAppTestHarness(fakeMetrics{})
	req := httptest.NewRequest(http.MethodGet, "/__dev/pagestream/traces", nil)
	rec := httptest.NewRecorder()
	server.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("production trace status = %d, want 404", rec.Code)
	}
}

func TestDevelopmentPageStreamSignalsEndpointReturnsStateAndSelectedHistory(t *testing.T) {
	t.Setenv("LEAPVIEW_PRODUCTION", "")
	server := newAppTestHarness(fakeMetrics{})
	server.runtime.pageStreamTrace.Record(pagestream.TraceRecord{
		StreamID: "trace:test", Stage: pagestream.TraceStageDelivered,
		Signals:    pagestream.SignalPatch{"status": map[string]any{"progressPercent": 0}},
		Generation: 7, Origin: "dashboard.refresh", CorrelationID: "refresh-7",
	})
	server.runtime.pageStreamTrace.Record(pagestream.TraceRecord{
		StreamID: "trace:test", Stage: pagestream.TraceStageDelivered,
		Signals:    pagestream.SignalPatch{"status": map[string]any{"progressPercent": 50}},
		Generation: 7, Origin: "dashboard.refresh", CorrelationID: "refresh-7",
	})

	req := httptest.NewRequest(http.MethodGet, "/__dev/pagestream/signals?streamId=trace%3Atest&path=%2Fstatus%2FprogressPercent", nil)
	rec := httptest.NewRecorder()
	server.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("signals response = %d headers=%v body=%s", rec.Code, rec.Header(), rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), `"streams"`) {
		t.Fatalf("signals response exposes obsolete stream summaries: %s", rec.Body.String())
	}
	var response struct {
		StreamID  string                    `json:"streamId"`
		State     map[string]any            `json:"state"`
		Leaves    []pagestream.SignalLeaf   `json:"leaves"`
		History   []pagestream.SignalChange `json:"history"`
		NextAfter uint64                    `json:"nextAfter"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode signal response: %v", err)
	}
	if response.StreamID != "trace:test" || len(response.Leaves) != 1 || len(response.History) != 2 {
		t.Fatalf("signal response = %#v", response)
	}
	status := response.State["status"].(map[string]any)
	if status["progressPercent"] != float64(50) || response.NextAfter != response.History[1].ID {
		t.Fatalf("signal response values = %#v", response)
	}
}

func TestProductionOmitsPageStreamSignalsEndpoint(t *testing.T) {
	t.Setenv("LEAPVIEW_PRODUCTION", "1")
	server := newAppTestHarness(fakeMetrics{})
	req := httptest.NewRequest(http.MethodGet, "/__dev/pagestream/signals", nil)
	rec := httptest.NewRecorder()
	server.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("production signals status = %d, want 404", rec.Code)
	}
}
