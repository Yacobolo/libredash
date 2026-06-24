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

func TestApplyInteractionReplaceUpdatesOnlySourceSelection(t *testing.T) {
	filters := Filters{
		Selections: []InteractionSelection{
			interactionSelectionFixture("visual", "orders", "point_selection", entryFixture("orders.status", "delivered")),
			interactionSelectionFixture("table", "orders_table", "row_selection", entryFixture("orders.order_id", "o1")),
		},
	}

	next := filters.ApplyInteraction(InteractionCommand{
		SourceKind:      "visual",
		SourceID:        "orders",
		InteractionKind: "point_selection",
		Action:          "replace",
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
			interactionSelectionFixture("visual", "orders", "point_selection", entryFixture("orders.status", "delivered")),
			interactionSelectionFixture("table", "orders_table", "row_selection", entryFixture("orders.order_id", "o1")),
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

func TestApplyInteractionReplaceStoresSingleSelectionAsFullTupleEntry(t *testing.T) {
	next := Filters{}.ApplyInteraction(InteractionCommand{
		SourceKind:      "visual",
		SourceID:        "state_status",
		InteractionKind: "point_selection",
		Action:          "replace",
		Mappings: []InteractionCommandMapping{
			{Field: "customers.state", Value: "SP", Label: "Sao Paulo"},
			{Field: "orders.status", Value: "delivered", Label: "Delivered"},
		},
	})

	if len(next.Selections) != 1 {
		t.Fatalf("selection count = %d, want 1: %#v", len(next.Selections), next.Selections)
	}
	selection := next.Selections[0]
	if len(selection.Entries) != 1 {
		t.Fatalf("entry count = %d, want 1: %#v", len(selection.Entries), selection.Entries)
	}
	entry := selection.Entries[0]
	if len(entry.Mappings) != 2 {
		t.Fatalf("entry mappings = %#v, want two tuple mappings", entry.Mappings)
	}
	if entry.Mappings[0].Field != "customers.state" || entry.Mappings[0].Value != "SP" {
		t.Fatalf("first tuple mapping = %#v, want customers.state=SP", entry.Mappings[0])
	}
	if entry.Mappings[1].Field != "orders.status" || entry.Mappings[1].Value != "delivered" {
		t.Fatalf("second tuple mapping = %#v, want orders.status=delivered", entry.Mappings[1])
	}
}

func TestApplyInteractionMultiTogglesExactTupleEntries(t *testing.T) {
	filters := Filters{}
	first := []InteractionCommandMapping{
		{Field: "customers.state", Value: "SP", Label: "SP"},
		{Field: "orders.status", Value: "delivered", Label: "Delivered"},
	}
	second := []InteractionCommandMapping{
		{Field: "customers.state", Value: "RJ", Label: "RJ"},
		{Field: "orders.status", Value: "shipped", Label: "Shipped"},
	}

	filters = filters.ApplyInteraction(interactionCommandFixture(first))
	filters = filters.ApplyInteraction(interactionCommandFixture(second))

	if len(filters.Selections) != 1 || len(filters.Selections[0].Entries) != 2 {
		t.Fatalf("selections = %#v, want one selection with two tuple entries", filters.Selections)
	}
	if got := selectionEntryValues(filters.Selections[0], "customers.state"); len(got) != 2 || got[0] != "SP" || got[1] != "RJ" {
		t.Fatalf("state tuple values = %#v, want [SP RJ]", got)
	}

	filters = filters.ApplyInteraction(interactionCommandFixture(first))
	if len(filters.Selections) != 1 || len(filters.Selections[0].Entries) != 1 {
		t.Fatalf("after exact tuple toggle selections = %#v, want one remaining tuple", filters.Selections)
	}
	remaining := filters.Selections[0].Entries[0]
	if got := tupleValue(remaining, "customers.state"); got != "RJ" {
		t.Fatalf("remaining tuple state = %q, want RJ", got)
	}
	if got := tupleValue(remaining, "orders.status"); got != "shipped" {
		t.Fatalf("remaining tuple status = %q, want shipped", got)
	}
}

func TestApplyInteractionSetAccumulatesEntries(t *testing.T) {
	filters := Filters{}
	filters = filters.ApplyInteraction(InteractionCommand{
		SourceKind:      "visual",
		SourceID:        "orders",
		InteractionKind: "point_selection",
		Action:          "set",
		Toggle:          true,
		Mappings:        []InteractionCommandMapping{{Field: "orders.status", Value: "delivered", Label: "delivered"}},
	})
	filters = filters.ApplyInteraction(InteractionCommand{
		SourceKind:      "visual",
		SourceID:        "orders",
		InteractionKind: "point_selection",
		Action:          "set",
		Toggle:          true,
		Mappings:        []InteractionCommandMapping{{Field: "orders.status", Value: "shipped", Label: "shipped"}},
	})

	if len(filters.Selections) != 1 {
		t.Fatalf("selection count = %d, want 1: %#v", len(filters.Selections), filters.Selections)
	}
	if got := selectionValues(filters, "visual", "orders", "orders.status"); len(got) != 2 || got[0] != "delivered" || got[1] != "shipped" {
		t.Fatalf("set values = %#v, want [delivered shipped]", got)
	}
}

func TestApplyInteractionReplaceReplacesEntries(t *testing.T) {
	filters := Filters{}.ApplyInteraction(InteractionCommand{
		SourceKind:      "table",
		SourceID:        "orders_table",
		InteractionKind: "row_selection",
		Action:          "set",
		Toggle:          true,
		Mappings:        []InteractionCommandMapping{{Field: "orders.order_id", Value: "o1", Label: "o1"}},
	})
	filters = filters.ApplyInteraction(InteractionCommand{
		SourceKind:      "table",
		SourceID:        "orders_table",
		InteractionKind: "row_selection",
		Action:          "set",
		Toggle:          true,
		Mappings:        []InteractionCommandMapping{{Field: "orders.order_id", Value: "o2", Label: "o2"}},
	})

	if got := selectionValues(filters, "table", "orders_table", "orders.order_id"); len(got) != 2 || got[0] != "o1" || got[1] != "o2" {
		t.Fatalf("multi toggle table values = %#v, want [o1 o2]", got)
	}

	filters = filters.ApplyInteraction(InteractionCommand{
		SourceKind:      "table",
		SourceID:        "orders_table",
		InteractionKind: "row_selection",
		Action:          "replace",
		Toggle:          false,
		Mappings:        []InteractionCommandMapping{{Field: "orders.order_id", Value: "o3", Label: "o3"}},
	})

	if len(filters.Selections) != 1 {
		t.Fatalf("selection count = %d, want 1: %#v", len(filters.Selections), filters.Selections)
	}
	if got := selectionValues(filters, "table", "orders_table", "orders.order_id"); len(got) != 1 || got[0] != "o3" {
		t.Fatalf("replace table values = %#v, want [o3]", got)
	}
}

func interactionCommandFixture(mappings []InteractionCommandMapping) InteractionCommand {
	return InteractionCommand{
		SourceKind:      "visual",
		SourceID:        "state_status",
		InteractionKind: "point_selection",
		Action:          "set",
		Toggle:          true,
		Mappings:        mappings,
	}
}

func interactionSelectionFixture(sourceKind, sourceID, interactionKind string, entries ...InteractionSelectionEntry) InteractionSelection {
	return InteractionSelection{
		ID:              sourceKind + ":" + sourceID + ":" + interactionKind,
		SourceKind:      sourceKind,
		SourceID:        sourceID,
		InteractionKind: interactionKind,
		Entries:         append([]InteractionSelectionEntry{}, entries...),
	}
}

func entryFixture(field, value string) InteractionSelectionEntry {
	return InteractionSelectionEntry{
		Mappings: []InteractionSelectionMapping{{
			Field: field,
			Value: value,
			Label: value,
		}},
		Label: value,
	}
}

func selectionValues(filters Filters, sourceKind, sourceID, field string) []string {
	for _, selection := range filters.Selections {
		if selection.SourceKind != sourceKind || selection.SourceID != sourceID {
			continue
		}
		return selectionEntryValues(selection, field)
	}
	return nil
}

func selectionEntryValues(selection InteractionSelection, field string) []string {
	values := []string{}
	for _, entry := range selection.Entries {
		for _, mapping := range entry.Mappings {
			if mapping.Field == field {
				values = append(values, mapping.Value)
			}
		}
	}
	return values
}

func tupleValue(entry InteractionSelectionEntry, field string) string {
	for _, mapping := range entry.Mappings {
		if mapping.Field == field {
			return mapping.Value
		}
	}
	return ""
}
