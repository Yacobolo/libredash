package cli

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	apigenapi "github.com/Yacobolo/libredash/internal/api/gen"
)

func TestAPICommandListsEveryGeneratedOperation(t *testing.T) {
	output := captureStdout(t, func() {
		cmd := apiCommand(context.Background(), &rootOptions{})
		cmd.SetArgs([]string{"list"})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("api list: %v", err)
		}
	})

	for operationID := range apigenapi.GetAPIGenOperationContracts() {
		if !strings.Contains(output, operationID) {
			t.Fatalf("api list missing %s:\n%s", operationID, output)
		}
	}
}

func TestAPICommandCallUsesGeneratedContract(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s", r.Method)
		}
		if r.URL.Path != "/api/v1/workspaces/test/agent/conversations/conv_1/turns" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("trace"); got != "1" {
			t.Fatalf("trace query = %q", got)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer token" {
			t.Fatalf("Authorization = %q", got)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Fatalf("Content-Type = %q", got)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body["input"] != "hello" {
			t.Fatalf("body = %#v", body)
		}
		writeCLIJSON(t, w, map[string]any{"ok": true})
	}))
	defer server.Close()

	output := captureStdout(t, func() {
		cmd := apiCommand(context.Background(), &rootOptions{target: server.URL, token: "token", workspaceID: "test"})
		cmd.SetArgs([]string{
			"call", "createAgentTurn",
			"--target", server.URL,
			"--token", "token",
			"--path", "conversation=conv_1",
			"--query", "trace=1",
			"--body-json", `{"input":"hello"}`,
		})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("api call: %v", err)
		}
	})
	if strings.TrimSpace(output) != `{"ok":true}` {
		t.Fatalf("output = %q", output)
	}
}

func TestAPICommandCallDefaultsJSONBodyFileContentTypeFromGeneratedContract(t *testing.T) {
	bodyPath := filepath.Join(t.TempDir(), "turn.json")
	if err := os.WriteFile(bodyPath, []byte(`{"input":"hello"}`), 0o644); err != nil {
		t.Fatalf("write body file: %v", err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Fatalf("Content-Type = %q", got)
		}
		writeCLIJSON(t, w, map[string]any{"ok": true})
	}))
	defer server.Close()

	captureStdout(t, func() {
		cmd := apiCommand(context.Background(), &rootOptions{target: server.URL, token: "token", workspaceID: "test"})
		cmd.SetArgs([]string{
			"call", "createAgentTurn",
			"--target", server.URL,
			"--token", "token",
			"--path", "conversation=conv_1",
			"--body-file", bodyPath,
		})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("api call: %v", err)
		}
	})
}

func TestAPICommandCallDefaultsBinaryBodyFileContentTypeFromGeneratedContract(t *testing.T) {
	bodyPath := filepath.Join(t.TempDir(), "artifact.tar.gz")
	if err := os.WriteFile(bodyPath, []byte("bundle"), 0o644); err != nil {
		t.Fatalf("write body file: %v", err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Fatalf("method = %s", r.Method)
		}
		if r.URL.Path != "/api/v1/workspaces/test/deployments/dep_1/artifact" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if got := r.Header.Get("Content-Type"); got != "application/octet-stream" {
			t.Fatalf("Content-Type = %q", got)
		}
		writeCLIJSON(t, w, map[string]any{"ok": true})
	}))
	defer server.Close()

	captureStdout(t, func() {
		cmd := apiCommand(context.Background(), &rootOptions{target: server.URL, token: "token", workspaceID: "test"})
		cmd.SetArgs([]string{
			"call", "uploadDeploymentArtifact",
			"--target", server.URL,
			"--token", "token",
			"--path", "deployment=dep_1",
			"--body-file", bodyPath,
		})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("api call: %v", err)
		}
	})
}

func TestAPICommandRejectsMissingPathParameter(t *testing.T) {
	cmd := apiCommand(context.Background(), &rootOptions{target: "https://libredash.example", token: "token", workspaceID: "test"})
	cmd.SetArgs([]string{"call", "getAgentRun", "--target", "https://libredash.example", "--token", "token", "--path", "conversation=conv_1"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "run") {
		t.Fatalf("err = %v, want missing run path parameter", err)
	}
}
