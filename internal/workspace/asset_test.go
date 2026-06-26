package workspace

import (
	"strings"
	"testing"
)

func TestNewAssetRequiresAllowedPayloadSchema(t *testing.T) {
	if _, err := NewAsset("test", "dep", AssetTypeDashboard, "sales", "", "Sales", "", "", map[string]any{}); err == nil {
		t.Fatal("NewAsset() error = nil, want missing payload schema failure")
	}
	if _, err := NewAsset("test", "dep", AssetTypeDashboard, "sales", "", "Sales", "", "visual.v1", map[string]any{}); err == nil || !strings.Contains(err.Error(), "want \"dashboard.v1\"") {
		t.Fatalf("NewAsset() error = %v, want unexpected payload schema failure", err)
	}
	if _, err := NewAsset("test", "dep", AssetType("unknown"), "sales", "", "Sales", "", "unknown.v1", map[string]any{}); err == nil || !strings.Contains(err.Error(), "not registered") {
		t.Fatalf("NewAsset() error = %v, want unregistered payload schema failure", err)
	}
	if _, err := NewAsset("test", "dep", AssetTypeDashboard, "sales", "", "Sales", "", "dashboard.v1", map[string]any{}); err != nil {
		t.Fatalf("NewAsset() error = %v, want allowed payload schema", err)
	}
}
