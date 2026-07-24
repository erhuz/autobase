package cluster

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"postgresql-cluster-console/internal/storage"

	"github.com/rs/zerolog"
)

type recoveryOperationStorage struct {
	*backupOperationStorage
	source          *storage.Cluster
	sourceServers   []storage.Server
	recoveryServers []storage.Server
}

func (s *recoveryOperationStorage) GetCluster(_ context.Context, id int64) (*storage.Cluster, error) {
	if id == s.source.ID {
		return s.source, nil
	}
	return s.cluster, nil
}

func (s *recoveryOperationStorage) GetClusterByName(_ context.Context, name string) (*storage.Cluster, error) {
	if name == s.source.Name {
		return s.source, nil
	}
	return s.cluster, nil
}

func (s *recoveryOperationStorage) GetClusterServers(_ context.Context, id int64) ([]storage.Server, error) {
	if id == s.source.ID {
		return s.sourceServers, nil
	}
	return s.recoveryServers, nil
}

func (s *recoveryOperationStorage) GetBackupEvidence(_ context.Context, id int64) (*storage.BackupEvidence, error) {
	if id == s.source.ID {
		return s.evidence, nil
	}
	return nil, nil
}

func recoveryFixture() (*recoveryOperationStorage, time.Time) {
	store, _ := backupFixture()
	now := time.Now().UTC().Truncate(time.Second)
	credentialID := int64(7)
	walContinuous := true
	store.cluster = &storage.Cluster{
		ID: 5, ProjectID: 3, Name: "recovery-cluster",
		SecretID:  &credentialID,
		ExtraVars: []byte(`{"pgbackrest_install":true,"recovery_target":true}`),
		Inventory: []byte(`{
			"all":{"children":{
				"master":{"hosts":{"10.1.0.1":{"hostname":"recovery-1"}}},
				"replica":{"hosts":{"10.1.0.2":{"hostname":"recovery-2"}}},
				"etcd_cluster":{"hosts":{"recovery-dcs":{}}}
			}}
		}`),
	}
	source := &storage.Cluster{
		ID: 6, ProjectID: 3, Name: "source-cluster",
		ExtraVars: []byte(`{"pgbackrest_install":true,"pgbackrest_stanza":"source_stanza"}`),
		Inventory: []byte(`{
			"all":{"children":{
				"master":{"hosts":{"10.0.0.1":{"hostname":"source-1"}}},
				"replica":{"hosts":{"10.0.0.2":{"hostname":"source-2"}}},
				"etcd_cluster":{"hosts":{"source-dcs":{}}}
			}}
		}`),
	}
	store.evidence.ObservedAt = now
	latestFull := now.Add(-time.Hour)
	store.evidence.LatestFull = &latestFull
	store.evidence.WalContinuous = &walContinuous
	store.evidence.SchedulerOwners = []byte(`["source-1"]`)
	return &recoveryOperationStorage{
		backupOperationStorage: store,
		source:                 source,
		sourceServers: []storage.Server{
			{Name: "source-1", Role: "leader", Status: "running", UpdatedAt: &now},
			{Name: "source-2", Role: "replica", Status: "streaming", UpdatedAt: &now},
		},
		recoveryServers: []storage.Server{
			{Name: "recovery-1", Role: "leader", Status: "running", UpdatedAt: &now},
			{Name: "recovery-2", Role: "replica", Status: "streaming", UpdatedAt: &now},
		},
	}, now
}

func TestRecoveryRestoreAndPITRUseFixedIsolatedAutomation(t *testing.T) {
	for _, operationType := range []string{storage.OperationTypeRestore, storage.OperationTypePITR} {
		t.Run(operationType, func(t *testing.T) {
			store, now := recoveryFixture()
			_, cfg := backupFixture()
			handler := NewGuardedOperationsHandler(store, nil, nil, blockedPreflightWatcher{}, cfg, zerolog.Nop())
			params := recoveryParams{SourceCluster: store.source.Name}
			if operationType == storage.OperationTypePITR {
				params.RecoveryTargetTime = now.Add(-30 * time.Minute).Format(time.RFC3339)
			}
			state, err := handler.recoveryPreflightState(
				context.Background(), store.cluster, operationType, "", mustJSON(params),
			)
			if err != nil {
				t.Fatal(err)
			}
			if len(state.blockers) != 0 || len(state.affectedNodes) != 2 ||
				!strings.Contains(state.confirmation, "ISOLATED recovery-cluster") {
				t.Fatalf("blockers=%v affected=%v confirmation=%q", state.blockers, state.affectedNodes, state.confirmation)
			}
			envs, payload, playbook, err := handler.operationInputs(
				context.Background(), store.cluster, operationType, mustJSON(state.desired),
			)
			if err != nil {
				t.Fatal(err)
			}
			var inputs map[string]any
			if json.Unmarshal(payload, &inputs) != nil || playbook != recoveryPlaybook ||
				!containsString(envs, "ANSIBLE_RUN_TAGS=point_in_time_recovery") ||
				inputs["recovery_isolated"] != true ||
				inputs["cloud_provider"] != "" ||
				inputs["pgbackrest_patroni_cluster_clean_bootstrap"] != true ||
				!strings.HasPrefix(inputs["pgbackrest_patroni_cluster_restore_command"].(string),
					"/usr/bin/pgbackrest --stanza=source_stanza") {
				t.Fatalf("envs=%v playbook=%q inputs=%v", envs, playbook, inputs)
			}
		})
	}
}

func TestRecoveryRejectsSourceOverwriteAndSharedInventory(t *testing.T) {
	store, _ := recoveryFixture()
	_, cfg := backupFixture()
	handler := NewGuardedOperationsHandler(store, nil, nil, blockedPreflightWatcher{}, cfg, zerolog.Nop())

	sourceOverwrite, err := handler.recoveryPreflightState(
		context.Background(), store.cluster, storage.OperationTypeRestore, "",
		mustJSON(recoveryParams{SourceCluster: store.cluster.Name}),
	)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(sourceOverwrite.blockers, ","), "source and recovery clusters differ") {
		t.Fatalf("source overwrite blockers=%v", sourceOverwrite.blockers)
	}

	store.cluster.Inventory = store.source.Inventory
	shared, err := handler.recoveryPreflightState(
		context.Background(), store.cluster, storage.OperationTypeRestore, "",
		mustJSON(recoveryParams{SourceCluster: store.source.Name}),
	)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(shared.blockers, ","), "source and recovery inventories disjoint") {
		t.Fatalf("shared inventory blockers=%v", shared.blockers)
	}
}

func TestRecoveryBindsFreshSourceRolesBeforeRestore(t *testing.T) {
	store, _ := recoveryFixture()
	_, cfg := backupFixture()
	handler := NewGuardedOperationsHandler(store, nil, nil, blockedPreflightWatcher{}, cfg, zerolog.Nop())
	params := mustJSON(recoveryParams{SourceCluster: store.source.Name})

	before, err := handler.recoveryPreflightState(
		context.Background(), store.cluster, storage.OperationTypeRestore, "", params,
	)
	if err != nil {
		t.Fatal(err)
	}
	store.sourceServers[0].Role = "replica"
	store.sourceServers[1].Role = "leader"
	after, err := handler.recoveryPreflightState(
		context.Background(), store.cluster, storage.OperationTypeRestore, "", params,
	)
	if err != nil {
		t.Fatal(err)
	}
	if before.topologyHash == after.topologyHash {
		t.Fatal("source role change did not invalidate recovery preflight")
	}

	stale := time.Now().UTC().Add(-time.Minute)
	store.sourceServers[0].UpdatedAt = &stale
	after, err = handler.recoveryPreflightState(
		context.Background(), store.cluster, storage.OperationTypeRestore, "", params,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(after.blockers, ","), "source roles refreshed now") {
		t.Fatalf("stale role blockers=%v", after.blockers)
	}
}

func containsString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}
