package materialize

import (
	"path/filepath"
	"testing"
)

func TestDatabasePathIgnoresRemovedLegacyOverride(t *testing.T) {
	t.Setenv("LEAPVIEW_DUCKDB_PATH", filepath.Join(t.TempDir(), "override.duckdb"))
	dbDir := filepath.Join("var", "leapview", "duckdb")

	if got, want := DatabasePath(dbDir, "sales"), filepath.Join(dbDir, "leapview-sales.duckdb"); got != want {
		t.Fatalf("DatabasePath() = %q, want %q", got, want)
	}
}
