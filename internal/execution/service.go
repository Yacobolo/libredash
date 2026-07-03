package execution

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Yacobolo/libredash/internal/dataquery"
)

var (
	ErrReadQueueFull = errors.New("query executor read queue is full")
	ErrJobQueueFull  = errors.New("query executor job queue is full")
)

type Config struct {
	MaxRunningReads int
	MaxQueuedReads  int
	ReadQueueWait   time.Duration
	MaxRunningJobs  int
	MaxQueuedJobs   int
}

type JobRef struct {
	WorkspaceID string
	RunID       string
	Kind        string
}

type Service struct {
	reads    chan struct{}
	waiting  chan struct{}
	jobs     chan struct{}
	jobWait  chan struct{}
	readWait time.Duration
}

type Stats struct {
	RunningReads int
	QueuedReads  int
	RunningJobs  int
	QueuedJobs   int
}

func New(config Config) *Service {
	config = config.withDefaults()
	return &Service{
		reads:    make(chan struct{}, config.MaxRunningReads),
		waiting:  make(chan struct{}, config.MaxQueuedReads),
		jobs:     make(chan struct{}, config.MaxRunningJobs),
		jobWait:  make(chan struct{}, config.MaxQueuedJobs),
		readWait: config.ReadQueueWait,
	}
}

func DefaultConfig() Config {
	return Config{
		MaxRunningReads: 4,
		MaxQueuedReads:  64,
		ReadQueueWait:   30 * time.Second,
		MaxRunningJobs:  1,
		MaxQueuedJobs:   64,
	}
}

func (c Config) withDefaults() Config {
	defaults := DefaultConfig()
	if c.MaxRunningReads <= 0 {
		c.MaxRunningReads = defaults.MaxRunningReads
	}
	if c.MaxQueuedReads == 0 {
		c.MaxQueuedReads = defaults.MaxQueuedReads
	} else if c.MaxQueuedReads < 0 {
		c.MaxQueuedReads = 0
	}
	if c.ReadQueueWait <= 0 {
		c.ReadQueueWait = defaults.ReadQueueWait
	}
	if c.MaxRunningJobs <= 0 {
		c.MaxRunningJobs = defaults.MaxRunningJobs
	}
	if c.MaxQueuedJobs == 0 {
		c.MaxQueuedJobs = defaults.MaxQueuedJobs
	} else if c.MaxQueuedJobs < 0 {
		c.MaxQueuedJobs = 0
	}
	return c
}

func (s *Service) SubmitRead(ctx context.Context, query dataquery.Query, execute func(context.Context) (dataquery.Result, error)) (dataquery.Result, error) {
	if s == nil {
		return execute(ctx)
	}
	if execute == nil {
		return dataquery.Result{}, fmt.Errorf("query executor read function is required")
	}
	release, queueWait, err := s.acquireRead(ctx)
	if err != nil {
		return dataquery.Result{
			QueueWaitMS:    durationMillis(queueWait),
			ExecutionState: executionStateForError(ctx, err, dataquery.ExecutionRejected),
		}, err
	}
	defer release()
	started := time.Now()
	result, err := execute(ctx)
	if result.QueueWaitMS == 0 {
		result.QueueWaitMS = durationMillis(queueWait)
	}
	if result.ExecutionMS == 0 {
		result.ExecutionMS = durationMillis(time.Since(started))
	}
	if result.ExecutionState == "" {
		if err == nil {
			result.ExecutionState = dataquery.ExecutionSucceeded
		} else {
			result.ExecutionState = executionStateForError(ctx, err, dataquery.ExecutionFailed)
		}
	}
	return result, err
}

func (s *Service) SubmitJob(ctx context.Context, ref JobRef, execute func(context.Context) error) error {
	if s == nil {
		return execute(ctx)
	}
	if execute == nil {
		return fmt.Errorf("query executor job function is required")
	}
	release, err := s.acquireJob(ctx)
	if err != nil {
		return err
	}
	defer release()
	return execute(ctx)
}

func (s *Service) DispatchJob(ref JobRef, execute func(context.Context) error, onError func(error)) error {
	if s == nil {
		go func() {
			if err := execute(context.Background()); err != nil && onError != nil {
				onError(err)
			}
		}()
		return nil
	}
	if !s.reserveJobWait() {
		return ErrJobQueueFull
	}
	go func() {
		defer s.releaseJobWait()
		err := s.SubmitJob(context.Background(), ref, execute)
		if err != nil && onError != nil {
			onError(err)
		}
	}()
	return nil
}

func (s *Service) Stats() Stats {
	if s == nil {
		return Stats{}
	}
	return Stats{
		RunningReads: len(s.reads),
		QueuedReads:  len(s.waiting),
		RunningJobs:  len(s.jobs),
		QueuedJobs:   len(s.jobWait),
	}
}

func (s *Service) acquireRead(ctx context.Context) (func(), time.Duration, error) {
	select {
	case s.reads <- struct{}{}:
		return func() { <-s.reads }, 0, nil
	default:
	}
	startWait := time.Now()
	select {
	case s.waiting <- struct{}{}:
		defer func() { <-s.waiting }()
	default:
		return nil, time.Since(startWait), ErrReadQueueFull
	}
	waitCtx, cancel := context.WithTimeout(ctx, s.readWait)
	defer cancel()
	select {
	case s.reads <- struct{}{}:
		return func() { <-s.reads }, time.Since(startWait), nil
	case <-waitCtx.Done():
		return nil, time.Since(startWait), waitCtx.Err()
	}
}

func (s *Service) acquireJob(ctx context.Context) (func(), error) {
	select {
	case s.jobs <- struct{}{}:
		return func() { <-s.jobs }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (s *Service) reserveJobWait() bool {
	select {
	case s.jobWait <- struct{}{}:
		return true
	default:
		return false
	}
}

func (s *Service) releaseJobWait() {
	<-s.jobWait
}

func durationMillis(duration time.Duration) int64 {
	if duration <= 0 {
		return 0
	}
	ms := duration.Milliseconds()
	if ms == 0 {
		return 1
	}
	return ms
}

func executionStateForError(ctx context.Context, err error, fallback string) string {
	if errors.Is(err, ErrReadQueueFull) {
		return dataquery.ExecutionRejected
	}
	if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
		return dataquery.ExecutionCanceled
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return dataquery.ExecutionTimeout
	}
	return fallback
}
