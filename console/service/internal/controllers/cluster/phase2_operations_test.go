package cluster

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"postgresql-cluster-console/internal/storage"

	"github.com/rs/zerolog"
)

func phase2Fixture() *backupOperationStorage {
	store, _ := backupFixture()
	store.cluster.PostgreVersion = 16
	store.cluster.ExtraVars = []byte(`{
		"pgbackrest_install":true,
		"dcs_type":"etcd",
		"patroni_maximum_lag_on_failover":100
	}`)
	store.cluster.Inventory = []byte(`{
		"all":{"children":{
			"master":{"hosts":{"10.0.0.1":{"hostname":"postgresql-1"}}},
			"replica":{"hosts":{
				"10.0.0.2":{"hostname":"postgresql-2"},
				"10.0.0.3":{"hostname":"postgresql-3"}
			}},
			"etcd_cluster":{"hosts":{"dcs-1":{},"dcs-2":{},"dcs-3":{}}}
		}}
	}`)
	return store
}

func TestPhase2RollingUpdateAndPostgreSQLUpgradeUseFixedAutomation(t *testing.T) {
	store := phase2Fixture()
	_, cfg := backupFixture()
	handler := NewGuardedOperationsHandler(store, nil, nil, blockedPreflightWatcher{}, cfg, zerolog.Nop())

	update, err := handler.phase2PreflightState(
		context.Background(), store.cluster, storage.OperationTypeRollingUpdate, "",
		mustJSON(phase2Params{UpdateTarget: "postgres"}),
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(update.blockers) != 0 || update.confirmation != "ROLLING UPDATE postgres" {
		t.Fatalf("update blockers=%v confirmation=%q", update.blockers, update.confirmation)
	}
	_, payload, playbook, err := handler.operationInputs(
		context.Background(), store.cluster, storage.OperationTypeRollingUpdate, mustJSON(update.desired),
	)
	if err != nil {
		t.Fatal(err)
	}
	var inputs map[string]any
	if json.Unmarshal(payload, &inputs) != nil || playbook != phase2Playbook ||
		inputs["phase2_operation"] != storage.OperationTypeRollingUpdate || inputs["target"] != "postgres" {
		t.Fatalf("playbook=%q inputs=%v", playbook, inputs)
	}

	upgrade, err := handler.phase2PreflightState(
		context.Background(), store.cluster, storage.OperationTypePostgreSQLUpgrade, "",
		mustJSON(phase2Params{PostgreSQLVersion: 17}),
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(upgrade.blockers) != 0 || upgrade.confirmation != "UPGRADE POSTGRESQL 16 TO 17 WITH DOWNTIME" {
		t.Fatalf("upgrade blockers=%v confirmation=%q", upgrade.blockers, upgrade.confirmation)
	}
	_, payload, playbook, err = handler.operationInputs(
		context.Background(), store.cluster, storage.OperationTypePostgreSQLUpgrade, mustJSON(upgrade.desired),
	)
	if err != nil {
		t.Fatal(err)
	}
	if json.Unmarshal(payload, &inputs) != nil || playbook != phase2Playbook ||
		inputs["pg_old_version"] != float64(16) || inputs["pg_new_version"] != float64(17) {
		t.Fatalf("playbook=%q inputs=%v", playbook, inputs)
	}
}

func TestPhase2EmergencyFailoverRequiresNoHealthyLeaderAndRetainedReplica(t *testing.T) {
	store := phase2Fixture()
	_, cfg := backupFixture()
	store.cluster.Status = storage.ClusterStatusUnhealthy
	store.servers[1].Status = "failed"
	handler := NewGuardedOperationsHandler(store, nil, nil, blockedPreflightWatcher{}, cfg, zerolog.Nop())

	state, err := handler.phase2PreflightState(
		context.Background(), store.cluster, storage.OperationTypeEmergencyFailover, "postgresql-2", nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(state.blockers) != 0 ||
		state.confirmation != "EMERGENCY FAILOVER TO postgresql-2 ACCEPT POSSIBLE DATA LOSS" {
		t.Fatalf("blockers=%v confirmation=%q", state.blockers, state.confirmation)
	}
	_, payload, playbook, err := handler.operationInputs(
		context.Background(), store.cluster, storage.OperationTypeEmergencyFailover, mustJSON(state.desired),
	)
	if err != nil {
		t.Fatal(err)
	}
	var inputs map[string]any
	if json.Unmarshal(payload, &inputs) != nil || playbook != phase2Playbook ||
		inputs["emergency_failover_candidate"] != "postgresql-2" ||
		inputs["emergency_failover_inventory_target"] != "10.0.0.2" {
		t.Fatalf("playbook=%q inputs=%v", playbook, inputs)
	}

	store.servers[1].Status = "running"
	changed, err := handler.phase2PreflightState(
		context.Background(), store.cluster, storage.OperationTypeEmergencyFailover, "postgresql-2", nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(changed.blockers, ","), "no healthy leader") {
		t.Fatalf("leader blockers=%v", changed.blockers)
	}

	store.servers[1].Status = "failed"
	store.servers[2].Status = "failed"
	changed, err = handler.phase2PreflightState(
		context.Background(), store.cluster, storage.OperationTypeEmergencyFailover, "postgresql-2", nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(changed.blockers, ","), "healthy failover capacity retained") {
		t.Fatalf("capacity blockers=%v", changed.blockers)
	}
}

func TestPhase2RejectsUnsupportedUpdateAndUpgradeInputs(t *testing.T) {
	store := phase2Fixture()
	_, cfg := backupFixture()
	handler := NewGuardedOperationsHandler(store, nil, nil, blockedPreflightWatcher{}, cfg, zerolog.Nop())

	update, err := handler.phase2PreflightState(
		context.Background(), store.cluster, storage.OperationTypeRollingUpdate, "",
		mustJSON(phase2Params{UpdateTarget: "arbitrary-package"}),
	)
	if err != nil {
		t.Fatal(err)
	}
	upgrade, err := handler.phase2PreflightState(
		context.Background(), store.cluster, storage.OperationTypePostgreSQLUpgrade, "",
		mustJSON(phase2Params{PostgreSQLVersion: 16}),
	)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(update.blockers, ","), "supported fixed update target") ||
		!strings.Contains(strings.Join(upgrade.blockers, ","), "supported forward PostgreSQL version") {
		t.Fatalf("update=%v upgrade=%v", update.blockers, upgrade.blockers)
	}
}
