// Command apigenstage creates isolated TypeSpec projects with named shared
// contract imports for APIGen, whose project staging is otherwise limited to a
// single source directory.
package main

import (
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

type stageDefinition struct {
	entry       string
	directories []string
}

var stages = map[string]stageDefinition{
	"leapview-v1": {entry: `import "./typespec/main.tsp";` + "\n", directories: []string{"typespec", "visualization"}},
	"ui-signals":   {entry: `import "./signals/main.tsp";` + "\n", directories: []string{"signals", "visualization"}},
}

func main() {
	target := flag.String("target", "", "APIGen target to stage")
	apiDir := flag.String("api-dir", "api", "canonical API source directory")
	flag.Parse()
	if err := stage(*apiDir, *target); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func stage(apiDir, target string) error {
	definition, ok := stages[target]
	if !ok {
		return fmt.Errorf("unknown APIGen stage target %q", target)
	}
	out := filepath.Join(apiDir, ".apigen", target)
	if err := os.RemoveAll(out); err != nil {
		return fmt.Errorf("reset APIGen stage %q: %w", target, err)
	}
	if err := os.MkdirAll(out, 0o750); err != nil {
		return fmt.Errorf("create APIGen stage %q: %w", target, err)
	}
	for _, directory := range definition.directories {
		if err := copyTypeSpecTree(filepath.Join(apiDir, directory), filepath.Join(out, directory)); err != nil {
			return err
		}
	}
	if err := os.WriteFile(filepath.Join(out, "main.tsp"), []byte(definition.entry), 0o640); err != nil {
		return fmt.Errorf("write APIGen stage entrypoint: %w", err)
	}
	return nil
}

func copyTypeSpecTree(source, destination string) error {
	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, relative)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o750)
		}
		if filepath.Ext(path) != ".tsp" {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read TypeSpec source %s: %w", path, err)
		}
		if err := os.WriteFile(target, content, 0o640); err != nil {
			return fmt.Errorf("stage TypeSpec source %s: %w", path, err)
		}
		return nil
	})
}
