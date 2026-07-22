package cli

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestRunnableCommandsDeclareDocumentationSafety(t *testing.T) {
	root := NewCommand(context.Background())
	seen := map[string]struct{}{}
	var visit func(*cobra.Command)
	visit = func(command *cobra.Command) {
		seen[command.CommandPath()] = struct{}{}
		if command.Runnable() {
			if command.Annotations[documentationEffectAnnotation] == "" {
				t.Errorf("runnable command %q has no documentation effect", command.CommandPath())
			}
			if command.Annotations[documentationConfirmationAnnotation] == "" {
				t.Errorf("runnable command %q has no documentation confirmation", command.CommandPath())
			}
		}
		for _, child := range command.Commands() {
			visit(child)
		}
	}
	visit(root)
	for path := range documentedCommandSafety {
		if _, ok := seen[path]; !ok {
			t.Errorf("documentation safety declares unknown command %q", path)
		}
	}
}

func TestRootHelpExposesOnlyProjectDeploy(t *testing.T) {
	originalArgs := os.Args
	t.Cleanup(func() { os.Args = originalArgs })

	help := func(args ...string) string {
		os.Args = append([]string{"leapview"}, args...)
		return captureStdout(t, func() {
			if err := Execute(context.Background()); err != nil {
				t.Fatalf("Execute(%v) error = %v", args, err)
			}
		})
	}

	output := help("--help")
	if !strings.Contains(output, "\n  deploy ") {
		t.Fatalf("root help missing deploy command:\n%s", output)
	}
	for _, removed := range []string{"publish", "publishes"} {
		if strings.Contains(output, "\n  "+removed+" ") {
			t.Fatalf("root help still exposes removed %s command:\n%s", removed, output)
		}
	}

	deployHelp := help("deploy", "--help")
	if !strings.Contains(deployHelp, "--revision") {
		t.Fatalf("deploy help missing managed revision pins:\n%s", deployHelp)
	}
	if strings.Contains(deployHelp, "--workspace") {
		t.Fatalf("project deploy help exposes workspace targeting:\n%s", deployHelp)
	}
	if strings.Contains(deployHelp, "--connection") {
		t.Fatalf("project deploy help exposes split data-deploy targeting:\n%s", deployHelp)
	}
}

func TestDeployCommandDescribesAtomicProjectRevisionPins(t *testing.T) {
	command := deployCommand(context.Background(), &rootOptions{})
	if command.Name() != "deploy" {
		t.Fatalf("command name = %q, want deploy", command.Name())
	}
	if !strings.Contains(strings.ToLower(command.Short), "atomically") || !strings.Contains(strings.ToLower(command.Short), "project") {
		t.Fatalf("deploy short help = %q, want atomic project scope", command.Short)
	}
	revision := command.Flags().Lookup("revision")
	if revision == nil {
		t.Fatal("deploy command missing repeatable managed revision pin flag")
	}
	for _, want := range []string{"connection=sha256:<digest>", "repeatable"} {
		if !strings.Contains(revision.Usage, want) {
			t.Fatalf("--revision help = %q, want %q", revision.Usage, want)
		}
	}
	for _, removed := range []string{"connection", "workspace"} {
		if command.Flags().Lookup(removed) != nil {
			t.Fatalf("deploy command still exposes removed --%s targeting flag", removed)
		}
	}
}

func TestAgentCommandIsGlobal(t *testing.T) {
	command := agentCommand(context.Background(), &rootOptions{})
	if command.PersistentFlags().Lookup("workspace") != nil {
		t.Fatal("global agent command still exposes --workspace")
	}
	if got := agentConversationEndpoint("https://leapview.example", nil); got != "https://leapview.example/api/v1/agent/conversations" {
		t.Fatalf("conversation endpoint = %q", got)
	}
	if got := agentRunEndpoint("https://leapview.example", "conv_1", "run_1"); got != "https://leapview.example/api/v1/agent/conversations/conv_1/runs/run_1" {
		t.Fatalf("run endpoint = %q", got)
	}
}
