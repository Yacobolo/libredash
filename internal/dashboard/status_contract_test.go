package dashboard

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestStatusJSONDoesNotExposeDataDirectory(t *testing.T) {
	encoded, err := json.Marshal(Status{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "dataDirectory") {
		t.Fatalf("Status JSON = %s, must not expose runtime data directory", encoded)
	}
}
