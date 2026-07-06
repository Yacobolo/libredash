package access_test

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

func TestAuthorizationReasonComparisonsUseTypedConstants(t *testing.T) {
	repoRoot := filepath.Clean(filepath.Join("..", ".."))
	rawReasonComparison := regexp.MustCompile(`\.Reason\s*(?:==|!=)\s*"`)
	allowedFiles := map[string]bool{
		filepath.Join("internal", "access", "authorization_reason_test.go"): true,
	}
	err := filepath.WalkDir(repoRoot, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", "node_modules", "static":
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" {
			return nil
		}
		rel, err := filepath.Rel(repoRoot, path)
		if err != nil {
			return err
		}
		if allowedFiles[rel] {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if rawReasonComparison.Match(body) {
			t.Errorf("%s compares AuthorizationDecision.Reason to a raw string; use access.Reason* constants", rel)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
