package stream

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	semanticmodel "github.com/Yacobolo/libredash/internal/analytics/model"
	"github.com/Yacobolo/libredash/internal/dashboard"
	"github.com/Yacobolo/libredash/internal/dashboard/command"
	reportdef "github.com/Yacobolo/libredash/internal/dashboard/report"
	"github.com/Yacobolo/libredash/internal/dashboard/reportmodel"
	"github.com/Yacobolo/libredash/internal/dataquery"
)

type TargetMetrics interface {
	DataDir() string
	QueryTablePage(ctx context.Context, dashboardID, pageID string, filters dashboard.Filters, request dashboard.TableRequest) (dashboard.Table, error)
}

// progressiveTableTargetMetrics keeps exact cardinality off the critical path
// for initial/reset table windows. Implementations must resolve both calls
// against the refresh context's snapshot and governance scope.
type progressiveTableTargetMetrics interface {
	QueryTableRowsPage(ctx context.Context, dashboardID, pageID string, filters dashboard.Filters, request dashboard.TableRequest) (dashboard.Table, error)
	QueryTableCountPage(ctx context.Context, dashboardID, pageID string, filters dashboard.Filters, request dashboard.TableRequest) (int, error)
}

type visualTargetMetrics interface {
	QueryVisualPage(ctx context.Context, dashboardID, pageID string, filters dashboard.Filters, visualID string) (dashboard.Visual, error)
}

type visualBatchTargetMetrics interface {
	QueryVisualsPage(ctx context.Context, dashboardID, pageID string, filters dashboard.Filters, visualIDs []string) (map[string]dashboard.Visual, error)
}

type visualBundleTargetMetrics interface {
	QueryVisualBundlePage(ctx context.Context, dashboardID, pageID string, filters dashboard.Filters, visualIDs []string) (map[string]dashboard.Visual, error)
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
	for _, job := range refreshJobsForDashboard(metrics, request.DashboardID, request.Filters, request.Plan) {
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
	mu              sync.Mutex
	count           int
	timings         map[string]float64
	reportedCount   int
	reportedTimings map[string]float64
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
		event.Queries = s.count - s.reportedCount
		s.reportedCount = s.count
		if s.reportedTimings == nil {
			s.reportedTimings = map[string]float64{}
		}
		for stage, duration := range s.timings {
			delta := duration - s.reportedTimings[stage]
			if delta != 0 {
				if event.StageTimingsMs == nil {
					event.StageTimingsMs = map[string]float64{}
				}
				event.StageTimingsMs[stage] = delta
			}
			s.reportedTimings[stage] = duration
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
	bundle  bool
}

func refreshJobs(plan command.RefreshPlan) []refreshJob {
	jobs := make([]refreshJob, 0, len(plan.Targets))
	for _, target := range plan.Targets {
		jobs = append(jobs, refreshJob{targets: []command.Target{target}})
	}
	sort.SliceStable(jobs, func(i, j int) bool {
		return targetKindPriority(jobs[i].targets[0].Kind) < targetKindPriority(jobs[j].targets[0].Kind)
	})
	return jobs
}

func refreshJobsForDashboard(metrics TargetMetrics, dashboardID string, filters dashboard.Filters, plan command.RefreshPlan) []refreshJob {
	_, batchOK := metrics.(visualBatchTargetMetrics)
	_, bundleOK := metrics.(visualBundleTargetMetrics)
	reportPort, reportOK := metrics.(reportTargetMetrics)
	if !batchOK || !reportOK {
		return refreshJobs(plan)
	}
	definition, model, ok := reportPort.Report(dashboardID)
	if !ok {
		return refreshJobs(plan)
	}
	type compatibilityGroup struct {
		key     string
		targets []command.Target
		bundle  bool
	}
	groups := []compatibilityGroup{}
	groupIndexes := map[string]int{}
	otherVisuals := []refreshJob{}
	filterOptions := []refreshJob{}
	tables := []refreshJob{}
	projectionGroups, projectedTargets := aggregateProjectionGroups(definition, model, filters, plan)
	for index, targets := range projectionGroups {
		key := fmt.Sprintf("projection:%d", index)
		groupIndexes[key] = len(groups)
		groups = append(groups, compatibilityGroup{key: key, targets: targets, bundle: true})
	}
	// A compatible scalar set that contains a multi-fact metric must remain a
	// model-scoped KPI batch. Otherwise the single-fact bundle pass below would
	// peel off local KPIs and leave the cross-fact scalar as an extra query.
	// Purely single-fact scalars may still join grouped visuals so they share the
	// fact scan.
	modelScalarKeys := map[string]bool{}
	for _, target := range plan.Targets {
		if target.Kind != command.TargetVisual || projectedTargets[target.ID] {
			continue
		}
		visual, ok := definition.Visuals[target.ID]
		if !ok || visual.ShapeOrDefault() != "single_value" {
			continue
		}
		facts, err := reportmodel.TargetFacts(&definition, model, "visual", target.ID)
		if err != nil || len(facts) < 2 {
			continue
		}
		key, err := singleValueCompatibilityKey(definition, model, filters, target.ID, visual)
		if err == nil {
			modelScalarKeys[key] = true
		}
	}
	for _, target := range plan.Targets {
		if target.Kind != command.TargetVisual {
			switch target.Kind {
			case command.TargetFilterOptions:
				filterOptions = append(filterOptions, refreshJob{targets: []command.Target{target}})
			case command.TargetTable:
				tables = append(tables, refreshJob{targets: []command.Target{target}})
			default:
				otherVisuals = append(otherVisuals, refreshJob{targets: []command.Target{target}})
			}
			continue
		}
		if projectedTargets[target.ID] {
			continue
		}
		visual, ok := definition.Visuals[target.ID]
		if ok && visual.ShapeOrDefault() == "single_value" {
			key, err := singleValueCompatibilityKey(definition, model, filters, target.ID, visual)
			if err == nil && modelScalarKeys[key] {
				key = "model-scalars:" + key
				index, exists := groupIndexes[key]
				if !exists {
					index = len(groups)
					groupIndexes[key] = index
					groups = append(groups, compatibilityGroup{key: key})
				}
				groups[index].targets = append(groups[index].targets, target)
				continue
			}
		}
		if bundleOK && ok && bundleEligibleVisual(visual) {
			facts, factErr := reportmodel.TargetFacts(&definition, model, "visual", target.ID)
			if factErr == nil && len(facts) == 1 {
				candidate := visual
				candidate.Query.Limit = 0
				key, err := singleValueCompatibilityKey(definition, model, filters, target.ID, candidate)
				if err == nil {
					key = "bundle:" + facts[0] + ":" + key
					index, exists := groupIndexes[key]
					if !exists {
						index = len(groups)
						groupIndexes[key] = index
						groups = append(groups, compatibilityGroup{key: key, bundle: true})
					}
					groups[index].targets = append(groups[index].targets, target)
					continue
				}
			}
		}
		if ok && visual.ShapeOrDefault() == "single_value" {
			key, err := singleValueCompatibilityKey(definition, model, filters, target.ID, visual)
			if err != nil {
				key = "unbatchable:" + target.ID
			}
			index, exists := groupIndexes[key]
			if !exists {
				index = len(groups)
				groupIndexes[key] = index
				groups = append(groups, compatibilityGroup{key: key})
			}
			groups[index].targets = append(groups[index].targets, target)
			continue
		}
		otherVisuals = append(otherVisuals, refreshJob{targets: []command.Target{target}})
	}
	jobs := make([]refreshJob, 0, len(groups)+len(otherVisuals)+len(filterOptions)+len(tables))
	for _, group := range groups {
		jobs = append(jobs, refreshJob{targets: group.targets, bundle: group.bundle})
	}
	jobs = append(jobs, otherVisuals...)
	jobs = append(jobs, filterOptions...)
	jobs = append(jobs, tables...)
	return jobs
}

func bundleEligibleVisual(visual reportdef.Visual) bool {
	switch visual.ShapeOrDefault() {
	case "single_value", "category_value", "category_series_value":
		return len(visual.Query.Measures) == 1
	case "category_multi_measure":
		return len(visual.Query.Measures) > 1
	default:
		return false
	}
}

type compatibleControl struct {
	ID      string                  `json:"id"`
	Control dashboard.FilterControl `json:"control"`
}

// singleValueCompatibilityKey describes the normalized semantic query scope
// that may be shared while measures are deduplicated by the runtime. Measure
// identity is intentionally excluded: batching exists to combine different
// measures, while table, limit, applicable controls, and targeted selections
// must match exactly. Governance identity and snapshot are constant for every
// target in one refresh lease and are applied to the combined query normally.
func singleValueCompatibilityKey(definition reportdef.Dashboard, model *semanticmodel.Model, filters dashboard.Filters, visualID string, visual reportdef.Visual) (string, error) {
	filters = filters.WithDefaults()
	controls := make([]compatibleControl, 0, len(definition.Filters))
	filterIDs := make([]string, 0, len(definition.Filters))
	for filterID := range definition.Filters {
		filterIDs = append(filterIDs, filterID)
	}
	sort.Strings(filterIDs)
	for _, filterID := range filterIDs {
		control, selected := filters.Controls[filterID]
		if !selected {
			continue
		}
		if model == nil {
			return "", errors.New("semantic model is required to resolve KPI filter scope")
		}
		applies, err := reportmodel.FilterAppliesToTarget(&definition, model, definition.Filters[filterID], "visual", visualID)
		if err != nil {
			return "", err
		}
		if applies {
			controls = append(controls, compatibleControl{ID: filterID, Control: control})
		}
	}
	selections := make([]dashboard.InteractionSelection, 0, len(filters.Selections))
	for _, selection := range filters.Selections {
		if selection.SourceKind == "" || selection.SourceID == "" || len(selection.Entries) == 0 {
			continue
		}
		if model == nil {
			return "", errors.New("semantic model is required to resolve KPI selection scope")
		}
		resolved, err := reportmodel.ResolveSelectionInteraction(&definition, model, selection.SourceKind, selection.SourceID)
		if err != nil {
			return "", err
		}
		for _, target := range resolved.Targets {
			if target.Kind == "visual" && target.ID == visualID {
				selections = append(selections, selection)
				break
			}
		}
	}
	scope := struct {
		Table      string                           `json:"table"`
		Limit      int                              `json:"limit"`
		Controls   []compatibleControl              `json:"controls"`
		Selections []dashboard.InteractionSelection `json:"selections"`
	}{
		Table:      visual.Query.Table,
		Limit:      visual.Query.Limit,
		Controls:   controls,
		Selections: selections,
	}
	encoded, err := json.Marshal(scope)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func targetKindPriority(kind command.TargetKind) int {
	switch kind {
	case command.TargetVisual:
		return 0
	case command.TargetFilterOptions:
		return 1
	case command.TargetTable:
		return 2
	default:
		return 1
	}
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
	if bundlePort, ok := metrics.(visualBundleTargetMetrics); ok && job.bundle {
		visuals, bundleErr := bundlePort.QueryVisualBundlePage(ctx, request.DashboardID, request.PageID, request.Filters, ids)
		if bundleErr == nil {
			publishVisualJobResults(job, visuals, startedAt, publish)
			return
		}
		var branchErr *dataquery.BundleBranchError
		if dataquery.IsBundleIncompatible(bundleErr) || errors.As(bundleErr, &branchErr) {
			for _, target := range job.targets {
				executeTarget(ctx, metrics, request, target, publish)
			}
			return
		}
		if errors.Is(bundleErr, context.Canceled) || errors.Is(bundleErr, context.DeadlineExceeded) || ctx.Err() != nil {
			return
		}
		for index, target := range job.targets {
			duration := time.Duration(0)
			if index == 0 {
				duration = time.Since(startedAt)
			}
			publish(RefreshEvent{Type: RefreshEventTargetError, Target: "visual:" + target.ID, Err: bundleErr, Duration: duration})
		}
		return
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
	publishVisualJobResults(job, visuals, startedAt, publish)
}

func publishVisualJobResults(job refreshJob, visuals map[string]dashboard.Visual, startedAt time.Time, publish RefreshPublisher) {
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
		executeTableTarget(ctx, metrics, request, target, startedAt, publish)
		return
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

func executeTableTarget(ctx context.Context, metrics TargetMetrics, request WorkRequest, target command.Target, startedAt time.Time, publish RefreshPublisher) {
	table, err := queryTableRowsTarget(ctx, metrics, request, target)
	if err == nil && table.Error != "" {
		if table.Error == context.Canceled.Error() || table.Error == context.DeadlineExceeded.Error() {
			return
		}
		err = errors.New(table.Error)
	}
	if err != nil {
		publishTargetError(ctx, target, err, startedAt, publish)
		return
	}
	if !publish(RefreshEvent{Type: RefreshEventTable, Target: target.ID, Value: table, Duration: time.Since(startedAt)}) {
		return
	}

	port, progressive := metrics.(progressiveTableTargetMetrics)
	requestDefaults := target.TableRequest.WithDefaults()
	if !progressive || table.TotalRowsKnown || !tableRequestNeedsExactCount(request.Plan.Command, requestDefaults) {
		return
	}
	totalRows, err := port.QueryTableCountPage(ctx, request.DashboardID, request.PageID, request.Filters, target.TableRequest)
	if err != nil {
		if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) && ctx.Err() == nil {
			// Rows are already usable. Preserve the successful table target while
			// still surfacing the auxiliary failure to audit/telemetry observers.
			publish(RefreshEvent{Type: RefreshEventTableCountErr, Target: target.ID, Err: err})
		}
		return
	}
	applyProgressiveTableTotal(&table, totalRows)
	publish(RefreshEvent{Type: RefreshEventTableMetadata, Target: target.ID, Value: table})
}

func tableRequestNeedsExactCount(commandName string, request dashboard.TableRequest) bool {
	return request.Block == "all" || commandName != "table_window" && request.Block == "a" && request.Start == 0
}

func queryTableRowsTarget(ctx context.Context, metrics TargetMetrics, request WorkRequest, target command.Target) (dashboard.Table, error) {
	if port, ok := metrics.(progressiveTableTargetMetrics); ok {
		return port.QueryTableRowsPage(ctx, request.DashboardID, request.PageID, request.Filters, target.TableRequest)
	}
	return metrics.QueryTablePage(ctx, request.DashboardID, request.PageID, request.Filters, target.TableRequest)
}

func publishTargetError(ctx context.Context, target command.Target, err error, startedAt time.Time, publish RefreshPublisher) {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || ctx.Err() != nil {
		return
	}
	publish(RefreshEvent{Type: RefreshEventTargetError, Target: string(target.Kind) + ":" + target.ID, Err: err, Duration: time.Since(startedAt)})
}

func applyProgressiveTableTotal(table *dashboard.Table, totalRows int) {
	table.TotalRows = totalRows
	table.TotalRowsKnown = true
	table.AvailableRows = min(totalRows, dashboard.TableInteractiveRowCap)
	table.IsCapped = totalRows > table.AvailableRows
}

func refreshConcurrency(metrics TargetMetrics) int {
	if capability, ok := metrics.(targetConcurrencyMetrics); ok {
		if value := capability.DashboardTargetConcurrency(); value > 0 {
			return value
		}
	}
	return 1
}
