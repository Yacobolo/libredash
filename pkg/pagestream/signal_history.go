package pagestream

import (
	"encoding/json"
	"sort"
	"strings"
	"time"
	"unicode"
)

type SignalChangeOperation string

const (
	SignalChangeSet     SignalChangeOperation = "set"
	SignalChangeRemoved SignalChangeOperation = "removed"
)

type SignalLeaf struct {
	Path        string `json:"path"`
	DisplayPath string `json:"displayPath"`
	Value       any    `json:"value"`
}

type SignalChange struct {
	ID            uint64                `json:"id"`
	TraceEventID  uint64                `json:"traceEventId"`
	Timestamp     time.Time             `json:"timestamp"`
	StreamID      string                `json:"streamId"`
	Path          string                `json:"path"`
	DisplayPath   string                `json:"displayPath"`
	Operation     SignalChangeOperation `json:"operation"`
	Value         any                   `json:"value,omitempty"`
	Generation    uint64                `json:"generation,omitempty"`
	Sequence      uint64                `json:"sequence"`
	Origin        string                `json:"origin,omitempty"`
	CorrelationID string                `json:"correlationId,omitempty"`
}

type SignalSnapshot struct {
	StreamID string         `json:"streamId"`
	State    map[string]any `json:"state"`
	Leaves   []SignalLeaf   `json:"leaves"`
}

type SignalHistoryQuery struct {
	After    uint64
	StreamID string
	Path     string
	Limit    int
}

type flattenedSignal struct {
	displayPath string
	value       any
}

// SignalSnapshot returns the delivered browser-visible state for a stream. An
// empty stream ID selects the most recently delivered stream.
func (s *TraceStore) SignalSnapshot(streamID string) (SignalSnapshot, bool) {
	if s == nil {
		return SignalSnapshot{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	streamID, buffer := s.signalBufferLocked(strings.TrimSpace(streamID))
	if buffer == nil || buffer.lastDeliveredID == 0 {
		return SignalSnapshot{}, false
	}
	state := cloneSignalMap(buffer.signalState)
	flattened := flattenSignals(state)
	paths := make([]string, 0, len(flattened))
	for path := range flattened {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	leaves := make([]SignalLeaf, 0, len(paths))
	for _, path := range paths {
		leaf := flattened[path]
		leaves = append(leaves, SignalLeaf{Path: path, DisplayPath: leaf.displayPath, Value: cloneSignalValue(leaf.value)})
	}
	return SignalSnapshot{StreamID: streamID, State: state, Leaves: leaves}, true
}

func (s *TraceStore) SignalChanges(query SignalHistoryQuery) []SignalChange {
	if s == nil {
		return nil
	}
	limit := query.Limit
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, buffer := s.signalBufferLocked(strings.TrimSpace(query.StreamID))
	if buffer == nil {
		return nil
	}
	changes := make([]SignalChange, 0, min(limit, len(buffer.signalChanges)))
	for _, change := range buffer.signalChanges {
		if change.ID <= query.After || (query.Path != "" && change.Path != query.Path) {
			continue
		}
		changes = append(changes, cloneSignalChange(change))
		if len(changes) == limit {
			break
		}
	}
	return changes
}

func (s *TraceStore) signalBufferLocked(streamID string) (string, *traceBuffer) {
	if streamID != "" {
		return streamID, s.streams[streamID]
	}
	var selected string
	var latest uint64
	for candidate, buffer := range s.streams {
		if buffer.lastDeliveredID > latest || (buffer.lastDeliveredID == latest && candidate < selected) {
			selected, latest = candidate, buffer.lastDeliveredID
		}
	}
	return selected, s.streams[selected]
}

func (s *TraceStore) applyDeliveredSignalsLocked(buffer *traceBuffer, event TraceEvent, patch map[string]any) {
	if buffer.signalState == nil {
		buffer.signalState = map[string]any{}
	}
	before := flattenSignals(buffer.signalState)
	applySignalMergePatch(buffer.signalState, patch)
	after := flattenSignals(buffer.signalState)
	paths := make([]string, 0, len(before)+len(after))
	seen := make(map[string]struct{}, len(before)+len(after))
	for path := range before {
		seen[path] = struct{}{}
	}
	for path := range after {
		seen[path] = struct{}{}
	}
	for path := range seen {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		oldValue, existed := before[path]
		newValue, exists := after[path]
		if existed == exists && signalValuesEqual(oldValue.value, newValue.value) {
			continue
		}
		s.nextSignalChangeID++
		change := SignalChange{
			ID: s.nextSignalChangeID, TraceEventID: event.ID, Timestamp: event.Timestamp,
			StreamID: event.StreamID, Path: path, Generation: event.Generation, Sequence: event.Sequence,
			Origin: event.Origin, CorrelationID: event.CorrelationID,
		}
		if exists {
			change.DisplayPath = newValue.displayPath
			change.Operation = SignalChangeSet
			change.Value = cloneSignalValue(newValue.value)
		} else {
			change.DisplayPath = oldValue.displayPath
			change.Operation = SignalChangeRemoved
		}
		buffer.signalChanges = append(buffer.signalChanges, change)
	}
	if overflow := len(buffer.signalChanges) - s.signalCapacityPerStream; overflow > 0 {
		buffer.signalChanges = append([]SignalChange(nil), buffer.signalChanges[overflow:]...)
	}
	buffer.lastDeliveredID = event.ID
}

func applySignalMergePatch(target map[string]any, patch map[string]any) {
	for key, value := range patch {
		if value == nil {
			delete(target, key)
			continue
		}
		childPatch, isObject := value.(map[string]any)
		if !isObject {
			target[key] = cloneSignalValue(value)
			continue
		}
		child, ok := target[key].(map[string]any)
		if !ok {
			child = map[string]any{}
			target[key] = child
		}
		applySignalMergePatch(child, childPatch)
	}
}

func flattenSignals(state map[string]any) map[string]flattenedSignal {
	flattened := map[string]flattenedSignal{}
	keys := make([]string, 0, len(state))
	for key := range state {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		flattenSignalValue(flattened, state[key], "/"+escapeJSONPointer(key), displaySignalSegment("", key))
	}
	return flattened
}

func flattenSignalValue(out map[string]flattenedSignal, value any, path, displayPath string) {
	if object, ok := value.(map[string]any); ok {
		keys := make([]string, 0, len(object))
		for key := range object {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			flattenSignalValue(out, object[key], path+"/"+escapeJSONPointer(key), displaySignalSegment(displayPath, key))
		}
		return
	}
	out[path] = flattenedSignal{displayPath: displayPath, value: cloneSignalValue(value)}
}

func escapeJSONPointer(value string) string {
	return strings.ReplaceAll(strings.ReplaceAll(value, "~", "~0"), "/", "~1")
}

func displaySignalSegment(parent, key string) string {
	if signalIdentifier(key) {
		if parent == "" {
			return key
		}
		return parent + "." + key
	}
	encoded, _ := json.Marshal(key)
	return parent + "[" + string(encoded) + "]"
}

func signalIdentifier(value string) bool {
	if value == "" {
		return false
	}
	for index, r := range value {
		if index == 0 {
			if r != '_' && r != '$' && !unicode.IsLetter(r) {
				return false
			}
			continue
		}
		if r != '_' && r != '$' && !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

func signalValuesEqual(left, right any) bool {
	leftJSON, _ := json.Marshal(left)
	rightJSON, _ := json.Marshal(right)
	return string(leftJSON) == string(rightJSON)
}

func cloneSignalMap(value map[string]any) map[string]any {
	if value == nil {
		return nil
	}
	cloned, _ := cloneSignalValue(value).(map[string]any)
	return cloned
}

func cloneSignalValue(value any) any {
	encoded, _ := json.Marshal(value)
	var cloned any
	_ = json.Unmarshal(encoded, &cloned)
	return cloned
}

func cloneSignalChange(change SignalChange) SignalChange {
	change.Value = cloneSignalValue(change.Value)
	return change
}
