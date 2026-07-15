package configspec

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func TestProductionGoEnvironmentReadsAreCataloged(t *testing.T) {
	known := knownSettings()
	root := filepath.Join("..", "..")
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			return err
		}
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok || len(call.Args) == 0 {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || (selector.Sel.Name != "Getenv" && selector.Sel.Name != "LookupEnv") {
				return true
			}
			literal, ok := call.Args[0].(*ast.BasicLit)
			if !ok || literal.Kind != token.STRING {
				return true
			}
			name, _ := strconv.Unquote(literal.Value)
			if strings.HasPrefix(name, "LIBREDASH_") {
				if _, ok := known[name]; !ok {
					t.Errorf("%s reads uncataloged environment variable %s", path, name)
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestOperationalEnvironmentReferencesAreCataloged(t *testing.T) {
	known := knownSettings()
	pattern := regexp.MustCompile(`\bLIBREDASH_[A-Z0-9_]+\b`)
	root := filepath.Join("..", "..")
	paths := []string{"README.md", "Taskfile.yml", "Dockerfile", ".env.example", "docs", "scripts", "deploy", "dashboards"}
	for _, relative := range paths {
		path := filepath.Join(root, relative)
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		visit := func(path string) {
			body, err := os.ReadFile(path)
			if err != nil {
				t.Error(err)
				return
			}
			for _, name := range pattern.FindAllString(string(body), -1) {
				if strings.HasPrefix(name, "LIBREDASH_TEST_") || strings.HasPrefix(name, "LIBREDASH_QUACK_TEST_") {
					continue
				}
				if _, ok := known[name]; !ok {
					t.Errorf("%s references uncataloged environment variable %s", path, name)
				}
			}
		}
		if !info.IsDir() {
			visit(path)
			continue
		}
		_ = filepath.WalkDir(path, func(path string, entry os.DirEntry, err error) error {
			if err == nil && !entry.IsDir() && !strings.Contains(path, ".terraform/") && !strings.Contains(path, "/.local/") && !strings.HasSuffix(path, ".tfstate") {
				visit(path)
			}
			return err
		})
	}
}

func knownSettings() map[string]struct{} {
	known := map[string]struct{}{}
	for _, setting := range Settings() {
		known[setting.Name] = struct{}{}
	}
	return known
}
