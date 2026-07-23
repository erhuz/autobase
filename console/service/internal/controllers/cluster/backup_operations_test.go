package cluster

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"postgresql-cluster-console/internal/configuration"
	"postgresql-cluster-console/internal/storage"
	"postgresql-cluster-console/models"
	"postgresql-cluster-console/pkg/tracer"
	clusterapi "postgresql-cluster-console/restapi/operations/cluster"

	"github.com/rs/zerolog"
)

type backupOperationStorage struct {
	*guardedOperationStorage
	evidence *storage.BackupEvidence
}

func (s *backupOperationStorage) GetBackupEvidence(context.Context, int64) (*storage.BackupEvidence, error) {
	return s.evidence, nil
}

func backupFixture() (*backupOperationStorage, *configuration.Config) {
	store, _ := switchoverFixture()
	store.cluster.ExtraVars = []byte(`{"pgbackrest_install":true}`)
	now := time.Now().UTC()
	storeWithBackup := &backupOperationStorage{
		guardedOperationStorage: store,
		evidence: &storage.BackupEvidence{
			ObservedAt: now, RepositoryReachable: true, LatestFull: &now,
			Retention: []byte(`{"full":7}`), Locks: []byte(`[]`),
			SchedulerOwners: []byte(`["postgresql-1"]`), FreshnessSeconds: 86400,
		},
	}
	cfg := &configuration.Config{}
	cfg.Backup.RunEvery = 5 * time.Minute
	return storeWithBackup, cfg
}

func backupPreflight(t *testing.T, handler *guardedOperationsHandler, store *backupOperationStorage, operationType string) {
	t.Helper()
	response := handler.HandlePreflight(clusterapi.PostClustersIDPreflightsParams{
		ID: 5, HTTPRequest: httptest.NewRequest("POST", "/clusters/5/preflights", nil),
		Body: &models.RequestOperationPreflight{Type: &operationType},
	})
	if _, ok := response.(*clusterapi.PostClustersIDPreflightsCreated); !ok {
		t.Fatalf("preflight response=%#v", response)
	}
}

func TestBackupPreflightRejectsDuplicateSchedulerOwners(t *testing.T) {
	store, cfg := backupFixture()
	store.evidence.SchedulerOwners = []byte(`["postgresql-1","postgresql-2"]`)
	handler := NewGuardedOperationsHandler(store, nil, nil, blockedPreflightWatcher{}, cfg, zerolog.Nop())
	backupPreflight(t, handler, store, storage.OperationTypeBackupFull)
	if !strings.Contains(string(store.preflight.Blockers), "exactly one scheduler owner") {
		t.Fatalf("blockers=%s", store.preflight.Blockers)
	}
}

func TestBackupLaunchesFixedFullAndDifferentialAutomation(t *testing.T) {
	for _, operationType := range []string{storage.OperationTypeBackupFull, storage.OperationTypeBackupDiff} {
		t.Run(operationType, func(t *testing.T) {
			store, cfg := backupFixture()
			docker := &operationDocker{}
			logs := &operationLogs{}
			handler := NewGuardedOperationsHandler(store, docker, logs, blockedPreflightWatcher{}, cfg, zerolog.Nop())
			backupPreflight(t, handler, store, operationType)
			request := httptest.NewRequest("POST", "/clusters/5/operations", nil)
			request = request.WithContext(context.WithValue(request.Context(), tracer.CtxCidKey{}, "test-cid"))
			response := handler.HandleOperation(clusterapi.PostClustersIDOperationsParams{
				ID: 5, HTTPRequest: request,
				Body: &models.RequestOperationStart{
					PreflightID: &store.preflight.ID, Confirmation: &store.preflight.Confirmation,
				},
			})
			if _, ok := response.(*clusterapi.PostClustersIDOperationsAccepted); !ok {
				t.Fatalf("operation response=%#v", response)
			}
			var extraVars map[string]any
			if err := json.Unmarshal([]byte(docker.config.ExtraVars), &extraVars); err != nil {
				t.Fatal(err)
			}
			expected := strings.TrimPrefix(operationType, "backup_")
			if store.reserved == nil || store.reserved.Type != operationType ||
				docker.config.Playbook != backupPlaybook ||
				extraVars["backup_operation_type"] != expected ||
				extraVars["pgbackrest_scheduler_host"] != "postgresql-1" ||
				docker.calls != 1 || logs.calls != 1 {
				t.Fatalf("reserved=%+v config=%+v vars=%+v", store.reserved, docker.config, extraVars)
			}
		})
	}
}
