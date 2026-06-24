package ssetest

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

const datastarPatchSignalsEvent = "datastar-patch-signals"

type Event struct {
	Event string
	ID    string
	Retry string
	Data  string
}

func Events(t testing.TB, body string) []Event {
	t.Helper()
	return ParseEvents(body)
}

func ParseEvents(body string) []Event {
	var events []Event
	var current Event
	var data []string
	hasField := false

	flush := func() {
		if !hasField {
			return
		}
		current.Data = strings.Join(data, "\n")
		events = append(events, current)
		current = Event{}
		data = nil
		hasField = false
	}

	for _, rawLine := range strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n") {
		if rawLine == "" {
			flush()
			continue
		}
		if strings.HasPrefix(rawLine, ":") {
			continue
		}

		name, value, ok := strings.Cut(rawLine, ":")
		if !ok {
			continue
		}
		if strings.HasPrefix(value, " ") {
			value = strings.TrimPrefix(value, " ")
		}

		switch name {
		case "event":
			current.Event = value
			hasField = true
		case "id":
			current.ID = value
			hasField = true
		case "retry":
			current.Retry = value
			hasField = true
		case "data":
			data = append(data, value)
			hasField = true
		}
	}
	flush()

	return events
}

func PatchSignals(t testing.TB, body string) []map[string]any {
	t.Helper()

	patches, err := DecodePatchSignals(body)
	if err != nil {
		t.Fatal(err)
	}
	return patches
}

func DecodePatchSignals(body string) ([]map[string]any, error) {
	var patches []map[string]any
	for _, event := range ParseEvents(body) {
		patch, ok, err := DecodePatchSignalEvent(event)
		if err != nil {
			return nil, err
		}
		if ok {
			patches = append(patches, patch)
		}
	}

	return patches, nil
}

func DecodePatchSignalEvent(event Event) (map[string]any, bool, error) {
	if event.Event != datastarPatchSignalsEvent {
		return nil, false, nil
	}

	payload, err := datastarSignalsPayload(event.Data)
	if err != nil {
		return nil, true, err
	}
	var patch map[string]any
	if err := json.Unmarshal([]byte(payload), &patch); err != nil {
		return nil, true, fmt.Errorf("unmarshal Datastar patch signals payload %q: %w", payload, err)
	}
	return patch, true, nil
}

func RequirePatchSignal(t testing.TB, body string, match func(map[string]any) bool) map[string]any {
	t.Helper()

	patches := PatchSignals(t, body)
	for _, patch := range patches {
		if match(patch) {
			return patch
		}
	}
	t.Fatalf("no Datastar patch signal matched predicate; patches: %#v", patches)
	return nil
}

func datastarSignalsPayload(data string) (string, error) {
	lines := strings.Split(data, "\n")
	payload := make([]string, 0, len(lines))
	for _, line := range lines {
		if strings.HasPrefix(line, "onlyIfMissing ") {
			continue
		}
		if !strings.HasPrefix(line, "signals ") {
			return "", fmt.Errorf("Datastar patch-signal data line %q is missing signals prefix", line)
		}
		payload = append(payload, strings.TrimPrefix(line, "signals "))
	}
	if len(payload) == 0 {
		return "", fmt.Errorf("Datastar patch-signal event did not include a signals payload")
	}
	return strings.Join(payload, "\n"), nil
}
