package sqlite

import (
	"context"
	"database/sql"

	"github.com/Yacobolo/libredash/internal/access"
	platformdb "github.com/Yacobolo/libredash/internal/platform/db"
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
	result, err := r.q.UpdateGrantByID(ctx, platformdb.UpdateGrantByIDParams{
		ObjectID: objectID, SubjectType: string(input.SubjectType), SubjectID: input.SubjectID,
		Privilege: string(input.Privilege), ID: id,
	})
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
	result, err := r.q.DeletePrincipalByID(ctx, id)
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
	rows, err := r.q.ListServicePrincipalSecretsByPrincipal(ctx, principalID)
	if err != nil {
		return nil, err
	}
	result := make([]access.ServicePrincipalSecret, 0, len(rows))
	for _, row := range rows {
		result = append(result, mapServicePrincipalSecret(row))
	}
	return result, nil
}

func (r *Repository) GetServicePrincipalSecret(ctx context.Context, principalID, secretID string) (access.ServicePrincipalSecret, error) {
	row, err := r.q.GetServicePrincipalSecretByID(ctx, platformdb.GetServicePrincipalSecretByIDParams{
		ServicePrincipalID: principalID, ID: secretID,
	})
	if err != nil {
		return access.ServicePrincipalSecret{}, err
	}
	return mapServicePrincipalSecret(row), nil
}

func mapServicePrincipalSecret(row platformdb.ServicePrincipalSecret) access.ServicePrincipalSecret {
	return access.ServicePrincipalSecret{
		ID: row.ID, ServicePrincipalID: row.ServicePrincipalID, Name: row.Name,
		ExpiresAt: row.ExpiresAt.String, CreatedAt: row.CreatedAt, RevokedAt: row.RevokedAt.String,
	}
}
