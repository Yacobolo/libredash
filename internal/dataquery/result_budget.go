package dataquery

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

type ResultLimitReason string

const (
	ResultRows  ResultLimitReason = "rows"
	ResultBytes ResultLimitReason = "bytes"
)

type ResultLimits struct {
	MaxRows  int
	MaxBytes int64
}

func (l ResultLimits) Validate() error {
	if l.MaxRows <= 0 || l.MaxBytes <= 0 {
		return fmt.Errorf("query result row and byte limits must be positive")
	}
	return nil
}

type ResultLimitError struct {
	Reason          ResultLimitReason
	Limit, Observed int64
}

func (e *ResultLimitError) Error() string {
	return fmt.Sprintf("query result %s limit exceeded: observed %d, limit %d", e.Reason, e.Observed, e.Limit)
}
func ResultLimitReasonOf(err error) (ResultLimitReason, bool) {
	var target *ResultLimitError
	if !errors.As(err, &target) {
		return "", false
	}
	return target.Reason, true
}

type resultBudgetKey struct{}
type ResultBudget struct {
	mu     sync.Mutex
	limits ResultLimits
	rows   int
	bytes  int64
}

func WithResultBudget(ctx context.Context, limits ResultLimits) context.Context {
	if _, ok := ResultBudgetFromContext(ctx); ok {
		return ctx
	}
	return WithIndependentResultBudget(ctx, limits)
}

// WithIndependentResultBudget starts a distinct accounting scope even when the
// parent operation already owns a logical result budget. Physical coalesced
// work uses this to bound its retained rows without charging one caller's
// logical result before the shared result is distributed.
func WithIndependentResultBudget(ctx context.Context, limits ResultLimits) context.Context {
	return context.WithValue(ctx, resultBudgetKey{}, &ResultBudget{limits: limits})
}
func ResultBudgetFromContext(ctx context.Context) (*ResultBudget, bool) {
	budget, ok := ctx.Value(resultBudgetKey{}).(*ResultBudget)
	return budget, ok && budget != nil
}

func (b *ResultBudget) ConsumeRow(row map[string]any) error {
	if b == nil {
		return nil
	}
	bytes := EstimateRowBytes(row)
	b.mu.Lock()
	defer b.mu.Unlock()
	nextRows := b.rows + 1
	nextBytes := b.bytes + bytes
	if nextRows > b.limits.MaxRows {
		return &ResultLimitError{Reason: ResultRows, Limit: int64(b.limits.MaxRows), Observed: int64(nextRows)}
	}
	if nextBytes > b.limits.MaxBytes {
		return &ResultLimitError{Reason: ResultBytes, Limit: b.limits.MaxBytes, Observed: nextBytes}
	}
	b.rows = nextRows
	b.bytes = nextBytes
	return nil
}
func (b *ResultBudget) ConsumeRows(rows []Row) error {
	for _, row := range rows {
		if err := b.ConsumeRow(row); err != nil {
			return err
		}
	}
	return nil
}

// ConsumeSize accounts a batch without requiring its transport representation
// to enter the generic data-query contract.
func (b *ResultBudget) ConsumeSize(rows int, bytes int64) error {
	if b == nil {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	nextRows := b.rows + rows
	nextBytes := b.bytes + bytes
	if nextRows > b.limits.MaxRows {
		return &ResultLimitError{Reason: ResultRows, Limit: int64(b.limits.MaxRows), Observed: int64(nextRows)}
	}
	if nextBytes > b.limits.MaxBytes {
		return &ResultLimitError{Reason: ResultBytes, Limit: b.limits.MaxBytes, Observed: nextBytes}
	}
	b.rows = nextRows
	b.bytes = nextBytes
	return nil
}
func (b *ResultBudget) Usage() (int, int64) {
	if b == nil {
		return 0, 0
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.rows, b.bytes
}

func EstimateRowBytes(row map[string]any) int64 {
	size := int64(48)
	for key, value := range row {
		size += int64(len(key)) + estimateResultValue(value)
	}
	return size
}
func estimateResultValue(value any) int64 {
	switch v := value.(type) {
	case nil:
		return 1
	case string:
		return int64(len(v)) + 16
	case []byte:
		return int64(len(v)) + 24
	case []string:
		n := int64(24)
		for _, x := range v {
			n += int64(len(x)) + 16
		}
		return n
	case []any:
		n := int64(24)
		for _, x := range v {
			n += estimateResultValue(x)
		}
		return n
	case map[string]any:
		n := int64(48)
		for k, x := range v {
			n += int64(len(k)) + estimateResultValue(x)
		}
		return n
	case bool:
		return 1
	default:
		return 16
	}
}
