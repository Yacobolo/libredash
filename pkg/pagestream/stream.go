package pagestream

import (
	"context"
	"errors"
	"net/http"

	"github.com/starfederation/datastar-go/datastar"
)

var errMissingForwardTarget = errors.New("pagestream: broker and streamID are required")

// SignalStream is one long-lived Datastar SSE response that emits signal
// patches.
type SignalStream struct {
	sse *datastar.ServerSentEventGenerator
}

// NewSignalStream opens a Datastar SSE signal stream for the request.
func NewSignalStream(w http.ResponseWriter, r *http.Request) SignalStream {
	return SignalStream{sse: datastar.NewSSE(w, r)}
}

// Redirect emits a Datastar redirect response for short-lived command handlers.
// Long-lived update streams should use SignalStream and Patch only.
func Redirect(w http.ResponseWriter, r *http.Request, location string) error {
	return datastar.NewSSE(w, r).Redirect(location)
}

// PatchResponse emits a single Datastar patch-signals response.
func PatchResponse(w http.ResponseWriter, r *http.Request, patch SignalPatch) error {
	return NewSignalStream(w, r).Patch(patch)
}

// Patch emits one Datastar patch-signals event. Empty patches are ignored.
func (s SignalStream) Patch(patch SignalPatch) error {
	if len(patch) == 0 {
		return nil
	}
	return s.sse.MarshalAndPatchSignals(patch)
}

// Forward relays signal patches published to streamID until ctx is canceled.
func (s SignalStream) Forward(ctx context.Context, broker *Broker, streamID string) error {
	if broker == nil || streamID == "" {
		return errMissingForwardTarget
	}
	updates, unsubscribe := broker.Subscribe(streamID)
	defer unsubscribe()
	for {
		select {
		case <-ctx.Done():
			return nil
		case patch, ok := <-updates:
			if !ok {
				return nil
			}
			if err := s.Patch(patch); err != nil {
				return err
			}
		}
	}
}

// Wait keeps a no-op stream open until ctx is canceled.
func (s SignalStream) Wait(ctx context.Context) {
	<-ctx.Done()
}
