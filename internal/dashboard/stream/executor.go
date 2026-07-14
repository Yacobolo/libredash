package stream

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"

	semanticmodel "github.com/Yacobolo/libredash/internal/analytics/model"
	"github.com/Yacobolo/libredash/internal/dashboard"
	"github.com/Yacobolo/libredash/internal/dashboard/command"
	reportdef "github.com/Yacobolo/libredash/internal/dashboard/report"
	"github.com/Yacobolo/libredash/internal/dataquery"
)

type TargetMetrics interface {
	DataDir() string
	QueryTablePage(ctx context.Context, dashboardID, pageID string, filters dashboard.Filters, request dashboard.TableRequest) (dashboard.Table, error)
}

type visualTargetMetrics interface {
	QueryVisualPage(ctx context.Context, dashboardID, pageID string, filters dashboard.Filters, visualID string) (dashboard.Visual, error)
}

type visualBatchTargetMetrics interface {
	QueryVisualsPage(ctx context.Context, dashboardID, pageID string, filters dashboard.Filters, visualIDs []string) (map[string]dashboard.Visual, error)
}

type reportTargetMetrics interface {
	Report(dashboardID string) (reportdef.Dashboard, *semanticmodel.Model, bool)
}

type filterOptionsTargetMetrics interface {
	QueryFilterOptionsPage(ctx context.Context, dashboardID, pageID string, filterIDs []string) (map[string][]dashboard.FilterOption, error)
}

type materializationTargetMetrics interface {
	RefreshMaterializations(context.Context, string) error
}

type targetConcurrencyMetrics interface {
	DashboardTargetConcurrency() int
}

type refreshLeaseMetrics interface {
	WithDashboardRefreshLease(context.Context, func(context.Context) error) error
}

type WorkRequest struct {
	DashboardID string
	PageID      string
	ModelID     string
	Filters     dashboard.Filters
	Plan        command.RefreshPlan
	Before      func(context.Context) error
	// Observers may be invoked concurrently by independent target jobs.
	EventObserved EventPublisher
	CacheObserved dataquery.CacheOutcomeObserver
}

// TargetWork executes independently queryable targets with bounded
// concurrency. Jobs are admitted in authored plan order and publish complete
// component payloads as soon as each finishes.
func TargetWork(metrics TargetMetrics, request WorkRequest) RefreshWork {
	return func(ctx context.Context, publish RefreshPublisher) {
		observedPublish := func(event RefreshEvent) bool {
			accepted := publish(event)
			if accepted && request.EventObserved != nil {
				request.EventObserved(event)
			}
			return accepted
		}
		ctx = dataquery.WithCacheOutcomeObserver(ctx, func(outcome string) {
			if observedPublish(RefreshEvent{Type: RefreshEventCacheOutcome, CacheOutcome: outcome}) && request.CacheObserved != nil {
				request.CacheObserved(outcome)
			}
		})
		if request.Before != nil {
			if err := request.Before(ctx); err != nil {
				if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
					observedPublish(RefreshEvent{Type: RefreshEventTargetError, Target: "refresh", Err: err})
				}
				return
			}
		}
		run := func(ctx context.Context) error {
			runTargetJobs(ctx, metrics, request, observedPublish)
			return nil
		}
		if capability, ok := metrics.(refreshLeaseMetrics); ok {
			if err := capability.WithDashboardRefreshLease(ctx, run); err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
				observedPublish(RefreshEvent{Type: RefreshEventTargetError, Target: "refresh", Err: err})
			}
			return
		}
		_ = run(ctx)
	}
}

func runTargetJobs(ctx context.Context, metrics TargetMetrics, request WorkRequest, publish RefreshPublisher) {
	limit := refreshConcurrency(metrics)
	semaphore := make(chan struct{}, limit)
	var group sync.WaitGroup
targetLoop:
	for _, job := range refreshJobsForDashboard(metrics, request.DashboardID, request.Plan) {
		if ctx.Err() != nil {
			break
		}
		select {
		case semaphore <- struct{}{}:
		case <-ctx.Done():
			break targetLoop
		}
		job := job
		group.Add(1)
		go func() {
			defer group.Done()
			defer func() { <-semaphore }()
			stats := &physicalQueryStats{}
			jobCtx := dataquery.WithPhysicalQueryObserver(targetContext(ctx, job), stats.observe)
			executeJob(jobCtx, metrics, request, job, stats.publisher(publish))
		}()
	}
	group.Wait()
}

type physicalQueryStats struct {
	mu       sync.Mutex
	count    int
	timings  map[string]float64
	reported bool
}

func (s *physicalQueryStats) observe(observation dataquery.PhysicalQueryObservation) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.count += observation.Count
	result := observation.Result
	if s.timings == nil {
		s.timings = map[string]float64{}
	}
	s.timings["admissionWait"] += float64(result.QueueWaitMS)
	s.timings["connectionWait"] += float64(result.ConnectionWaitMS)
	s.timings["planning"] += float64(result.PlanningMS)
	s.timings["database"] += float64(result.DatabaseMS)
	s.timings["execution"] += float64(result.ExecutionMS)
}

func (s *physicalQueryStats) publisher(next RefreshPublisher) RefreshPublisher {
	return func(event RefreshEvent) bool {
		s.mu.Lock()
		if !s.reported {
			event.Queries = s.count
			event.StageTimingsMs = make(map[string]float64, len(s.timings))
			for stage, duration := range s.timings {
				event.StageTimingsMs[stage] = duration
			}
			s.reported = true
		}
		s.mu.Unlock()
		return next(event)
	}
}

func targetContext(ctx context.Context, job refreshJob) context.Context {
	consumers := make([]string, 0, len(job.targets))
	for _, target := range job.targets {
		consumers = append(consumers, string(target.Kind)+":"+target.ID)
	}
	sort.Strings(consumers)
	metadata := dataquery.MetadataFromContext(ctx)
	metadata.ObjectType = "dashboard_refresh_targets"
	metadata.ObjectID = strings.Join(consumers, ",")
	return dataquery.WithMetadata(ctx, metadata)
}

type refreshJob struct {
	targets []command.Target
}

func refreshJobs(plan command.RefreshPlan) []refreshJob {
	jobs := make([]refreshJob, 0, len(plan.Targets))
	for _, target := range plan.Targets {
		jobs = append(jobs, refreshJob{targets: []command.Target{target}})
	}
	return jobs
}

func refreshJobsForDashboard(metrics TargetMetrics, dashboardID string, plan command.RefreshPlan) []refreshJob {
	_, batchOK := metrics.(visualBatchTargetMetrics)
	reportPort, reportOK := metrics.(reportTargetMetrics)
	if !batchOK || !reportOK {
		return refreshJobs(plan)
	}
	definition, _, ok := reportPort.Report(dashboardID)
	if !ok {
		return refreshJobs(plan)
	}
	batch := []command.Target{}
	firstBatchIndex := -1
	for index, target := range plan.Targets {
		if target.Kind != command.TargetVisual {
			continue
		}
		visual, ok := definition.Visuals[target.ID]
		if ok && visual.ShapeOrDefault() == "single_value" {
			if firstBatchIndex < 0 {
				firstBatchIndex = index
			}
			batch = append(batch, target)
		}
	}
	if len(batch) < 2 {
		return refreshJobs(plan)
	}
	jobs := make([]refreshJob, 0, len(plan.Targets)-len(batch)+1)
	for index, target := range plan.Targets {
		if index == firstBatchIndex {
			jobs = append(jobs, refreshJob{targets: batch})
		}
		if target.Kind == command.TargetVisual {
			if visual, ok := definition.Visuals[target.ID]; ok && visual.ShapeOrDefault() == "single_value" {
				continue
			}
		}
		jobs = append(jobs, refreshJob{targets: []command.Target{target}})
	}
	return jobs
}

func executeJob(ctx context.Context, metrics TargetMetrics, request WorkRequest, job refreshJob, publish RefreshPublisher) {
	startedAt := time.Now()
	if len(job.targets) <= 1 {
		executeTarget(ctx, metrics, request, job.targets[0], publish)
		return
	}
	port, ok := metrics.(visualBatchTargetMetrics)
	if !ok {
		for _, target := range job.targets {
			executeTarget(ctx, metrics, request, target, publish)
		}
		return
	}
	ids := make([]string, len(job.targets))
	for index, target := range job.targets {
		ids[index] = target.ID
	}
	visuals, err := port.QueryVisualsPage(ctx, request.DashboardID, request.PageID, request.Filters, ids)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || ctx.Err() != nil {
			return
		}
		for index, target := range job.targets {
			duration := time.Duration(0)
			if index == 0 {
				duration = time.Since(startedAt)
			}
			publish(RefreshEvent{Type: RefreshEventTargetError, Target: "visual:" + target.ID, Err: err, Duration: duration})
		}
		return
	}
	for index, target := range job.targets {
		duration := time.Duration(0)
		if index == 0 {
			duration = time.Since(startedAt)
		}
		publish(RefreshEvent{Type: RefreshEventVisual, Target: target.ID, Value: visuals[target.ID], Duration: duration})
	}
}

func executeTarget(ctx context.Context, metrics TargetMetrics, request WorkRequest, target command.Target, publish RefreshPublisher) {
	startedAt := time.Now()
	var (
		event RefreshEvent
		err   error
	)
	switch target.Kind {
	case command.TargetFilterOptions:
		port, ok := metrics.(filterOptionsTargetMetrics)
		if !ok {
			err = errors.New("targeted filter option queries are not configured")
			break
		}
		event.Type = RefreshEventFilterOptions
		event.Value, err = port.QueryFilterOptionsPage(ctx, request.DashboardID, request.PageID, []string{target.ID})
	case command.TargetVisual:
		port, ok := metrics.(visualTargetMetrics)
		if !ok {
			err = errors.New("targeted visual queries are not configured")
			break
		}
		event.Type = RefreshEventVisual
		event.Value, err = port.QueryVisualPage(ctx, request.DashboardID, request.PageID, request.Filters, target.ID)
	case command.TargetTable:
		event.Type = RefreshEventTable
		var table dashboard.Table
		table, err = metrics.QueryTablePage(ctx, request.DashboardID, request.PageID, request.Filters, target.TableRequest)
		if err == nil && table.Error != "" {
			if table.Error == context.Canceled.Error() || table.Error == context.DeadlineExceeded.Error() {
				return
			}
			err = errors.New(table.Error)
		} else {
			event.Value = table
		}
	default:
		err = errors.New("unknown dashboard refresh target")
	}
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || ctx.Err() != nil {
			return
		}
		publish(RefreshEvent{Type: RefreshEventTargetError, Target: string(target.Kind) + ":" + target.ID, Err: err, Duration: time.Since(startedAt)})
		return
	}
	event.Target = target.ID
	event.Duration = time.Since(startedAt)
	publish(event)
}

func refreshConcurrency(metrics TargetMetrics) int {
	if capability, ok := metrics.(targetConcurrencyMetrics); ok {
		if value := capability.DashboardTargetConcurrency(); value > 0 {
			return value
		}
	}
	return 1
}
