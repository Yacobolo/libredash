// Package sqlite loads the cursor signing key ring from durable instance state.
package sqlite

import (
	"context"
	"crypto/rand"
	"fmt"
	"time"

	"github.com/Yacobolo/leapview/internal/cursorsigning"
	platformdb "github.com/Yacobolo/leapview/internal/platform/db"
)

func Configure(ctx context.Context, database platformdb.DBTX) error {
	queries := platformdb.New(database)
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return fmt.Errorf("generate cursor signing key: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if err := queries.CreateInitialAPICursorSigningKey(ctx, platformdb.CreateInitialAPICursorSigningKeyParams{Secret: secret, CreatedAt: now}); err != nil {
		return fmt.Errorf("create cursor signing key: %w", err)
	}
	rows, err := queries.ListAPICursorSigningKeys(ctx)
	if err != nil {
		return fmt.Errorf("list cursor signing keys: %w", err)
	}
	keys := map[string][]byte{}
	current := ""
	for _, row := range rows {
		keys[row.KeyID] = append([]byte(nil), row.Secret...)
		if row.Active != 0 {
			current = row.KeyID
		}
	}
	return cursorsigning.Configure(current, keys)
}
