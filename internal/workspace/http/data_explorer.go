package http

import (
	nethttp "net/http"
	"net/url"
	"strings"

	lddatastar "github.com/Yacobolo/libredash/internal/dashboard/datastar"
	"github.com/Yacobolo/libredash/internal/ui"
	uisignals "github.com/Yacobolo/libredash/internal/ui/signals"
	"github.com/go-chi/chi/v5"
	"github.com/starfederation/datastar-go/datastar"
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
	clientID := lddatastar.EnsureClientID(w, r)
	sse := datastar.NewSSE(w, r)
	updates, unsubscribe := h.broker().Subscribe(dataExplorerStreamID(clientID))
	defer unsubscribe()
	for {
		select {
		case <-r.Context().Done():
			return
		case patch := <-updates:
			if err := sse.MarshalAndPatchSignals(patch); err != nil {
				return
			}
		}
	}
}

func (h Handler) DataExplorerCommand(w nethttp.ResponseWriter, r *nethttp.Request) {
	clientID := lddatastar.EnsureClientID(w, r)
	signals := dataExplorerCommandSignals{}
	if err := datastar.ReadSignals(r, &signals); err != nil {
		nethttp.Error(w, err.Error(), nethttp.StatusBadRequest)
		return
	}
	if explorer, ok := dataExplorerResizeOnlyPatch(signals.DataExplorer, signals.DataExplorerCommand); ok {
		h.broker().Publish(dataExplorerStreamID(clientID), map[string]any{
			"dataExplorer":        explorer,
			"dataExplorerCommand": explorer.Command,
		})
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
	h.broker().Publish(dataExplorerStreamID(clientID), map[string]any{
		"dataExplorer":        explorer,
		"dataExplorerCommand": explorer.Command,
	})
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
		WorkspaceID: strings.TrimSpace(workspaceID),
		ObjectKey:   strings.TrimSpace(object),
		Limit:       dataExplorerDefaultLimit,
		Count:       dataExplorerDefaultLimit,
		Block:       "all",
	})
}

func DataExplorerCommandFromQuery(workspaceID, object string) uisignals.DataExplorerCommand {
	return dataExplorerCommandFromQuery(workspaceID, object)
}

func normalizeDataExplorerCommand(command uisignals.DataExplorerCommand) uisignals.DataExplorerCommand {
	command.WorkspaceID = strings.TrimSpace(command.WorkspaceID)
	command.ObjectKey = strings.TrimSpace(command.ObjectKey)
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
	if command.Block != "a" && command.Block != "b" && command.Block != "c" && command.Block != "all" {
		command.Block = "all"
	}
	if command.Sort.Direction != "asc" && command.Sort.Direction != "desc" {
		command.Sort.Direction = ""
	}
	if strings.TrimSpace(command.Sort.Column) == "" {
		command.Sort = uisignals.DataPreviewSortSignal{}
	}
	command.ColumnWidths = normalizeDataExplorerColumnWidths(command.ColumnWidths)
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
	if len(nextCommand.ColumnWidths) == 0 || current.SelectedObject == nil {
		return uisignals.DataExplorerSignal{}, false
	}
	next := normalizeDataExplorerCommand(nextCommand)
	previous := normalizeDataExplorerCommand(current.Command)
	if !dataExplorerCommandsEqualExceptColumnWidths(previous, next) {
		return uisignals.DataExplorerSignal{}, false
	}
	current.Command = next
	current.SelectedWorkspaceID = firstNonEmpty(current.SelectedWorkspaceID, next.WorkspaceID)
	current.SelectedKey = firstNonEmpty(current.SelectedKey, next.ObjectKey)
	return current, true
}

func dataExplorerCommandsEqualExceptColumnWidths(left, right uisignals.DataExplorerCommand) bool {
	return left.WorkspaceID == right.WorkspaceID &&
		left.ObjectKey == right.ObjectKey &&
		left.Offset == right.Offset &&
		left.Limit == right.Limit &&
		left.Block == right.Block &&
		left.Start == right.Start &&
		left.Count == right.Count &&
		left.RequestSeq == right.RequestSeq &&
		left.ResetVersion == right.ResetVersion &&
		left.Sort == right.Sort &&
		stringSlicesEqual(left.VisibleColumns, right.VisibleColumns)
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
