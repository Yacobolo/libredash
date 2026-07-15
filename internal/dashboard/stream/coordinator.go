package stream

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Yacobolo/libredash/internal/dashboard"
	"github.com/Yacobolo/libredash/internal/dataquery"
)

type RefreshEventType string

const (
	RefreshEventStart         RefreshEventType = "start"
	RefreshEventFilterOptions RefreshEventType = "filter_options"
	RefreshEventVisual        RefreshEventType = "visual"
	RefreshEventTable         RefreshEventType = "table"
	RefreshEventTableMetadata RefreshEventType = "table_metadata"
	RefreshEventTableCountErr RefreshEventType = "table_count_error"
	RefreshEventTargetError   RefreshEventType = "target_error"
	RefreshEventCacheOutcome  RefreshEventType = "cache_outcome"
	RefreshEventProgress      RefreshEventType = "progress"
	RefreshEventComplete      RefreshEventType = "complete"
)

type RefreshEvent struct {
	Type            RefreshEventType
	RefreshID       string
	Generation      uint64
	Command         string
	Filters         dashboard.Filters
	Targets         []string
	Target          string
	Value           any
	Err             error
	Queries         int
	Duration        time.Duration
	StageTimingsMs  map[string]float64
	CacheOutcome    string
	ProgressPercent *float64
}

type Refresh struct {
	ID         string
	Generation uint64
	Command    string
	Filters    dashboard.Filters
}

type RefreshPreparation struct {
	Filters dashboard.Filters
	Command string
	Targets []string
	Plan    any
}

type RefreshSummary struct {
	RefreshID          string             `json:"refreshId"`
	Generation         uint64             `json:"generation"`
	Command            string             `json:"command"`
	PlannedTargets     int                `json:"plannedTargets"`
	TargetSuccesses    int                `json:"targetSuccesses"`
	TargetErrors       int                `json:"targetErrors"`
	QueryCount         int                `json:"queryCount"`
	CancellationCount  int                `json:"cancellationCount"`
	CancellationReason string             `json:"cancellationReason,omitempty"`
	CacheOutcomes      map[string]int     `json:"cacheOutcomes"`
	StageTimingsMs     map[string]float64 `json:"stageTimingsMs"`
	Outcome            string             `json:"outcome"`
}

type RefreshPublisher func(RefreshEvent) bool
type RefreshWork func(context.Context, RefreshPublisher)
type FilterMutation func(dashboard.Filters) (dashboard.Filters, error)
type RefreshPrepare func(dashboard.Filters) (RefreshPreparation, error)
type EventPublisher func(RefreshEvent)
type StartObserver func(Refresh)
type SummaryObserver func(RefreshSummary)

var refreshSequence atomic.Uint64

// refreshExecutionDebounce lets a burst of interaction commands settle while
// still publishing the canonical filters and loading state synchronously. The
// generation context makes the timer latest-only: superseded work never
// reaches a query port.
const refreshExecutionDebounce = 35 * time.Millisecond

var ErrCoordinatorClosed = errors.New("dashboard refresh coordinator is closed")

// Coordinator owns the canonical filters and active refresh generation for a
// single rendered page stream. Work contexts outlive command POST requests and
// are canceled only when superseded or when the page stream is closed.
type Coordinator struct {
	mu         sync.Mutex
	ctx        context.Context
	cancel     context.CancelFunc
	workCancel context.CancelFunc
	filters    dashboard.Filters
	generation uint64
	closed     bool
	publish    EventPublisher
	started    StartObserver
	observer   SummaryObserver
	active     *activeRefresh
}

type activeRefresh struct {
	refresh          Refresh
	startedAt        time.Time
	queryCount       int
	targetSuccesses  int
	targetErrors     int
	plannedTargets   int
	firstTargetErr   error
	setupRequiredErr error
	refreshError     error
	cacheOutcomes    map[string]int
	targetDuration   time.Duration
	stageTimingsMs   map[string]float64
	progressPercent  *float64
}

func NewCoordinator(parent context.Context, publish EventPublisher) *Coordinator {
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)
	return &Coordinator{
		ctx:     ctx,
		cancel:  cancel,
		filters: cloneFilters(dashboard.Filters{}.WithDefaults()),
		publish: publish,
	}
}

// Begin applies mutate to coordinator-owned filters, cancels the previous
// generation, and synchronously publishes start before launching query work.
func (c *Coordinator) Begin(mutate FilterMutation, work RefreshWork) (Refresh, error) {
	return c.BeginPrepared(func(current dashboard.Filters) (RefreshPreparation, error) {
		filters := current
		var err error
		if mutate != nil {
			filters, err = mutate(current)
		}
		return RefreshPreparation{Filters: filters}, err
	}, func(RefreshPreparation) RefreshWork { return work })
}

// BeginPrepared atomically derives filters and target metadata from the latest
// coordinator state. This prevents rapid commands from applying to stale
// client-posted filters.
func (c *Coordinator) BeginPrepared(prepare RefreshPrepare, work func(RefreshPreparation) RefreshWork) (Refresh, error) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return Refresh{}, ErrCoordinatorClosed
	}
	preparation := RefreshPreparation{Filters: cloneFilters(c.filters)}
	if prepare != nil {
		var err error
		preparation, err = prepare(cloneFilters(c.filters))
		if err != nil {
			c.mu.Unlock()
			return Refresh{}, err
		}
	}
	filters := cloneFilters(preparation.Filters.WithDefaults())
	preparation.Filters = cloneFilters(filters)
	var canceledSummary *RefreshSummary
	if c.workCancel != nil {
		c.workCancel()
		if c.active != nil {
			summary := c.summaryLocked(c.active, "canceled", 1, "superseded")
			canceledSummary = &summary
			c.active = nil
		}
	}
	ctx, cancel := context.WithCancel(c.ctx)
	c.workCancel = cancel
	c.filters = filters
	c.generation++
	refresh := Refresh{
		ID:         fmt.Sprintf("refresh-%d", refreshSequence.Add(1)),
		Generation: c.generation,
		Command:    preparation.Command,
		Filters:    cloneFilters(filters),
	}
	c.active = &activeRefresh{
		refresh: refresh, startedAt: time.Now(), plannedTargets: len(preparation.Targets),
		progressPercent: dashboard.NormalizeProgressPercent(nil, true),
	}
	c.mu.Unlock()

	if canceledSummary != nil {
		c.observe(*canceledSummary)
	}
	c.notifyStarted(refresh)
	c.emitCurrent(refresh, RefreshEvent{Type: RefreshEventStart, Command: preparation.Command, Filters: cloneFilters(filters), Targets: append([]string(nil), preparation.Targets...)})
	var refreshWork RefreshWork
	if work != nil {
		refreshWork = work(preparation)
	}
	go c.run(ctx, refresh, refreshWork)
	return refresh, nil
}

func (c *Coordinator) SetObserver(observer SummaryObserver) {
	c.mu.Lock()
	c.observer = observer
	c.mu.Unlock()
}

func (c *Coordinator) SetStartObserver(observer StartObserver) {
	c.mu.Lock()
	c.started = observer
	c.mu.Unlock()
}

func (c *Coordinator) Filters() dashboard.Filters {
	c.mu.Lock()
	defer c.mu.Unlock()
	return cloneFilters(c.filters)
}

func (c *Coordinator) Close() {
	c.CloseWithReason("closed")
}

func (c *Coordinator) CloseWithReason(reason string) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.closed = true
	if c.workCancel != nil {
		c.workCancel()
	}
	var summary *RefreshSummary
	if c.active != nil {
		value := c.summaryLocked(c.active, "canceled", 1, reason)
		summary = &value
		c.active = nil
	}
	c.cancel()
	c.mu.Unlock()
	if summary != nil {
		c.observe(*summary)
	}
}

func (c *Coordinator) run(ctx context.Context, refresh Refresh, work RefreshWork) {
	metadata := dataquery.MetadataFromContext(ctx)
	metadata.CorrelationID = refresh.ID
	ctx = dataquery.WithMetadata(ctx, metadata)
	if work != nil && debounceRefreshCommand(refresh.Command) {
		timer := time.NewTimer(refreshExecutionDebounce)
		select {
		case <-timer.C:
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return
		}
	}
	if work != nil {
		work(ctx, func(event RefreshEvent) bool {
			if ctx.Err() != nil {
				return false
			}
			return c.emitCurrent(refresh, event)
		})
	}
	if ctx.Err() != nil {
		return
	}
	if !c.emitCurrent(refresh, RefreshEvent{Type: RefreshEventComplete, Err: c.refreshError(refresh)}) {
		return
	}
	c.finish(refresh, "complete", 0)
}

func debounceRefreshCommand(command string) bool {
	return command == "select" || command == "clear_selection"
}

func (c *Coordinator) emitCurrent(refresh Refresh, event RefreshEvent) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || c.generation != refresh.Generation || c.ctx.Err() != nil {
		return false
	}
	event.RefreshID = refresh.ID
	event.Generation = refresh.Generation
	if event.Command == "" {
		event.Command = refresh.Command
	}
	if c.active != nil && c.active.refresh.Generation == refresh.Generation && event.Queries > 0 {
		c.active.queryCount += event.Queries
	}
	if c.active != nil && c.active.refresh.Generation == refresh.Generation {
		switch event.Type {
		case RefreshEventStart:
			event.ProgressPercent = cloneProgressPercent(c.active.progressPercent)
		case RefreshEventProgress:
			next := dashboard.NormalizeProgressPercent(event.ProgressPercent, true)
			if progressRegresses(c.active.progressPercent, next) {
				return false
			}
			c.active.progressPercent = next
			event.ProgressPercent = cloneProgressPercent(next)
		case RefreshEventComplete:
			c.active.progressPercent = dashboard.NormalizeProgressPercent(nil, false)
			event.ProgressPercent = cloneProgressPercent(c.active.progressPercent)
		}
		c.active.targetDuration += event.Duration
		if len(event.StageTimingsMs) > 0 {
			if c.active.stageTimingsMs == nil {
				c.active.stageTimingsMs = map[string]float64{}
			}
			for stage, duration := range event.StageTimingsMs {
				c.active.stageTimingsMs[stage] += duration
			}
		}
		switch event.Type {
		case RefreshEventFilterOptions, RefreshEventVisual, RefreshEventTable:
			c.active.targetSuccesses++
		case RefreshEventTargetError:
			if event.Target == "refresh" {
				c.active.refreshError = event.Err
			} else {
				c.active.targetErrors++
				if c.active.firstTargetErr == nil {
					c.active.firstTargetErr = event.Err
				}
				if c.active.setupRequiredErr == nil && setupRequired(event.Err) {
					c.active.setupRequiredErr = event.Err
				}
			}
		case RefreshEventCacheOutcome:
			if event.CacheOutcome != "" {
				if c.active.cacheOutcomes == nil {
					c.active.cacheOutcomes = map[string]int{}
				}
				c.active.cacheOutcomes[event.CacheOutcome]++
			}
		}
	}
	if c.publish != nil {
		c.publish(event)
	}
	return true
}

func progressRegresses(current, next *float64) bool {
	return current != nil && (next == nil || *next < *current)
}

func cloneProgressPercent(value *float64) *float64 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func (c *Coordinator) finish(refresh Refresh, outcome string, cancellationCount int) {
	c.mu.Lock()
	if c.active == nil || c.active.refresh.Generation != refresh.Generation {
		c.mu.Unlock()
		return
	}
	summary := c.summaryLocked(c.active, outcome, cancellationCount, "")
	c.active = nil
	c.mu.Unlock()
	c.observe(summary)
}

func (c *Coordinator) summaryLocked(active *activeRefresh, outcome string, cancellationCount int, cancellationReason string) RefreshSummary {
	if outcome == "complete" {
		switch {
		case active.refreshError != nil:
			outcome = "error"
		case active.targetErrors > 0 && active.targetSuccesses > 0:
			outcome = "partial"
		case active.targetErrors > 0:
			outcome = "error"
		}
	}
	stageTimings := make(map[string]float64, len(active.stageTimingsMs)+2)
	for stage, duration := range active.stageTimingsMs {
		stageTimings[stage] = duration
	}
	stageTimings["endToEnd"] = float64(time.Since(active.startedAt).Microseconds()) / 1000
	stageTimings["targetExecution"] = float64(active.targetDuration.Microseconds()) / 1000
	cacheOutcomes := make(map[string]int, len(active.cacheOutcomes))
	for cacheOutcome, count := range active.cacheOutcomes {
		cacheOutcomes[cacheOutcome] = count
	}
	return RefreshSummary{
		RefreshID:          active.refresh.ID,
		Generation:         active.refresh.Generation,
		Command:            active.refresh.Command,
		PlannedTargets:     active.plannedTargets,
		TargetSuccesses:    active.targetSuccesses,
		TargetErrors:       active.targetErrors,
		QueryCount:         active.queryCount,
		CancellationCount:  cancellationCount,
		CancellationReason: cancellationReason,
		CacheOutcomes:      cacheOutcomes,
		StageTimingsMs:     stageTimings,
		Outcome:            outcome,
	}
}

func (c *Coordinator) refreshError(refresh Refresh) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.active == nil || c.active.refresh.Generation != refresh.Generation {
		return nil
	}
	if c.active.refreshError != nil {
		return c.active.refreshError
	}
	if c.active.setupRequiredErr != nil {
		return c.active.setupRequiredErr
	}
	if c.active.targetErrors > 0 && c.active.targetSuccesses == 0 {
		return c.active.firstTargetErr
	}
	return nil
}

func setupRequired(err error) bool {
	var setup interface{ SetupRequired() bool }
	return errors.As(err, &setup) && setup.SetupRequired()
}

func (c *Coordinator) observe(summary RefreshSummary) {
	c.mu.Lock()
	observer := c.observer
	c.mu.Unlock()
	if observer != nil {
		observer(summary)
	}
}

func (c *Coordinator) notifyStarted(refresh Refresh) {
	c.mu.Lock()
	observer := c.started
	c.mu.Unlock()
	if observer != nil {
		observer(refresh)
	}
}

func cloneFilters(filters dashboard.Filters) dashboard.Filters {
	clone := dashboard.Filters{
		Controls:   make(map[string]dashboard.FilterControl, len(filters.Controls)),
		Selections: make([]dashboard.InteractionSelection, len(filters.Selections)),
	}
	for id, control := range filters.Controls {
		control.Values = append([]string(nil), control.Values...)
		clone.Controls[id] = control
	}
	for index, selection := range filters.Selections {
		selection.Entries = make([]dashboard.InteractionSelectionEntry, len(selection.Entries))
		for entryIndex, entry := range filters.Selections[index].Entries {
			entry.Mappings = append([]dashboard.InteractionSelectionMapping(nil), entry.Mappings...)
			selection.Entries[entryIndex] = entry
		}
		clone.Selections[index] = selection
	}
	return clone.WithDefaults()
}
