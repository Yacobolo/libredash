package duckdb

import (
	"context"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	semanticquery "github.com/Yacobolo/libredash/internal/analytics/query"
	"github.com/Yacobolo/libredash/internal/dataquery"
)

func TestQueryConnectionWaitReportsPoolSaturation(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, filepath.Join(t.TempDir(), "wait.duckdb"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SQLDB().SetMaxOpenConns(1)
	db.SQLDB().SetMaxIdleConns(1)

	held, err := db.SQLDB().Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	var observed atomic.Int64
	queryCtx := dataquery.WithConnectionWaitObserver(ctx, func(wait time.Duration) { observed.Add(int64(wait)) })
	go func() {
		_, queryErr := db.Count(queryCtx, semanticquery.Plan{SQL: "SELECT 1"})
		result <- queryErr
	}()

	waitForPoolWait(t, db, time.Second)
	time.Sleep(20 * time.Millisecond)
	if err := held.Close(); err != nil {
		t.Fatal(err)
	}
	if err := <-result; err != nil {
		t.Fatal(err)
	}
	if wait := time.Duration(observed.Load()); wait < 20*time.Millisecond {
		t.Fatalf("connection wait = %s, want at least 20ms", wait)
	}
}

func TestHistogramAndDistributionObserveOneConnectionAcquisitionEach(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, filepath.Join(t.TempDir(), "operations.duckdb"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	var acquisitions atomic.Int64
	ctx = dataquery.WithConnectionWaitObserver(ctx, func(time.Duration) { acquisitions.Add(1) })
	plan := semanticquery.Plan{SQL: "SELECT * FROM (VALUES (1, 'a'), (2, 'a'), (3, 'b')) AS source(value, label)"}
	if _, err := db.Histogram(ctx, plan, semanticquery.HistogramSpec{ValueColumn: "value", BinCount: 2}); err != nil {
		t.Fatal(err)
	}
	if got := acquisitions.Load(); got != 1 {
		t.Fatalf("histogram connection acquisitions = %d, want 1", got)
	}
	if _, err := db.Distribution(ctx, plan, semanticquery.DistributionSpec{GroupColumn: "label", ValueColumn: "value"}); err != nil {
		t.Fatal(err)
	}
	if got := acquisitions.Load(); got != 2 {
		t.Fatalf("total connection acquisitions = %d, want 2", got)
	}
}

func waitForPoolWait(t *testing.T, db *Database, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for db.SQLDB().Stats().WaitCount == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if db.SQLDB().Stats().WaitCount == 0 {
		t.Fatal("query did not wait for the saturated connection pool")
	}
}
