package http

import (
	"testing"

	"github.com/Yacobolo/leapview/internal/dashboard"
	"github.com/Yacobolo/leapview/internal/dashboard/command"
)

func TestVisualWindowRefreshUsesPerVisualRequestOrdering(t *testing.T) {
	prepared := command.PreparedRefresh{
		Plan: command.RefreshPlan{Command: "visual_window", Targets: []command.Target{{
			Kind: command.TargetTable,
			ID:   "orders",
			TableRequest: dashboard.TableRequest{
				Table: "orders", Block: "b", Start: 500, Count: 50, RequestSeq: 12, ResetVersion: 4,
			},
		}}},
	}
	preparation := streamPreparation(prepared)
	if preparation.SequenceKey != "window:orders" || preparation.Sequence != 12 || preparation.SequenceEpoch != 4 {
		t.Fatalf("sequence = %q/%d/%d", preparation.SequenceKey, preparation.Sequence, preparation.SequenceEpoch)
	}
}
