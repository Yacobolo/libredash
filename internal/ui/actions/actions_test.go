package actions

import "testing"

func TestRequestEscapesPathAndSignalPatterns(t *testing.T) {
	got := Post(`/workspaces/it's\here`, "runtime", "filters.controls", "table[0]")
	want := `@post('/workspaces/it\'s\\here', {filterSignals: {include: /^(?:runtime|filters[.]controls|table\[0\])(?:[.]|$)/}, headers: window.LeapViewCommand.headers()})`
	if got != want {
		t.Fatalf("Post() = %q, want %q", got, want)
	}
}

func TestRequestWithoutSignalFilter(t *testing.T) {
	if got, want := Get("/search"), `@get('/search', {headers: window.LeapViewCommand.headers()})`; got != want {
		t.Fatalf("Get() = %q, want %q", got, want)
	}
	if got, want := Patch("/api/config"), `@patch('/api/config', {headers: window.LeapViewCommand.headers()})`; got != want {
		t.Fatalf("Patch() = %q, want %q", got, want)
	}
}
