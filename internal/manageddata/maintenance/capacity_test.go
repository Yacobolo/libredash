package maintenance

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
)

func TestCapacityCheckerReservesAtomically(t *testing.T) {
	checker, err := newCapacityChecker("/managed", 20, func(string) (uint64, error) { return 100, nil })
	if err != nil {
		t.Fatal(err)
	}

	start := make(chan struct{})
	var accepted atomic.Int64
	var reservationsMu sync.Mutex
	var reservations []*CapacityReservation
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			reservation, reserveErr := checker.Reserve(t.Context(), 50)
			if reserveErr == nil {
				accepted.Add(1)
				reservationsMu.Lock()
				reservations = append(reservations, reservation)
				reservationsMu.Unlock()
				return
			}
			if !errors.Is(reserveErr, ErrInsufficientCapacity) {
				t.Errorf("Reserve() error = %v", reserveErr)
			}
		}()
	}
	close(start)
	wg.Wait()
	if got := accepted.Load(); got != 1 {
		t.Fatalf("accepted reservations = %d, want 1", got)
	}
	for _, reservation := range reservations {
		reservation.Release()
	}
	if _, err := checker.Reserve(t.Context(), 80); err != nil {
		t.Fatalf("Reserve() after release = %v", err)
	}
}

func TestCapacityReservationConsumeReleasesAllocatedBytes(t *testing.T) {
	checker, err := newCapacityChecker("/managed", 10, func(string) (uint64, error) { return 100, nil })
	if err != nil {
		t.Fatal(err)
	}
	reservation, err := checker.Reserve(t.Context(), 80)
	if err != nil {
		t.Fatal(err)
	}
	if err := reservation.Consume(30); err != nil {
		t.Fatal(err)
	}
	if _, err := checker.Reserve(t.Context(), 30); err != nil {
		t.Fatalf("Reserve() after Consume() = %v", err)
	}
	if err := reservation.Consume(51); !errors.Is(err, ErrInvalidCapacity) {
		t.Fatalf("over-consume error = %v", err)
	}
	reservation.Release()
	reservation.Release()
}

func TestCapacityCheckerValidatesAndHonorsContext(t *testing.T) {
	if _, err := newCapacityChecker("", 0, func(string) (uint64, error) { return 0, nil }); !errors.Is(err, ErrInvalidCapacity) {
		t.Fatalf("empty path error = %v", err)
	}
	if _, err := newCapacityChecker("/managed", -1, func(string) (uint64, error) { return 0, nil }); !errors.Is(err, ErrInvalidCapacity) {
		t.Fatalf("negative reserve error = %v", err)
	}
	checker, err := newCapacityChecker("/managed", 0, func(string) (uint64, error) { return ^uint64(0), nil })
	if err != nil {
		t.Fatal(err)
	}
	if _, err := checker.Reserve(t.Context(), -1); !errors.Is(err, ErrInvalidCapacity) {
		t.Fatalf("negative request error = %v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := checker.Reserve(ctx, 1); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled Reserve() error = %v", err)
	}
}

func TestCapacityCheckerUsesPlatformFilesystemProbe(t *testing.T) {
	checker, err := NewCapacityChecker(t.TempDir(), 0)
	if err != nil {
		t.Fatal(err)
	}
	reservation, err := checker.Reserve(t.Context(), 0)
	if err != nil {
		t.Fatal(err)
	}
	reservation.Release()
}
