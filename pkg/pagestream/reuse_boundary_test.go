package pagestream

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPackageHasNoLeapViewSpecificDependencies(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		body, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(body), "window.LeapView") {
			t.Fatalf("%s references a LeapView-specific browser global", file)
		}
		if strings.Contains(string(body), "github.com/Yacobolo/leapview/internal/") {
			t.Fatalf("%s imports a LeapView-internal package", file)
		}
	}
}
