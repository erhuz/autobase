package patroni

import (
	"encoding/json"
	"testing"
)

func TestV60MonitoringInfoDecodesDCSLastSeen(t *testing.T) {
	var info MonitoringInfo
	if err := json.Unmarshal([]byte(`{"state":"running","dcs_last_seen":1692356928}`), &info); err != nil {
		t.Fatal(err)
	}
	if info.DCSLastSeen != 1692356928 {
		t.Fatalf("dcs_last_seen = %d", info.DCSLastSeen)
	}
}
