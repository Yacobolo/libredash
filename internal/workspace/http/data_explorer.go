package http

import (
	nethttp "net/http"
	"net/url"
	"strings"

	"github.com/Yacobolo/leapview/internal/ui"
	uisignals "github.com/Yacobolo/leapview/internal/ui/signals"
	"github.com/Yacobolo/leapview/pkg/pagestream"
	"github.com/go-chi/chi/v5"
)

const (
	dataExplorerDefaultLimit = 100
	dataExplorerMaxLimit     = 1000
	dataExplorerRowHeight    = 34
)

const (
	DataExplorerDefaultLimit = dataExplorerDefaultLimit
	DataExplorerRowHeight    = dataExplorerRowHeight
)

var dataExplorerBlockIDs = []string{"a", "b", "c"}

type dataExplorerCommandSignals struct {
	DataExplorerCommand uisignals.DataExplorerCommand `json:"dataExplorerCommand"`
	DataExplorer        uisignals.DataExplorerSignal  `json:"dataExplorer"`
}

func (h Handler) DataExplorer(w nethttp.ResponseWriter, r *nethttp.Request) {
	page, explorer, err := h.globalDataExplorerState(r, dataExplorerCommandFromQuery(r.URL.Query().Get("workspace"), r.URL.Query().Get("object")))
	if err != nil {
		nethttp.Error(w, err.Error(), statusForNotFound(err))
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(nethttp.StatusOK)
	if err := ui.DataExplorerPage(h.catalogForWorkspacesPage(r, nil), page, explorer, h.currentRoleLabel(r), h.csrfToken(r), h.chromeOptions(r)...).Render(w); err != nil {
		nethttp.Error(w, err.Error(), nethttp.StatusInternalServerError)
	}
}

func (h Handler) WorkspaceDataExplorerRedirect(w nethttp.ResponseWriter, r *nethttp.Request) {
	values := url.Values{}
	for key, entries := range r.URL.Query() {
		for _, entry := range entries {
			values.Add(key, entry)
		}
	}
	values.Set("workspace", h.workspaceID(chi.URLParam(r, "workspace")))
	target := "/data"
	if encoded := values.Encode(); encoded != "" {
		target += "?" + encoded
	}
	nethttp.Redirect(w, r, target, nethttp.StatusFound)
}

func (h Handler) DataExplorerUpdates(w nethttp.ResponseWriter, r *nethttp.Request) {
	clientID := pagestream.EnsureClientID(w, r)
	streamID := dataExplorerStreamID(clientID)
	broker := h.broker()
	var trace *pagestream.TraceStore
	if broker != nil {
		trace = broker.TraceStore()
	}
	updates := pagestream.NewSignalStream(w, r, pagestream.WithStreamTrace(trace, streamID, "data-explorer.bootstrap"))
	page, explorer, err := h.globalDataExplorerState(r, dataExplorerCommandFromQuery(r.URL.Query().Get("workspace"), r.URL.Query().Get("object")))
	if err != nil {
		nethttp.Error(w, err.Error(), statusForNotFound(err))
		return
	}
	if err := updates.Patch(ui.DataExplorerBootstrapSignals(h.catalogForWorkspacesPage(r, nil), page, explorer, h.currentRoleLabel(r), h.chromeOptions(r)...)); err != nil {
		return
	}
	if broker != nil {
		_ = updates.Forward(r.Context(), broker, streamID)
		return
	}
	updates.Wait(r.Context())
}

func (h Handler) DataExplorerCommand(w nethttp.ResponseWriter, r *nethttp.Request) {
	clientID := pagestream.EnsureClientID(w, r)
	signals := dataExplorerCommandSignals{}
	if err := pagestream.ReadSignals(r, &signals); err != nil {
		nethttp.Error(w, err.Error(), nethttp.StatusBadRequest)
		return
	}
	if explorer, ok := dataExplorerResizeOnlyPatch(signals.DataExplorer, signals.DataExplorerCommand); ok {
		if broker := h.broker(); broker != nil {
			broker.Publish(dataExplorerStreamID(clientID), pagestream.SignalPatch{
				"dataExplorer":        explorer,
				"dataExplorerCommand": explorer.Command,
			})
		}
		w.WriteHeader(nethttp.StatusNoContent)
		return
	}
	_, explorer, err := h.globalDataExplorerStateWithCurrent(r, signals.DataExplorerCommand, &signals.DataExplorer)
	if err != nil {
		nethttp.Error(w, err.Error(), statusForNotFound(err))
		return
	}
	if dataPreviewCanceled(explorer.Preview) {
		w.WriteHeader(nethttp.StatusNoContent)
		return
	}
	if broker := h.broker(); broker != nil {
		broker.Publish(dataExplorerStreamID(clientID), pagestream.SignalPatch{
			"dataExplorer":        explorer,
			"dataExplorerCommand": explorer.Command,
		})
	}
	w.WriteHeader(nethttp.StatusNoContent)
}

func dataExplorerStreamID(clientID string) string {
	if strings.TrimSpace(clientID) == "" {
		clientID = "default"
	}
	return "data-explorer:" + clientID
}

func dataExplorerCommandFromQuery(workspaceID, object string) uisignals.DataExplorerCommand {
	return normalizeDataExplorerCommand(uisignals.DataExplorerCommand{
		WorkspaceID: uisignals.Optional(strings.TrimSpace(workspaceID)),
		ObjectKey:   uisignals.Optional(strings.TrimSpace(object)),
		Limit:       dataExplorerDefaultLimit,
		Count:       dataExplorerDefaultLimit,
		Block:       uisignals.Pointer("all"),
	})
}

func DataExplorerCommandFromQuery(workspaceID, object string) uisignals.DataExplorerCommand {
	return dataExplorerCommandFromQuery(workspaceID, object)
}

func normalizeDataExplorerCommand(command uisignals.DataExplorerCommand) uisignals.DataExplorerCommand {
	command.WorkspaceID = uisignals.Optional(strings.TrimSpace(uisignals.ValueOrZero(command.WorkspaceID)))
	command.ObjectKey = uisignals.Optional(strings.TrimSpace(uisignals.ValueOrZero(command.ObjectKey)))
	if command.Limit <= 0 {
		command.Limit = dataExplorerDefaultLimit
	}
	if command.Limit > dataExplorerMaxLimit {
		command.Limit = dataExplorerMaxLimit
	}
	if command.Count <= 0 {
		command.Count = command.Limit
	}
	if command.Count > dataExplorerMaxLimit {
		command.Count = dataExplorerMaxLimit
	}
	if command.Offset < 0 {
		command.Offset = 0
	}
	if command.Start < 0 {
		command.Start = 0
	}
	if command.Start == 0 && command.Offset > 0 {
		command.Start = command.Offset
	}
	block := uisignals.ValueOrZero(command.Block)
	if block != "a" && block != "b" && block != "c" && block != "all" {
		command.Block = uisignals.Pointer("all")
	}
	direction := uisignals.ValueOrZero(command.Sort.Direction)
	if direction != "asc" && direction != "desc" {
		command.Sort.Direction = nil
	}
	if strings.TrimSpace(uisignals.ValueOrZero(command.Sort.Column)) == "" {
		command.Sort = uisignals.DataPreviewSortSignal{}
	}
	columnWidths := normalizeDataExplorerColumnWidths(uisignals.ValueOrZero(command.ColumnWidths))
	command.ColumnWidths = nil
	if len(columnWidths) > 0 {
		command.ColumnWidths = &columnWidths
	}
	return command
}

func normalizeDataExplorerColumnWidths(widths map[string]float64) map[string]float64 {
	if len(widths) == 0 {
		return nil
	}
	out := map[string]float64{}
	for key, width := range widths {
		key = strings.TrimSpace(key)
		if key == "" || width <= 0 {
			continue
		}
		out[key] = width
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func dataExplorerResizeOnlyPatch(current uisignals.DataExplorerSignal, nextCommand uisignals.DataExplorerCommand) (uisignals.DataExplorerSignal, bool) {
	if len(uisignals.ValueOrZero(nextCommand.ColumnWidths)) == 0 || current.SelectedObject == nil {
		return uisignals.DataExplorerSignal{}, false
	}
	next := normalizeDataExplorerCommand(nextCommand)
	previous := normalizeDataExplorerCommand(current.Command)
	if !dataExplorerCommandsEqualExceptColumnWidths(previous, next) {
		return uisignals.DataExplorerSignal{}, false
	}
	current.Command = next
	current.SelectedWorkspaceID = uisignals.Optional(firstNonEmpty(uisignals.ValueOrZero(current.SelectedWorkspaceID), uisignals.ValueOrZero(next.WorkspaceID)))
	current.SelectedKey = uisignals.Optional(firstNonEmpty(uisignals.ValueOrZero(current.SelectedKey), uisignals.ValueOrZero(next.ObjectKey)))
	return current, true
}

func dataExplorerCommandsEqualExceptColumnWidths(left, right uisignals.DataExplorerCommand) bool {
	return uisignals.ValueOrZero(left.WorkspaceID) == uisignals.ValueOrZero(right.WorkspaceID) &&
		uisignals.ValueOrZero(left.ObjectKey) == uisignals.ValueOrZero(right.ObjectKey) &&
		left.Offset == right.Offset &&
		left.Limit == right.Limit &&
		uisignals.ValueOrZero(left.Block) == uisignals.ValueOrZero(right.Block) &&
		left.Start == right.Start &&
		left.Count == right.Count &&
		left.RequestSeq == right.RequestSeq &&
		left.ResetVersion == right.ResetVersion &&
		uisignals.ValueOrZero(left.Sort.Column) == uisignals.ValueOrZero(right.Sort.Column) &&
		uisignals.ValueOrZero(left.Sort.Direction) == uisignals.ValueOrZero(right.Sort.Direction) &&
		stringSlicesEqual(uisignals.ValueOrZero(left.VisibleColumns), uisignals.ValueOrZero(right.VisibleColumns))
}

func stringSlicesEqual(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
