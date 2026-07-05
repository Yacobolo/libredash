package compiler

import (
	"path/filepath"
	"testing"

	"github.com/Yacobolo/libredash/internal/workspace"
)

func TestMovieLensExperimentProjectCompiles(t *testing.T) {
	projectPath := filepath.Join("..", "..", "..", "dashboards", "experiments", "movielens", "libredash.yaml")

	compiled, err := CompileProject(projectPath, Options{ServingStateID: "dep_test"})
	if err != nil {
		t.Fatalf("CompileProject(MovieLens) error = %v", err)
	}
	if len(compiled.Workspaces) != 1 {
		t.Fatalf("compiled workspaces = %d, want 1", len(compiled.Workspaces))
	}
	workspaceProject := compiled.Workspaces["movielens"]
	if workspaceProject.Definition == nil {
		t.Fatal("movielens workspace definition is nil")
	}
	for _, modelID := range []string{"movie_ratings", "genre_ratings"} {
		model, ok := workspaceProject.Definition.Models[modelID]
		if !ok {
			t.Fatalf("missing semantic model %q", modelID)
		}
		for _, relationship := range model.Relationships {
			if !relationship.Active {
				t.Fatalf("semantic model %q has inactive relationship %#v", modelID, relationship)
			}
			if relationship.Cardinality != "many_to_one" && relationship.Cardinality != "one_to_one" {
				t.Fatalf("semantic model %q has unsafe relationship cardinality %#v", modelID, relationship)
			}
		}
	}
	assertGraphAsset(t, workspaceProject.Workspace.Graph, "connection:movielens")
	assertGraphAsset(t, workspaceProject.Workspace.Graph, "source:movielens.ratings")
	assertGraphAsset(t, workspaceProject.Workspace.Graph, "source:movielens.movies")
	assertGraphAsset(t, workspaceProject.Workspace.Graph, "source:movielens.tags")
	assertGraphAsset(t, workspaceProject.Workspace.Graph, "source:movielens.links")
	assertGraphAsset(t, workspaceProject.Workspace.Graph, "model_table:movielens.ratings")
	assertGraphAsset(t, workspaceProject.Workspace.Graph, "model_table:movielens.rating_genres")
	assertGraphAsset(t, workspaceProject.Workspace.Graph, "semantic_model:movielens.movie_ratings")
	assertGraphAsset(t, workspaceProject.Workspace.Graph, "semantic_model:movielens.genre_ratings")
	assertGraphAsset(t, workspaceProject.Workspace.Graph, "dashboard:movielens.ratings-overview")
	assertGraphAsset(t, workspaceProject.Workspace.Graph, "dashboard:movielens.genre-analysis")
	assertAssetSourceFileContains(t, workspaceProject.Workspace.Graph, "source:movielens.ratings", filepath.Join("sources", "ratings.yaml"))
}

func TestMovieLensExperimentStaysOutOfDefaultProject(t *testing.T) {
	defaultProjectPath := filepath.Join("..", "..", "..", "dashboards", "libredash.yaml")

	compiled, err := CompileProject(defaultProjectPath, Options{ServingStateID: "dep_test"})
	if err != nil {
		t.Fatalf("CompileProject(default) error = %v", err)
	}
	if _, ok := compiled.Workspaces["movielens"]; ok {
		t.Fatal("default project unexpectedly includes movielens workspace")
	}
	for _, compiledWorkspace := range compiled.Workspaces {
		for _, asset := range compiledWorkspace.Workspace.Graph.Assets {
			if asset.Type == workspace.AssetTypeSource && asset.Key == "movielens.ratings" {
				t.Fatalf("default project workspace %s unexpectedly includes MovieLens source", compiledWorkspace.Workspace.ID)
			}
		}
	}
}
