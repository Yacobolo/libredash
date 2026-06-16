package platform

import (
	"context"
	"time"

	"github.com/Yacobolo/libredash/internal/platform/db"
)

func (s *Store) CreateSession(ctx context.Context, principalID string, ttl time.Duration) (string, error) {
	token := newSecret()
	hash := tokenHash(token)
	expires := time.Now().Add(ttl).UTC().Format(time.RFC3339)
	return token, s.q.CreateSession(ctx, db.CreateSessionParams{
		ID:          newID("session"),
		PrincipalID: principalID,
		TokenHash:   hash,
		ExpiresAt:   expires,
	})
}

func (s *Store) PrincipalForToken(ctx context.Context, token string) (db.Principal, error) {
	session, err := s.q.GetSessionByTokenHash(ctx, tokenHash(token))
	if err != nil {
		return db.Principal{}, err
	}
	_ = s.q.TouchSession(ctx, session.ID)
	return s.q.GetPrincipal(ctx, session.PrincipalID)
}

func (s *Store) DeleteSession(ctx context.Context, token string) error {
	return s.q.DeleteSessionByTokenHash(ctx, tokenHash(token))
}

func (s *Store) CreateAPIToken(ctx context.Context, principalID, name string) (string, error) {
	token := newSecret()
	return token, s.q.CreateAPIToken(ctx, db.CreateAPITokenParams{
		ID:          newID("token"),
		PrincipalID: principalID,
		Name:        name,
		TokenHash:   tokenHash(token),
	})
}

func (s *Store) PrincipalForAPIToken(ctx context.Context, token string) (db.Principal, error) {
	apiToken, err := s.q.GetAPITokenByHash(ctx, tokenHash(token))
	if err != nil {
		return db.Principal{}, err
	}
	_ = s.q.TouchAPIToken(ctx, apiToken.ID)
	return s.q.GetPrincipal(ctx, apiToken.PrincipalID)
}
