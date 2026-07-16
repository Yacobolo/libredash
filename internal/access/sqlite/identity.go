package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/Yacobolo/libredash/internal/access"
	platformdb "github.com/Yacobolo/libredash/internal/platform/db"
	"strings"
)

func (r *Repository) PrincipalByID(ctx context.Context, id string) (access.Principal, error) {
	row, err := r.q.GetPrincipal(ctx, id)
	if err != nil {
		return access.Principal{}, err
	}
	return mapPrincipal(row), nil
}

func (r *Repository) ListPrincipals(ctx context.Context, filter access.PrincipalFilter) ([]access.Principal, error) {
	if r == nil || r.db == nil {
		return []access.Principal{}, nil
	}
	email := strings.TrimSpace(filter.Email)
	query := strings.TrimSpace(filter.Query)
	rows, err := r.db.QueryContext(ctx, `
SELECT id, kind, email, display_name, disabled_at, created_at, updated_at
FROM principals
WHERE (? = '' OR lower(email) = lower(?))
  AND (? = '' OR lower(email) LIKE '%' || lower(?) || '%' OR lower(display_name) LIKE '%' || lower(?) || '%')
ORDER BY email, id
`, email, email, query, query, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []access.Principal{}
	for rows.Next() {
		var principal access.Principal
		var disabledAt sql.NullString
		if err := rows.Scan(&principal.ID, &principal.Kind, &principal.Email, &principal.DisplayName, &disabledAt, &principal.CreatedAt, &principal.UpdatedAt); err != nil {
			return nil, err
		}
		if disabledAt.Valid {
			principal.DisabledAt = disabledAt.String
		}
		out = append(out, principal)
	}
	return out, rows.Err()
}

func (r *Repository) principalDisabled(ctx context.Context, principalID string) (bool, error) {
	row, err := r.q.GetPrincipal(ctx, principalID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return row.DisabledAt.Valid && row.DisabledAt.String != "", nil
}

func (r *Repository) UpsertPrincipal(ctx context.Context, input access.PrincipalInput) (access.Principal, error) {
	access.ClearAuthorizationCache(ctx)
	if strings.TrimSpace(input.ID) == "" {
		id, err := newID("principal")
		if err != nil {
			return access.Principal{}, err
		}
		input.ID = id
	}
	if input.Kind == "" {
		input.Kind = access.PrincipalKindUser
	}
	if err := r.q.UpsertPrincipal(ctx, platformdb.UpsertPrincipalParams{
		ID:          input.ID,
		Kind:        string(input.Kind),
		Email:       input.Email,
		DisplayName: input.DisplayName,
	}); err != nil {
		return access.Principal{}, err
	}
	row, err := r.q.GetPrincipal(ctx, input.ID)
	if err != nil {
		return access.Principal{}, err
	}
	return mapPrincipal(row), nil
}

func (r *Repository) CreateLocalUser(ctx context.Context, input access.LocalUserInput) (access.LocalPasswordReset, error) {
	access.ClearAuthorizationCache(ctx)
	email := access.NormalizeEmail(input.Email)
	if email == "" {
		return access.LocalPasswordReset{}, fmt.Errorf("email is required")
	}
	password := strings.TrimSpace(input.Password)
	if password == "" {
		generated, err := newTemporaryPassword()
		if err != nil {
			return access.LocalPasswordReset{}, err
		}
		password = generated
	}
	verifier, err := newSecretVerifier(password)
	if err != nil {
		return access.LocalPasswordReset{}, err
	}
	principalID := access.PrincipalIDForEmail(email)
	if existing, err := r.q.GetPrincipalByEmail(ctx, email); err == nil {
		principalID = existing.ID
	} else if !errors.Is(err, sql.ErrNoRows) {
		return access.LocalPasswordReset{}, err
	}
	principal, err := r.UpsertPrincipal(ctx, access.PrincipalInput{
		ID:          principalID,
		Kind:        access.PrincipalKindUser,
		Email:       email,
		DisplayName: firstNonEmpty(strings.TrimSpace(input.DisplayName), email),
	})
	if err != nil {
		return access.LocalPasswordReset{}, err
	}
	if err := r.upsertLocalCredential(ctx, principal.ID, verifier, input.MustChange); err != nil {
		return access.LocalPasswordReset{}, err
	}
	return access.LocalPasswordReset{Principal: principal, Password: password}, nil
}

func (r *Repository) VerifyLocalPassword(ctx context.Context, email, password string) (access.Principal, access.LocalCredential, error) {
	email = access.NormalizeEmail(email)
	if email == "" || strings.TrimSpace(password) == "" {
		return access.Principal{}, access.LocalCredential{}, sql.ErrNoRows
	}
	principal, credential, verifier, err := r.localCredentialByEmail(ctx, email)
	if err != nil {
		return access.Principal{}, access.LocalCredential{}, err
	}
	if principal.DisabledAt != "" || !verifySecret(password, verifier) {
		return access.Principal{}, access.LocalCredential{}, sql.ErrNoRows
	}
	return principal, credential, nil
}

func (r *Repository) ResetLocalPassword(ctx context.Context, principalID string) (access.LocalPasswordReset, error) {
	principalID = strings.TrimSpace(principalID)
	if principalID == "" {
		return access.LocalPasswordReset{}, fmt.Errorf("principal id is required")
	}
	principal, err := r.PrincipalByID(ctx, principalID)
	if err != nil {
		return access.LocalPasswordReset{}, err
	}
	if principal.Kind != access.PrincipalKindUser {
		return access.LocalPasswordReset{}, fmt.Errorf("local passwords are only supported for user principals")
	}
	password, err := newTemporaryPassword()
	if err != nil {
		return access.LocalPasswordReset{}, err
	}
	verifier, err := newSecretVerifier(password)
	if err != nil {
		return access.LocalPasswordReset{}, err
	}
	if err := r.upsertLocalCredential(ctx, principal.ID, verifier, true); err != nil {
		return access.LocalPasswordReset{}, err
	}
	return access.LocalPasswordReset{Principal: principal, Password: password}, nil
}

func (r *Repository) ChangeLocalPassword(ctx context.Context, principalID, currentPassword, newPassword string) (access.LocalCredential, error) {
	principalID = strings.TrimSpace(principalID)
	if principalID == "" {
		return access.LocalCredential{}, fmt.Errorf("principal id is required")
	}
	if strings.TrimSpace(newPassword) == "" {
		return access.LocalCredential{}, fmt.Errorf("new password is required")
	}
	principal, credential, verifier, err := r.localCredentialByPrincipalID(ctx, principalID)
	if err != nil {
		return access.LocalCredential{}, err
	}
	if principal.DisabledAt != "" || !verifySecret(currentPassword, verifier) {
		return access.LocalCredential{}, sql.ErrNoRows
	}
	newVerifier, err := newSecretVerifier(newPassword)
	if err != nil {
		return access.LocalCredential{}, err
	}
	if _, err := r.db.ExecContext(ctx, `
UPDATE local_user_credentials
SET password_verifier = ?, must_change_password = 0, updated_at = CURRENT_TIMESTAMP, password_changed_at = CURRENT_TIMESTAMP
WHERE principal_id = ?
`, newVerifier, principalID); err != nil {
		return access.LocalCredential{}, err
	}
	credential, err = r.LocalCredential(ctx, principalID)
	if err != nil {
		return access.LocalCredential{}, err
	}
	return credential, nil
}

func (r *Repository) LocalCredential(ctx context.Context, principalID string) (access.LocalCredential, error) {
	_, credential, _, err := r.localCredentialByPrincipalID(ctx, principalID)
	return credential, err
}

func (r *Repository) upsertLocalCredential(ctx context.Context, principalID, verifier string, mustChange bool) error {
	mustChangeValue := 0
	if mustChange {
		mustChangeValue = 1
	}
	_, err := r.db.ExecContext(ctx, `
INSERT INTO local_user_credentials (principal_id, password_verifier, must_change_password, updated_at, password_changed_at)
VALUES (?, ?, ?, CURRENT_TIMESTAMP, NULL)
ON CONFLICT(principal_id) DO UPDATE SET
  password_verifier = excluded.password_verifier,
  must_change_password = excluded.must_change_password,
  updated_at = CURRENT_TIMESTAMP,
  password_changed_at = NULL
`, principalID, verifier, mustChangeValue)
	return err
}

func (r *Repository) localCredentialByEmail(ctx context.Context, email string) (access.Principal, access.LocalCredential, string, error) {
	return r.scanLocalCredential(ctx, `
SELECT p.id, p.kind, p.email, p.display_name, p.disabled_at, p.created_at, p.updated_at,
       c.password_verifier, c.must_change_password, c.created_at, c.updated_at, c.password_changed_at
FROM principals p
JOIN local_user_credentials c ON c.principal_id = p.id
WHERE lower(p.email) = lower(?) AND p.email <> ''
LIMIT 1
`, email)
}

func (r *Repository) localCredentialByPrincipalID(ctx context.Context, principalID string) (access.Principal, access.LocalCredential, string, error) {
	return r.scanLocalCredential(ctx, `
SELECT p.id, p.kind, p.email, p.display_name, p.disabled_at, p.created_at, p.updated_at,
       c.password_verifier, c.must_change_password, c.created_at, c.updated_at, c.password_changed_at
FROM principals p
JOIN local_user_credentials c ON c.principal_id = p.id
WHERE p.id = ?
LIMIT 1
`, strings.TrimSpace(principalID))
}

func (r *Repository) scanLocalCredential(ctx context.Context, query, arg string) (access.Principal, access.LocalCredential, string, error) {
	var principal access.Principal
	var credential access.LocalCredential
	var disabledAt, passwordChangedAt sql.NullString
	var verifier string
	var mustChange int
	err := r.db.QueryRowContext(ctx, query, arg).Scan(
		&principal.ID,
		&principal.Kind,
		&principal.Email,
		&principal.DisplayName,
		&disabledAt,
		&principal.CreatedAt,
		&principal.UpdatedAt,
		&verifier,
		&mustChange,
		&credential.CreatedAt,
		&credential.UpdatedAt,
		&passwordChangedAt,
	)
	if err != nil {
		return access.Principal{}, access.LocalCredential{}, "", err
	}
	principal.DisabledAt = nullString(disabledAt)
	credential.PrincipalID = principal.ID
	credential.MustChangePassword = mustChange != 0
	credential.PasswordChangedAt = nullString(passwordChangedAt)
	return principal, credential, verifier, nil
}

func (r *Repository) SetPrincipalRole(ctx context.Context, input access.PrincipalRoleInput) (access.Principal, error) {
	access.ClearAuthorizationCache(ctx)
	email := access.NormalizeEmail(input.Email)
	if email == "" {
		return access.Principal{}, fmt.Errorf("email is required")
	}
	if strings.TrimSpace(input.Role) == "" {
		return access.Principal{}, fmt.Errorf("role is required")
	}
	role, err := r.q.GetRoleByName(ctx, input.Role)
	if err != nil {
		return access.Principal{}, err
	}
	principal, err := r.UpsertPrincipal(ctx, access.PrincipalInput{
		ID:          access.PrincipalIDForEmail(email),
		Email:       email,
		DisplayName: firstNonEmpty(strings.TrimSpace(input.DisplayName), email),
	})
	if err != nil {
		return access.Principal{}, err
	}
	if err := r.q.DeletePrincipalRoleBindings(ctx, platformdb.DeletePrincipalRoleBindingsParams{
		WorkspaceID: input.WorkspaceID,
		PrincipalID: sql.NullString{String: principal.ID, Valid: true},
	}); err != nil {
		return access.Principal{}, err
	}
	bindingID := stableAccessID("rolebinding", input.WorkspaceID, principal.ID+"|"+input.Role)
	if err := r.deleteRoleBindingGrants(ctx, bindingID); err != nil {
		return access.Principal{}, err
	}
	if err := r.q.InsertRoleBinding(ctx, platformdb.InsertRoleBindingParams{
		ID:          bindingID,
		WorkspaceID: input.WorkspaceID,
		RoleID:      role.ID,
		PrincipalID: sql.NullString{String: principal.ID, Valid: true},
	}); err != nil {
		return access.Principal{}, err
	}
	if err := r.syncRoleBindingGrants(ctx, bindingID, input.WorkspaceID, input.Role, access.SubjectPrincipal, principal.ID); err != nil {
		return access.Principal{}, err
	}
	return principal, nil
}

func (r *Repository) SetPlatformRole(ctx context.Context, input access.PlatformRoleInput) (access.Principal, error) {
	access.ClearAuthorizationCache(ctx)
	principalID := strings.TrimSpace(input.PrincipalID)
	email := access.NormalizeEmail(input.Email)
	if principalID == "" && email == "" {
		return access.Principal{}, fmt.Errorf("principal id or email is required")
	}
	if strings.TrimSpace(input.Role) == "" {
		return access.Principal{}, fmt.Errorf("role is required")
	}
	role, err := r.q.GetRoleByName(ctx, input.Role)
	if err != nil {
		return access.Principal{}, err
	}
	if principalID == "" {
		principalID = access.PrincipalIDForEmail(email)
	}
	principal, err := r.UpsertPrincipal(ctx, access.PrincipalInput{
		ID:          principalID,
		Email:       email,
		DisplayName: firstNonEmpty(strings.TrimSpace(input.DisplayName), email, principalID),
	})
	if err != nil {
		return access.Principal{}, err
	}
	bindingID, err := newID("platformrolebinding")
	if err != nil {
		return access.Principal{}, err
	}
	if err := r.q.InsertPlatformRoleBinding(ctx, platformdb.InsertPlatformRoleBindingParams{
		ID:          bindingID,
		RoleID:      role.ID,
		PrincipalID: principal.ID,
	}); err != nil {
		return access.Principal{}, err
	}
	privileges, err := r.rolePrivileges(ctx, firstNonEmpty(input.Role, access.RolePlatformAdmin))
	if err != nil {
		return access.Principal{}, err
	}
	for _, privilege := range privileges {
		if err := r.upsertGrantWithID(ctx, "grant_platform_"+stableID(principal.ID+"|"+string(privilege)), access.GrantInput{
			Object:      access.PlatformObject(),
			SubjectType: access.SubjectPrincipal,
			SubjectID:   principal.ID,
			Privilege:   privilege,
		}); err != nil {
			return access.Principal{}, err
		}
	}
	return principal, nil
}

func (r *Repository) RemovePrincipalRoles(ctx context.Context, workspaceID, principalID string) error {
	access.ClearAuthorizationCache(ctx)
	if strings.TrimSpace(principalID) == "" {
		return fmt.Errorf("principal id is required")
	}
	return r.q.DeletePrincipalRoleBindings(ctx, platformdb.DeletePrincipalRoleBindingsParams{
		WorkspaceID: workspaceID,
		PrincipalID: sql.NullString{String: principalID, Valid: true},
	})
}
