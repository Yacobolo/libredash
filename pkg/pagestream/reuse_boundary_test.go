package pagestream

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPackageHasNoLibreDashSpecificDependencies(t *testing.T) {
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
		if strings.Contains(string(body), "window.LibreDash") {
			t.Fatalf("%s references a LibreDash-specific browser global", file)
		}
		if strings.Contains(string(body), "github.com/Yacobolo/libredash/internal/") {
			t.Fatalf("%s imports a LibreDash-internal package", file)
		}
	}
}
