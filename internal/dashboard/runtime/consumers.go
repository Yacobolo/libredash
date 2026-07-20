package runtime

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Yacobolo/leapview/internal/dashboard"
	"github.com/Yacobolo/leapview/internal/dashboard/consumer"
	reportdef "github.com/Yacobolo/leapview/internal/dashboard/report"
	"github.com/Yacobolo/leapview/internal/dataquery"
)

func (m *Service) ExecuteConsumersPage(ctx context.Context, request consumer.Request, publish consumer.Publisher) error {
	return m.queries.ExecuteConsumersPage(ctx, request, publish)
}

func (s *QueryService) ExecuteConsumersPage(ctx context.Context, request consumer.Request, publish consumer.Publisher) error {
	if publish == nil {
		return fmt.Errorf("consumer publisher is required")
	}
	report, runtime, err := s.snapshots.reports.reportRuntime(request.DashboardID, s.snapshots.runtimes)
	if err != nil {
		return err
	}
	if !runtime.ready {
		return runtime.missing
	}
	page := dashboardPage(report, request.PageID)
	request.PageID = page.ID
	request.Filters = report.NormalizeFiltersForPage(page.ID, request.Filters)
	logical := make([]consumer.LogicalQuery, 0, len(request.Targets))
	pageVisuals := stringSetFromSlice(pageVisualIDs(page))
	pageTables := stringSetFromSlice(pageTableIDs(page))
	pageFilters := stringSetFromSlice(report.PageFilterIDs(page.ID))
	for _, target := range request.Targets {
		item := consumer.LogicalQuery{Target: target}
		switch target.Kind {
		case consumer.KindVisual:
			if !pageVisuals[target.ID] {
				return fmt.Errorf("visual %q is not on page %q", target.ID, page.ID)
			}
			visual, ok := report.Visuals[target.ID]
			if !ok {
				return fmt.Errorf("unknown visual %q", target.ID)
			}
			aggregate, compileErr := s.snapshots.visuals.bundleAggregateRequest(ctx, runtime, report, request.Filters, target.ID, visual)
			if compileErr == nil {
				item.Query = reportAggregateDataQuery(report.SemanticModel, aggregate)
			} else if !dataquery.IsBundleIncompatible(compileErr) {
				return compileErr
			}
		case consumer.KindFilterOptions:
			if !pageFilters[target.ID] {
				return fmt.Errorf("filter %q is not on page %q", target.ID, page.ID)
			}
		case consumer.KindTable:
			table, ok := report.Tables[target.ID]
			if !ok {
				return fmt.Errorf("unknown table %q", target.ID)
			}
			if !pageTables[target.ID] {
				return fmt.Errorf("table %q is not on page %q", target.ID, page.ID)
			}
			item.Target.ExactCardinality = table.CardinalityOrDefault() == reportdef.TableCardinalityExact
		default:
			return fmt.Errorf("unknown consumer kind %q", target.Kind)
		}
		logical = append(logical, item)
	}
	plan, err := runtime.optimizer.OptimizeForConcurrency(logical, request.Concurrency)
	if err != nil {
		return err
	}
	if request.Progress != nil {
		request.Progress(consumer.Progress{Total: len(plan.Jobs)})
	}
	executionStarted := time.Now()
	limit := request.Concurrency
	if limit <= 0 {
		limit = 1
	}
	semaphore := make(chan struct{}, limit)
	var group sync.WaitGroup
	var progressMu sync.Mutex
	completedJobs := 0
	publishProgress := func(workDuration time.Duration) {
		if request.Progress == nil || ctx.Err() != nil {
			return
		}
		progressMu.Lock()
		defer progressMu.Unlock()
		if ctx.Err() != nil {
			return
		}
		completedJobs++
		progress := consumer.Progress{Completed: completedJobs, Total: len(plan.Jobs), WorkDuration: workDuration}
		if completedJobs == len(plan.Jobs) {
			progress.CriticalPathDuration = time.Since(executionStarted)
		}
		request.Progress(progress)
	}
jobLoop:
	for _, job := range plan.Jobs {
		if ctx.Err() != nil {
			break
		}
		select {
		case semaphore <- struct{}{}:
		case <-ctx.Done():
			break jobLoop
		}
		job := job
		group.Add(1)
		go func() {
			defer group.Done()
			defer func() { <-semaphore }()
			startedAt := time.Now()
			s.executeConsumerJob(ctx, request, job, publish)
			publishProgress(time.Since(startedAt))
		}()
	}
	group.Wait()
	return ctx.Err()
}

type consumerQueryStats struct {
	mu       sync.Mutex
	count    int
	reported int
	timings  map[string]float64
	last     map[string]float64
}

func (s *consumerQueryStats) observe(observation dataquery.PhysicalQueryObservation) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.count += observation.Count
	if s.timings == nil {
		s.timings = map[string]float64{}
	}
	result := observation.Result
	s.timings["admissionWait"] += float64(result.QueueWaitMS)
	s.timings["connectionWait"] += float64(result.ConnectionWaitMS)
	s.timings["planning"] += float64(result.PlanningMS)
	s.timings["database"] += float64(result.DatabaseMS)
	s.timings["execution"] += float64(result.ExecutionMS)
}

func (s *consumerQueryStats) attach(result *consumer.Result) {
	s.mu.Lock()
	defer s.mu.Unlock()
	result.Queries = s.count - s.reported
	s.reported = s.count
	if s.last == nil {
		s.last = map[string]float64{}
	}
	for stage, duration := range s.timings {
		if delta := duration - s.last[stage]; delta != 0 {
			if result.StageTimingsMs == nil {
				result.StageTimingsMs = map[string]float64{}
			}
			result.StageTimingsMs[stage] = delta
		}
		s.last[stage] = duration
	}
}

func (s *QueryService) executeConsumerJob(ctx context.Context, request consumer.Request, job consumer.Job, publish consumer.Publisher) {
	stats := &consumerQueryStats{}
	jobCtx := dataquery.WithPhysicalQueryObserver(consumerTargetContext(ctx, job), stats.observe)
	emit := func(result consumer.Result) bool {
		stats.attach(&result)
		return publish(result)
	}
	startedAt := time.Now()
	if len(job.Queries) == 0 {
		return
	}
	switch job.Queries[0].Target.Kind {
	case consumer.KindVisual:
		s.executeVisualConsumerJob(jobCtx, request, job, startedAt, emit)
	case consumer.KindFilterOptions:
		for _, query := range job.Queries {
			options, err := s.snapshots.queryFilterOptionsPage(jobCtx, request.DashboardID, request.PageID, []string{query.Target.ID})
			emit(consumer.Result{Target: query.Target, FilterOptions: options, Err: err, Duration: time.Since(startedAt)})
		}
	case consumer.KindTable:
		for _, query := range job.Queries {
			s.executeTableConsumer(jobCtx, request, query.Target, startedAt, emit)
		}
	}
}

func (s *QueryService) executeVisualConsumerJob(ctx context.Context, request consumer.Request, job consumer.Job, startedAt time.Time, publish consumer.Publisher) {
	ids := make([]string, len(job.Queries))
	for index, query := range job.Queries {
		ids[index] = query.Target.ID
	}
	var (
		visuals map[string]dashboard.Visual
		err     error
	)
	switch job.Strategy {
	case consumer.StrategyBundle:
		visuals, err = s.snapshots.queryVisualBundlePage(ctx, request.DashboardID, request.PageID, request.Filters, ids)
	case consumer.StrategyBatch:
		visuals, err = s.snapshots.queryVisualsPage(ctx, request.DashboardID, request.PageID, request.Filters, ids)
	default:
		visual, queryErr := s.snapshots.queryVisualPage(ctx, request.DashboardID, request.PageID, request.Filters, ids[0])
		visuals = map[string]dashboard.Visual{ids[0]: visual}
		err = queryErr
	}
	if err != nil && len(ids) > 1 && ctx.Err() == nil {
		var branchErr *dataquery.BundleBranchError
		if job.Strategy == consumer.StrategyBatch || dataquery.IsBundleIncompatible(err) || errors.As(err, &branchErr) {
			for _, query := range job.Queries {
				visual, queryErr := s.snapshots.queryVisualPage(ctx, request.DashboardID, request.PageID, request.Filters, query.Target.ID)
				publish(consumer.Result{Target: query.Target, Visual: visual, Err: queryErr, Duration: time.Since(startedAt)})
			}
			return
		}
	}
	for _, query := range job.Queries {
		publish(consumer.Result{Target: query.Target, Visual: visuals[query.Target.ID], Err: err, Duration: time.Since(startedAt)})
	}
}

func (s *QueryService) executeTableConsumer(ctx context.Context, request consumer.Request, target consumer.Target, startedAt time.Time, publish consumer.Publisher) {
	table, err := s.tables.queryTableRowsPage(ctx, request.DashboardID, request.PageID, request.Filters, target.TableRequest)
	if err == nil && table.Error != "" {
		err = errors.New(table.Error)
	}
	if err != nil || !publish(consumer.Result{Target: target, Table: table, Err: err, Duration: time.Since(startedAt)}) {
		return
	}
	defaults := target.TableRequest.WithDefaults()
	_, totalKnown := table.Cardinality.ExactValue()
	if totalKnown || !consumerTableNeedsExactCount(request.Command, defaults, target.ExactCardinality) {
		return
	}
	total, err := s.tables.queryTableCountPage(ctx, request.DashboardID, request.PageID, request.Filters, target.TableRequest)
	if err != nil {
		if ctx.Err() == nil {
			publish(consumer.Result{Target: target, Err: err, TableMetadata: true})
		}
		return
	}
	applyTableTotal(&table, total)
	publish(consumer.Result{Target: target, Table: table, TableMetadata: true})
}

func consumerTableNeedsExactCount(command string, request dashboard.TableRequest, exact bool) bool {
	return exact && (request.Block == "all" || command != "visual_window" && request.Block == "a" && request.Start == 0)
}

func consumerTargetContext(ctx context.Context, job consumer.Job) context.Context {
	consumers := make([]string, 0, len(job.Queries))
	for _, query := range job.Queries {
		consumers = append(consumers, query.Target.Key())
	}
	sort.Strings(consumers)
	metadata := dataquery.MetadataFromContext(ctx)
	metadata.ObjectType = "dashboard_refresh_targets"
	metadata.ObjectID = strings.Join(consumers, ",")
	return dataquery.WithMetadata(ctx, metadata)
}

func stringSetFromSlice(values []string) map[string]bool {
	result := make(map[string]bool, len(values))
	for _, value := range values {
		result[value] = true
	}
	return result
}
