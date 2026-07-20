package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestGenerateWritesVersionedMachineManifest(t *testing.T) {
	root := &cobra.Command{Use: "leapview"}
	deploy := &cobra.Command{
		Use:     "deploy <project>",
		Short:   "Deploy a project",
		Long:    "Compile and atomically deploy a project.",
		Example: "leapview deploy ./dashboards --apply",
		Args:    cobra.ExactArgs(1),
		RunE:    func(*cobra.Command, []string) error { return nil },
		Annotations: map[string]string{
			"leapview.dev/effect":       "write",
			"leapview.dev/confirmation": "conditional",
		},
	}
	deploy.Flags().Bool("apply", false, "Apply the deployment")
	root.PersistentFlags().String("target", "", "LeapView server URL")
	root.AddCommand(deploy)

	out := t.TempDir()
	if err := generate(root, out); err != nil {
		t.Fatalf("generate CLI documentation: %v", err)
	}

	contents, err := os.ReadFile(filepath.Join(out, "manifest.json"))
	if err != nil {
		t.Fatalf("read machine manifest: %v", err)
	}
	var manifest machineManifest
	if err := json.Unmarshal(contents, &manifest); err != nil {
		t.Fatalf("decode machine manifest: %v", err)
	}
	if got, want := manifest.SchemaVersion, 1; got != want {
		t.Fatalf("schema version = %d, want %d", got, want)
	}
	if got, want := len(manifest.Commands), 1; got != want {
		t.Fatalf("commands = %d, want %d", got, want)
	}
	command := manifest.Commands[0]
	if command.ID != "deploy" || !strings.HasPrefix(command.Usage, "leapview deploy <project>") {
		t.Errorf("command identity = %#v", command)
	}
	if command.Effect != "write" || command.Confirmation != "conditional" {
		t.Errorf("command safety = effect %q confirmation %q", command.Effect, command.Confirmation)
	}
	if len(command.Arguments) != 1 || command.Arguments[0] != "project" {
		t.Errorf("arguments = %#v", command.Arguments)
	}
	if len(command.Options) != 1 || command.Options[0].Name != "apply" || command.Options[0].Type != "bool" {
		t.Errorf("options = %#v", command.Options)
	}
	if len(command.InheritedOptions) != 1 || command.InheritedOptions[0].Name != "target" {
		t.Errorf("inherited options = %#v", command.InheritedOptions)
	}
	if len(command.Examples) != 1 || command.Examples[0] != "leapview deploy ./dashboards --apply" {
		t.Errorf("examples = %#v", command.Examples)
	}
	article, err := os.ReadFile(filepath.Join(out, "deploy.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"## Behavior", "| `write` | `conditional` |", "/docs/cli/commands/deploy.json"} {
		if !strings.Contains(string(article), want) {
			t.Errorf("generated article missing %q:\n%s", want, article)
		}
	}
}

func TestGenerateRejectsRunnableCommandWithoutSafetyMetadata(t *testing.T) {
	root := &cobra.Command{Use: "leapview"}
	root.AddCommand(&cobra.Command{Use: "mutate", Run: func(*cobra.Command, []string) {}})

	err := generate(root, t.TempDir())
	if err == nil || err.Error() != `command "leapview mutate" is runnable but missing leapview.dev/effect annotation` {
		t.Fatalf("generate error = %v", err)
	}
}

func TestGenerateGroupsSubcommandsOnTopLevelCommandPage(t *testing.T) {
	root := &cobra.Command{Use: "leapview"}
	semanticModels := &cobra.Command{Use: "semantic-models", Short: "Inspect semantic models"}
	list := &cobra.Command{
		Use: "list", Short: "List semantic models", Run: func(*cobra.Command, []string) {},
		Annotations: map[string]string{effectAnnotation: "read", confirmationAnnotation: "never"},
	}
	query := &cobra.Command{
		Use: "query <model> <dataset>", Short: "Query a semantic model dataset", Run: func(*cobra.Command, []string) {},
		Annotations: map[string]string{effectAnnotation: "read", confirmationAnnotation: "never"},
	}
	query.Flags().Int("limit", 100, "maximum rows")
	semanticModels.AddCommand(list, query)
	root.AddCommand(semanticModels)

	out := t.TempDir()
	if err := generate(root, out); err != nil {
		t.Fatalf("generate CLI documentation: %v", err)
	}

	var generatedCatalog catalog
	contents, err := os.ReadFile(filepath.Join(out, "catalog.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(contents, &generatedCatalog); err != nil {
		t.Fatal(err)
	}
	if got, want := len(generatedCatalog.Documents), 1; got != want {
		t.Fatalf("human documents = %d, want %d", got, want)
	}
	if got, want := generatedCatalog.Documents[0].Slug, "semantic-models"; got != want {
		t.Fatalf("human document slug = %q, want %q", got, want)
	}

	article, err := os.ReadFile(filepath.Join(out, "semantic-models.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"# leapview semantic-models",
		"## Subcommands",
		"[`list`](#list)",
		"[`query`](#query)",
		"## list",
		"## query",
		"leapview semantic-models query <model> <dataset>",
		"/docs/cli/commands/semantic-models-query.json",
		"| `--limit` | int | `100` | maximum rows |",
	} {
		if !strings.Contains(string(article), want) {
			t.Errorf("grouped article missing %q:\n%s", want, article)
		}
	}
	if _, err := os.Stat(filepath.Join(out, "semantic-models-query.md")); !os.IsNotExist(err) {
		t.Fatalf("leaf human page exists: %v", err)
	}

	manifestContents, err := os.ReadFile(filepath.Join(out, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest machineManifest
	if err := json.Unmarshal(manifestContents, &manifest); err != nil {
		t.Fatal(err)
	}
	if got, want := len(manifest.Commands), 3; got != want {
		t.Fatalf("machine commands = %d, want %d", got, want)
	}
}
