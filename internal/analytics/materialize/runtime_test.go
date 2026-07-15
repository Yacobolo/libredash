package materialize

import (
	"path/filepath"
	"testing"
)

func TestDatabasePathIgnoresRemovedLegacyOverride(t *testing.T) {
	t.Setenv("LIBREDASH_DUCKDB_PATH", filepath.Join(t.TempDir(), "override.duckdb"))
	dbDir := filepath.Join("var", "libredash", "duckdb")

	if got, want := DatabasePath(dbDir, "sales"), filepath.Join(dbDir, "libredash-sales.duckdb"); got != want {
		t.Fatalf("DatabasePath() = %q, want %q", got, want)
	}
}
