package query

import (
	"context"
	"fmt"
	"strings"
)

type Row map[string]any

type Rows []Row

type FloatBounds struct {
	Min   float64
	Max   float64
	Valid bool
}

type HistogramBin struct {
	Bucket int
	Count  int
	Start  float64
	End    float64
}

type DistributionSpec struct {
	GroupColumn string
	ValueColumn string
	Sort        []Sort
	Limit       int
}

type HistogramSpec struct {
	ValueColumn string
	BinCount    int
}

type Executor interface {
	Query(ctx context.Context, plan Plan) (Rows, error)
	Count(ctx context.Context, plan Plan) (int, error)
	FloatBounds(ctx context.Context, plan Plan, valueColumn string) (FloatBounds, error)
	Histogram(ctx context.Context, plan Plan, spec HistogramSpec) ([]HistogramBin, error)
	Distribution(ctx context.Context, plan Plan, spec DistributionSpec) (Rows, error)
}

type Service struct {
	planner  *Planner
	executor Executor
}

func NewService(planner *Planner, executor Executor) *Service {
	return &Service{planner: planner, executor: executor}
}

func (s *Service) Query(ctx context.Context, request Request) (Rows, error) {
	plan, err := s.planner.Plan(request)
	if err != nil {
		return nil, err
	}
	return s.executor.Query(ctx, plan)
}

// QueryBundle executes all compatible aggregate branches as one physical
// statement and restores each branch's authored output aliases.
func (s *Service) QueryBundle(ctx context.Context, requests []BundleRequest) (map[string]Rows, error) {
	plan, err := s.planner.PlanBundle(requests)
	if err != nil {
		return nil, err
	}
	rows, err := s.executor.Query(ctx, plan.Plan)
	if err != nil {
		return nil, err
	}
	return plan.Decode(rows)
}

func (s *Service) Rows(ctx context.Context, request RowRequest) (Rows, error) {
	plan, err := s.planner.PlanRows(request)
	if err != nil {
		return nil, err
	}
	return s.executor.Query(ctx, plan)
}

func (s *Service) Count(ctx context.Context, request CountRequest) (int, error) {
	plan, err := s.planner.PlanCount(request)
	if err != nil {
		return 0, err
	}
	return s.executor.Count(ctx, plan)
}

func (s *Service) FloatBounds(ctx context.Context, request RawValueRequest) (FloatBounds, error) {
	plan, err := s.planner.PlanRawValues(request)
	if err != nil {
		return FloatBounds{}, err
	}
	valueColumn := request.Measure.Alias
	if valueColumn == "" {
		valueColumn = "value"
	}
	return s.executor.FloatBounds(ctx, plan, valueColumn)
}

func (s *Service) Histogram(ctx context.Context, request RawValueRequest, binCount int) ([]HistogramBin, error) {
	plan, err := s.planner.PlanRawValues(request)
	if err != nil {
		return nil, err
	}
	valueColumn := request.Measure.Alias
	if valueColumn == "" {
		valueColumn = "value"
	}
	return s.executor.Histogram(ctx, plan, HistogramSpec{
		ValueColumn: valueColumn,
		BinCount:    binCount,
	})
}

func (s *Service) Distribution(ctx context.Context, request RawValueRequest, sort []Sort, limit int) (Rows, error) {
	plan, err := s.planner.PlanRawValues(request)
	if err != nil {
		return nil, err
	}
	valueColumn := request.Measure.Alias
	if valueColumn == "" {
		valueColumn = "value"
	}
	groupColumn := "label"
	if len(request.Dimensions) > 0 && request.Dimensions[0].Alias != "" {
		groupColumn = request.Dimensions[0].Alias
	}
	if err := validateDistributionSort(sort); err != nil {
		return nil, err
	}
	return s.executor.Distribution(ctx, plan, DistributionSpec{
		GroupColumn: groupColumn,
		ValueColumn: valueColumn,
		Sort:        sort,
		Limit:       limit,
	})
}

func validateDistributionSort(sort []Sort) error {
	for _, sortSpec := range sort {
		field := sortSpec.Field
		if field == "" {
			field = "label"
		}
		switch field {
		case "label", "min", "q1", "median", "q3", "max":
		default:
			return fmt.Errorf("unsupported distribution sort field %q", sortSpec.Field)
		}
		if sortSpec.Direction != "" && !strings.EqualFold(sortSpec.Direction, "asc") && !strings.EqualFold(sortSpec.Direction, "desc") {
			return fmt.Errorf("unsupported sort direction %q", sortSpec.Direction)
		}
	}
	return nil
}
