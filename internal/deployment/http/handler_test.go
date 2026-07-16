package http

import (
	"context"
	"encoding/json"
	"errors"
	stdhttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Yacobolo/libredash/internal/deployment/apiadapter"
)

func TestCreateResponseUsesProjectDeploymentWireContract(t *testing.T) {
	coordinator := &fakeCoordinator{response: apiadapter.Deployment{
		ID: "deployment_1", Project: "project", Environment: "prod", RequestDigest: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Status: apiadapter.StatusPending, CreatedAt: "2026-07-14T10:00:00Z",
		Targets:     []apiadapter.Target{{Workspace: "sales", CandidateID: "state_2", Status: apiadapter.TargetStatusPending}},
		Connections: []apiadapter.Connection{},
	}}
	handler := NewHandler(Options{Coordinator: coordinator, CurrentPrincipal: func(*stdhttp.Request) (Principal, bool) { return Principal{ID: "principal"}, true }})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(stdhttp.MethodPost, "/api/v1/projects/project/deployments", strings.NewReader(`{"environment":"prod","targets":[{"workspace":"sales","candidateId":"state_2"}]}`))
	request.Header.Set("Content-Type", "application/json")

	handler.Create(recorder, request, "project", CreateHeaders{IdempotencyKey: "deploy-1"})
	if recorder.Code != stdhttp.StatusCreated {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	targets := body["targets"].([]any)
	target := targets[0].(map[string]any)
	if body["project"] != "project" || body["status"] != "pending" || target["workspace"] != "sales" || target["candidateId"] != "state_2" {
		t.Fatalf("response = %#v", body)
	}
}

func TestUnexpectedCoordinatorErrorIsGenericInternalServerError(t *testing.T) {
	handler := NewHandler(Options{
		Coordinator: &fakeCoordinator{err: errors.New("secret sqlite path /srv/libredash.db")},
		CurrentPrincipal: func(*stdhttp.Request) (Principal, bool) {
			return Principal{ID: "principal"}, true
		},
	})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(stdhttp.MethodGet, "/api/v1/projects/project/deployments/deployment_1", nil)

	handler.Get(recorder, request, "project", "deployment_1")
	if recorder.Code != stdhttp.StatusInternalServerError {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), "sqlite") || !strings.Contains(recorder.Body.String(), "internal server error") {
		t.Fatalf("body leaked backend error: %s", recorder.Body.String())
	}
}

func TestTypedInvalidCoordinatorErrorIsBadRequest(t *testing.T) {
	handler := NewHandler(Options{
		Coordinator: &fakeCoordinator{err: apiadapter.ErrInvalid},
		CurrentPrincipal: func(*stdhttp.Request) (Principal, bool) {
			return Principal{ID: "principal"}, true
		},
	})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(stdhttp.MethodGet, "/api/v1/projects/project/deployments/deployment_1", nil)

	handler.Get(recorder, request, "project", "deployment_1")
	if recorder.Code != stdhttp.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
}

type fakeCoordinator struct {
	response apiadapter.Deployment
	err      error
}

func (c *fakeCoordinator) Create(context.Context, apiadapter.CreateRequest) (apiadapter.Deployment, error) {
	return c.response, c.err
}
func (c *fakeCoordinator) Get(context.Context, apiadapter.Scope) (apiadapter.Deployment, error) {
	return c.response, c.err
}
func (c *fakeCoordinator) Activate(context.Context, apiadapter.ActivateRequest) (apiadapter.Deployment, error) {
	return c.response, c.err
}
func (c *fakeCoordinator) Cancel(context.Context, apiadapter.Scope) (apiadapter.Deployment, error) {
	return c.response, c.err
}
