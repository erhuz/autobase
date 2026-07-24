package cluster

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"postgresql-cluster-console/internal/storage"
)

func TestExtensionAdminUsesSupportedNativeInputs(t *testing.T) {
	if !supportedOperationType(storage.OperationTypeExtensionAdmin) {
		t.Fatal("extension administration is not wired into guarded operations")
	}
	store, handler := databaseAdminFixture()
	change := phase3ServicesParams{
		ExtensionAction: extensionPresent,
		Extension:       "pgvector",
		Database:        "app_db",
		Schema:          "public",
	}
	state, err := handler.phase3ServicesPreflightState(
		context.Background(), store.cluster, storage.OperationTypeExtensionAdmin, "", mustJSON(change),
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(state.blockers) != 0 || len(state.affectedNodes) != 3 {
		t.Fatalf("blockers=%v affected=%v", state.blockers, state.affectedNodes)
	}
	roundTrip, err := operationParams(storage.OperationTypeExtensionAdmin, mustJSON(state.desired))
	if err != nil || !sameJSON(roundTrip, mustJSON(change)) {
		t.Fatalf("params=%s err=%v", roundTrip, err)
	}

	envs, payload, playbook, err := handler.operationInputs(
		context.Background(), store.cluster, storage.OperationTypeExtensionAdmin, mustJSON(state.desired),
	)
	if err != nil {
		t.Fatal(err)
	}
	var inputs map[string]any
	if json.Unmarshal(payload, &inputs) != nil || playbook != phase3ServicesPlaybook ||
		!containsString(envs, "ANSIBLE_RUN_TAGS=phase3_services,postgresql_extensions,pgvector") ||
		inputs["phase3_extension"] != "vector" || inputs["enable_pgvector"] != true ||
		inputs["cloud_provider"] != "" || strings.Contains(string(payload), "password") {
		t.Fatalf("envs=%v playbook=%q inputs=%v", envs, playbook, inputs)
	}
	items, ok := inputs["postgresql_extensions"].([]any)
	if !ok || len(items) != 1 {
		t.Fatalf("postgresql_extensions=%#v", inputs["postgresql_extensions"])
	}
	item, ok := items[0].(map[string]any)
	if !ok || item["ext"] != "pgvector" || item["db"] != "app_db" ||
		item["schema"] != "public" || item["state"] != "present" || item["cascade"] != false {
		t.Fatalf("extension item=%#v", items[0])
	}
}

func TestPgBouncerAdminPreservesPoolsAndOmittedLimits(t *testing.T) {
	if !supportedOperationType(storage.OperationTypePgBouncerAdmin) {
		t.Fatal("PgBouncer administration is not wired into guarded operations")
	}
	store, handler := databaseAdminFixture()
	store.cluster.ExtraVars = []byte(`{
		"pgbouncer_install":true,
		"pgbouncer_pools":[
			{"name":"postgres","dbname":"postgres","pool_parameters":{"pool_size":100,"pool_mode":"session"}}
		],
		"pgbouncer_max_client_conn":5000,
		"pgbouncer_max_db_connections":500,
		"pgbouncer_default_pool_size":50,
		"pgbouncer_query_wait_timeout":90,
		"pgbouncer_processes":1,
		"postgresql_parameters":[{"option":"max_connections","value":"500"}],
		"postgresql_databases":[{"db":"app_db"}]
	}`)
	size := int32(40)
	change := phase3ServicesParams{
		PgBouncerAction: poolPresent,
		PoolName:        "app",
		Database:        "app_db",
		PoolSize:        &size,
		PoolMode:        "transaction",
	}
	state, err := handler.phase3ServicesPreflightState(
		context.Background(), store.cluster, storage.OperationTypePgBouncerAdmin, "", mustJSON(change),
	)
	if err != nil {
		t.Fatal(err)
	}
	desired, ok := state.desired.(phase3ServicesDesired)
	if len(state.blockers) != 0 || !ok || desired.PgBouncer == nil ||
		desired.PgBouncer.MaxClientConnections != 5000 {
		t.Fatalf("blockers=%v", state.blockers)
	}

	envs, payload, playbook, err := handler.operationInputs(
		context.Background(), store.cluster, storage.OperationTypePgBouncerAdmin, mustJSON(state.desired),
	)
	if err != nil {
		t.Fatal(err)
	}
	var inputs map[string]any
	if json.Unmarshal(payload, &inputs) != nil || playbook != phase3ServicesPlaybook ||
		!containsString(envs, "ANSIBLE_RUN_TAGS=phase3_services,pgbouncer_conf") ||
		inputs["pgbouncer_max_client_conn"] != float64(5000) ||
		inputs["pgbouncer_max_db_connections"] != float64(500) ||
		inputs["pgbouncer_default_pool_size"] != float64(50) ||
		inputs["pgbouncer_query_wait_timeout"] != float64(90) ||
		inputs["phase3_max_client_connections"] != float64(-1) {
		t.Fatalf("envs=%v playbook=%q inputs=%v", envs, playbook, inputs)
	}
	pools, ok := inputs["pgbouncer_pools"].([]any)
	if !ok || len(pools) != 2 {
		t.Fatalf("pgbouncer_pools=%#v", inputs["pgbouncer_pools"])
	}

	defaultSize := int32(60)
	next := mergePgBouncerConfig(pgbouncerConfig{
		MaxClientConnections: 5000, MaxDatabaseConnections: 500,
		DefaultPoolSize: 50, QueryWaitTimeout: 90,
	}, phase3ServicesParams{PgBouncerAction: poolLimitsUpdate, DefaultPoolSize: &defaultSize})
	if next.MaxClientConnections != 5000 || next.MaxDatabaseConnections != 500 ||
		next.DefaultPoolSize != 60 || next.QueryWaitTimeout != 90 {
		t.Fatalf("omitted limits were not preserved: %+v", next)
	}
}

func TestPhase3ServicesRejectUnsafeInputsAndCapacity(t *testing.T) {
	one := int32(1)
	invalid := []struct {
		operation string
		change    phase3ServicesParams
	}{
		{
			operation: storage.OperationTypeExtensionAdmin,
			change: phase3ServicesParams{
				ExtensionAction: extensionPresent, Extension: "pg_stat_monitor",
				Database: "app_db", Schema: "public",
			},
		},
		{
			operation: storage.OperationTypeExtensionAdmin,
			change: phase3ServicesParams{
				ExtensionAction: extensionPresent, Extension: "pg_cron",
				Database: "app_db", Schema: "public",
			},
		},
		{
			operation: storage.OperationTypePgBouncerAdmin,
			change: phase3ServicesParams{
				PgBouncerAction: poolPresent, PoolName: `app;drop`,
				Database: "app_db", PoolSize: &one, PoolMode: "statement",
			},
		},
	}
	for _, test := range invalid {
		if validPhase3ServicesParams(test.operation, test.change) {
			t.Errorf("accepted unsafe change: %s %+v", test.operation, test.change)
		}
	}

	store, handler := databaseAdminFixture()
	store.cluster.ExtraVars = []byte(`{
		"pgbouncer_install":true,
		"pgbouncer_pools":[
			{"name":"postgres","dbname":"postgres","pool_parameters":{"pool_size":50}}
		],
		"postgresql_parameters":[{"option":"max_connections","value":"100"}],
		"postgresql_databases":[{"db":"app_db"}]
	}`)
	size := int32(200)
	state, err := handler.phase3ServicesPreflightState(
		context.Background(), store.cluster, storage.OperationTypePgBouncerAdmin, "",
		mustJSON(phase3ServicesParams{
			PgBouncerAction: poolPresent, PoolName: "app",
			Database: "app_db", PoolSize: &size, PoolMode: "transaction",
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	if !containsString(state.blockers, "desired pool capacity within PostgreSQL max_connections") {
		t.Fatalf("missing capacity blocker: %v", state.blockers)
	}

	for _, parameters := range []any{
		map[string]any{"pool_size": 0},
		"pool_size=-1 pool_mode=transaction",
		"pool_size=invalid",
	} {
		if _, valid := configuredPoolSize(parameters, 100); valid {
			t.Errorf("accepted unsafe stored pool parameters: %#v", parameters)
		}
	}
}
