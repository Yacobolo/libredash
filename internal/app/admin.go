package app

import (
	"database/sql"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/Yacobolo/libredash/internal/access"
	lddatastar "github.com/Yacobolo/libredash/internal/dashboard/datastar"
	"github.com/Yacobolo/libredash/internal/queryaudit"
	"github.com/Yacobolo/libredash/internal/ui"
	uisignals "github.com/Yacobolo/libredash/internal/ui/signals"
	"github.com/Yacobolo/libredash/internal/workspace"
	"github.com/go-chi/chi/v5"
	"github.com/starfederation/datastar-go/datastar"
)

const adminQueryHistoryDefaultLimit = 50

type adminQueryHistoryCommandSignals struct {
	AdminQueryHistory        uisignals.AdminQueryHistorySignal  `json:"adminQueryHistory"`
	AdminQueryDetail         uisignals.AdminQueryDetailSignal   `json:"adminQueryDetail"`
	AdminQueryHistoryCommand uisignals.AdminQueryHistoryCommand `json:"adminQueryHistoryCommand"`
}

func (s *Server) adminGeneral(w http.ResponseWriter, r *http.Request) {
	s.renderAdminPage(w, r, "general")
}

func (s *Server) adminPrincipals(w http.ResponseWriter, r *http.Request) {
	s.renderAdminPage(w, r, "principals")
}

func (s *Server) adminPrincipalDetail(w http.ResponseWriter, r *http.Request) {
	data, err := s.adminData(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	principalID := chi.URLParam(r, "principal")
	for i := range data.Principals {
		if data.Principals[i].ID == principalID {
			data.SelectedPrincipal = &data.Principals[i]
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			if err := ui.AdminPage(s.metrics.Catalog(), "principal-detail", s.currentAdminRoleLabel(r), data, s.chatChromeOption(r)).Render(w); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
			return
		}
	}
	http.NotFound(w, r)
}

func (s *Server) adminGroups(w http.ResponseWriter, r *http.Request) {
	s.renderAdminPage(w, r, "groups")
}

func (s *Server) adminAgent(w http.ResponseWriter, r *http.Request) {
	s.renderAdminPage(w, r, "agent")
}

func (s *Server) adminStorage(w http.ResponseWriter, r *http.Request) {
	_ = lddatastar.EnsureClientID(w, r)
	s.renderAdminPage(w, r, "storage")
}

func (s *Server) adminQueries(w http.ResponseWriter, r *http.Request) {
	_ = lddatastar.EnsureClientID(w, r)
	s.renderAdminPage(w, r, "queries")
}

func (s *Server) adminGroupDetail(w http.ResponseWriter, r *http.Request) {
	data, err := s.adminData(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	groupID := chi.URLParam(r, "group")
	for i := range data.Groups {
		if data.Groups[i].ID == groupID {
			data.SelectedGroup = &data.Groups[i]
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			if err := ui.AdminPage(s.metrics.Catalog(), "group-detail", s.currentAdminRoleLabel(r), data, s.chatChromeOption(r)).Render(w); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
			return
		}
	}
	http.NotFound(w, r)
}

func (s *Server) renderAdminPage(w http.ResponseWriter, r *http.Request, active string) {
	data, err := s.adminData(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if err := ui.AdminPage(s.metrics.Catalog(), active, s.currentAdminRoleLabel(r), data, s.chatChromeOption(r)).Render(w); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) currentAdminRoleLabel(r *http.Request) string {
	if s.auth == nil {
		return "Local platform"
	}
	principal, ok := s.auth.Principal(r)
	if ok && principal.DevBypass {
		return "Platform admin"
	}
	return "Platform access"
}

func (s *Server) adminData(r *http.Request) (ui.AdminData, error) {
	data := ui.AdminData{
		Workspace:       workspace.WorkspaceView{ID: "platform", Title: "Platform"},
		CSRFToken:       csrfToken(r, s.auth),
		AuthConfigured:  s.auth != nil,
		RBACConfigured:  s.store != nil,
		RBACStatusLabel: "Configured",
	}
	var err error
	data.Agent, err = s.adminAgentData(r)
	if err != nil {
		return data, err
	}
	repo, err := s.accessRepository()
	if err != nil {
		return data, err
	}
	if repo == nil {
		data.RBACConfigured = false
		data.RBACStatusLabel = "RBAC store is not configured"
		data.RoleCount = len(defaultWorkspaceRoles())
		data.Storage = s.adminStorageData(r)
		return data, nil
	}
	principals, err := s.adminPrincipalsData(r)
	if err != nil {
		return data, err
	}
	groups, err := s.adminGroupsData(r)
	if err != nil {
		return data, err
	}
	bindings, roles, err := s.adminRoleBindingsAndRoles(r)
	if err != nil {
		return data, err
	}
	membersByGroup := map[string][]ui.AdminPrincipalRef{}
	groupsByID := map[string]access.Group{}
	for _, group := range groups {
		groupsByID[group.ID] = group
		members := s.adminGroupMembersData(r, group.ID)
		for _, member := range members {
			membersByGroup[group.ID] = append(membersByGroup[group.ID], ui.AdminPrincipalRef{
				ID:          member.ID,
				Email:       member.Email,
				DisplayName: member.DisplayName,
			})
		}
	}
	data.RoleCount = len(roles)
	data.BindingCount = len(bindings)
	data.Principals = buildAdminPrincipals(principals, bindings, groupsByID, membersByGroup)
	data.Groups = buildAdminGroups(groups, bindings, membersByGroup)
	data.Storage = s.adminStorageData(r)
	data.QueryHistory = s.adminQueryHistoryData(r, uisignals.AdminQueryHistoryFilters{}, "", adminQueryHistoryDefaultLimit)
	data.PrincipalCount = len(data.Principals)
	data.GroupCount = len(data.Groups)
	return data, nil
}

func (s *Server) adminQueryEventsData(r *http.Request) []ui.AdminQueryEvent {
	return s.adminQueryHistoryData(r, uisignals.AdminQueryHistoryFilters{}, "", 100).Events
}

func (s *Server) adminQueryHistoryData(r *http.Request, filters uisignals.AdminQueryHistoryFilters, pageToken string, limit int) ui.AdminQueryHistoryData {
	repo, err := s.queryAuditRepository()
	if err != nil || repo == nil {
		return ui.AdminQueryHistoryData{Filters: filters, Limit: normalizeAdminQueryHistoryLimit(limit), Error: queryHistoryErrorText(err)}
	}
	filters = normalizeAdminQueryHistoryFilters(filters)
	events, nextCursor, hasMore, err := s.listAdminQueryHistoryPage(r, repo, filters, pageToken, limit)
	if err != nil {
		return ui.AdminQueryHistoryData{Filters: filters, Limit: normalizeAdminQueryHistoryLimit(limit), Error: err.Error()}
	}
	return ui.AdminQueryHistoryData{
		Events:      events,
		FilterMenus: s.adminQueryHistoryFilterMenus(r, repo, filters, "", ""),
		Filters:     filters,
		NextCursor:  nextCursor,
		HasMore:     hasMore,
		Limit:       normalizeAdminQueryHistoryLimit(limit),
	}
}

func (s *Server) adminQueryHistoryUpdates(w http.ResponseWriter, r *http.Request) {
	clientID := lddatastar.EnsureClientID(w, r)
	sse := datastar.NewSSE(w, r)
	updates, unsubscribe := s.broker.Subscribe(adminQueryHistoryStreamID(clientID))
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

func (s *Server) adminQueryHistoryCommand(w http.ResponseWriter, r *http.Request) {
	clientID := lddatastar.EnsureClientID(w, r)
	var signals adminQueryHistoryCommandSignals
	if err := datastar.ReadSignals(r, &signals); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	command := normalizeAdminQueryHistoryCommand(signals.AdminQueryHistoryCommand)
	repo, err := s.queryAuditRepository()
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
			s.publishAdminQueryDetailPatch(clientID, detail)
			w.WriteHeader(http.StatusNoContent)
			return
		}
		history := signals.AdminQueryHistory
		history.Loading = false
		history.Error = errorText
		s.publishAdminQueryHistoryPatch(clientID, history)
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
			s.publishAdminQueryDetailPatch(clientID, detail)
			w.WriteHeader(http.StatusNoContent)
			return
		}
		s.publishAdminQueryDetailPatch(clientID, ui.AdminQueryDetailSignalFromEvent(adminQueryEventFromAudit(event)))
		w.WriteHeader(http.StatusNoContent)
		return
	case "close_detail":
		s.publishAdminQueryDetailPatch(clientID, uisignals.AdminQueryDetailSignal{})
		w.WriteHeader(http.StatusNoContent)
		return
	case "filter_search":
		history := signals.AdminQueryHistory
		history.Loading = false
		history.Error = ""
		history.FilterMenus = s.adminQueryHistoryFilterMenus(r, repo, command.Filters, command.FilterMenu.MenuID, command.FilterMenu.Search)
		s.publishAdminQueryHistoryPatch(clientID, history)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if command.Action == "filter_toggle" || command.Action == "filter_clear" {
		command.Filters = applyAdminQueryFilterMenuCommand(command.Filters, command.FilterMenu)
		command.PageToken = ""
	}
	events, nextCursor, hasMore, err := s.listAdminQueryHistoryPage(r, repo, command.Filters, command.PageToken, command.Limit)
	history := signals.AdminQueryHistory
	incomingCount := len(history.Table.Rows)
	if command.Action == "load_more" {
		nextTable := ui.AdminQueryHistorySignalFromData(ui.AdminQueryHistoryData{Events: events}).Table
		history.Table.Rows = append(history.Table.Rows, nextTable.Rows...)
	} else {
		history.Table = ui.AdminQueryHistorySignalFromData(ui.AdminQueryHistoryData{Events: events}).Table
		incomingCount = 0
	}
	history.FilterMenus = s.adminQueryHistoryFilterMenus(r, repo, command.Filters, "", "")
	history.Filters = command.Filters
	history.NextCursor = nextCursor
	history.HasMore = hasMore
	history.LoadedCountLabel = queryHistoryLoadedCountLabel(incomingCount + len(events))
	history.Loading = false
	history.Error = ""
	history.Limit = normalizeAdminQueryHistoryLimit(command.Limit)
	if err != nil {
		history.Loading = false
		history.Error = err.Error()
	}
	s.publishAdminQueryHistoryPatch(clientID, history)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) publishAdminQueryHistoryPatch(clientID string, history uisignals.AdminQueryHistorySignal) {
	s.broker.Publish(adminQueryHistoryStreamID(clientID), map[string]any{
		"adminQueryHistory": history,
		"adminQueryHistoryCommand": uisignals.AdminQueryHistoryCommand{
			Action:    "load_more",
			Filters:   history.Filters,
			PageToken: history.NextCursor,
			Limit:     history.Limit,
		},
	})
}

func (s *Server) publishAdminQueryDetailPatch(clientID string, detail uisignals.AdminQueryDetailSignal) {
	s.broker.Publish(adminQueryHistoryStreamID(clientID), map[string]any{
		"adminQueryDetail": detail,
	})
}

func normalizeAdminQueryHistoryCommand(command uisignals.AdminQueryHistoryCommand) uisignals.AdminQueryHistoryCommand {
	action := strings.TrimSpace(command.Action)
	switch action {
	case "load_more", "select_detail", "close_detail", "filter_search", "filter_toggle", "filter_clear":
	default:
		action = "reset"
		command.PageToken = ""
	}
	command.Action = action
	command.Limit = normalizeAdminQueryHistoryLimit(command.Limit)
	command.PageToken = strings.TrimSpace(command.PageToken)
	command.EventID = strings.TrimSpace(command.EventID)
	command.Filters = normalizeAdminQueryHistoryFilters(command.Filters)
	command.FilterMenu = normalizeFilterMenuCommand(command.FilterMenu)
	return command
}

func normalizeAdminQueryHistoryFilters(filters uisignals.AdminQueryHistoryFilters) uisignals.AdminQueryHistoryFilters {
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

func applyAdminQueryFilterMenuCommand(filters uisignals.AdminQueryHistoryFilters, command uisignals.FilterMenuCommand) uisignals.AdminQueryHistoryFilters {
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
	return normalizeAdminQueryHistoryFilters(filters)
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

func (s *Server) adminQueryHistoryFilterMenus(r *http.Request, repo queryaudit.Repository, filters uisignals.AdminQueryHistoryFilters, searchMenuID, search string) []uisignals.FilterMenuSignal {
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
			menu.labels = s.queryHistoryPrincipalLabels(r, mapKeys(menu.values))
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

func (s *Server) queryHistoryPrincipalLabels(r *http.Request, values []string) map[string]string {
	labels := map[string]string{}
	var current Principal
	var hasCurrent bool
	if s.auth != nil {
		current, hasCurrent = s.auth.Principal(r)
	}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if hasCurrent && value == current.ID {
			identity := firstNonEmpty(current.Email, current.DisplayName, current.ID)
			labels[value] = "Me (" + identity + ")"
			continue
		}
		labels[value] = value
	}
	return labels
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

func adminQueryHistoryStreamID(clientID string) string {
	if strings.TrimSpace(clientID) == "" {
		clientID = "default"
	}
	return "admin-queries:" + clientID
}

func (s *Server) listAdminQueryHistoryPage(r *http.Request, repo queryaudit.Repository, filters uisignals.AdminQueryHistoryFilters, pageToken string, limit int) ([]ui.AdminQueryEvent, string, bool, error) {
	limit = normalizeAdminQueryHistoryLimit(limit)
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
		out = append(out, adminQueryEventFromAudit(row))
	}
	return out, nextCursor, hasMore, nil
}

func adminQueryEventFromAudit(row queryaudit.Event) ui.AdminQueryEvent {
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

func normalizeAdminQueryHistoryLimit(limit int) int {
	if limit <= 0 {
		return adminQueryHistoryDefaultLimit
	}
	if limit > maxAPILimit {
		return maxAPILimit
	}
	return limit
}

func (s *Server) adminAgentData(r *http.Request) (ui.AdminAgentData, error) {
	details, err := s.adminAgentDetails(r.Context())
	if err != nil {
		return ui.AdminAgentData{}, err
	}
	data := ui.AdminAgentData{
		Enabled:      details.Enabled,
		Model:        details.Model,
		SystemPrompt: details.SystemPrompt,
		CSRFToken:    csrfToken(r, s.auth),
		UpdatePath:   "/api/v1/admin/agent/config",
		CanWrite:     true,
	}
	for _, tool := range details.Tools {
		data.Tools = append(data.Tools, ui.AdminAgentTool{
			Name:        tool.Name,
			Description: tool.Description,
			InputSchema: tool.InputSchema,
		})
	}
	if s.auth == nil {
		return data, nil
	}
	principal, ok := s.auth.Principal(r)
	if !ok || principal.DevBypass {
		return data, nil
	}
	repo, err := s.accessRepository()
	if err != nil || repo == nil {
		return data, err
	}
	allowed, err := repo.HasPermission(r.Context(), s.defaultWorkspaceID, principal.ID, access.PermissionRBACWrite)
	if err != nil {
		return data, err
	}
	data.CanWrite = allowed
	return data, nil
}

func (s *Server) adminGroupsData(r *http.Request) ([]access.Group, error) {
	if s.store == nil {
		return nil, nil
	}
	rows, err := s.store.SQLDB().QueryContext(r.Context(), `
SELECT id, workspace_id, provider, external_id, name, created_at
FROM groups
ORDER BY workspace_id, name, id
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	groups := []access.Group{}
	for rows.Next() {
		var group access.Group
		if err := rows.Scan(&group.ID, &group.WorkspaceID, &group.Provider, &group.ExternalID, &group.Name, &group.CreatedAt); err != nil {
			return nil, err
		}
		groups = append(groups, group)
	}
	return groups, rows.Err()
}

func (s *Server) adminGroupMembersData(r *http.Request, groupID string) []ui.AdminPrincipalRef {
	if s.store == nil {
		return nil
	}
	rows, err := s.store.SQLDB().QueryContext(r.Context(), `
SELECT gm.principal_id, p.email, p.display_name
FROM group_members gm
JOIN principals p ON p.id = gm.principal_id
WHERE gm.group_id = ?
ORDER BY p.email, p.display_name, gm.principal_id
`, groupID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	members := []ui.AdminPrincipalRef{}
	for rows.Next() {
		var member ui.AdminPrincipalRef
		if err := rows.Scan(&member.ID, &member.Email, &member.DisplayName); err != nil {
			return nil
		}
		members = append(members, member)
	}
	return members
}

func (s *Server) adminRoleBindingsAndRoles(r *http.Request) ([]workspace.RoleBindingView, []workspace.RoleView, error) {
	repo, err := s.accessRepository()
	if err != nil {
		return nil, nil, err
	}
	if repo == nil {
		return nil, defaultWorkspaceRoles(), nil
	}
	roleRows, err := repo.ListRoles(r.Context())
	if err != nil {
		return nil, nil, err
	}
	bindings, err := s.adminRoleBindingsData(r)
	if err != nil {
		return nil, nil, err
	}
	roles := make([]workspace.RoleView, 0, len(roleRows))
	for _, row := range roleRows {
		roles = append(roles, roleView(row))
	}
	return bindings, roles, nil
}

func (s *Server) adminRoleBindingsData(r *http.Request) ([]workspace.RoleBindingView, error) {
	if s.store == nil {
		return nil, nil
	}
	rows, err := s.store.SQLDB().QueryContext(r.Context(), `
SELECT
  rb.id,
  rb.workspace_id,
  CASE WHEN NULLIF(rb.principal_id, '') IS NOT NULL THEN 'principal' ELSE 'group' END AS subject_type,
  COALESCE(NULLIF(rb.principal_id, ''), rb.group_id, '') AS subject_id,
  rb.principal_id,
  rb.group_id,
  p.email,
  p.display_name,
  g.name AS group_name,
  r.name AS role_name,
  rb.created_at
FROM role_bindings rb
JOIN roles r ON r.id = rb.role_id
LEFT JOIN principals p ON p.id = NULLIF(rb.principal_id, '')
LEFT JOIN groups g ON g.id = rb.group_id
ORDER BY rb.workspace_id, subject_type, p.email, g.name, r.name
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	bindings := []workspace.RoleBindingView{}
	for rows.Next() {
		var binding workspace.RoleBindingView
		var principalID, groupID, email, displayName, groupName sql.NullString
		if err := rows.Scan(&binding.ID, &binding.WorkspaceID, &binding.SubjectType, &binding.SubjectID, &principalID, &groupID, &email, &displayName, &groupName, &binding.Role, &binding.CreatedAt); err != nil {
			return nil, err
		}
		binding.PrincipalID = adminNullString(principalID)
		binding.GroupID = adminNullString(groupID)
		binding.Email = adminNullString(email)
		binding.DisplayName = firstNonEmpty(adminNullString(displayName), adminNullString(groupName))
		binding.GroupName = adminNullString(groupName)
		bindings = append(bindings, binding)
	}
	return bindings, rows.Err()
}

func (s *Server) adminPrincipalsData(r *http.Request) ([]ui.AdminPrincipal, error) {
	rows, err := s.queryPrincipals(r)
	if err != nil {
		return nil, err
	}
	principals := make([]ui.AdminPrincipal, 0, len(rows))
	for _, row := range rows {
		principals = append(principals, ui.AdminPrincipal{
			ID:          stringMapValue(row, "id"),
			Email:       stringMapValue(row, "email"),
			DisplayName: stringMapValue(row, "displayName"),
			CreatedAt:   stringMapValue(row, "createdAt"),
			UpdatedAt:   stringMapValue(row, "updatedAt"),
		})
	}
	sort.SliceStable(principals, func(i, j int) bool {
		return adminPrincipalSortKey(principals[i]) < adminPrincipalSortKey(principals[j])
	})
	return principals, nil
}

func buildAdminPrincipals(principals []ui.AdminPrincipal, bindings []workspace.RoleBindingView, groupsByID map[string]access.Group, membersByGroup map[string][]ui.AdminPrincipalRef) []ui.AdminPrincipal {
	byID := make(map[string]*ui.AdminPrincipal, len(principals))
	out := make([]ui.AdminPrincipal, 0, len(principals))
	for _, principal := range principals {
		row := principal
		byID[row.ID] = &row
		out = append(out, row)
	}
	for _, binding := range bindings {
		if binding.SubjectType == string(access.SubjectPrincipal) && binding.PrincipalID != "" {
			if principal := byID[binding.PrincipalID]; principal != nil {
				principal.DirectRoles = appendUnique(principal.DirectRoles, binding.Role)
			}
		}
	}
	for groupID, members := range membersByGroup {
		group := groupsByID[groupID]
		for _, member := range members {
			if principal := byID[member.ID]; principal != nil {
				principal.Groups = append(principal.Groups, ui.AdminGroupRef{
					ID:         group.ID,
					Name:       group.Name,
					ExternalID: group.ExternalID,
				})
			}
		}
	}
	for i := range out {
		if principal := byID[out[i].ID]; principal != nil {
			sort.Strings(principal.DirectRoles)
			sort.SliceStable(principal.Groups, func(i, j int) bool {
				return principal.Groups[i].Name < principal.Groups[j].Name
			})
			out[i] = *principal
		}
	}
	return out
}

func buildAdminGroups(groups []access.Group, bindings []workspace.RoleBindingView, membersByGroup map[string][]ui.AdminPrincipalRef) []ui.AdminGroup {
	out := make([]ui.AdminGroup, 0, len(groups))
	byID := make(map[string]*ui.AdminGroup, len(groups))
	for _, group := range groups {
		row := ui.AdminGroup{
			ID:         group.ID,
			Name:       group.Name,
			Provider:   group.Provider,
			ExternalID: group.ExternalID,
			CreatedAt:  group.CreatedAt,
			Members:    membersByGroup[group.ID],
		}
		sort.SliceStable(row.Members, func(i, j int) bool {
			return adminPrincipalRefSortKey(row.Members[i]) < adminPrincipalRefSortKey(row.Members[j])
		})
		byID[row.ID] = &row
		out = append(out, row)
	}
	for _, binding := range bindings {
		if binding.SubjectType == string(access.SubjectGroup) && binding.GroupID != "" {
			if group := byID[binding.GroupID]; group != nil {
				group.Roles = appendUnique(group.Roles, binding.Role)
			}
		}
	}
	for i := range out {
		if group := byID[out[i].ID]; group != nil {
			sort.Strings(group.Roles)
			out[i] = *group
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})
	return out
}

func appendUnique(values []string, value string) []string {
	if value == "" {
		return values
	}
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func adminNullString(value sql.NullString) string {
	if value.Valid {
		return value.String
	}
	return ""
}

func stringMapValue(row map[string]any, key string) string {
	if value, ok := row[key].(string); ok {
		return value
	}
	return ""
}

func adminPrincipalSortKey(row ui.AdminPrincipal) string {
	return firstNonEmpty(row.Email, row.DisplayName, row.ID)
}

func adminPrincipalRefSortKey(row ui.AdminPrincipalRef) string {
	return firstNonEmpty(row.Email, row.DisplayName, row.ID)
}
