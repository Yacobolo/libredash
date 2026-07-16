package actions

import "testing"

func TestRequestEscapesPathAndSignalPatterns(t *testing.T) {
	got := Post(`/workspaces/it's\here`, "runtime", "filters.controls", "table[0]")
	want := `@post('/workspaces/it\'s\\here', {filterSignals: {include: /^(?:runtime|filters[.]controls|table\[0\])(?:[.]|$)/}, headers: window.LibreDashCommand.headers()})`
	if got != want {
		t.Fatalf("Post() = %q, want %q", got, want)
	}
}

func TestRequestWithoutSignalFilter(t *testing.T) {
	if got, want := Patch("/api/config"), `@patch('/api/config', {headers: window.LibreDashCommand.headers()})`; got != want {
		t.Fatalf("Patch() = %q, want %q", got, want)
	}
}
