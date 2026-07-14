package stream

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/Yacobolo/libredash/internal/dashboard"
	"github.com/Yacobolo/libredash/internal/dashboard/command"
	"github.com/Yacobolo/libredash/internal/dashboard/consumer"
	"github.com/Yacobolo/libredash/internal/dataquery"
)

type consumerExecutorStub struct {
	execute func(context.Context, consumer.Request, consumer.Publisher) error
	leases  atomic.Int32
}

func (s *consumerExecutorStub) ExecuteConsumersPage(ctx context.Context, request consumer.Request, publish consumer.Publisher) error {
	return s.execute(ctx, request, publish)
}

func (s *consumerExecutorStub) DashboardTargetConcurrency() int { return 3 }

func (s *consumerExecutorStub) WithDashboardRefreshLease(ctx context.Context, run func(context.Context) error) error {
	s.leases.Add(1)
	return run(ctx)
}

func TestTargetWorkPublishesProgressiveConsumerResultsWithoutPresentationKnowledge(t *testing.T) {
	executor := &consumerExecutorStub{execute: func(_ context.Context, request consumer.Request, publish consumer.Publisher) error {
		if request.Concurrency != 3 {
			t.Fatalf("concurrency = %d", request.Concurrency)
		}
		publish(consumer.Result{Target: request.Targets[1], Table: dashboard.Table{Title: "Orders"}})
		publish(consumer.Result{Target: request.Targets[0], Visual: dashboard.Visual{ID: "revenue"}})
		publish(consumer.Result{Target: request.Targets[1], Table: dashboard.Table{Title: "Orders", Cardinality: dashboard.ExactCardinality(42)}, TableMetadata: true})
		return nil
	}}
	events := runTargetWork(t, executor, command.RefreshPlan{Targets: []command.Target{
		{Kind: command.TargetVisual, ID: "revenue"},
		{Kind: command.TargetTable, ID: "orders"},
	}})
	if len(events) != 3 || events[0].Type != RefreshEventTable || events[1].Type != RefreshEventVisual || events[2].Type != RefreshEventTableMetadata {
		t.Fatalf("progressive events = %#v", events)
	}
	if executor.leases.Load() != 1 {
		t.Fatalf("refresh leases = %d", executor.leases.Load())
	}
}

func TestTargetWorkScopesConsumerFailuresAndSuppressesCancellation(t *testing.T) {
	executor := &consumerExecutorStub{execute: func(_ context.Context, request consumer.Request, publish consumer.Publisher) error {
		publish(consumer.Result{Target: request.Targets[0], Err: errors.New("denied")})
		publish(consumer.Result{Target: request.Targets[1], Err: context.Canceled})
		return nil
	}}
	events := runTargetWork(t, executor, command.RefreshPlan{Targets: []command.Target{
		{Kind: command.TargetVisual, ID: "margin"},
		{Kind: command.TargetVisual, ID: "orders"},
	}})
	if len(events) != 1 || events[0].Type != RefreshEventTargetError || events[0].Target != "visual:margin" {
		t.Fatalf("events = %#v", events)
	}
}

func TestTargetWorkPublishesAcceptedCacheOutcomes(t *testing.T) {
	executor := &consumerExecutorStub{execute: func(ctx context.Context, _ consumer.Request, _ consumer.Publisher) error {
		dataquery.ObserveCacheOutcome(ctx, dataquery.CacheHit)
		return nil
	}}
	observed := ""
	events := []RefreshEvent{}
	TargetWork(executor, WorkRequest{
		DashboardID: "commerce",
		PageID:      "overview",
		CacheObserved: func(outcome string) {
			observed = outcome
		},
	})(context.Background(), func(event RefreshEvent) bool {
		events = append(events, event)
		return true
	})
	if observed != dataquery.CacheHit || len(events) != 1 || events[0].CacheOutcome != dataquery.CacheHit {
		t.Fatalf("observed=%q events=%#v", observed, events)
	}
}

func runTargetWork(t *testing.T, executor TargetMetrics, plan command.RefreshPlan) []RefreshEvent {
	t.Helper()
	events := []RefreshEvent{}
	TargetWork(executor, WorkRequest{DashboardID: "commerce", PageID: "overview", Plan: plan})(context.Background(), func(event RefreshEvent) bool {
		events = append(events, event)
		return true
	})
	return events
}
