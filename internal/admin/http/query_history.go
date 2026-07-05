package http

import (
	"encoding/base64"
	"net/http"
	"sort"
	"strconv"
	"strings"

	lddatastar "github.com/Yacobolo/libredash/internal/dashboard/datastar"
	"github.com/Yacobolo/libredash/internal/queryaudit"
	"github.com/Yacobolo/libredash/internal/ui"
	uisignals "github.com/Yacobolo/libredash/internal/ui/signals"
	"github.com/starfederation/datastar-go/datastar"
)

const (
	adminQueryHistoryDefaultLimit = 50
	adminQueryHistoryMaxLimit     = 100
)

type queryHistoryCommandSignals struct {
	AdminQueryHistory        uisignals.AdminQueryHistorySignal  `json:"adminQueryHistory"`
	AdminQueryDetail         uisignals.AdminQueryDetailSignal   `json:"adminQueryDetail"`
	AdminQueryHistoryCommand uisignals.AdminQueryHistoryCommand `json:"adminQueryHistoryCommand"`
}

func (h Handler) QueryHistoryData(r *http.Request, filters uisignals.AdminQueryHistoryFilters, pageToken string, limit int) ui.AdminQueryHistoryData {
	repo, err := h.queryAuditRepository()
	if err != nil || repo == nil {
		return ui.AdminQueryHistoryData{Filters: filters, Limit: normalizeQueryHistoryLimit(limit), Error: queryHistoryErrorText(err)}
	}
	filters = normalizeQueryHistoryFilters(filters)
	events, nextCursor, hasMore, err := h.queryHistoryPage(r, repo, filters, pageToken, limit)
	if err != nil {
		return ui.AdminQueryHistoryData{Filters: filters, Limit: normalizeQueryHistoryLimit(limit), Error: err.Error()}
	}
	return ui.AdminQueryHistoryData{
		Events:      events,
		FilterMenus: h.queryHistoryFilterMenus(r, repo, filters, "", ""),
		Filters:     filters,
		NextCursor:  nextCursor,
		HasMore:     hasMore,
		Limit:       normalizeQueryHistoryLimit(limit),
	}
}

func (h Handler) queryHistoryUpdates(w http.ResponseWriter, r *http.Request) {
	clientID := lddatastar.EnsureClientID(w, r)
	if h.Broker == nil {
		http.Error(w, "admin query-history broker is not configured", http.StatusInternalServerError)
		return
	}
	sse := datastar.NewSSE(w, r)
	updates, unsubscribe := h.Broker.Subscribe(queryHistoryStreamID(clientID))
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

func (h Handler) queryHistoryCommand(w http.ResponseWriter, r *http.Request) {
	clientID := lddatastar.EnsureClientID(w, r)
	signals := queryHistoryCommandSignals{}
	if err := datastar.ReadSignals(r, &signals); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	command := normalizeQueryHistoryCommand(signals.AdminQueryHistoryCommand)
	repo, err := h.queryAuditRepository()
	if err != nil || repo == nil {
		errorText := queryHistoryErrorText(err)
		if errorText == "" {
			errorText = "Query audit repository is not configured."
		}
		if command.Action == "select_detail" {
			detail := signals.AdminQueryDetail
			detail.EventID = command.EventID
			detail.Loading = false
			detail.Error = errorText
			h.publishQueryDetailPatch(clientID, detail)
			w.WriteHeader(http.StatusNoContent)
			return
		}
		history := signals.AdminQueryHistory
		history.Loading = false
		history.Error = errorText
		h.publishQueryHistoryPatch(clientID, history)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	switch command.Action {
	case "select_detail":
		event, err := repo.GetQueryEvent(r.Context(), command.EventID)
		if err != nil {
			detail := signals.AdminQueryDetail
			detail.EventID = command.EventID
			detail.Loading = false
			detail.Error = err.Error()
			h.publishQueryDetailPatch(clientID, detail)
			w.WriteHeader(http.StatusNoContent)
			return
		}
		h.publishQueryDetailPatch(clientID, ui.AdminQueryDetailSignalFromEvent(queryEventFromAudit(event)))
		w.WriteHeader(http.StatusNoContent)
		return
	case "close_detail":
		h.publishQueryDetailPatch(clientID, uisignals.AdminQueryDetailSignal{})
		w.WriteHeader(http.StatusNoContent)
		return
	case "filter_search":
		history := signals.AdminQueryHistory
		history.Loading = false
		history.Error = ""
		history.FilterMenus = h.queryHistoryFilterMenus(r, repo, command.Filters, command.FilterMenu.MenuID, command.FilterMenu.Search)
		h.publishQueryHistoryPatch(clientID, history)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if command.Action == "filter_toggle" || command.Action == "filter_clear" {
		command.Filters = applyFilterMenuCommand(command.Filters, command.FilterMenu)
		command.PageToken = ""
	}
	events, nextCursor, hasMore, err := h.queryHistoryPage(r, repo, command.Filters, command.PageToken, command.Limit)
	history := signals.AdminQueryHistory
	incomingCount := len(history.Table.Rows)
	if command.Action == "load_more" {
		nextTable := ui.AdminQueryHistorySignalFromData(ui.AdminQueryHistoryData{Events: events}).Table
		history.Table.Rows = append(history.Table.Rows, nextTable.Rows...)
	} else {
		history.Table = ui.AdminQueryHistorySignalFromData(ui.AdminQueryHistoryData{Events: events}).Table
		incomingCount = 0
	}
	history.FilterMenus = h.queryHistoryFilterMenus(r, repo, command.Filters, "", "")
	history.Filters = command.Filters
	history.NextCursor = nextCursor
	history.HasMore = hasMore
	history.LoadedCountLabel = queryHistoryLoadedCountLabel(incomingCount + len(events))
	history.Loading = false
	history.Error = ""
	history.Limit = normalizeQueryHistoryLimit(command.Limit)
	if err != nil {
		history.Loading = false
		history.Error = err.Error()
	}
	h.publishQueryHistoryPatch(clientID, history)
	w.WriteHeader(http.StatusNoContent)
}

func (h Handler) publishQueryHistoryPatch(clientID string, history uisignals.AdminQueryHistorySignal) {
	if h.Broker == nil {
		return
	}
	h.Broker.Publish(queryHistoryStreamID(clientID), map[string]any{
		"adminQueryHistory": history,
		"adminQueryHistoryCommand": uisignals.AdminQueryHistoryCommand{
			Action:    "load_more",
			Filters:   history.Filters,
			PageToken: history.NextCursor,
			Limit:     history.Limit,
		},
	})
}

func (h Handler) publishQueryDetailPatch(clientID string, detail uisignals.AdminQueryDetailSignal) {
	if h.Broker == nil {
		return
	}
	h.Broker.Publish(queryHistoryStreamID(clientID), map[string]any{"adminQueryDetail": detail})
}

func normalizeQueryHistoryCommand(command uisignals.AdminQueryHistoryCommand) uisignals.AdminQueryHistoryCommand {
	action := strings.TrimSpace(command.Action)
	switch action {
	case "load_more", "select_detail", "close_detail", "filter_search", "filter_toggle", "filter_clear":
	default:
		action = "reset"
		command.PageToken = ""
	}
	command.Action = action
	command.Limit = normalizeQueryHistoryLimit(command.Limit)
	command.PageToken = strings.TrimSpace(command.PageToken)
	command.EventID = strings.TrimSpace(command.EventID)
	command.Filters = normalizeQueryHistoryFilters(command.Filters)
	command.FilterMenu = normalizeFilterMenuCommand(command.FilterMenu)
	return command
}

func normalizeQueryHistoryFilters(filters uisignals.AdminQueryHistoryFilters) uisignals.AdminQueryHistoryFilters {
	return uisignals.AdminQueryHistoryFilters{
		Workspaces: cleanStringSlice(filters.Workspaces),
		Principals: cleanStringSlice(filters.Principals),
		Surfaces:   cleanStringSlice(filters.Surfaces),
		Kinds:      cleanStringSlice(filters.Kinds),
		Statuses:   cleanStringSlice(filters.Statuses),
		Target:     strings.TrimSpace(filters.Target),
		Search:     strings.TrimSpace(filters.Search),
		From:       strings.TrimSpace(filters.From),
		To:         strings.TrimSpace(filters.To),
	}
}

func normalizeFilterMenuCommand(command uisignals.FilterMenuCommand) uisignals.FilterMenuCommand {
	return uisignals.FilterMenuCommand{
		MenuID:   strings.TrimSpace(command.MenuID),
		Action:   strings.TrimSpace(command.Action),
		Search:   strings.TrimSpace(command.Search),
		Value:    strings.TrimSpace(command.Value),
		Selected: cleanStringSlice(command.Selected),
	}
}

func applyFilterMenuCommand(filters uisignals.AdminQueryHistoryFilters, command uisignals.FilterMenuCommand) uisignals.AdminQueryHistoryFilters {
	if command.Action == "clear" {
		command.Selected = nil
	}
	selected := cleanStringSlice(command.Selected)
	if command.Action == "toggle" {
		selected = toggleStringSelection(selected, command.Value)
	}
	switch command.MenuID {
	case "workspace":
		filters.Workspaces = selected
	case "principal":
		filters.Principals = selected
	case "surface":
		filters.Surfaces = selected
	case "kind":
		filters.Kinds = selected
	case "status":
		filters.Statuses = selected
	}
	return normalizeQueryHistoryFilters(filters)
}

func toggleStringSelection(selected []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return selected
	}
	out := make([]string, 0, len(selected)+1)
	removed := false
	for _, item := range selected {
		if item == value {
			removed = true
			continue
		}
		out = append(out, item)
	}
	if !removed {
		out = append(out, value)
	}
	return out
}

func cleanStringSlice(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func queryHistoryLoadedCountLabel(count int) string {
	if count == 1 {
		return "1 query loaded"
	}
	return strconv.Itoa(count) + " queries loaded"
}

func (h Handler) queryHistoryFilterMenus(r *http.Request, repo queryaudit.Repository, filters uisignals.AdminQueryHistoryFilters, searchMenuID, search string) []uisignals.FilterMenuSignal {
	menus := []struct {
		id          string
		label       string
		placeholder string
		empty       string
		selected    []string
		values      map[string]int
		labels      map[string]string
		icon        string
	}{
		{id: "workspace", label: "Workspace", placeholder: "Search workspaces", empty: "No workspaces found.", selected: filters.Workspaces, values: map[string]int{}, labels: map[string]string{}, icon: "workspace"},
		{id: "principal", label: "User", placeholder: "Search users", empty: "No users found.", selected: filters.Principals, values: map[string]int{}, labels: map[string]string{}, icon: "user"},
		{id: "surface", label: "Source type", placeholder: "Search source types", empty: "No source types found.", selected: filters.Surfaces, values: map[string]int{}, labels: map[string]string{}, icon: "source"},
		{id: "kind", label: "Kind", placeholder: "Search kinds", empty: "No kinds found.", selected: filters.Kinds, values: map[string]int{}, labels: map[string]string{}, icon: "kind"},
		{id: "status", label: "Status", placeholder: "Search statuses", empty: "No statuses found.", selected: filters.Statuses, values: map[string]int{}, labels: map[string]string{}, icon: "status"},
	}
	out := make([]uisignals.FilterMenuSignal, 0, len(menus))
	for _, menu := range menus {
		menuSearch := ""
		loading := false
		if menu.id == searchMenuID {
			menuSearch = strings.TrimSpace(search)
		}
		options, err := repo.ListQueryEventFilterOptions(r.Context(), menu.id, menuSearch, 100)
		if err != nil {
			return queryHistoryFilterMenusWithError(filters, err.Error())
		}
		for _, option := range options {
			menu.values[option.Value] = option.Count
		}
		if menu.id == "principal" {
			menu.labels = h.queryPrincipalLabels(r, mapKeys(menu.values))
		}
		out = append(out, queryHistoryFilterMenu(menu.id, menu.label, menu.placeholder, menu.empty, menu.icon, menu.values, menu.labels, menu.selected, menuSearch, loading, ""))
	}
	return out
}

func queryHistoryFilterMenusWithError(filters uisignals.AdminQueryHistoryFilters, message string) []uisignals.FilterMenuSignal {
	return []uisignals.FilterMenuSignal{
		{ID: "workspace", Label: "Workspace", SummaryLabel: queryHistoryFilterSummary("Workspace", filters.Workspaces, nil), Mode: "multi", Selected: filters.Workspaces, Loading: false, Error: message, Placeholder: "Search workspaces", EmptyLabel: "No workspaces found."},
		{ID: "principal", Label: "User", SummaryLabel: queryHistoryFilterSummary("User", filters.Principals, nil), Mode: "multi", Selected: filters.Principals, Loading: false, Error: message, Placeholder: "Search users", EmptyLabel: "No users found."},
		{ID: "surface", Label: "Source type", SummaryLabel: queryHistoryFilterSummary("Source type", filters.Surfaces, nil), Mode: "multi", Selected: filters.Surfaces, Loading: false, Error: message, Placeholder: "Search source types", EmptyLabel: "No source types found."},
		{ID: "kind", Label: "Kind", SummaryLabel: queryHistoryFilterSummary("Kind", filters.Kinds, nil), Mode: "multi", Selected: filters.Kinds, Loading: false, Error: message, Placeholder: "Search kinds", EmptyLabel: "No kinds found."},
		{ID: "status", Label: "Status", SummaryLabel: queryHistoryFilterSummary("Status", filters.Statuses, nil), Mode: "multi", Selected: filters.Statuses, Loading: false, Error: message, Placeholder: "Search statuses", EmptyLabel: "No statuses found."},
	}
}

func queryHistoryFilterMenu(id, label, placeholder, emptyLabel, icon string, values map[string]int, labels map[string]string, selected []string, search string, loading bool, errorText string) uisignals.FilterMenuSignal {
	selected = cleanStringSlice(selected)
	options := queryHistoryFilterOptions(values, labels, selected, search, icon)
	return uisignals.FilterMenuSignal{
		ID:           id,
		Label:        label,
		SummaryLabel: queryHistoryFilterSummary(label, selected, labels),
		Mode:         "multi",
		Search:       search,
		Selected:     selected,
		Options:      options,
		Loading:      loading,
		Error:        errorText,
		Placeholder:  placeholder,
		EmptyLabel:   emptyLabel,
	}
}

func queryHistoryFilterOptions(values map[string]int, labels map[string]string, selected []string, search, icon string) []uisignals.FilterMenuOptionSignal {
	search = strings.ToLower(strings.TrimSpace(search))
	selectedSet := stringSet(selected)
	options := make([]uisignals.FilterMenuOptionSignal, 0, len(values)+len(selected))
	seen := map[string]struct{}{}
	for value, count := range values {
		label := queryHistoryOptionLabel(value, labels)
		if search != "" && !strings.Contains(strings.ToLower(label+" "+value), search) {
			continue
		}
		seen[value] = struct{}{}
		options = append(options, uisignals.FilterMenuOptionSignal{
			Value:      value,
			Label:      label,
			Icon:       icon,
			CountLabel: strconv.Itoa(count),
			Selected:   selectedSet[value],
		})
	}
	for _, value := range selected {
		if _, ok := seen[value]; ok {
			continue
		}
		label := queryHistoryOptionLabel(value, labels)
		if search != "" && !strings.Contains(strings.ToLower(label+" "+value), search) {
			continue
		}
		options = append(options, uisignals.FilterMenuOptionSignal{
			Value:    value,
			Label:    label,
			Icon:     icon,
			Selected: true,
		})
	}
	sort.Slice(options, func(i, j int) bool {
		if options[i].Selected != options[j].Selected {
			return options[i].Selected
		}
		return strings.ToLower(options[i].Label) < strings.ToLower(options[j].Label)
	})
	return options
}

func queryHistoryOptionLabel(value string, labels map[string]string) string {
	if labels != nil {
		if label := strings.TrimSpace(labels[value]); label != "" {
			return label
		}
	}
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	return value
}

func queryHistoryFilterSummary(label string, selected []string, labels map[string]string) string {
	selected = cleanStringSlice(selected)
	switch len(selected) {
	case 0:
		return label
	case 1:
		return queryHistoryOptionLabel(selected[0], labels)
	default:
		return strconv.Itoa(len(selected)) + " selected"
	}
}

func stringSet(values []string) map[string]bool {
	out := make(map[string]bool, len(values))
	for _, value := range values {
		out[value] = true
	}
	return out
}

func mapKeys(values map[string]int) []string {
	out := make([]string, 0, len(values))
	for value := range values {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func queryHistoryStreamID(clientID string) string {
	if strings.TrimSpace(clientID) == "" {
		clientID = "default"
	}
	return "admin-queries:" + clientID
}

func (h Handler) queryHistoryPage(r *http.Request, repo queryaudit.Repository, filters uisignals.AdminQueryHistoryFilters, pageToken string, limit int) ([]ui.AdminQueryEvent, string, bool, error) {
	limit = normalizeQueryHistoryLimit(limit)
	rows, err := repo.ListQueryEvents(r.Context(), queryaudit.Filter{
		WorkspaceIDs: cleanStringSlice(filters.Workspaces),
		PrincipalIDs: cleanStringSlice(filters.Principals),
		Surfaces:     cleanStringSlice(filters.Surfaces),
		QueryKinds:   cleanStringSlice(filters.Kinds),
		Target:       strings.TrimSpace(filters.Target),
		Statuses:     cleanStringSlice(filters.Statuses),
		Search:       strings.TrimSpace(filters.Search),
		From:         strings.TrimSpace(filters.From),
		To:           strings.TrimSpace(filters.To),
		PageToken:    strings.TrimSpace(pageToken),
		Limit:        limit + 1,
	})
	if err != nil {
		return nil, "", false, err
	}
	nextCursor := ""
	hasMore := len(rows) > limit
	if hasMore {
		last := rows[limit-1]
		nextCursor = encodeCursor(last.CreatedAt, last.ID)
		rows = rows[:limit]
	}
	out := make([]ui.AdminQueryEvent, 0, len(rows))
	for _, row := range rows {
		out = append(out, queryEventFromAudit(row))
	}
	return out, nextCursor, hasMore, nil
}

func queryEventFromAudit(row queryaudit.Event) ui.AdminQueryEvent {
	return ui.AdminQueryEvent{
		ID:            row.ID,
		WorkspaceID:   row.WorkspaceID,
		PrincipalID:   row.PrincipalID,
		Surface:       row.Surface,
		Operation:     row.Operation,
		QueryKind:     row.QueryKind,
		ModelID:       row.ModelID,
		Target:        row.Target,
		ObjectType:    row.ObjectType,
		ObjectID:      row.ObjectID,
		RequestID:     row.RequestID,
		CorrelationID: row.CorrelationID,
		Status:        row.Status,
		DurationMS:    row.DurationMS,
		RowsReturned:  row.RowsReturned,
		Error:         row.Error,
		SQL:           row.SQL,
		PlanText:      row.PlanText,
		QueryJSON:     row.QueryJSON,
		CreatedAt:     row.CreatedAt,
	}
}

func queryHistoryErrorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func normalizeQueryHistoryLimit(limit int) int {
	if limit <= 0 {
		return adminQueryHistoryDefaultLimit
	}
	if limit > adminQueryHistoryMaxLimit {
		return adminQueryHistoryMaxLimit
	}
	return limit
}

func encodeCursor(createdAt, id string) string {
	if strings.TrimSpace(createdAt) == "" || strings.TrimSpace(id) == "" {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString([]byte(createdAt + "\x00" + id))
}
