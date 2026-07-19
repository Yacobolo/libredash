package command

import (
	"fmt"

	"github.com/Yacobolo/libredash/internal/dashboard"
	"github.com/Yacobolo/libredash/internal/dashboard/consumer"
	"github.com/Yacobolo/libredash/internal/dashboard/report"
)

type TargetKind = consumer.Kind

const (
	TargetFilterOptions TargetKind = consumer.KindFilterOptions
	TargetVisual        TargetKind = consumer.KindVisual
	TargetTable         TargetKind = consumer.KindTable
)

type Target = consumer.Target

type RefreshPlan struct {
	Command string
	Targets []Target
}

type PreparedRefresh struct {
	Filters dashboard.Filters
	Plan    RefreshPlan
}

// PrepareSelect applies the command to coordinator-owned filters and plans
// only the interaction's explicit targets. A source is therefore queried only
// when it explicitly targets itself.
func (s Service) PrepareSelect(request Request, authoritative dashboard.Filters) (PreparedRefresh, error) {
	filters := report.NormalizeFilters(s.Metrics, request.DashboardID, request.PageID, authoritative)
	command, err := canonicalInteractionCommand(s.Metrics, request.DashboardID, request.InteractionCommand)
	if err != nil {
		return PreparedRefresh{}, fmt.Errorf("invalid interaction selection: %w", err)
	}
	filters = report.NormalizeFilters(s.Metrics, request.DashboardID, request.PageID, filters.ApplyInteraction(command))
	targets, err := s.selectionTargets(request, command.SourceKind, command.SourceID)
	if err != nil {
		return PreparedRefresh{}, err
	}
	return PreparedRefresh{Filters: filters, Plan: RefreshPlan{Command: "select", Targets: targets}}, nil
}

// PrepareClearSelection queries the ordered union of targets affected by the
// selections being removed.
func (s Service) PrepareClearSelection(request Request, authoritative dashboard.Filters) (PreparedRefresh, error) {
	filters := report.NormalizeFilters(s.Metrics, request.DashboardID, request.PageID, authoritative)
	targets := []Target{}
	seen := map[string]struct{}{}
	for _, selection := range filters.Selections {
		selectionTargets, err := s.selectionTargets(request, selection.SourceKind, selection.SourceID)
		if err != nil {
			return PreparedRefresh{}, err
		}
		for _, target := range selectionTargets {
			key := string(target.Kind) + ":" + target.ID
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			targets = append(targets, target)
		}
	}
	filters.Selections = []dashboard.InteractionSelection{}
	return PreparedRefresh{Filters: filters, Plan: RefreshPlan{Command: "clear_selection", Targets: targets}}, nil
}

func (s Service) PrepareReload(request Request, authoritative dashboard.Filters) (PreparedRefresh, error) {
	filters := report.NormalizeFilters(s.Metrics, request.DashboardID, request.PageID, authoritative)
	return PreparedRefresh{Filters: filters, Plan: s.fullPlan(request, "reload")}, nil
}

func (s Service) PrepareResetFilters(request Request) (PreparedRefresh, error) {
	filters := report.DefaultFilters(s.Metrics, request.DashboardID, request.PageID)
	return PreparedRefresh{Filters: filters, Plan: s.fullPlan(request, "reset_filters")}, nil
}

func (s Service) PrepareInitial(request Request, initial dashboard.Filters) (PreparedRefresh, error) {
	filters := report.NormalizeFilters(s.Metrics, request.DashboardID, request.PageID, initial)
	return PreparedRefresh{Filters: filters, Plan: s.fullPlan(request, "initial")}, nil
}

func (s Service) PrepareTableWindow(request Request, authoritative dashboard.Filters) (PreparedRefresh, error) {
	filters := report.NormalizeFilters(s.Metrics, request.DashboardID, request.PageID, authoritative)
	tableRequest := s.Metrics.NormalizeTableRequest(request.DashboardID, request.TableCommand)
	return PreparedRefresh{
		Filters: filters,
		Plan: RefreshPlan{Command: "table_window", Targets: []Target{{
			Kind:         TargetTable,
			ID:           tableRequest.Table,
			TableRequest: tableRequest,
		}}},
	}, nil
}

func (s Service) fullPlan(request Request, commandName string) RefreshPlan {
	definition, _, ok := s.Metrics.Report(request.DashboardID)
	if !ok {
		return RefreshPlan{Command: commandName}
	}
	page, ok := definition.PageOrDefault(request.PageID)
	if !ok {
		return RefreshPlan{Command: commandName}
	}
	tableRequest := s.Metrics.NormalizeTableRequest(request.DashboardID, request.TableCommand).Reset()
	targets := make([]Target, 0, len(page.Visuals))
	seen := map[string]struct{}{}
	for _, item := range page.Visuals {
		var target Target
		switch {
		case item.Kind == "filter_card" && item.Filter != "":
			target = Target{Kind: TargetFilterOptions, ID: item.Filter}
		case item.Visual != "":
			target = Target{Kind: TargetVisual, ID: item.Visual}
		case item.Table != "":
			requestForTable := tableRequest
			requestForTable.Table = item.Table
			target = Target{Kind: TargetTable, ID: item.Table, TableRequest: requestForTable}
		default:
			continue
		}
		key := string(target.Kind) + ":" + target.ID
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		targets = append(targets, target)
	}
	// PageFilterIDs is the canonical page filter scope. Keep any filter not
	// represented by a filter-card placement deterministic as well.
	for _, filterID := range definition.PageFilterIDs(page.ID) {
		key := string(TargetFilterOptions) + ":" + filterID
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		targets = append(targets, Target{Kind: TargetFilterOptions, ID: filterID})
	}
	return RefreshPlan{Command: commandName, Targets: targets}
}

func (s Service) selectionTargets(request Request, sourceKind, sourceID string) ([]Target, error) {
	definition, _, ok := s.Metrics.Report(request.DashboardID)
	if !ok {
		return nil, fmt.Errorf("dashboard %q is not published", request.DashboardID)
	}
	var ids []string
	switch sourceKind {
	case "visual":
		visual, ok := definition.Visuals[sourceID]
		if !ok {
			return nil, fmt.Errorf("unknown source visual %q", sourceID)
		}
		ids = visual.Interaction.PointSelection.Targets
	case "table":
		table, ok := definition.Tables[sourceID]
		if !ok {
			return nil, fmt.Errorf("unknown source table %q", sourceID)
		}
		ids = table.Interaction.RowSelection.Targets
	default:
		return nil, fmt.Errorf("unknown source kind %q", sourceKind)
	}
	tableRequest := s.Metrics.NormalizeTableRequest(request.DashboardID, request.TableCommand).Reset()
	targets := make([]Target, 0, len(ids))
	seen := map[string]struct{}{}
	for _, id := range ids {
		var target Target
		if _, ok := definition.Visuals[id]; ok {
			target = Target{Kind: TargetVisual, ID: id}
		} else if _, ok := definition.Tables[id]; ok {
			requestForTable := tableRequest
			requestForTable.Table = id
			target = Target{Kind: TargetTable, ID: id, TableRequest: requestForTable}
		} else {
			return nil, fmt.Errorf("interaction references unknown target %q", id)
		}
		key := string(target.Kind) + ":" + target.ID
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		seen[key] = struct{}{}
		targets = append(targets, target)
	}
	return targets, nil
}
