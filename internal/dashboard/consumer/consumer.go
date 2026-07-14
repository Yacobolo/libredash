package consumer

import (
	"context"
	"time"

	"github.com/Yacobolo/libredash/internal/dashboard"
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

type Request struct {
	DashboardID string
	PageID      string
	ModelID     string
	Command     string
	Filters     dashboard.Filters
	Targets     []Target
	Concurrency int
}

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
