package watcher

import (
	"testing"
	"time"

	"postgresql-cluster-console/internal/storage"
)

func TestBindRestoreEvidenceToReservedRecoveryState(t *testing.T) {
	verifiedAt := time.Date(2026, 7, 23, 10, 1, 0, 0, time.UTC)
	for _, operationType := range []string{storage.OperationTypeRestore, storage.OperationTypePITR} {
		op := &storage.Operation{
			ClusterID: 5,
			Type:      operationType,
			SanitizedParams: []byte(`{
				"action":"` + operationType + `",
				"source_cluster":"source",
				"source_cluster_id":6,
				"recovery_cluster":"recovery",
				"recovery_cluster_id":5
			}`),
		}
		evidence := &storage.RestoreEvidence{
			VerifiedAt: verifiedAt, SourceCluster: "source",
			RecoveryCluster: "recovery", Operation: operationType,
		}
		sourceID, err := bindRestoreEvidence(op, &storage.Cluster{ID: 5, Name: "recovery"}, evidence)
		if err != nil || sourceID != 6 {
			t.Fatalf("%s source = %d, %v", operationType, sourceID, err)
		}

		evidence.SourceCluster = "other"
		if _, err = bindRestoreEvidence(op, &storage.Cluster{ID: 5, Name: "recovery"}, evidence); err == nil {
			t.Fatalf("%s mismatched restore evidence was accepted", operationType)
		}
	}
}
