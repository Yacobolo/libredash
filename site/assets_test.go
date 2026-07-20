package siteassets

import (
	"io/fs"
	"strings"
	"testing"
)

func TestFaviconUsesSelectedApertureRing(t *testing.T) {
	contents, err := fs.ReadFile(Static(), "favicon.svg")
	if err != nil {
		t.Fatalf("read favicon: %v", err)
	}
	icon := string(contents)
	for _, expected := range []string{`<circle cx="12" cy="12" r="10"`, `d="m14.31 8 5.74 9.94"`} {
		if !strings.Contains(icon, expected) {
			t.Errorf("favicon does not contain %q", expected)
		}
	}
}

func TestSpinnerLabExploresAccessibleApertureLoaders(t *testing.T) {
	contents, err := fs.ReadFile(Static(), "spinner-lab.html")
	if err != nil {
		t.Fatalf("read spinner lab: %v", err)
	}
	page := string(contents)
	for _, expected := range []string{
		`<title>LeapView Spinner Lab</title>`,
		`data-variant="iris"`,
		`data-variant="trace"`,
		`data-variant="orbit"`,
		`data-variant="continuous"`,
		`animation: spin 1.8s linear infinite;`,
		`data-variant="pulse"`,
		`data-variant="halo"`,
		`data-variant="progress"`,
		`data-variant="fill-chase"`,
		`data-variant="fill-rotor"`,
		`data-variant="fill-twin"`,
		`<polygon class="fill-blade"`,
		`role="status"`,
		`aria-label="Loading"`,
		`@media (prefers-reduced-motion: reduce)`,
		`<circle cx="12" cy="12" r="10"`,
		`<script type="module" src="/static/spinner-lab.mjs"></script>`,
	} {
		if !strings.Contains(page, expected) {
			t.Errorf("spinner lab does not contain %q", expected)
		}
	}
	for _, forbidden := range []string{"https://", "http://"} {
		if strings.Contains(page, forbidden) {
			t.Errorf("spinner lab contains external dependency %q", forbidden)
		}
	}
	if count := strings.Count(page, `data-context-spinner="continuous"`); count != 3 {
		t.Errorf("spinner lab has %d continuous context spinners, want 3", count)
	}
	if count := strings.Count(page, `data-variant=`); count != 10 {
		t.Errorf("spinner lab has %d spinner variants, want 10", count)
	}
	script, err := fs.ReadFile(Static(), "spinner-lab.mjs")
	if err != nil {
		t.Fatalf("read spinner lab script: %v", err)
	}
	for _, expected := range []string{"motion-toggle", "theme-toggle", "data-sizes"} {
		if !strings.Contains(string(script), expected) {
			t.Errorf("spinner lab script does not contain %q", expected)
		}
	}
}

func TestIntegrationLogoReturnsTrustedVendoredSVG(t *testing.T) {
	logo, err := IntegrationLogo("apacheiceberg")
	if err != nil {
		t.Fatalf("read integration logo: %v", err)
	}
	for _, expected := range []string{"<svg", "main-text-secondary-color", "main-text-tertiary-color"} {
		if !strings.Contains(logo, expected) {
			t.Errorf("integration logo does not contain %q", expected)
		}
	}
}

func TestIntegrationLogoRejectsPathTraversal(t *testing.T) {
	if _, err := IntegrationLogo("../favicon"); err == nil {
		t.Fatal("expected path traversal to be rejected")
	}
}

func TestMCPMarkReturnsOfficialVendoredSVG(t *testing.T) {
	mark, err := MCPMark()
	if err != nil {
		t.Fatalf("read MCP mark: %v", err)
	}
	for _, expected := range []string{`viewBox="0 0 180 180"`, `stroke="currentColor"`} {
		if !strings.Contains(mark, expected) {
			t.Errorf("MCP mark does not contain %q", expected)
		}
	}
}
