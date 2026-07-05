package cli

import (
	"testing"

	"github.com/Yacobolo/libredash/internal/runtimehost"
	servingstate "github.com/Yacobolo/libredash/internal/servingstate"
)

func TestRuntimeDataDirPrefersArtifactDataRoot(t *testing.T) {
	input := runtimehost.RuntimeInput{
		State:    servingstate.State{WorkspaceID: "movielens"},
		Artifact: servingstate.Artifact{DataRoot: ".data/movielens"},
		DataDir:  ".data/olist",
	}
	if got := runtimeDataDir(input, ".data/olist"); got != ".data/movielens" {
		t.Fatalf("runtimeDataDir = %q, want artifact data root", got)
	}
}
