package configspec

import (
	"sort"
	"strconv"
	"testing"
	"time"
)

func TestCatalogIsCompleteAndDeterministic(t *testing.T) {
	settings := Settings()
	if len(settings) < 40 {
		t.Fatalf("Settings() returned %d entries, want the complete application and tooling catalog", len(settings))
	}

	orderedNames := make([]string, 0, len(settings))
	knownNames := map[string]string{}
	fields := map[string]string{}
	for _, setting := range settings {
		if setting.Name == "" || setting.Description == "" || setting.Category == "" || setting.Scope == "" || setting.Lifecycle == "" {
			t.Fatalf("setting is missing required metadata: %#v", setting)
		}
		if previous, ok := knownNames[setting.Name]; ok {
			t.Fatalf("duplicate environment variable %s on %s and %s", setting.Name, previous, setting.Field)
		}
		knownNames[setting.Name] = setting.Field
		if setting.Runtime && setting.Field == "" {
			t.Fatalf("runtime setting %s has no generated Config field", setting.Name)
		}
		if setting.Runtime {
			if previous, ok := fields[setting.Field]; ok {
				t.Fatalf("generated Config field %s is shared by %s and %s", setting.Field, previous, setting.Name)
			}
			fields[setting.Field] = setting.Name
		}
		if setting.Secret && setting.Example != "" && setting.Example != SecretPlaceholder {
			t.Fatalf("secret setting %s exposes a non-placeholder example", setting.Name)
		}
		if setting.Default != "" {
			switch setting.Type {
			case TypeBool:
				if _, err := strconv.ParseBool(setting.Default); err != nil {
					t.Fatalf("setting %s has invalid boolean default %q", setting.Name, setting.Default)
				}
			case TypeInt:
				if _, err := strconv.Atoi(setting.Default); err != nil {
					t.Fatalf("setting %s has invalid integer default %q", setting.Name, setting.Default)
				}
			case TypeDuration:
				if _, err := time.ParseDuration(setting.Default); err != nil {
					t.Fatalf("setting %s has invalid duration default %q", setting.Name, setting.Default)
				}
			}
		}
		orderedNames = append(orderedNames, setting.Name)
	}

	want := append([]string(nil), orderedNames...)
	sort.Strings(want)
	for index := range orderedNames {
		if orderedNames[index] != want[index] {
			t.Fatalf("catalog order is not stable at index %d: got %s, want %s", index, orderedNames[index], want[index])
		}
	}
}

func TestRulesOnlyReferenceCatalogSettings(t *testing.T) {
	known := map[string]struct{}{}
	for _, setting := range Settings() {
		known[setting.Name] = struct{}{}
	}
	for _, rule := range Rules() {
		if rule.ID == "" || rule.Description == "" || rule.Message == "" {
			t.Fatalf("rule is missing required metadata: %#v", rule)
		}
		for _, name := range rule.References() {
			if _, ok := known[name]; !ok {
				t.Fatalf("rule %s references unknown setting %s", rule.ID, name)
			}
		}
	}
}
