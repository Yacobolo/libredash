package pagestream

import (
	"context"
	"net/http"

	"github.com/starfederation/datastar-go/datastar"
)

type StreamSpec struct {
	Broker         *Broker
	StreamID       string
	InitialPatches []Patch
	Snapshot       func(context.Context) []Patch
}

func ServeStream(w http.ResponseWriter, r *http.Request, spec StreamSpec) {
	sse := datastar.NewSSE(w, r)
	patchAll := func(patches []Patch) bool {
		for _, patch := range patches {
			if len(patch) == 0 {
				continue
			}
			if err := sse.MarshalAndPatchSignals(patch); err != nil {
				return false
			}
		}
		return true
	}
	if !patchAll(spec.InitialPatches) {
		return
	}
	if spec.Snapshot != nil && !patchAll(spec.Snapshot(r.Context())) {
		return
	}
	if spec.Broker == nil || spec.StreamID == "" {
		<-r.Context().Done()
		return
	}
	updates, unsubscribe := spec.Broker.Subscribe(spec.StreamID)
	defer unsubscribe()
	for {
		select {
		case <-r.Context().Done():
			return
		case patch, ok := <-updates:
			if !ok {
				return
			}
			if err := sse.MarshalAndPatchSignals(patch); err != nil {
				return
			}
		}
	}
}
