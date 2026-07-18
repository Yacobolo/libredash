package docs

import (
	"io/fs"
	"strings"
	"testing"

	"github.com/Yacobolo/libredash/internal/docvalidation"
)

func TestMarkdownYAMLExamplesAreValid(t *testing.T) {
	t.Parallel()

	err := fs.WalkDir(Files, ".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}
		content, err := Files.ReadFile(path)
		if err != nil {
			return err
		}
		for _, issue := range docvalidation.ValidateMarkdown("docs/"+path, content) {
			t.Error(issue.String())
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
