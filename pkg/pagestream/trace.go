package pagestream

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"
)

type TraceStage string

const (
	TraceStagePublished TraceStage = "published"
	TraceStageCoalesced TraceStage = "coalesced"
	TraceStageDropped   TraceStage = "dropped"
	TraceStageDelivered TraceStage = "delivered"
)

type TraceOptions struct {
	CapacityPerStream       int
	SignalCapacityPerStream int
	MaxStreams              int
	Logger                  *slog.Logger
	IncludePayloads         bool
}

type TraceRecord struct {
	StreamID          string
	Stage             TraceStage
	Signals           SignalPatch
	Sequence          uint64
	Generation        uint64
	Origin            string
	CorrelationID     string
	QueueMilliseconds float64
	Coalesced         int
	Outcome           string
}

type TraceEvent struct {
	ID                uint64         `json:"id"`
	Timestamp         time.Time      `json:"timestamp"`
	StreamID          string         `json:"streamId"`
	Sequence          uint64         `json:"sequence"`
	Stage             TraceStage     `json:"stage"`
	Generation        uint64         `json:"generation,omitempty"`
	Origin            string         `json:"origin,omitempty"`
	CorrelationID     string         `json:"correlationId,omitempty"`
	Roots             []string       `json:"roots"`
	Bytes             int            `json:"bytes"`
	Digest            string         `json:"digest,omitempty"`
	QueueMilliseconds float64        `json:"queueMilliseconds,omitempty"`
	Coalesced         int            `json:"coalesced,omitempty"`
	Outcome           string         `json:"outcome,omitempty"`
	Payload           map[string]any `json:"payload,omitempty"`
}

type TraceQuery struct {
	After    uint64
	StreamID string
	Limit    int
}

type traceBuffer struct {
	events          []TraceEvent
	signalState     map[string]any
	signalChanges   []SignalChange
	sequence        uint64
	lastID          uint64
	lastDeliveredID uint64
}

// TraceStore is a development-only, bounded record of page-stream lifecycle
// events. Payloads are sanitized before retention and are never logged.
type TraceStore struct {
	mu                      sync.Mutex
	capacityPerStream       int
	signalCapacityPerStream int
	maxStreams              int
	logger                  *slog.Logger
	includePayloads         bool
	nextID                  uint64
	nextSignalChangeID      uint64
	streams                 map[string]*traceBuffer
}

func NewTraceStore(options TraceOptions) *TraceStore {
	capacity := options.CapacityPerStream
	if capacity <= 0 {
		capacity = 512
	}
	maxStreams := options.MaxStreams
	if maxStreams <= 0 {
		maxStreams = 32
	}
	return &TraceStore{
		capacityPerStream:       capacity,
		signalCapacityPerStream: signalCapacity(options.SignalCapacityPerStream),
		maxStreams:              maxStreams,
		logger:                  options.Logger,
		includePayloads:         options.IncludePayloads,
		streams:                 map[string]*traceBuffer{},
	}
}

func (s *TraceStore) SetLogger(logger *slog.Logger) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.logger = logger
	s.mu.Unlock()
}

func (s *TraceStore) Record(record TraceRecord) TraceEvent {
	if s == nil || strings.TrimSpace(record.StreamID) == "" {
		return TraceEvent{}
	}
	encoded, _ := json.Marshal(record.Signals)
	sanitized := sanitizedPayload(encoded)
	digest := ""
	if len(encoded) > 0 {
		sum := sha256.Sum256(encoded)
		digest = hex.EncodeToString(sum[:8])
	}
	roots := make([]string, 0, len(record.Signals))
	for root := range record.Signals {
		roots = append(roots, root)
	}
	sort.Strings(roots)

	s.mu.Lock()
	buffer := s.streams[record.StreamID]
	if buffer == nil {
		s.evictOldestStreamLocked()
		buffer = &traceBuffer{}
		s.streams[record.StreamID] = buffer
	}
	s.nextID++
	sequence := record.Sequence
	if sequence == 0 {
		buffer.sequence++
		sequence = buffer.sequence
	} else if sequence > buffer.sequence {
		buffer.sequence = sequence
	}
	event := TraceEvent{
		ID:                s.nextID,
		Timestamp:         time.Now().UTC(),
		StreamID:          record.StreamID,
		Sequence:          sequence,
		Stage:             record.Stage,
		Generation:        record.Generation,
		Origin:            strings.TrimSpace(record.Origin),
		CorrelationID:     strings.TrimSpace(record.CorrelationID),
		Roots:             roots,
		Bytes:             len(encoded),
		Digest:            digest,
		QueueMilliseconds: max(0, record.QueueMilliseconds),
		Coalesced:         max(0, record.Coalesced),
		Outcome:           strings.TrimSpace(record.Outcome),
	}
	if s.includePayloads {
		event.Payload = cloneSignalMap(sanitized)
	}
	buffer.events = append(buffer.events, event)
	if overflow := len(buffer.events) - s.capacityPerStream; overflow > 0 {
		buffer.events = append([]TraceEvent(nil), buffer.events[overflow:]...)
	}
	buffer.lastID = event.ID
	if record.Stage == TraceStageDelivered {
		s.applyDeliveredSignalsLocked(buffer, event, sanitized)
	}
	logger := s.logger
	s.mu.Unlock()

	if logger != nil {
		logger.Info("pagestream signal",
			"stage", event.Stage,
			"stream_id", event.StreamID,
			"sequence", event.Sequence,
			"generation", event.Generation,
			"origin", event.Origin,
			"correlation_id", event.CorrelationID,
			"roots", strings.Join(event.Roots, ","),
			"bytes", event.Bytes,
			"digest", event.Digest,
			"queue_ms", event.QueueMilliseconds,
			"coalesced", event.Coalesced,
			"outcome", event.Outcome,
		)
	}
	return event
}

func signalCapacity(value int) int {
	if value <= 0 {
		return 4096
	}
	return value
}

func (s *TraceStore) Events(query TraceQuery) []TraceEvent {
	if s == nil {
		return nil
	}
	limit := query.Limit
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]TraceEvent, 0, limit)
	for streamID, buffer := range s.streams {
		if query.StreamID != "" && streamID != query.StreamID {
			continue
		}
		for _, event := range buffer.events {
			if event.ID > query.After {
				result = append(result, cloneTraceEvent(event))
			}
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	if len(result) > limit {
		result = result[:limit]
	}
	return result
}

func (s *TraceStore) evictOldestStreamLocked() {
	if len(s.streams) < s.maxStreams {
		return
	}
	oldestID := ^uint64(0)
	oldestStream := ""
	for streamID, buffer := range s.streams {
		if buffer.lastID < oldestID {
			oldestID = buffer.lastID
			oldestStream = streamID
		}
	}
	delete(s.streams, oldestStream)
}

func sanitizedPayload(encoded []byte) map[string]any {
	if len(encoded) == 0 {
		return nil
	}
	var value map[string]any
	if json.Unmarshal(encoded, &value) != nil {
		return nil
	}
	for key, item := range value {
		value[key] = sanitizeTraceValue(key, item, 0)
	}
	return value
}

func sanitizeTraceValue(key string, value any, depth int) any {
	if sensitiveTraceKey(key) {
		return "[REDACTED]"
	}
	if depth >= 12 {
		return "[TRUNCATED DEPTH]"
	}
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for childKey, child := range typed {
			out[childKey] = sanitizeTraceValue(childKey, child, depth+1)
		}
		return out
	case []any:
		limit := min(len(typed), 50)
		out := make([]any, 0, limit+1)
		for index := 0; index < limit; index++ {
			out = append(out, sanitizeTraceValue("", typed[index], depth+1))
		}
		if len(typed) > limit {
			out = append(out, "[TRUNCATED "+jsonNumber(len(typed)-limit)+" ITEMS]")
		}
		return out
	case string:
		if len(typed) > 2048 {
			return typed[:2048] + "…[TRUNCATED]"
		}
		return typed
	default:
		return value
	}
}

func sensitiveTraceKey(key string) bool {
	normalized := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(key, "-", "_"), " ", "_"))
	for _, fragment := range []string{"password", "passwd", "secret", "token", "authorization", "cookie", "credential", "api_key", "apikey"} {
		if strings.Contains(normalized, fragment) {
			return true
		}
	}
	return false
}

func jsonNumber(value int) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}

func cloneTraceEvent(event TraceEvent) TraceEvent {
	event.Roots = append([]string(nil), event.Roots...)
	if event.Payload != nil {
		encoded, _ := json.Marshal(event.Payload)
		_ = json.Unmarshal(encoded, &event.Payload)
	}
	return event
}
