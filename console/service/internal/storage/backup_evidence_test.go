package storage

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestDecodeBackupEvidence(t *testing.T) {
	raw := `{"observed_at":"2026-07-23T10:00:00Z","repository_reachable":true,"latest_full":"2026-07-23T09:00:00Z","latest_differential":null,"retention":{"full":7},"wal_continuous":true,"locks":[],"scheduler_owners":["postgresql-1"],"freshness_seconds":86400,"restore_tested_at":null}`
	message := `ok: [postgresql-1] => {"msg":"` + BackupEvidenceMarker + base64.StdEncoding.EncodeToString([]byte(raw)) + `"}`
	evidence, err := DecodeBackupEvidence(message, 5)
	if err != nil {
		t.Fatal(err)
	}
	if evidence.ClusterID != 5 || !evidence.RepositoryReachable || evidence.LatestFull == nil ||
		!strings.Contains(string(evidence.Retention), `"full":7`) ||
		string(evidence.SchedulerOwners) != `["postgresql-1"]` {
		t.Fatalf("evidence = %+v", evidence)
	}
}
