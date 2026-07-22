package ducklake

import (
	"context"
	"database/sql"
	"sync/atomic"
	"testing"
	"time"

	semanticquery "github.com/Yacobolo/leapview/internal/analytics/query"
	"github.com/Yacobolo/leapview/internal/dataquery"
)

func TestEnvironmentQueryConnectionWaitReportsPoolSaturation(t *testing.T) {
	db, err := sql.Open("duckdb", "")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	environment := &Environment{db: db, readConcurrency: 1}
	ctx := context.Background()
	held, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}

	var observed atomic.Int64
	result := make(chan error, 1)
	queryCtx := dataquery.WithConnectionWaitObserver(ctx, func(wait time.Duration) { observed.Add(int64(wait)) })
	go func() {
		_, queryErr := environment.Count(queryCtx, semanticquery.Plan{SQL: "SELECT 1"})
		result <- queryErr
	}()
	deadline := time.Now().Add(time.Second)
	for db.Stats().WaitCount == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if db.Stats().WaitCount == 0 {
		t.Fatal("query did not wait for the saturated connection pool")
	}
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

func TestEnvironmentHistogramAndDistributionAcquireOneConnectionEach(t *testing.T) {
	db, err := sql.Open("duckdb", "")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	environment := &Environment{db: db, readConcurrency: 1}
	var acquisitions atomic.Int64
	ctx := dataquery.WithConnectionWaitObserver(context.Background(), func(time.Duration) { acquisitions.Add(1) })
	plan := semanticquery.Plan{SQL: "SELECT * FROM (VALUES (1, 'a'), (2, 'a'), (3, 'b')) AS source(value, label)"}

	if _, err := environment.Histogram(ctx, plan, semanticquery.HistogramSpec{ValueColumn: "value", BinCount: 2}); err != nil {
		t.Fatal(err)
	}
	if got := acquisitions.Load(); got != 1 {
		t.Fatalf("histogram connection acquisitions = %d, want 1", got)
	}
	if _, err := environment.Distribution(ctx, plan, semanticquery.DistributionSpec{GroupColumn: "label", ValueColumn: "value"}); err != nil {
		t.Fatal(err)
	}
	if got := acquisitions.Load(); got != 2 {
		t.Fatalf("total connection acquisitions = %d, want 2", got)
	}
}
