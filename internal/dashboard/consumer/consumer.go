package consumer

import (
	"context"
	"time"

	"github.com/Yacobolo/leapview/internal/dashboard"
)

type Kind string

const (
	KindFilterOptions Kind = "filter_options"
	KindVisual        Kind = "visual"
	KindTable         Kind = "table"
)

type Target struct {
	Kind         Kind
	ID           string
	TableRequest dashboard.TableRequest
	// ExactCardinality is resolved from the authored table contract. The
	// default bounded mode never schedules a separate COUNT(*) query.
	ExactCardinality bool
}

// Key is the renderer-neutral identity used by status, audit, and
// observability surfaces. Kind remains internal execution metadata.
func (t Target) Key() string {
	if t.Kind == KindVisual || t.Kind == KindTable {
		return "visual:" + t.ID
	}
	return string(t.Kind) + ":" + t.ID
}

type Request struct {
	DashboardID string
	PageID      string
	ModelID     string
	Command     string
	Filters     dashboard.Filters
	Targets     []Target
	Concurrency int
	Progress    ProgressPublisher
}

type Progress struct {
	Completed            int
	Total                int
	WorkDuration         time.Duration
	CriticalPathDuration time.Duration
}

type ProgressPublisher func(Progress)

type Result struct {
	Target         Target
	Visual         dashboard.Visual
	Table          dashboard.Table
	FilterOptions  map[string][]dashboard.FilterOption
	TableMetadata  bool
	Err            error
	Duration       time.Duration
	Queries        int
	StageTimingsMs map[string]float64
}

type Publisher func(Result) bool

type Executor interface {
	ExecuteConsumersPage(context.Context, Request, Publisher) error
}
