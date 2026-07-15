package cli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/Yacobolo/libredash/internal/api"
	apigenapi "github.com/Yacobolo/libredash/internal/api/gen"
	servingstatefs "github.com/Yacobolo/libredash/internal/servingstate/filesystem"
)

func TestDeployPreparesCompleteProjectBeforeOneAtomicActivation(t *testing.T) {
	projectPath := filepath.Join("..", "..", "dashboards", "libredash.yaml")
	revision := "sha256:" + strings.Repeat("a", 64)
	workspaces := []string{"operations", "sales", "visuals"}
	var mu sync.Mutex
	var sequence []string
	artifactDigests := map[string]string{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		sequence = append(sequence, r.Method+" "+r.URL.Path)
		mu.Unlock()
		workspaceID := workspaceIDFromAPIPath(r.URL.Path)
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/active-asset-graph"):
			writeCLIJSON(t, w, activeGraphResponse(nil, nil))
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/workspaces/"+workspaceID+"/deployment-candidates"):
			writeCLIJSON(t, w, apigenapi.DeploymentCandidateResponse{Id: "state-" + workspaceID, Project: "libredash-showcase", Workspace: workspaceID, Environment: "prod", Status: "pending"})
		case r.Method == http.MethodPut && strings.HasSuffix(r.URL.Path, "/artifact"):
			pins, digest := readManagedDataPinsFromUpload(t, r.Body)
			if len(pins) != 1 || pins["olist"] != revision {
				t.Fatalf("%s managed pins = %#v", workspaceID, pins)
			}
			artifactDigests[workspaceID] = digest
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/validate"):
			writeCLIJSON(t, w, apigenapi.DeploymentCandidateResponse{Id: "state-" + workspaceID, Project: "libredash-showcase", Workspace: workspaceID, Environment: "prod", Status: "validated", Digest: artifactDigests[workspaceID]})
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/deployment-candidates/") && strings.HasSuffix(r.URL.Path, "/activate"):
			t.Fatalf("deploy activated an individual workspace: %s", r.URL.Path)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/projects/libredash-showcase/deployments":
			assertRequestsBefore(t, sequence, r.Method+" "+r.URL.Path, []string{
				"POST /api/v1/projects/libredash-showcase/workspaces/operations/deployment-candidates/state-operations/validate",
				"POST /api/v1/projects/libredash-showcase/workspaces/sales/deployment-candidates/state-sales/validate",
				"POST /api/v1/projects/libredash-showcase/workspaces/visuals/deployment-candidates/state-visuals/validate",
			})
			writeCLIJSON(t, w, map[string]any{
				"id": "deployment-1", "project": "libredash-showcase", "environment": "prod", "status": "pending",
				"targets": []map[string]string{{"workspace": "operations", "candidateId": "state-operations", "status": "pending"}, {"workspace": "sales", "candidateId": "state-sales", "status": "pending"}, {"workspace": "visuals", "candidateId": "state-visuals", "status": "pending"}},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/projects/libredash-showcase/deployments/deployment-1/activate":
			writeCLIJSON(t, w, map[string]any{
				"id": "deployment-1", "project": "libredash-showcase", "environment": "prod", "status": "active",
				"targets": []map[string]string{{"workspace": "operations", "candidateId": "state-operations", "status": "active"}, {"workspace": "sales", "candidateId": "state-sales", "status": "active"}, {"workspace": "visuals", "candidateId": "state-visuals", "status": "active"}},
			})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	var out bytes.Buffer
	err := runDeploy(context.Background(), deployRequest{
		ProjectPath: projectPath,
		Environment: "prod",
		Revisions:   map[string]string{"olist": revision},
		Target:      server.URL,
		Token:       "secret-token",
		AutoApprove: true,
		Out:         &out,
		HTTPClient:  server.Client(),
	})
	if err != nil {
		t.Fatalf("runDeploy() error = %v", err)
	}
	if strings.Contains(out.String(), "secret-token") || !strings.Contains(out.String(), "deployed libredash-showcase deployment=deployment-1 environment=prod status=active") {
		t.Fatalf("output = %q", out.String())
	}
	assertSequenceContainsInOrder(t, sequence, []string{
		"GET /api/v1/workspaces/operations/active-asset-graph",
		"GET /api/v1/workspaces/sales/active-asset-graph",
		"POST /api/v1/projects/libredash-showcase/workspaces/operations/deployment-candidates",
		"POST /api/v1/projects/libredash-showcase/workspaces/sales/deployment-candidates",
		"POST /api/v1/projects/libredash-showcase/workspaces/visuals/deployment-candidates",
		"POST /api/v1/projects/libredash-showcase/deployments",
		"POST /api/v1/projects/libredash-showcase/deployments/deployment-1/activate",
	})
	for _, workspaceID := range workspaces {
		if artifactDigests[workspaceID] == "" {
			t.Fatalf("workspace %s was not prepared", workspaceID)
		}
	}
}

func TestDeployRejectsIncompleteManagedRevisionSetBeforeNetworkAccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("invalid deployment reached server")
	}))
	defer server.Close()

	err := runDeploy(context.Background(), deployRequest{
		ProjectPath: filepath.Join("..", "..", "dashboards", "libredash.yaml"),
		Environment: "prod",
		Revisions:   map[string]string{},
		Target:      server.URL,
		Token:       "token",
		AutoApprove: true,
		Out:         &bytes.Buffer{},
		HTTPClient:  server.Client(),
	})
	if err == nil || !strings.Contains(err.Error(), "olist") || !strings.Contains(err.Error(), "revision") {
		t.Fatalf("runDeploy() error = %v, want missing olist revision", err)
	}
}

func TestDeployRejectsInheritedWorkspaceTargeting(t *testing.T) {
	command := deployCommand(context.Background(), &rootOptions{workspaceID: "sales"})
	command.SetArgs(nil)
	err := command.Execute()
	if err == nil || !strings.Contains(err.Error(), "project-wide") {
		t.Fatalf("deploy error = %v, want project-wide rejection", err)
	}
}

func assertRequestsBefore(t *testing.T, sequence []string, current string, required []string) {
	t.Helper()
	for _, want := range required {
		found := false
		for _, got := range sequence {
			if got == current {
				break
			}
			if got == want {
				found = true
			}
		}
		if !found {
			t.Fatalf("%s occurred before %s; sequence=%#v", current, want, sequence)
		}
	}
}

func readManagedDataPinsFromUpload(t *testing.T, body io.Reader) (map[string]string, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "artifact.tar.gz")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.New()
	if _, err := io.Copy(io.MultiWriter(file, digest), body); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := servingstatefs.ExtractArtifact(path, root); err != nil {
		t.Fatal(err)
	}
	compiled, _, err := servingstatefs.LoadCompiledWorkspaceArtifact(root)
	if err != nil {
		t.Fatal(err)
	}
	return compiled.ManagedDataRevisions, hex.EncodeToString(digest.Sum(nil))
}

func workspaceIDFromAPIPath(path string) string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	for index := range parts {
		if parts[index] == "workspaces" && index+1 < len(parts) {
			return parts[index+1]
		}
	}
	return ""
}

func assertSequenceContainsInOrder(t *testing.T, sequence, want []string) {
	t.Helper()
	position := 0
	for _, request := range sequence {
		if position < len(want) && request == want[position] {
			position++
		}
	}
	if position != len(want) {
		t.Fatalf("sequence = %#v, want in order %#v", sequence, want)
	}
}

func activeGraphResponse(_ any, _ any) api.WorkspaceAssetGraphResponse {
	return api.WorkspaceAssetGraphResponse{}
}
