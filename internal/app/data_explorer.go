package app

import (
	"net/http"
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

var dataExplorerBlockIDs = []string{"a", "b", "c"}

type dataExplorerCommandSignals struct {
	DataExplorerCommand uisignals.DataExplorerCommand `json:"dataExplorerCommand"`
	DataExplorer        uisignals.DataExplorerSignal  `json:"dataExplorer"`
}

func (s *Server) dataExplorer(w http.ResponseWriter, r *http.Request) {
	page, explorer, err := s.globalDataExplorerState(r, dataExplorerCommandFromQuery(r.URL.Query().Get("workspace"), r.URL.Query().Get("object")))
	if err != nil {
		http.Error(w, err.Error(), statusForNotFound(err))
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if err := ui.DataExplorerPage(s.catalogForWorkspacesPage(r, nil), page, explorer, s.currentRoleLabel(r), csrfToken(r, s.auth), s.chatChromeOption(r)).Render(w); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) workspaceDataExplorerRedirect(w http.ResponseWriter, r *http.Request) {
	values := url.Values{}
	for key, entries := range r.URL.Query() {
		for _, entry := range entries {
			values.Add(key, entry)
		}
	}
	values.Set("workspace", s.workspaceID(chi.URLParam(r, "workspace")))
	target := "/data"
	if encoded := values.Encode(); encoded != "" {
		target += "?" + encoded
	}
	http.Redirect(w, r, target, http.StatusFound)
}

func (s *Server) dataExplorerUpdates(w http.ResponseWriter, r *http.Request) {
	clientID := lddatastar.EnsureClientID(w, r)
	sse := datastar.NewSSE(w, r)
	updates, unsubscribe := s.broker.Subscribe(dataExplorerStreamID(clientID))
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

func (s *Server) dataExplorerCommand(w http.ResponseWriter, r *http.Request) {
	clientID := lddatastar.EnsureClientID(w, r)
	signals := dataExplorerCommandSignals{}
	if err := datastar.ReadSignals(r, &signals); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if explorer, ok := dataExplorerResizeOnlyPatch(signals.DataExplorer, signals.DataExplorerCommand); ok {
		s.broker.Publish(dataExplorerStreamID(clientID), map[string]any{
			"dataExplorer":        explorer,
			"dataExplorerCommand": explorer.Command,
		})
		w.WriteHeader(http.StatusNoContent)
		return
	}
	_, explorer, err := s.globalDataExplorerStateWithCurrent(r, signals.DataExplorerCommand, &signals.DataExplorer)
	if err != nil {
		http.Error(w, err.Error(), statusForNotFound(err))
		return
	}
	if dataPreviewCanceled(explorer.Preview) {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	s.broker.Publish(dataExplorerStreamID(clientID), map[string]any{
		"dataExplorer":        explorer,
		"dataExplorerCommand": explorer.Command,
	})
	w.WriteHeader(http.StatusNoContent)
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
