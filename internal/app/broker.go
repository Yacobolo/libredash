package app

import "sync"

type signalPatch map[string]any

type broker struct {
	mu      sync.Mutex
	clients map[string]map[chan signalPatch]struct{}
}

func newBroker() *broker {
	return &broker{clients: map[string]map[chan signalPatch]struct{}{}}
}

func (b *broker) subscribe(clientID string) (<-chan signalPatch, func()) {
	ch := make(chan signalPatch, 8)

	b.mu.Lock()
	if b.clients[clientID] == nil {
		b.clients[clientID] = map[chan signalPatch]struct{}{}
	}
	b.clients[clientID][ch] = struct{}{}
	b.mu.Unlock()

	return ch, func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		delete(b.clients[clientID], ch)
		if len(b.clients[clientID]) == 0 {
			delete(b.clients, clientID)
		}
		close(ch)
	}
}

func (b *broker) publish(clientID string, patch signalPatch) {
	b.mu.Lock()
	defer b.mu.Unlock()

	for ch := range b.clients[clientID] {
		select {
		case ch <- patch:
		default:
		}
	}
}
