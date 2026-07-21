package app

import (
	"context"
	"sync"
	"time"

	"github.com/Yacobolo/leapview/internal/dashboard/publication"
)

const publicationMonitorInterval = 500 * time.Millisecond

type publicationStreamRegistry struct {
	mu      sync.Mutex
	streams map[string]map[string]*publicationStream
}

type publicationStream struct {
	cancel  context.CancelFunc
	version publicationStreamVersion
}

type publicationStreamVersion struct {
	PublicID       string
	ServingStateID string
}

func newPublicationStreamRegistry() *publicationStreamRegistry {
	return &publicationStreamRegistry{streams: map[string]map[string]*publicationStream{}}
}

func (r *publicationStreamRegistry) Register(parent context.Context, publicationID, streamID string, version publicationStreamVersion) (context.Context, func()) {
	ctx, cancel := context.WithCancel(parent)
	r.mu.Lock()
	if r.streams[publicationID] == nil {
		r.streams[publicationID] = map[string]*publicationStream{}
	}
	if previous := r.streams[publicationID][streamID]; previous != nil {
		previous.cancel()
	}
	registration := &publicationStream{cancel: cancel, version: version}
	r.streams[publicationID][streamID] = registration
	r.mu.Unlock()
	return ctx, func() {
		r.mu.Lock()
		if current := r.streams[publicationID][streamID]; current == registration {
			delete(r.streams[publicationID], streamID)
			if len(r.streams[publicationID]) == 0 {
				delete(r.streams, publicationID)
			}
		}
		r.mu.Unlock()
		cancel()
	}
}

func (r *publicationStreamRegistry) Active(publicationID, streamID string, version publicationStreamVersion) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	stream := r.streams[publicationID][streamID]
	return stream != nil && stream.version == version
}

func (r *publicationStreamRegistry) CloseStale(active map[string]publicationStreamVersion) {
	r.mu.Lock()
	stale := []context.CancelFunc{}
	for publicationID, streams := range r.streams {
		current, ok := active[publicationID]
		for streamID, stream := range streams {
			if ok && stream.version == current {
				continue
			}
			stale = append(stale, stream.cancel)
			delete(streams, streamID)
		}
		if len(streams) == 0 {
			delete(r.streams, publicationID)
		}
	}
	r.mu.Unlock()
	for _, cancel := range stale {
		cancel()
	}
}

func (r *publicationStreamRegistry) ClosePublication(publicationID string) {
	r.mu.Lock()
	streams := r.streams[publicationID]
	delete(r.streams, publicationID)
	r.mu.Unlock()
	for _, cancel := range streams {
		cancel.cancel()
	}
}

func (s *Server) startPublicationMonitor(ctx context.Context) {
	s.jobDispatchWG.Add(1)
	go func() {
		defer s.jobDispatchWG.Done()
		reconcile := func() {
			rows, err := s.publicationRepo.ListAll(ctx)
			if err != nil {
				if ctx.Err() == nil {
					s.logger.WarnContext(ctx, "dashboard publication stream reconciliation failed", "error", err)
				}
				return
			}
			active := make(map[string]publicationStreamVersion, len(rows))
			for _, row := range rows {
				if row.Status() != publication.StatusActive {
					continue
				}
				active[row.ID] = publicationStreamVersion{PublicID: row.PublicID, ServingStateID: row.ServingStateID}
			}
			s.publicationStreams.CloseStale(active)
		}
		reconcile()
		ticker := time.NewTicker(publicationMonitorInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				reconcile()
			}
		}
	}()
}
