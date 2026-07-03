package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Yacobolo/libredash/internal/dashboard"
	reportdef "github.com/Yacobolo/libredash/internal/dashboard/report"
	"github.com/Yacobolo/libredash/internal/dataquery"
	"github.com/Yacobolo/libredash/internal/execution"
)

func TestExecutionMetricsBoundsDataQueries(t *testing.T) {
	inner := &blockingQueryMetrics{
		started: make(chan struct{}, 2),
		release: make(chan struct{}),
	}
	metrics := executionMetrics{
		QueryMetrics: inner,
		executor: execution.New(execution.Config{
			MaxRunningReads: 1,
			MaxQueuedReads:  1,
			ReadQueueWait:   time.Second,
			MaxRunningJobs:  1,
			MaxQueuedJobs:   1,
		}),
	}

	go func() {
		_, _ = metrics.ExecuteDataQuery(context.Background(), dataquery.SemanticRows("sales", "orders", []dataquery.Field{{Field: "orders.id"}}, nil, nil, nil, 0, 1, false))
	}()
	<-inner.started
	secondDone := make(chan error, 1)
	go func() {
		_, err := metrics.ExecuteDataQuery(context.Background(), dataquery.SemanticRows("sales", "orders", []dataquery.Field{{Field: "orders.id"}}, nil, nil, nil, 0, 1, false))
		secondDone <- err
	}()

	select {
	case <-inner.started:
		t.Fatal("queued query executed before read capacity opened")
	case <-time.After(50 * time.Millisecond):
	}

	_, err := metrics.ExecuteDataQuery(context.Background(), dataquery.SemanticRows("sales", "orders", []dataquery.Field{{Field: "orders.id"}}, nil, nil, nil, 0, 1, false))
	if !errors.Is(err, execution.ErrReadQueueFull) {
		t.Fatalf("third query error = %v, want ErrReadQueueFull", err)
	}

	close(inner.release)
	if err := <-secondDone; err != nil {
		t.Fatalf("queued query error = %v", err)
	}
}

func TestExecutionMetricsWrapsDashboardReads(t *testing.T) {
	inner := &blockingQueryMetrics{
		started: make(chan struct{}, 2),
		release: make(chan struct{}),
	}
	metrics := executionMetrics{
		QueryMetrics: inner,
		executor: execution.New(execution.Config{
			MaxRunningReads: 1,
			MaxQueuedReads:  1,
			ReadQueueWait:   time.Second,
			MaxRunningJobs:  1,
			MaxQueuedJobs:   1,
		}),
	}

	go func() {
		_, _ = metrics.QueryDashboardPage(context.Background(), "sales", "overview", dashboard.Filters{})
	}()
	<-inner.started
	secondDone := make(chan error, 1)
	go func() {
		_, err := metrics.QueryDashboardPage(context.Background(), "sales", "overview", dashboard.Filters{})
		secondDone <- err
	}()

	select {
	case <-inner.started:
		t.Fatal("queued dashboard query executed before read capacity opened")
	case <-time.After(50 * time.Millisecond):
	}
	close(inner.release)
	if err := <-secondDone; err != nil {
		t.Fatalf("queued dashboard query error = %v", err)
	}
}

type blockingQueryMetrics struct {
	fakeMetrics
	started chan struct{}
	release chan struct{}
}

func (m *blockingQueryMetrics) ExecuteDataQuery(context.Context, dataquery.Query) (dataquery.Result, error) {
	m.started <- struct{}{}
	<-m.release
	return dataquery.Result{Columns: dataquery.ColumnsFromNames([]string{"id"}), Rows: []dataquery.Row{{"id": "1"}}}, nil
}

func (m *blockingQueryMetrics) QueryDashboardPage(context.Context, string, string, dashboard.Filters) (dashboard.Patch, error) {
	m.started <- struct{}{}
	<-m.release
	return dashboard.Patch{}, nil
}

func (m *blockingQueryMetrics) QuerySemantic(context.Context, string, reportdef.AggregateQuery) (reportdef.QueryRows, error) {
	m.started <- struct{}{}
	<-m.release
	return nil, nil
}
