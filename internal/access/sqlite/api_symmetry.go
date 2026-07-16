package sqlite

import (
	"context"
	"database/sql"

	"github.com/Yacobolo/libredash/internal/access"
)

func (r *Repository) UpdateGrant(ctx context.Context, workspaceID, id string, input access.GrantInput) (access.Grant, error) {
	access.ClearAuthorizationCache(ctx)
	if _, err := r.GetGrant(ctx, workspaceID, id); err != nil {
		return access.Grant{}, err
	}
	objectID, err := r.ensureSecurableObject(ctx, input.Object)
	if err != nil {
		return access.Grant{}, err
	}
	result, err := r.db.ExecContext(ctx, `UPDATE grants SET object_id = ?, subject_type = ?, subject_id = ?, privilege = ? WHERE id = ?`,
		objectID, string(input.SubjectType), input.SubjectID, string(input.Privilege), id)
	if err != nil {
		return access.Grant{}, err
	}
	if count, err := result.RowsAffected(); err != nil || count != 1 {
		if err != nil {
			return access.Grant{}, err
		}
		return access.Grant{}, sql.ErrNoRows
	}
	return r.GetGrant(ctx, workspaceID, id)
}

func (r *Repository) DeletePrincipal(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM principals WHERE id = ?`, id)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *Repository) ListServicePrincipalSecrets(ctx context.Context, principalID string) ([]access.ServicePrincipalSecret, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, service_principal_id, name, COALESCE(expires_at, ''), created_at, COALESCE(revoked_at, '')
      FROM service_principal_secrets WHERE service_principal_id = ? ORDER BY created_at DESC, id DESC`, principalID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []access.ServicePrincipalSecret{}
	for rows.Next() {
		var item access.ServicePrincipalSecret
		if err := rows.Scan(&item.ID, &item.ServicePrincipalID, &item.Name, &item.ExpiresAt, &item.CreatedAt, &item.RevokedAt); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (r *Repository) GetServicePrincipalSecret(ctx context.Context, principalID, secretID string) (access.ServicePrincipalSecret, error) {
	var item access.ServicePrincipalSecret
	err := r.db.QueryRowContext(ctx, `SELECT id, service_principal_id, name, COALESCE(expires_at, ''), created_at, COALESCE(revoked_at, '')
      FROM service_principal_secrets WHERE service_principal_id = ? AND id = ?`, principalID, secretID).
		Scan(&item.ID, &item.ServicePrincipalID, &item.Name, &item.ExpiresAt, &item.CreatedAt, &item.RevokedAt)
	return item, err
}
