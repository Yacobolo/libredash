package search

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func AssertNoForbiddenImports(t *testing.T) {
	t.Helper()
	forbidden := map[string]bool{
		"github.com/Yacobolo/leapview/internal/api": true,
		"github.com/Yacobolo/leapview/internal/app": true,
		"github.com/Yacobolo/leapview/internal/ui":  true,
		"github.com/go-chi/chi/v5":                   true,
		"net/http":                                   true,
	}
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("glob search package: %v", err)
	}
	fset := token.NewFileSet()
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		info, err := os.Stat(file)
		if err != nil {
			t.Fatalf("stat %s: %v", file, err)
		}
		if info.IsDir() {
			continue
		}
		parsed, err := parser.ParseFile(fset, file, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse imports in %s: %v", file, err)
		}
		for _, imported := range parsed.Imports {
			path := strings.Trim(imported.Path.Value, "\"")
			if forbidden[path] {
				t.Fatalf("%s imports forbidden package %s", file, path)
			}
		}
	}
}
