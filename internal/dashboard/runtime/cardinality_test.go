package runtime

import (
	"testing"

	"github.com/Yacobolo/leapview/internal/dashboard"
)

func TestConsumerTableCountIsExplicitlyOptIn(t *testing.T) {
	initial := dashboard.TableRequest{Block: "a", Start: 0}.WithDefaults()
	if consumerTableNeedsExactCount("selection", initial, false) {
		t.Fatal("bounded table scheduled an implicit exact count")
	}
	if !consumerTableNeedsExactCount("selection", initial, true) {
		t.Fatal("exact table did not schedule its requested count")
	}
	if consumerTableNeedsExactCount("visual_window", initial, true) {
		t.Fatal("scrolling window scheduled a count")
	}
}
