package http

import (
	"context"
	nethttp "net/http"

	lddatastar "github.com/Yacobolo/libredash/internal/dashboard/datastar"
	"github.com/Yacobolo/libredash/internal/dashboard/stream"
	"github.com/Yacobolo/libredash/pkg/pagestream"
)

func (h Handler) Updates(w nethttp.ResponseWriter, r *nethttp.Request) {
	metrics, ok := h.metricsForRequest(r)
	if !ok {
		nethttp.NotFound(w, r)
		return
	}
	signals, ok := h.readSignals(w, r)
	if !ok {
		return
	}
	dashboardID := lddatastar.DashboardID(r, signals, metrics.DefaultDashboardID())
	pageID := lddatastar.PageID(r, signals)
	clientID := lddatastar.ClientStreamID(r, signals, dashboardID, pageID)
	request := stream.SnapshotRequest{
		DashboardID:  dashboardID,
		PageID:       pageID,
		Filters:      signals.Filters,
		TableCommand: signals.TableCommand,
	}

	pagestream.ServeStream(w, r, pagestream.StreamSpec{
		Broker:         h.Broker,
		StreamID:       clientID,
		InitialPatches: []pagestream.Patch{lddatastar.LoadingPatch(metrics.DataDir())},
		Snapshot: func(ctx context.Context) []pagestream.Patch {
			snapshot := stream.Service{Metrics: metrics}.Snapshot(ctx, request)
			return lddatastar.SnapshotPatches(snapshot)
		},
	})
}
