package siteassets

import (
	"strings"
	"testing"
)

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
