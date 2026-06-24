package dashboard

import "testing"

func TestTableRequestWithDefaultsClampsRuntimePolicy(t *testing.T) {
	request := TableRequest{
		Table:      "orders",
		Block:      "z",
		Start:      -20,
		Count:      TableMaxRequestCount + 500,
		RequestSeq: -8,
		Sort:       TableSort{Key: "revenue", Direction: "sideways"},
	}.WithDefaults()

	if request.Block != "all" {
		t.Fatalf("block = %q, want all", request.Block)
	}
	if request.Start != 0 {
		t.Fatalf("start = %d, want 0", request.Start)
	}
	if request.Count != TableMaxRequestCount {
		t.Fatalf("count = %d, want %d", request.Count, TableMaxRequestCount)
	}
	if request.RequestSeq != 0 {
		t.Fatalf("request seq = %d, want 0", request.RequestSeq)
	}
	if request.Sort.Direction != "desc" {
		t.Fatalf("sort direction = %q, want desc", request.Sort.Direction)
	}
}

func TestTableRequestResetRequestsInitialBlocks(t *testing.T) {
	request := TableRequest{
		Table:        "orders",
		Block:        "b",
		Start:        600,
		Count:        500,
		RequestSeq:   42,
		ResetVersion: 4,
		Sort:         TableSort{Key: "revenue", Direction: "asc"},
	}.Reset()

	if request.Block != "all" || request.Start != 0 || request.Count != TableChunkSize {
		t.Fatalf("reset request = %#v, want all at top with chunk size", request)
	}
	if request.ResetVersion != 5 {
		t.Fatalf("reset version = %d, want 5", request.ResetVersion)
	}
	if request.RequestSeq != 42 {
		t.Fatalf("request seq = %d, want 42", request.RequestSeq)
	}
	if request.Sort.Key != "revenue" || request.Sort.Direction != "asc" {
		t.Fatalf("reset sort = %#v, want preserved revenue asc", request.Sort)
	}
}

func TestApplyInteractionReplacesOnlySourceSelection(t *testing.T) {
	filters := Filters{
		Selections: []InteractionSelection{
			interactionSelectionFixture("visual", "orders", "point_selection", "orders.status", "delivered"),
			interactionSelectionFixture("table", "orders_table", "row_selection", "orders.order_id", "o1"),
		},
	}

	next := filters.ApplyInteraction(InteractionCommand{
		SourceKind:      "visual",
		SourceID:        "orders",
		InteractionKind: "point_selection",
		Action:          "set",
		Mode:            "single",
		Toggle:          true,
		Mappings:        []InteractionCommandMapping{{Field: "orders.status", Value: "shipped", Label: "shipped"}},
	})

	if len(next.Selections) != 2 {
		t.Fatalf("selection count = %d, want 2: %#v", len(next.Selections), next.Selections)
	}
	if got := selectionValues(next, "visual", "orders", "orders.status"); len(got) != 1 || got[0] != "shipped" {
		t.Fatalf("orders visual values = %#v, want [shipped]", got)
	}
	if got := selectionValues(next, "table", "orders_table", "orders.order_id"); len(got) != 1 || got[0] != "o1" {
		t.Fatalf("orders table values = %#v, want [o1]", got)
	}
}

func TestApplyInteractionClearsOnlySourceSelection(t *testing.T) {
	filters := Filters{
		Selections: []InteractionSelection{
			interactionSelectionFixture("visual", "orders", "point_selection", "orders.status", "delivered"),
			interactionSelectionFixture("table", "orders_table", "row_selection", "orders.order_id", "o1"),
		},
	}

	next := filters.ApplyInteraction(InteractionCommand{
		SourceKind:      "visual",
		SourceID:        "orders",
		InteractionKind: "point_selection",
		Action:          "clear",
	})

	if len(next.Selections) != 1 {
		t.Fatalf("selection count = %d, want 1: %#v", len(next.Selections), next.Selections)
	}
	if got := selectionValues(next, "table", "orders_table", "orders.order_id"); len(got) != 1 || got[0] != "o1" {
		t.Fatalf("remaining table values = %#v, want [o1]", got)
	}
}

func interactionSelectionFixture(sourceKind, sourceID, interactionKind, field string, values ...string) InteractionSelection {
	return InteractionSelection{
		ID:              sourceKind + ":" + sourceID + ":" + interactionKind,
		SourceKind:      sourceKind,
		SourceID:        sourceID,
		InteractionKind: interactionKind,
		Mode:            "single",
		Mappings: []InteractionSelectionMapping{{
			Field:  field,
			Values: append([]string{}, values...),
			Label:  values[0],
		}},
	}
}

func selectionValues(filters Filters, sourceKind, sourceID, field string) []string {
	for _, selection := range filters.Selections {
		if selection.SourceKind != sourceKind || selection.SourceID != sourceID {
			continue
		}
		for _, mapping := range selection.Mappings {
			if mapping.Field == field {
				return mapping.Values
			}
		}
	}
	return nil
}
