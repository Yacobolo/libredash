package apiadapter

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Yacobolo/libredash/internal/manageddata"
	managedhttp "github.com/Yacobolo/libredash/internal/manageddata/http"
	"github.com/Yacobolo/libredash/internal/manageddata/rollout"
)

const (
	digestA = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	digestB = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	digestC = "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
)

func TestRepositoryRevisionIDsAreCanonicalDigests(t *testing.T) {
	repo := fixtureRepository()
	adapter := mustNew(t, repo, &fakeRollouts{})

	for _, test := range []struct {
		name    string
		id      string
		wantErr error
	}{
		{name: "internal id", id: "revision_a", wantErr: managedhttp.ErrInvalid},
		{name: "uppercase digest", id: "sha256:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", wantErr: managedhttp.ErrInvalid},
		{name: "unknown digest", id: "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc", wantErr: managedhttp.ErrNotFound},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := adapter.RevisionByID(context.Background(), test.id)
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("RevisionByID() error = %v, want %v", err, test.wantErr)
			}
		})
	}

	got, err := adapter.RevisionByID(context.Background(), digestA)
	if err != nil {
		t.Fatalf("RevisionByID() error = %v", err)
	}
	if got.Revision.ID != digestA || got.Revision.Digest != digestA || got.UploadSessionID != "upload_a" {
		t.Fatalf("RevisionByID() = %#v", got)
	}
}

func TestListRevisionsIsDeterministicAndIncludesProvenance(t *testing.T) {
	repo := fixtureRepository()
	repo.revisions = append(repo.revisions,
		manageddata.Revision{ID: "revision_pending", CollectionID: "collection_a", Sequence: 3, Digest: "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc", Status: manageddata.RevisionStatusPending},
	)
	repo.uploads["revision_b"] = "upload_b"
	adapter := mustNew(t, repo, &fakeRollouts{})

	got, err := adapter.ListRevisions(context.Background(), "collection_a")
	if err != nil {
		t.Fatalf("ListRevisions() error = %v", err)
	}
	if len(got) != 2 || got[0].Revision.ID != digestB || got[0].UploadSessionID != "upload_b" || got[1].Revision.ID != digestA {
		t.Fatalf("ListRevisions() = %#v", got)
	}
}

func TestEnvironmentPointerRewritesInternalRevisionID(t *testing.T) {
	adapter := mustNew(t, fixtureRepository(), &fakeRollouts{})
	got, err := adapter.EnvironmentPointer(context.Background(), "collection_a", "prod")
	if err != nil {
		t.Fatalf("EnvironmentPointer() error = %v", err)
	}
	if got.RevisionID != digestA {
		t.Fatalf("EnvironmentPointer().RevisionID = %q", got.RevisionID)
	}
}

func TestCreateRolloutResolvesDigestWithinCollectionAndIsDeterministic(t *testing.T) {
	for _, test := range []struct {
		name       string
		collection string
		digest     string
		wantErr    error
	}{
		{name: "ready revision", collection: "collection_a", digest: digestA},
		{name: "cross collection", collection: "collection_a", digest: digestC, wantErr: managedhttp.ErrNotFound},
		{name: "internal id", collection: "collection_a", digest: "revision_a", wantErr: managedhttp.ErrInvalid},
	} {
		t.Run(test.name, func(t *testing.T) {
			repo := fixtureRepository()
			service := &fakeRollouts{createResult: domainRollout("rollout_result", "collection_a", "revision_a", manageddata.RolloutStatusPending)}
			adapter := mustNew(t, repo, service)
			request := managedhttp.RolloutCreateRequest{
				Project: "project_a", Connection: "orders", CollectionID: test.collection, RevisionID: test.digest,
				Environment: "prod", Actor: "operator_a", IdempotencyKey: "deploy-42",
				Targets: []managedhttp.RolloutTargetRequest{{Workspace: "sales", ServingStateID: "state_new"}},
			}
			got, err := adapter.Create(context.Background(), request)
			if test.wantErr != nil {
				if !errors.Is(err, test.wantErr) {
					t.Fatalf("Create() error = %v, want %v", err, test.wantErr)
				}
				if service.createCalls != 0 {
					t.Fatalf("Create() called domain service %d times", service.createCalls)
				}
				return
			}
			if err != nil {
				t.Fatalf("Create() error = %v", err)
			}
			if service.create.RevisionID != "revision_a" || service.create.Project != request.Project || service.create.Connection != request.Connection || service.create.Actor != request.Actor {
				t.Fatalf("domain create request = %#v", service.create)
			}
			if !strings.HasPrefix(service.create.ID, "rollout_") || got.RevisionID != digestA {
				t.Fatalf("Create() = %#v, domain id = %q", got, service.create.ID)
			}

			firstID := service.create.ID
			_, err = adapter.Create(context.Background(), request)
			if err != nil || service.create.ID != firstID {
				t.Fatalf("idempotent Create() id = %q, error = %v; want %q", service.create.ID, err, firstID)
			}
			request.Actor = "operator_b"
			_, _ = adapter.Create(context.Background(), request)
			if service.create.ID == firstID {
				t.Fatal("rollout id did not preserve actor scope")
			}
		})
	}
}

func TestRolloutMappingAndStrictScope(t *testing.T) {
	repo := fixtureRepository()
	repo.rollouts = []manageddata.Rollout{
		domainRollout("rollout_old", "collection_a", "revision_a", manageddata.RolloutStatusSuperseded),
		domainRollout("rollout_new", "collection_a", "revision_a", manageddata.RolloutStatusPending),
		domainRollout("rollout_other", "collection_b", "revision_b", manageddata.RolloutStatusActive),
	}
	service := &fakeRollouts{getResult: repo.rollouts[0], activateResult: repo.rollouts[1]}
	adapter := mustNew(t, repo, service)

	listed, err := adapter.List(context.Background(), managedhttp.RolloutListRequest{Project: "project_a", Connection: "orders", CollectionID: "collection_a"})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(listed) != 2 || listed[0].ID != "rollout_old" || listed[1].ID != "rollout_new" {
		t.Fatalf("List() = %#v", listed)
	}
	if listed[0].Status != managedhttp.RolloutStatusRolledBack || listed[1].Status != managedhttp.RolloutStatusDraft {
		t.Fatalf("mapped statuses = %q, %q", listed[0].Status, listed[1].Status)
	}
	if listed[0].Targets[0].PreviousRevisionID != digestB {
		t.Fatalf("previous revision = %q", listed[0].Targets[0].PreviousRevisionID)
	}

	_, err = adapter.Get(context.Background(), managedhttp.RolloutRequest{Project: "project_a", Connection: "orders", CollectionID: "collection_b", RolloutID: "rollout_old"})
	if !errors.Is(err, managedhttp.ErrNotFound) || service.getCalls != 0 {
		t.Fatalf("cross-scope Get() error = %v, calls = %d", err, service.getCalls)
	}
}

func TestActivateAndRollbackPreserveScopeAndSanitizeResults(t *testing.T) {
	repo := fixtureRepository()
	service := &fakeRollouts{
		activateResult: domainRollout("rollout_a", "collection_a", "revision_a", manageddata.RolloutStatusActive),
		rollbackResult: domainRollout("rollout_compensating", "collection_a", "revision_b", manageddata.RolloutStatusActive),
	}
	adapter := mustNew(t, repo, service)
	request := managedhttp.RolloutRequest{Project: "project_a", Connection: "orders", CollectionID: "collection_a", RolloutID: "rollout_a", Actor: "operator_a", IdempotencyKey: "activate-1"}

	activated, err := adapter.Activate(context.Background(), request)
	if err != nil {
		t.Fatalf("Activate() error = %v", err)
	}
	if service.activateScope.Project != request.Project || service.activateScope.Connection != request.Connection || activated.RevisionID != digestA {
		t.Fatalf("Activate() = %#v, scope = %#v", activated, service.activateScope)
	}

	rolledBack, err := adapter.Rollback(context.Background(), managedhttp.RolloutRollbackRequest{RolloutRequest: request, Reason: "bad source"})
	if err != nil {
		t.Fatalf("Rollback() error = %v", err)
	}
	if service.rollbackReason != "bad source" || service.rollbackScope.Project != request.Project || rolledBack.ID != request.RolloutID || rolledBack.Status != managedhttp.RolloutStatusRolledBack || rolledBack.RevisionID != digestB {
		t.Fatalf("Rollback() = %#v, scope = %#v, reason = %q", rolledBack, service.rollbackScope, service.rollbackReason)
	}
}

func TestAdapterErrorsAreSanitized(t *testing.T) {
	for _, test := range []struct {
		name string
		call func(*Adapter) error
	}{
		{name: "repository", call: func(adapter *Adapter) error {
			_, err := adapter.ListRevisions(context.Background(), "collection_a")
			return err
		}},
		{name: "rollout service", call: func(adapter *Adapter) error {
			_, err := adapter.Activate(context.Background(), managedhttp.RolloutRequest{Project: "project_a", Connection: "orders", CollectionID: "collection_a", RolloutID: "rollout_a"})
			return err
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			repo := fixtureRepository()
			service := &fakeRollouts{}
			if test.name == "repository" {
				repo.err = errors.New("database password=super-secret")
			} else {
				service.err = errors.New("runtime path=/secret/data")
			}
			err := test.call(mustNew(t, repo, service))
			if !errors.Is(err, managedhttp.ErrBackend) {
				t.Fatalf("adapter error = %v", err)
			}
			if strings.Contains(err.Error(), "super-secret") || strings.Contains(err.Error(), "/secret/data") {
				t.Fatalf("error leaked backend details: %v", err)
			}
		})
	}
}

func TestStatusMappings(t *testing.T) {
	for _, test := range []struct {
		name   string
		domain manageddata.RolloutStatus
		public managedhttp.RolloutStatus
	}{
		{name: "pending", domain: manageddata.RolloutStatusPending, public: managedhttp.RolloutStatusDraft},
		{name: "active", domain: manageddata.RolloutStatusActive, public: managedhttp.RolloutStatusActive},
		{name: "failed", domain: manageddata.RolloutStatusFailed, public: managedhttp.RolloutStatusFailed},
		{name: "superseded", domain: manageddata.RolloutStatusSuperseded, public: managedhttp.RolloutStatusRolledBack},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := publicRolloutStatus(test.domain)
			if err != nil || got != test.public {
				t.Fatalf("publicRolloutStatus(%q) = %q, %v", test.domain, got, err)
			}
		})
	}
}

func TestRolloutIDScope(t *testing.T) {
	base := deterministicRolloutID("project_a", "orders", "operator_a", "deploy-42")
	for _, values := range [][4]string{
		{"project_b", "orders", "operator_a", "deploy-42"},
		{"project_a", "customers", "operator_a", "deploy-42"},
		{"project_a", "orders", "operator_b", "deploy-42"},
		{"project_a", "orders", "operator_a", "deploy-43"},
	} {
		if got := deterministicRolloutID(values[0], values[1], values[2], values[3]); got == base {
			t.Fatalf("deterministicRolloutID%v collided with base", values)
		}
	}
	if got := deterministicRolloutID("project_a", "orders", "operator_a", "deploy-42"); got != base {
		t.Fatalf("deterministicRolloutID() = %q, want %q", got, base)
	}
}

func mustNew(t *testing.T, repo *fakeRepository, rollouts *fakeRollouts) *Adapter {
	t.Helper()
	adapter, err := New(repo, rollouts)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return adapter
}

func fixtureRepository() *fakeRepository {
	return &fakeRepository{
		collections: map[string]manageddata.Collection{
			"project_a\x00orders": {ID: "collection_a", ProjectID: "project_a", ConnectionName: "orders", Status: manageddata.CollectionStatusActive},
			"project_b\x00orders": {ID: "collection_b", ProjectID: "project_b", ConnectionName: "orders", Status: manageddata.CollectionStatusActive},
		},
		revisions: []manageddata.Revision{
			{ID: "revision_a", CollectionID: "collection_a", Sequence: 1, Digest: digestA, Status: manageddata.RevisionStatusReady, CreatedAt: "2026-01-01T00:00:00Z"},
			{ID: "revision_b", CollectionID: "collection_a", Sequence: 2, Digest: digestB, Status: manageddata.RevisionStatusReady, CreatedAt: "2026-01-02T00:00:00Z"},
			{ID: "revision_other", CollectionID: "collection_b", Sequence: 1, Digest: digestC, Status: manageddata.RevisionStatusReady, CreatedAt: "2026-01-02T00:00:00Z"},
		},
		uploads: map[string]string{"revision_a": "upload_a"},
		pointer: manageddata.EnvironmentPointer{CollectionID: "collection_a", Environment: "prod", RevisionID: "revision_a"},
		bindings: map[string][]manageddata.ServingStateBinding{
			"state_old": {{ServingStateID: "state_old", CollectionID: "collection_a", RevisionID: "revision_b", Environment: "prod"}},
		},
	}
}

type fakeRepository struct {
	collections map[string]manageddata.Collection
	revisions   []manageddata.Revision
	uploads     map[string]string
	pointer     manageddata.EnvironmentPointer
	rollouts    []manageddata.Rollout
	bindings    map[string][]manageddata.ServingStateBinding
	err         error
}

func (r *fakeRepository) CollectionByProjectConnection(_ context.Context, project, connection string) (manageddata.Collection, error) {
	if r.err != nil {
		return manageddata.Collection{}, r.err
	}
	value, ok := r.collections[project+"\x00"+connection]
	if !ok {
		return manageddata.Collection{}, manageddata.ErrNotFound
	}
	return value, nil
}

func (r *fakeRepository) RevisionByDigest(_ context.Context, digest string) (manageddata.Revision, error) {
	if r.err != nil {
		return manageddata.Revision{}, r.err
	}
	var found *manageddata.Revision
	for i := range r.revisions {
		if r.revisions[i].Digest != digest {
			continue
		}
		if found != nil {
			return manageddata.Revision{}, manageddata.ErrConflict
		}
		copy := r.revisions[i]
		found = &copy
	}
	if found == nil {
		return manageddata.Revision{}, manageddata.ErrNotFound
	}
	return *found, nil
}

func (r *fakeRepository) RevisionByID(_ context.Context, id string) (manageddata.Revision, error) {
	if r.err != nil {
		return manageddata.Revision{}, r.err
	}
	for _, revision := range r.revisions {
		if revision.ID == id {
			return revision, nil
		}
	}
	return manageddata.Revision{}, manageddata.ErrNotFound
}

func (r *fakeRepository) ListRevisions(_ context.Context, collectionID string) ([]manageddata.Revision, error) {
	if r.err != nil {
		return nil, r.err
	}
	var out []manageddata.Revision
	for _, revision := range r.revisions {
		if revision.CollectionID == collectionID {
			out = append(out, revision)
		}
	}
	return out, nil
}

func (r *fakeRepository) UploadSessionIDByRevisionID(_ context.Context, revisionID string) (string, error) {
	if r.err != nil {
		return "", r.err
	}
	id, ok := r.uploads[revisionID]
	if !ok {
		return "", manageddata.ErrNotFound
	}
	return id, nil
}

func (r *fakeRepository) EnvironmentPointer(context.Context, string, manageddata.Environment) (manageddata.EnvironmentPointer, error) {
	if r.err != nil {
		return manageddata.EnvironmentPointer{}, r.err
	}
	return r.pointer, nil
}

func (r *fakeRepository) ListRollouts(_ context.Context, collectionID string) ([]manageddata.Rollout, error) {
	if r.err != nil {
		return nil, r.err
	}
	var out []manageddata.Rollout
	for _, value := range r.rollouts {
		if value.CollectionID == collectionID {
			out = append(out, value)
		}
	}
	return out, nil
}

func (r *fakeRepository) ListServingStateBindings(_ context.Context, stateID string) ([]manageddata.ServingStateBinding, error) {
	if r.err != nil {
		return nil, r.err
	}
	return append([]manageddata.ServingStateBinding(nil), r.bindings[stateID]...), nil
}

type fakeRollouts struct {
	createResult   manageddata.Rollout
	getResult      manageddata.Rollout
	activateResult manageddata.Rollout
	rollbackResult manageddata.Rollout
	err            error
	createCalls    int
	getCalls       int
	create         rollout.CreateRequest
	activateScope  rollout.Scope
	rollbackScope  rollout.Scope
	rollbackReason string
}

func (s *fakeRollouts) Create(_ context.Context, request rollout.CreateRequest) (manageddata.Rollout, error) {
	s.createCalls++
	s.create = request
	result := s.createResult
	result.ID = request.ID
	result.CreatedBy = request.Actor
	result.Targets = make([]manageddata.RolloutTarget, len(request.Targets))
	for i, target := range request.Targets {
		result.Targets[i] = manageddata.RolloutTarget{RolloutID: request.ID, WorkspaceID: target.WorkspaceID, ServingStateID: target.ServingStateID, Status: manageddata.TargetStatusPending}
	}
	return result, s.err
}

func (s *fakeRollouts) Get(_ context.Context, scope rollout.Scope) (manageddata.Rollout, error) {
	s.getCalls++
	return s.getResult, s.err
}

func (s *fakeRollouts) Activate(_ context.Context, scope rollout.Scope) (manageddata.Rollout, error) {
	s.activateScope = scope
	return s.activateResult, s.err
}

func (s *fakeRollouts) Rollback(_ context.Context, scope rollout.Scope, reason string) (manageddata.Rollout, error) {
	s.rollbackScope = scope
	s.rollbackReason = reason
	return s.rollbackResult, s.err
}

func domainRollout(id, collectionID, revisionID string, status manageddata.RolloutStatus) manageddata.Rollout {
	return manageddata.Rollout{
		ID: id, CollectionID: collectionID, RevisionID: revisionID, Environment: "prod", Status: status,
		CreatedAt: "2026-01-01T00:00:00Z", CompletedAt: "2026-01-02T00:00:00Z",
		Targets: []manageddata.RolloutTarget{{RolloutID: id, WorkspaceID: "sales", ServingStateID: "state_new", PriorServingStateID: "state_old", Status: manageddata.TargetStatusPending}},
	}
}

func TestInterfaceConformance(t *testing.T) {
	var _ managedhttp.Repository = (*Adapter)(nil)
	var _ managedhttp.RolloutCoordinator = (*Adapter)(nil)
}
