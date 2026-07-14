package pagestream

import (
	"reflect"
	"sync"
	"time"
)

// SignalPatch is a Datastar signal patch. pagestream intentionally streams
// signal patches only; it does not transport element morphs or scripts.
type SignalPatch map[string]any

// Broker fans patches out to every subscriber of a stream. Each subscriber
// owns a coalescing mailbox: replace-mode signals retain their newest value,
// while independently completed component maps are merged. Publishing never
// waits for a slow SSE connection and current-generation component updates are
// never discarded.
type Broker struct {
	mu      sync.Mutex
	clients map[string]map[*brokerSubscription]struct{}
}

type brokerSubscription struct {
	mu         sync.Mutex
	pending    []pendingSignalPatch
	generation uint64
	closed     bool
	out        chan SignalPatch
	wake       chan struct{}
	done       chan struct{}
	once       sync.Once
}

type pendingSignalPatch struct {
	patch      SignalPatch
	generation uint64
}

var mergeSignalKeys = map[string]struct{}{
	"componentStatus": {},
	"filterOptions":   {},
	"tables":          {},
	"visuals":         {},
}

func NewBroker() *Broker {
	return &Broker{clients: map[string]map[*brokerSubscription]struct{}{}}
}

func (b *Broker) Subscribe(streamID string) (<-chan SignalPatch, func()) {
	subscription := &brokerSubscription{
		out:  make(chan SignalPatch, 1),
		wake: make(chan struct{}, 1),
		done: make(chan struct{}),
	}

	b.mu.Lock()
	if b.clients[streamID] == nil {
		b.clients[streamID] = map[*brokerSubscription]struct{}{}
	}
	b.clients[streamID][subscription] = struct{}{}
	b.mu.Unlock()

	go subscription.forward()
	return subscription.out, func() {
		subscription.once.Do(func() {
			b.mu.Lock()
			delete(b.clients[streamID], subscription)
			if len(b.clients[streamID]) == 0 {
				delete(b.clients, streamID)
			}
			b.mu.Unlock()
			subscription.close()
		})
	}
}

func (b *Broker) Publish(streamID string, patch SignalPatch) {
	if len(patch) == 0 {
		return
	}
	b.mu.Lock()
	subscriptions := make([]*brokerSubscription, 0, len(b.clients[streamID]))
	for subscription := range b.clients[streamID] {
		subscriptions = append(subscriptions, subscription)
	}
	b.mu.Unlock()

	for _, subscription := range subscriptions {
		subscription.enqueue(patch)
	}
}

func (b *Broker) SubscriberCount(streamID string) int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.clients[streamID])
}

func (s *brokerSubscription) enqueue(patch SignalPatch) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	generation, explicitGeneration := patchGeneration(patch)
	if explicitGeneration && generation < s.generation {
		s.mu.Unlock()
		return
	}
	if explicitGeneration && generation > s.generation {
		s.generation = generation
		// A queued or output-buffered patch belongs to an older generation.
		// Evict it atomically before the new start becomes observable so a
		// slow subscriber can never receive stale component data afterward.
		kept := s.pending[:0]
		for _, pending := range s.pending {
			if pending.generation >= generation {
				kept = append(kept, pending)
			}
		}
		s.pending = kept
		select {
		case <-s.out:
		default:
		}
	}
	if !explicitGeneration {
		generation = s.generation
	}
	next := pendingSignalPatch{
		patch:      coalesceSignalPatches(nil, patch),
		generation: generation,
	}
	if len(s.pending) == 0 {
		s.pending = append(s.pending, next)
	} else if preservesProgressBoundary(s.pending[len(s.pending)-1].patch, patch) {
		if len(s.pending) == 2 {
			// Keep the bounded mailbox focused on the newest observable
			// start/complete pair when generations overtake a slow client.
			s.pending = s.pending[1:]
		}
		s.pending = append(s.pending, next)
	} else {
		last := len(s.pending) - 1
		s.pending[last].patch = coalesceSignalPatches(s.pending[last].patch, patch)
	}
	s.mu.Unlock()
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

func (s *brokerSubscription) forward() {
	defer close(s.out)
	for {
		s.mu.Lock()
		pending := len(s.pending) > 0
		sent := false
		if pending {
			select {
			case s.out <- s.pending[0].patch:
				s.pending = s.pending[1:]
				sent = true
			default:
			}
		}
		s.mu.Unlock()
		if sent {
			continue
		}
		if !pending {
			select {
			case <-s.done:
				return
			case <-s.wake:
			}
			continue
		}
		// The subscriber output is full. Retry only while backpressured;
		// enqueue remains non-blocking and can atomically supersede this data.
		timer := time.NewTimer(time.Millisecond)
		select {
		case <-s.done:
			if !timer.Stop() {
				<-timer.C
			}
			return
		case <-s.wake:
			if !timer.Stop() {
				<-timer.C
			}
		case <-timer.C:
		}
	}
}

func patchGeneration(patch SignalPatch) (uint64, bool) {
	if generation, ok := valueGeneration(reflect.ValueOf(patch["status"])); ok {
		return generation, true
	}
	statuses := reflect.ValueOf(patch["componentStatus"])
	for statuses.IsValid() && (statuses.Kind() == reflect.Pointer || statuses.Kind() == reflect.Interface) {
		if statuses.IsNil() {
			return 0, false
		}
		statuses = statuses.Elem()
	}
	if !statuses.IsValid() || statuses.Kind() != reflect.Map {
		return 0, false
	}
	iterator := statuses.MapRange()
	var newest uint64
	found := false
	for iterator.Next() {
		if generation, ok := valueGeneration(iterator.Value()); ok && (!found || generation > newest) {
			newest = generation
			found = true
		}
	}
	return newest, found
}

func valueGeneration(value reflect.Value) (uint64, bool) {
	for value.IsValid() && (value.Kind() == reflect.Pointer || value.Kind() == reflect.Interface) {
		if value.IsNil() {
			return 0, false
		}
		value = value.Elem()
	}
	if !value.IsValid() {
		return 0, false
	}
	var generation reflect.Value
	switch value.Kind() {
	case reflect.Map:
		if value.Type().Key().Kind() != reflect.String {
			return 0, false
		}
		generation = value.MapIndex(reflect.ValueOf("generation").Convert(value.Type().Key()))
	case reflect.Struct:
		generation = value.FieldByName("Generation")
	default:
		return 0, false
	}
	for generation.IsValid() && (generation.Kind() == reflect.Pointer || generation.Kind() == reflect.Interface) {
		if generation.IsNil() {
			return 0, false
		}
		generation = generation.Elem()
	}
	if !generation.IsValid() {
		return 0, false
	}
	switch generation.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if generation.Int() < 0 {
			return 0, false
		}
		return uint64(generation.Int()), true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return generation.Uint(), true
	default:
		return 0, false
	}
}

func preservesLoadingBoundary(current, next SignalPatch) bool {
	currentLoading, currentOK := patchLoading(current)
	nextLoading, nextOK := patchLoading(next)
	return currentOK && nextOK && currentLoading && !nextLoading
}

func preservesProgressBoundary(current, next SignalPatch) bool {
	if preservesLoadingBoundary(current, next) {
		return true
	}
	currentRunning, currentOK := patchRunning(current)
	nextRunning, nextOK := patchRunning(next)
	return currentOK && nextOK && currentRunning && !nextRunning
}

func patchRunning(patch SignalPatch) (bool, bool) {
	if running, ok := nestedBool(reflect.ValueOf(patch["page"]), "refresh", "running"); ok {
		return running, true
	}
	return nestedBool(reflect.ValueOf(patch["assetRefresh"]), "running")
}

func nestedBool(value reflect.Value, path ...string) (bool, bool) {
	for _, part := range path {
		for value.IsValid() && (value.Kind() == reflect.Pointer || value.Kind() == reflect.Interface) {
			if value.IsNil() {
				return false, false
			}
			value = value.Elem()
		}
		if !value.IsValid() {
			return false, false
		}
		switch value.Kind() {
		case reflect.Map:
			if value.Type().Key().Kind() != reflect.String {
				return false, false
			}
			value = value.MapIndex(reflect.ValueOf(part).Convert(value.Type().Key()))
		case reflect.Struct:
			field := part
			if field != "" && field[0] >= 'a' && field[0] <= 'z' {
				field = string(field[0]-'a'+'A') + field[1:]
			}
			value = value.FieldByName(field)
		default:
			return false, false
		}
	}
	for value.IsValid() && (value.Kind() == reflect.Pointer || value.Kind() == reflect.Interface) {
		if value.IsNil() {
			return false, false
		}
		value = value.Elem()
	}
	if !value.IsValid() || value.Kind() != reflect.Bool {
		return false, false
	}
	return value.Bool(), true
}

func patchLoading(patch SignalPatch) (bool, bool) {
	status, ok := patch["status"]
	if !ok || status == nil {
		return false, false
	}
	value := reflect.ValueOf(status)
	for value.IsValid() && (value.Kind() == reflect.Pointer || value.Kind() == reflect.Interface) {
		if value.IsNil() {
			return false, false
		}
		value = value.Elem()
	}
	switch value.Kind() {
	case reflect.Map:
		if value.Type().Key().Kind() != reflect.String {
			return false, false
		}
		loading := value.MapIndex(reflect.ValueOf("loading").Convert(value.Type().Key()))
		for loading.IsValid() && (loading.Kind() == reflect.Interface || loading.Kind() == reflect.Pointer) {
			if loading.IsNil() {
				return false, false
			}
			loading = loading.Elem()
		}
		if loading.IsValid() && loading.Kind() == reflect.Bool {
			return loading.Bool(), true
		}
	case reflect.Struct:
		loading := value.FieldByName("Loading")
		if loading.IsValid() && loading.Kind() == reflect.Bool {
			return loading.Bool(), true
		}
	}
	return false, false
}

func (s *brokerSubscription) close() {
	s.mu.Lock()
	s.closed = true
	s.pending = nil
	s.mu.Unlock()
	close(s.done)
}

func coalesceSignalPatches(current, next SignalPatch) SignalPatch {
	result := make(SignalPatch, len(current)+len(next))
	for key, value := range current {
		result[key] = value
	}
	for key, value := range next {
		if _, merge := mergeSignalKeys[key]; merge {
			if combined, ok := mergeStringMaps(result[key], value); ok {
				result[key] = combined
				continue
			}
		}
		result[key] = value
	}
	return result
}

// mergeStringMaps preserves the concrete map type used by signal contracts
// (for example map[string]dashboard.Visual) while merging target entries.
func mergeStringMaps(current, next any) (any, bool) {
	nextValue := reflect.ValueOf(next)
	if !nextValue.IsValid() || nextValue.Kind() != reflect.Map || nextValue.Type().Key().Kind() != reflect.String {
		return nil, false
	}
	currentValue := reflect.ValueOf(current)
	if current == nil {
		currentValue = reflect.MakeMap(nextValue.Type())
	}
	if !currentValue.IsValid() || currentValue.Kind() != reflect.Map || currentValue.Type() != nextValue.Type() {
		return nil, false
	}
	merged := reflect.MakeMapWithSize(nextValue.Type(), currentValue.Len()+nextValue.Len())
	iterator := currentValue.MapRange()
	for iterator.Next() {
		merged.SetMapIndex(iterator.Key(), iterator.Value())
	}
	iterator = nextValue.MapRange()
	for iterator.Next() {
		merged.SetMapIndex(iterator.Key(), iterator.Value())
	}
	return merged.Interface(), true
}
