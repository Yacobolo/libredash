package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/Yacobolo/libredash/internal/platform/db"
)

type PrincipalInput struct {
	ID          string
	Email       string
	DisplayName string
}

func (s *Store) UpsertPrincipal(ctx context.Context, input PrincipalInput) (db.Principal, error) {
	if input.ID == "" {
		input.ID = newID("principal")
	}
	if err := s.q.UpsertPrincipal(ctx, db.UpsertPrincipalParams{
		ID:          input.ID,
		Email:       input.Email,
		DisplayName: input.DisplayName,
	}); err != nil {
		return db.Principal{}, err
	}
	return s.q.GetPrincipal(ctx, input.ID)
}

func (s *Store) BindRole(ctx context.Context, workspaceID, principalID, roleName string) error {
	role, err := s.q.GetRoleByName(ctx, roleName)
	if err != nil {
		return err
	}
	return s.q.InsertRoleBinding(ctx, db.InsertRoleBindingParams{
		ID:          newID("rolebinding"),
		WorkspaceID: workspaceID,
		RoleID:      role.ID,
		PrincipalID: sql.NullString{String: principalID, Valid: principalID != ""},
	})
}

func (s *Store) BootstrapAdmin(ctx context.Context, workspaceID, email string) error {
	email = strings.TrimSpace(email)
	if email == "" {
		return nil
	}
	principalID := PrincipalIDForEmail(email)
	principal, err := s.UpsertPrincipal(ctx, PrincipalInput{ID: principalID, Email: email, DisplayName: email})
	if err != nil {
		return err
	}
	return s.BindRole(ctx, workspaceID, principal.ID, "owner")
}

type ExternalIdentityInput struct {
	Provider    string
	TenantID    string
	Subject     string
	Email       string
	DisplayName string
}

func (s *Store) ResolveExternalPrincipal(ctx context.Context, input ExternalIdentityInput) (db.Principal, error) {
	input.Email = normalizeEmail(input.Email)
	if input.Provider == "" || input.Subject == "" {
		return db.Principal{}, fmt.Errorf("external identity requires provider and subject")
	}
	identity, err := s.q.GetExternalIdentity(ctx, db.GetExternalIdentityParams{
		Provider: input.Provider,
		TenantID: input.TenantID,
		Subject:  input.Subject,
	})
	if err == nil {
		principal, err := s.UpsertPrincipal(ctx, PrincipalInput{ID: identity.PrincipalID, Email: input.Email, DisplayName: input.DisplayName})
		if err != nil {
			return db.Principal{}, err
		}
		return principal, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return db.Principal{}, err
	}

	var principal db.Principal
	if input.Email != "" {
		principal, err = s.q.GetPrincipalByEmail(ctx, input.Email)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return db.Principal{}, err
		}
	}
	if principal.ID == "" {
		id := "external_" + stableID(input.Provider+"|"+input.TenantID+"|"+input.Subject)
		principal, err = s.UpsertPrincipal(ctx, PrincipalInput{ID: id, Email: input.Email, DisplayName: input.DisplayName})
		if err != nil {
			return db.Principal{}, err
		}
	} else {
		principal, err = s.UpsertPrincipal(ctx, PrincipalInput{ID: principal.ID, Email: input.Email, DisplayName: input.DisplayName})
		if err != nil {
			return db.Principal{}, err
		}
	}

	if err := s.q.UpsertExternalIdentity(ctx, db.UpsertExternalIdentityParams{
		ID:          "identity_" + stableID(input.Provider+"|"+input.TenantID+"|"+input.Subject),
		PrincipalID: principal.ID,
		Provider:    input.Provider,
		TenantID:    input.TenantID,
		Subject:     input.Subject,
		Email:       input.Email,
	}); err != nil {
		return db.Principal{}, err
	}
	return principal, nil
}

func (s *Store) HasPermission(ctx context.Context, workspaceID, principalID, permission string) (bool, error) {
	if principalID == "" {
		return false, nil
	}
	rows, err := s.q.ListPrincipalRolePermissions(ctx, db.ListPrincipalRolePermissionsParams{
		WorkspaceID: workspaceID,
		PrincipalID: sql.NullString{String: principalID, Valid: true},
	})
	if err != nil {
		return false, err
	}
	for _, row := range rows {
		var permissions []string
		if err := json.Unmarshal([]byte(row), &permissions); err != nil {
			return false, err
		}
		for _, candidate := range permissions {
			if candidate == permission {
				return true, nil
			}
		}
	}
	return false, nil
}
