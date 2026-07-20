package ui

import (
	"html"
	"strings"
	"testing"

	g "maragu.dev/gomponents"
)

func TestDashboardDocumentHeadUsesLeapViewFavicon(t *testing.T) {
	var output strings.Builder
	if err := g.Group(pageHead()).Render(&output); err != nil {
		t.Fatalf("render dashboard head: %v", err)
	}
	rendered := html.UnescapeString(output.String())
	if !strings.Contains(rendered, `<link rel="icon" href="/static/favicon.svg?v=dev" type="image/svg+xml">`) {
		t.Fatalf("dashboard head does not contain the LeapView favicon: %s", rendered)
	}
}
