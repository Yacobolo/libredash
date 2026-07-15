package maintenance

import (
	"context"
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"sync"
)

var (
	ErrInvalidCapacity      = errors.New("invalid capacity request")
	ErrInsufficientCapacity = errors.New("insufficient managed-data capacity")
	ErrCapacityUnavailable  = errors.New("managed-data capacity unavailable")
)

type freeBytesFunc func(string) (uint64, error)

// CapacityChecker serializes admission decisions for uploads sharing a local
// filesystem. Reservations account for uploads accepted but not yet allocated.
type CapacityChecker struct {
	root      string
	reserve   int64
	freeBytes freeBytesFunc

	mu       sync.Mutex
	reserved int64
}

func NewCapacityChecker(root string, reserveBytes int64) (*CapacityChecker, error) {
	return newCapacityChecker(root, reserveBytes, filesystemFreeBytes)
}

func newCapacityChecker(root string, reserveBytes int64, freeBytes freeBytesFunc) (*CapacityChecker, error) {
	if root == "" || reserveBytes < 0 || freeBytes == nil {
		return nil, ErrInvalidCapacity
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, ErrInvalidCapacity
	}
	return &CapacityChecker{root: absolute, reserve: reserveBytes, freeBytes: freeBytes}, nil
}

// Reserve atomically checks current free space and outstanding reservations.
// The caller must release the returned reservation on every terminal path.
func (c *CapacityChecker) Reserve(ctx context.Context, requestedBytes int64) (*CapacityReservation, error) {
	if requestedBytes < 0 {
		return nil, ErrInvalidCapacity
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	available, err := c.freeBytes(c.root)
	if err != nil {
		return nil, fmt.Errorf("%w: inspect filesystem", ErrCapacityUnavailable)
	}
	if c.reserved > math.MaxInt64-c.reserve || requestedBytes > math.MaxInt64-c.reserve-c.reserved {
		return nil, ErrInsufficientCapacity
	}
	required := c.reserve + c.reserved + requestedBytes
	if uint64(required) > available {
		return nil, ErrInsufficientCapacity
	}
	c.reserved += requestedBytes
	return &CapacityReservation{checker: c, remaining: requestedBytes}, nil
}

// CapacityReservation tracks bytes accepted but not yet reflected in the
// filesystem's free-space counter.
type CapacityReservation struct {
	checker   *CapacityChecker
	remaining int64
	released  bool
}

// Consume reports bytes already allocated on disk, preventing conservative
// double accounting while preserving the configured free-space reserve.
func (r *CapacityReservation) Consume(bytes int64) error {
	if r == nil || r.checker == nil || bytes < 0 {
		return ErrInvalidCapacity
	}
	r.checker.mu.Lock()
	defer r.checker.mu.Unlock()
	if r.released || bytes > r.remaining {
		return ErrInvalidCapacity
	}
	r.remaining -= bytes
	r.checker.reserved -= bytes
	return nil
}

// Release returns all unconsumed bytes. It is safe to call more than once.
func (r *CapacityReservation) Release() {
	if r == nil || r.checker == nil {
		return
	}
	r.checker.mu.Lock()
	defer r.checker.mu.Unlock()
	if r.released {
		return
	}
	r.checker.reserved -= r.remaining
	r.remaining = 0
	r.released = true
}
