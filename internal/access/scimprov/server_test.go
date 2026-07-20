package scimprov

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Yacobolo/leapview/internal/access"
)

func TestGroupPatchDoesNotAuditMemberSuccessWhenPersistFails(t *testing.T) {
	repo := &failingSCIMPatchRepo{
		group: access.Group{ID: "group_1", Provider: "scim", ExternalID: "group-ext", Name: "Analysts", CreatedAt: "2026-01-01T00:00:00Z"},
		members: []access.GroupMember{{
			GroupID:     "group_1",
			PrincipalID: "principal_1",
			Email:       "one@example.com",
			DisplayName: "One",
		}},
	}
	handler, err := NewHandler(Options{Repository: repo, BearerToken: "scim-token"})
	if err != nil {
		t.Fatalf("new handler: %v", err)
	}
	body := map[string]any{
		"schemas":    []string{"urn:ietf:params:scim:api:messages:2.0:PatchOp"},
		"Operations": []map[string]any{{"op": "add", "path": "members", "value": []map[string]any{{"value": "principal_2"}}}},
	}
	payload, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPatch, "/Groups/group_1", bytes.NewReader(payload))
	req.Header.Set("Authorization", "Bearer scim-token")
	req.Header.Set("Content-Type", "application/scim+json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code == http.StatusOK {
		t.Fatalf("patch status = %d, want failure body=%s", rec.Code, rec.Body.String())
	}
	for _, event := range repo.auditEvents {
		if event.Action == "scim.group.member.add" && event.Status == "success" {
			t.Fatalf("recorded member success audit despite failed persistence: %#v", repo.auditEvents)
		}
	}
	if len(repo.auditEvents) != 1 || repo.auditEvents[0].Action != "scim.group.update" || repo.auditEvents[0].Status != "error" {
		t.Fatalf("audit events = %#v, want one group update error", repo.auditEvents)
	}
}

type failingSCIMPatchRepo struct {
	group       access.Group
	members     []access.GroupMember
	auditEvents []access.AuditEventInput
}

func (r *failingSCIMPatchRepo) UpsertSCIMUser(context.Context, access.SCIMUserInput) (access.SCIMUser, error) {
	return access.SCIMUser{}, errors.New("not implemented")
}

func (r *failingSCIMPatchRepo) ListSCIMUsers(context.Context, access.SCIMUserFilter) ([]access.SCIMUser, error) {
	return nil, errors.New("not implemented")
}

func (r *failingSCIMPatchRepo) DisableSCIMUser(context.Context, string) (access.SCIMUser, error) {
	return access.SCIMUser{}, errors.New("not implemented")
}

func (r *failingSCIMPatchRepo) UpsertSCIMGroup(context.Context, access.SCIMGroupInput) (access.Group, error) {
	return access.Group{}, errors.New("persist failed")
}

func (r *failingSCIMPatchRepo) ListSCIMGroups(_ context.Context, filter access.SCIMGroupFilter) ([]access.Group, error) {
	if filter.ID == r.group.ID {
		return []access.Group{r.group}, nil
	}
	return nil, nil
}

func (r *failingSCIMPatchRepo) DeleteSCIMGroup(context.Context, string) error {
	return errors.New("not implemented")
}

func (r *failingSCIMPatchRepo) AddSCIMGroupMember(context.Context, string, string) error {
	return errors.New("not implemented")
}

func (r *failingSCIMPatchRepo) RemoveSCIMGroupMember(context.Context, string, string) error {
	return errors.New("not implemented")
}

func (r *failingSCIMPatchRepo) ListSCIMGroupMembers(context.Context, string) ([]access.GroupMember, error) {
	return append([]access.GroupMember(nil), r.members...), nil
}

func (r *failingSCIMPatchRepo) RecordAuditEvent(_ context.Context, input access.AuditEventInput) error {
	r.auditEvents = append(r.auditEvents, input)
	return nil
}
