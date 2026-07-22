package http

import (
	"database/sql"
	nethttp "net/http"
	"strings"

	"github.com/Yacobolo/leapview/internal/dashboard"
	"github.com/Yacobolo/leapview/internal/queryaudit"
	"github.com/Yacobolo/leapview/internal/ui"
	"github.com/Yacobolo/leapview/pkg/pagestream"
	"github.com/go-chi/chi/v5"
)

type QueryAuditRepositoryProvider func() (queryaudit.Repository, error)

type Handler struct {
	Catalog          func() dashboard.Catalog
	ReadModel        ReadModel
	CurrentRoleLabel func(*nethttp.Request) string
	ChromeOption     func(*nethttp.Request) ui.ChromeOption
	EnsureClientID   func(nethttp.ResponseWriter, *nethttp.Request)
	Broker           *pagestream.Broker
}

type storageCommandSignals struct {
	AdminStorageCommand ui.AdminStorageCommand `json:"adminStorageCommand"`
}

func (h Handler) General(w nethttp.ResponseWriter, r *nethttp.Request) {
	h.renderPage(w, r, "general")
}

func (h Handler) Principals(w nethttp.ResponseWriter, r *nethttp.Request) {
	h.renderPage(w, r, "principals")
}

func (h Handler) PrincipalDetail(w nethttp.ResponseWriter, r *nethttp.Request) {
	data, err := h.adminData(r)
	if err != nil {
		nethttp.Error(w, err.Error(), nethttp.StatusInternalServerError)
		return
	}
	principalID := chi.URLParam(r, "principal")
	for i := range data.Principals {
		if data.Principals[i].ID == principalID {
			data.SelectedPrincipal = &data.Principals[i]
			h.writePage(w, r, "principal-detail", data)
			return
		}
	}
	nethttp.NotFound(w, r)
}

func (h Handler) Groups(w nethttp.ResponseWriter, r *nethttp.Request) {
	h.renderPage(w, r, "groups")
}

func (h Handler) GroupDetail(w nethttp.ResponseWriter, r *nethttp.Request) {
	data, err := h.adminData(r)
	if err != nil {
		nethttp.Error(w, err.Error(), nethttp.StatusInternalServerError)
		return
	}
	groupID := chi.URLParam(r, "group")
	for i := range data.Groups {
		if data.Groups[i].ID == groupID {
			data.SelectedGroup = &data.Groups[i]
			h.writePage(w, r, "group-detail", data)
			return
		}
	}
	nethttp.NotFound(w, r)
}

func (h Handler) Agent(w nethttp.ResponseWriter, r *nethttp.Request) {
	h.renderPage(w, r, "agent")
}

func (h Handler) Storage(w nethttp.ResponseWriter, r *nethttp.Request) {
	h.ensureClientID(w, r)
	h.renderPage(w, r, "storage")
}

func (h Handler) Queries(w nethttp.ResponseWriter, r *nethttp.Request) {
	h.ensureClientID(w, r)
	h.renderPage(w, r, "queries")
}

func (h Handler) BootstrapUpdates(w nethttp.ResponseWriter, r *nethttp.Request) {
	active := strings.TrimSpace(r.URL.Query().Get("section"))
	if active == "" {
		active = "general"
	}
	data, err := h.adminDataForUpdates(r, active)
	if err != nil {
		if err == sql.ErrNoRows {
			nethttp.NotFound(w, r)
			return
		}
		nethttp.Error(w, err.Error(), nethttp.StatusInternalServerError)
		return
	}
	h.patchAndWait(w, r, ui.AdminBootstrapSignals(h.catalog(), active, h.roleLabel(r), data, h.chromeOption(r)))
}

func (h Handler) QueryUpdates(w nethttp.ResponseWriter, r *nethttp.Request) {
	h.queryHistoryUpdates(w, r)
}

func (h Handler) QueryCommand(w nethttp.ResponseWriter, r *nethttp.Request) {
	h.queryHistoryCommand(w, r)
}

func (h Handler) StorageSignalUpdates(w nethttp.ResponseWriter, r *nethttp.Request) {
	clientID := pagestream.EnsureClientID(w, r)
	if h.Broker == nil {
		nethttp.Error(w, "admin storage broker is not configured", nethttp.StatusInternalServerError)
		return
	}
	streamID := adminStorageStreamID(clientID)
	updates := pagestream.NewSignalStream(w, r, pagestream.WithStreamTrace(h.Broker.TraceStore(), streamID, "admin.storage.bootstrap"))
	data, err := h.adminDataForUpdates(r, "storage")
	if err != nil {
		nethttp.Error(w, err.Error(), nethttp.StatusInternalServerError)
		return
	}
	if err := updates.Patch(ui.AdminBootstrapSignals(h.catalog(), "storage", h.roleLabel(r), data, h.chromeOption(r))); err != nil {
		return
	}
	_ = updates.Forward(r.Context(), h.Broker, streamID)
}

func (h Handler) StorageTableSelect(w nethttp.ResponseWriter, r *nethttp.Request) {
	clientID := pagestream.EnsureClientID(w, r)
	signals := storageCommandSignals{}
	if err := pagestream.ReadSignals(r, &signals); err != nil {
		nethttp.Error(w, err.Error(), nethttp.StatusBadRequest)
		return
	}
	selectedTable, err := h.readModel().StorageService.SelectTable(r.Context(), signals.AdminStorageCommand)
	if err != nil {
		nethttp.Error(w, err.Error(), nethttp.StatusBadRequest)
		return
	}
	if h.Broker == nil {
		nethttp.Error(w, "admin storage broker is not configured", nethttp.StatusInternalServerError)
		return
	}
	h.Broker.Publish(adminStorageStreamID(clientID), map[string]any{
		"adminStorage": map[string]any{
			"selectedKey":   selectedTable.Key,
			"selectedTable": selectedTable,
		},
	})
	w.WriteHeader(nethttp.StatusNoContent)
}

func (h Handler) renderPage(w nethttp.ResponseWriter, r *nethttp.Request, active string) {
	data, err := h.adminData(r)
	if err != nil {
		nethttp.Error(w, err.Error(), nethttp.StatusInternalServerError)
		return
	}
	h.writePage(w, r, active, data)
}

func (h Handler) writePage(w nethttp.ResponseWriter, r *nethttp.Request, active string, data ui.AdminData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(nethttp.StatusOK)
	if err := ui.AdminPage(h.catalog(), active, h.roleLabel(r), data, h.chromeOption(r)).Render(w); err != nil {
		nethttp.Error(w, err.Error(), nethttp.StatusInternalServerError)
	}
}

func (h Handler) adminData(r *nethttp.Request) (ui.AdminData, error) {
	return h.readModel().Data(r)
}

func (h Handler) adminDataForUpdates(r *nethttp.Request, active string) (ui.AdminData, error) {
	data, err := h.adminData(r)
	if err != nil {
		return data, err
	}
	switch active {
	case "principal-detail":
		principalID := strings.TrimSpace(r.URL.Query().Get("principal"))
		for i := range data.Principals {
			if data.Principals[i].ID == principalID {
				data.SelectedPrincipal = &data.Principals[i]
				return data, nil
			}
		}
		return data, sql.ErrNoRows
	case "group-detail":
		groupID := strings.TrimSpace(r.URL.Query().Get("group"))
		for i := range data.Groups {
			if data.Groups[i].ID == groupID {
				data.SelectedGroup = &data.Groups[i]
				return data, nil
			}
		}
		return data, sql.ErrNoRows
	default:
		return data, nil
	}
}

func (h Handler) patchAndWait(w nethttp.ResponseWriter, r *nethttp.Request, patch pagestream.SignalPatch) {
	clientID := pagestream.EnsureClientID(w, r)
	var trace *pagestream.TraceStore
	if h.Broker != nil {
		trace = h.Broker.TraceStore()
	}
	updates := pagestream.NewSignalStream(w, r, pagestream.WithStreamTrace(trace, "admin:"+clientID, "admin.bootstrap"))
	if err := updates.Patch(patch); err != nil {
		return
	}
	updates.Wait(r.Context())
}

func (h Handler) catalog() dashboard.Catalog {
	if h.Catalog == nil {
		return dashboard.Catalog{}
	}
	return h.Catalog()
}

func (h Handler) roleLabel(r *nethttp.Request) string {
	if h.CurrentRoleLabel == nil {
		return ""
	}
	return h.CurrentRoleLabel(r)
}

func (h Handler) chromeOption(r *nethttp.Request) ui.ChromeOption {
	if h.ChromeOption == nil {
		return nil
	}
	return h.ChromeOption(r)
}

func (h Handler) ensureClientID(w nethttp.ResponseWriter, r *nethttp.Request) {
	if h.EnsureClientID != nil {
		h.EnsureClientID(w, r)
		return
	}
	_ = pagestream.EnsureClientID(w, r)
}

func (h Handler) readModel() ReadModel {
	return h.ReadModel
}

func adminStorageStreamID(clientID string) string {
	if strings.TrimSpace(clientID) == "" {
		clientID = "default"
	}
	return "admin-storage:" + clientID
}
