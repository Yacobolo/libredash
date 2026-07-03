package cli

import (
	"testing"

	"github.com/Yacobolo/libredash/internal/deployment"
	"github.com/Yacobolo/libredash/internal/runtimehost"
)

func TestRuntimeDataDirPrefersArtifactDataRoot(t *testing.T) {
	input := runtimehost.RuntimeInput{
		Deployment: deployment.Deployment{WorkspaceID: "movielens"},
		Artifact:   deployment.Artifact{DataRoot: ".data/movielens"},
		DataDir:    ".data/olist",
	}
	if got := runtimeDataDir(input, ".data/olist"); got != ".data/movielens" {
		t.Fatalf("runtimeDataDir = %q, want artifact data root", got)
	}
}
