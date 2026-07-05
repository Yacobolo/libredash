package http

import (
	"fmt"
	nethttp "net/http"
	"strings"

	adminstorage "github.com/Yacobolo/libredash/internal/admin/storage"
	"github.com/Yacobolo/libredash/internal/dashboard"
	lddatastar "github.com/Yacobolo/libredash/internal/dashboard/datastar"
	"github.com/Yacobolo/libredash/internal/dashboard/stream"
	"github.com/Yacobolo/libredash/internal/queryaudit"
	"github.com/Yacobolo/libredash/internal/ui"
	"github.com/go-chi/chi/v5"
	"github.com/starfederation/datastar-go/datastar"
)

type Broker interface {
	Subscribe(string) (<-chan stream.Patch, func())
	Publish(string, stream.Patch)
}

type QueryAuditRepositoryProvider func() (queryaudit.Repository, error)
type PrincipalLabelsProvider func(*nethttp.Request, []string) map[string]string

type Handler struct {
	Catalog              func() dashboard.Catalog
	Data                 func(*nethttp.Request) (ui.AdminData, error)
	CurrentRoleLabel     func(*nethttp.Request) string
	ChromeOption         func(*nethttp.Request) ui.ChromeOption
	EnsureClientID       func(nethttp.ResponseWriter, *nethttp.Request)
	Broker               Broker
	StorageService       adminstorage.Service
	QueryAuditRepository QueryAuditRepositoryProvider
	PrincipalLabels      PrincipalLabelsProvider
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

func (h Handler) QueryUpdates(w nethttp.ResponseWriter, r *nethttp.Request) {
	h.queryHistoryUpdates(w, r)
}

func (h Handler) QueryCommand(w nethttp.ResponseWriter, r *nethttp.Request) {
	h.queryHistoryCommand(w, r)
}

func (h Handler) StorageSignalUpdates(w nethttp.ResponseWriter, r *nethttp.Request) {
	clientID := lddatastar.EnsureClientID(w, r)
	if h.Broker == nil {
		nethttp.Error(w, "admin storage broker is not configured", nethttp.StatusInternalServerError)
		return
	}
	sse := datastar.NewSSE(w, r)
	updates, unsubscribe := h.Broker.Subscribe(adminStorageStreamID(clientID))
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

func (h Handler) StorageTableSelect(w nethttp.ResponseWriter, r *nethttp.Request) {
	clientID := lddatastar.EnsureClientID(w, r)
	signals := storageCommandSignals{}
	if err := datastar.ReadSignals(r, &signals); err != nil {
		nethttp.Error(w, err.Error(), nethttp.StatusBadRequest)
		return
	}
	selectedTable, err := h.StorageService.SelectTable(r.Context(), signals.AdminStorageCommand)
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
	if h.Data == nil {
		return ui.AdminData{}, fmt.Errorf("admin data provider is not configured")
	}
	return h.Data(r)
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
	}
}

func (h Handler) queryAuditRepository() (queryaudit.Repository, error) {
	if h.QueryAuditRepository == nil {
		return nil, fmt.Errorf("query audit repository is not configured")
	}
	return h.QueryAuditRepository()
}

func (h Handler) queryPrincipalLabels(r *nethttp.Request, values []string) map[string]string {
	if h.PrincipalLabels == nil {
		labels := map[string]string{}
		for _, value := range values {
			value = strings.TrimSpace(value)
			if value != "" {
				labels[value] = value
			}
		}
		return labels
	}
	return h.PrincipalLabels(r, values)
}

func adminStorageStreamID(clientID string) string {
	if strings.TrimSpace(clientID) == "" {
		clientID = "default"
	}
	return "admin-storage:" + clientID
}
