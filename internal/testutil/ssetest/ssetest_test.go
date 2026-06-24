package ssetest

import "testing"

func TestEventsParsesMultipleEvents(t *testing.T) {
	body := ": ignored comment\n" +
		"event: first\n" +
		"id: 1\n" +
		"retry: 250\n" +
		"data: hello\n" +
		"data: world\n" +
		"unknown: ignored\n" +
		"\n" +
		"event: second\n" +
		"data: done\n" +
		"\n"

	events := Events(t, body)
	if len(events) != 2 {
		t.Fatalf("events = %#v, want 2 events", events)
	}
	if got := events[0]; got.Event != "first" || got.ID != "1" || got.Retry != "250" || got.Data != "hello\nworld" {
		t.Fatalf("first event = %#v", got)
	}
	if got := events[1]; got.Event != "second" || got.Data != "done" {
		t.Fatalf("second event = %#v", got)
	}
}

func TestPatchSignalsDecodesDatastarEvents(t *testing.T) {
	body := "event: datastar-patch-elements\n" +
		"data: elements <div></div>\n" +
		"\n" +
		"event: datastar-patch-signals\n" +
		"data: signals {\"status\":{\"loading\":true}}\n" +
		"\n" +
		"event: datastar-patch-signals\n" +
		"data: onlyIfMissing true\n" +
		"data: signals {\n" +
		"data: signals \"filters\":{\"controls\":{\"state\":{\"values\":[\"SP\"]}}}\n" +
		"data: signals }\n" +
		"\n"

	patches := PatchSignals(t, body)
	if len(patches) != 2 {
		t.Fatalf("patches = %#v, want 2 patches", patches)
	}
	status := patches[0]["status"].(map[string]any)
	if status["loading"] != true {
		t.Fatalf("status patch = %#v", patches[0])
	}
	filters := patches[1]["filters"].(map[string]any)
	controls := filters["controls"].(map[string]any)
	state := controls["state"].(map[string]any)
	values := state["values"].([]any)
	if len(values) != 1 || values[0] != "SP" {
		t.Fatalf("state values = %#v", values)
	}
}

func TestRequirePatchSignalReturnsMatchingPatch(t *testing.T) {
	body := "event: datastar-patch-signals\n" +
		"data: signals {\"status\":{\"loading\":true}}\n" +
		"\n" +
		"event: datastar-patch-signals\n" +
		"data: signals {\"status\":{\"loading\":false}}\n" +
		"\n"

	patch := RequirePatchSignal(t, body, func(patch map[string]any) bool {
		status := patch["status"].(map[string]any)
		return status["loading"] == false
	})
	status := patch["status"].(map[string]any)
	if status["loading"] != false {
		t.Fatalf("matched patch = %#v", patch)
	}
}
