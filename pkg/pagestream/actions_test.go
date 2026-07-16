package pagestream

import "testing"

func TestRequestActionEscapesPathAndSignalPatterns(t *testing.T) {
	got := PostAction(`/workspaces/it's\here`, "runtime", "filters.controls", "table[0]")
	want := `@post('/workspaces/it\'s\\here', {filterSignals: {include: /^(?:runtime|filters[.]controls|table\[0\])(?:[.]|$)/}, headers: window.LibreDashCommand.headers()})`
	if got != want {
		t.Fatalf("post action = %q, want %q", got, want)
	}
}

func TestRequestActionWithoutSignalFilter(t *testing.T) {
	if got, want := PatchAction("/api/config"), `@patch('/api/config', {headers: window.LibreDashCommand.headers()})`; got != want {
		t.Fatalf("patch action = %q, want %q", got, want)
	}
}
