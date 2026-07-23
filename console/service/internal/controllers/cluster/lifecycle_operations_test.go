package cluster

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"postgresql-cluster-console/internal/configuration"
	"postgresql-cluster-console/internal/storage"

	"github.com/rs/zerolog"
)

func lifecycleFixture() (*backupOperationStorage, *configuration.Config) {
	store, cfg := backupFixture()
	store.cluster.Inventory = []byte(`{
		"all":{"children":{
			"master":{"hosts":{"10.0.0.1":{"hostname":"postgresql-1"}}},
			"replica":{"hosts":{
				"10.0.0.2":{"hostname":"postgresql-2"},
				"10.0.0.3":{"hostname":"postgresql-3"},
				"10.0.0.4":{"hostname":"postgresql-4","new_node":true}
			}},
			"etcd_cluster":{"hosts":{"dcs-1":{},"dcs-2":{},"dcs-3":{}}}
		}}
	}`)
	walContinuous := true
	store.evidence.WalContinuous = &walContinuous
	store.evidence.ObservedAt = time.Now().UTC()
	store.evidence.SchedulerOwners = []byte(`["10.0.0.1"]`)
	return store, cfg
}

func TestLifecycleAddAndRemoveBindStoredReplicaInventory(t *testing.T) {
	store, cfg := lifecycleFixture()
	handler := NewGuardedOperationsHandler(store, nil, nil, blockedPreflightWatcher{}, cfg, zerolog.Nop())

	add, err := handler.lifecyclePreflightState(context.Background(), store.cluster, storage.OperationTypeNodeAdd, "postgresql-4", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(add.blockers) != 0 || add.confirmation != "ADD REPLICA 10.0.0.4" {
		t.Fatalf("add blockers=%v confirmation=%q", add.blockers, add.confirmation)
	}
	envs, payload, playbook, err := handler.operationInputs(
		context.Background(), store.cluster, storage.OperationTypeNodeAdd, mustJSON(add.desired),
	)
	if err != nil {
		t.Fatal(err)
	}
	var inputs map[string]any
	if json.Unmarshal(payload, &inputs) != nil || len(envs) == 0 || playbook != lifecyclePlaybook ||
		inputs["lifecycle_target"] != "10.0.0.4" || inputs["lifecycle_operation"] != storage.OperationTypeNodeAdd {
		t.Fatalf("envs=%v playbook=%q inputs=%v", envs, playbook, inputs)
	}

	remove, err := handler.lifecyclePreflightState(context.Background(), store.cluster, storage.OperationTypeNodeRemove, "postgresql-3", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(remove.blockers) != 0 || remove.confirmation != "REMOVE REPLICA 10.0.0.3 DELETE LOCAL DATA" {
		t.Fatalf("remove blockers=%v confirmation=%q", remove.blockers, remove.confirmation)
	}
	store.servers[2].Role = "leader"
	changed, err := handler.lifecyclePreflightState(context.Background(), store.cluster, storage.OperationTypeNodeRemove, "postgresql-3", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(changed.blockers, ","), "selected inventory replica") {
		t.Fatalf("role-change blockers=%v", changed.blockers)
	}
}

func TestLifecycleConfigAllowsOnlySupportedNonSecretParameters(t *testing.T) {
	store, cfg := lifecycleFixture()
	handler := NewGuardedOperationsHandler(store, nil, nil, blockedPreflightWatcher{}, cfg, zerolog.Nop())
	params := lifecycleParams{PostgreSQLParameters: []postgresqlParameter{
		{Option: "work_mem", Value: "64MB"},
		{Option: "log_statement", Value: "ddl"},
	}}
	state, err := handler.lifecyclePreflightState(
		context.Background(), store.cluster, storage.OperationTypeConfigUpdate, "", mustJSON(params),
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(state.blockers) != 0 || state.confirmation != "CONFIGURE POSTGRESQL work_mem,log_statement" {
		t.Fatalf("blockers=%v confirmation=%q", state.blockers, state.confirmation)
	}
	rehydrated, err := operationParams(storage.OperationTypeConfigUpdate, mustJSON(state.desired))
	if err != nil || !sameJSON(rehydrated, mustJSON(params)) {
		t.Fatalf("rehydrated=%s err=%v", rehydrated, err)
	}
	_, payload, playbook, err := handler.operationInputs(
		context.Background(), store.cluster, storage.OperationTypeConfigUpdate, mustJSON(state.desired),
	)
	if err != nil {
		t.Fatal(err)
	}
	var inputs map[string]any
	if json.Unmarshal(payload, &inputs) != nil || playbook != lifecyclePlaybook ||
		len(inputs["postgresql_parameters_overrides"].([]any)) != 2 {
		t.Fatalf("playbook=%q inputs=%v", playbook, inputs)
	}
	for _, rejected := range []lifecycleParams{
		{PostgreSQLParameters: []postgresqlParameter{{Option: "password_encryption", Value: "scram-sha-256"}}},
		{PostgreSQLParameters: []postgresqlParameter{{Option: "work_mem", Value: "secret"}}},
		{PostgreSQLParameters: []postgresqlParameter{{Option: "work_mem", Value: "64MB"}, {Option: "work_mem", Value: "32MB"}}},
	} {
		if validLifecycleParameters(rejected.PostgreSQLParameters) {
			t.Fatalf("accepted unsupported parameters: %+v", rejected)
		}
	}
}
