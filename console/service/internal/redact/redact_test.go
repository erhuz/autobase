package redact

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSensitiveJSONAndLogTextAreRedacted(t *testing.T) {
	input := []byte(`{"secret_id":7,"nested":{"query_analytics_monitor_password":"open sesame","client_ip":"192.0.2.1"},"plan":["restart replica"]}`)
	var got map[string]any
	if err := json.Unmarshal(JSON(input), &got); err != nil {
		t.Fatal(err)
	}
	nested := got["nested"].(map[string]any)
	if got["secret_id"] != float64(7) || nested["query_analytics_monitor_password"] != Mask ||
		nested["client_ip"] != Mask || got["plan"].([]any)[0] != "restart replica" {
		t.Fatalf("redacted JSON = %#v", got)
	}

	log := Text(`PASSWORD=open-sesame Authorization: Bearer abc123 postgres://monitor:hunter2@db/query`)
	for _, secret := range []string{"open-sesame", "abc123", "hunter2"} {
		if strings.Contains(log, secret) {
			t.Fatalf("secret %q remains in %q", secret, log)
		}
	}
}
