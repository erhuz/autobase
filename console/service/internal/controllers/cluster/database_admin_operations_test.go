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

func databaseAdminFixture() (*guardedOperationStorage, *guardedOperationsHandler) {
	now := time.Now().UTC()
	credentialID := int64(7)
	superuserID := int64(8)
	store := &guardedOperationStorage{
		cluster: &storage.Cluster{
			ID: 5, ProjectID: 3, Name: "cluster-1", PostgreVersion: 16,
			SecretID: &credentialID, PostgresSuperuserSecretID: &superuserID,
			Inventory: []byte(`{
				"all":{"children":{
					"master":{"hosts":{"postgresql-1":{"hostname":"postgresql-1"}}},
					"replica":{"hosts":{
						"postgresql-2":{"hostname":"postgresql-2"},
						"postgresql-3":{"hostname":"postgresql-3"}
					}}
				}}
			}`),
			ExtraVars: []byte(`{"playbook":"untrusted.yml","postgresql_users":[{"name":"unrelated"}]}`),
		},
		servers: []storage.Server{
			{Name: "postgresql-1", Role: "leader", Status: "running", UpdatedAt: &now},
			{Name: "postgresql-2", Role: "replica", Status: "streaming", UpdatedAt: &now},
			{Name: "postgresql-3", Role: "replica", Status: "streaming", UpdatedAt: &now},
		},
	}
	return store, NewGuardedOperationsHandler(
		store, nil, nil, blockedPreflightWatcher{}, &configuration.Config{}, zerolog.Nop(),
	)
}

func TestDatabaseAdminUsesFixedNativeInputs(t *testing.T) {
	if !supportedOperationType(storage.OperationTypeDatabaseAdmin) {
		t.Fatal("database administration is not wired into guarded operations")
	}
	limit := int32(20)
	tests := []struct {
		name, tag, list, flag string
		change                databaseAdminParams
	}{
		{
			name: "database owner", tag: "postgresql_databases", list: "postgresql_databases",
			change: databaseAdminParams{
				Action: databasePresent, Database: "app_db", Owner: "app_owner", ConnectionLimit: &limit,
			},
		},
		{
			name: "login user", tag: "postgresql_users", list: "postgresql_users", flag: "LOGIN",
			change: databaseAdminParams{Action: userPresent, Role: "app_user"},
		},
		{
			name: "no-login role", tag: "postgresql_users", list: "postgresql_users", flag: "NOLOGIN",
			change: databaseAdminParams{Action: rolePresent, Role: "app_role"},
		},
		{
			name: "table grant", tag: "postgresql_privs", list: "postgresql_privs",
			change: databaseAdminParams{
				Action: grantPresent, Database: "app_db", Role: "app_user", ObjectType: "table",
				Objects: []string{"orders"}, Schema: "public", Privileges: []string{"SELECT", "INSERT"},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store, handler := databaseAdminFixture()
			state, err := handler.databaseAdminPreflightState(
				context.Background(), store.cluster, "", mustJSON(test.change),
			)
			if err != nil {
				t.Fatal(err)
			}
			if len(state.blockers) != 0 || len(state.affectedNodes) != 1 ||
				state.affectedNodes[0] != "postgresql-1" {
				t.Fatalf("blockers=%v affected=%v", state.blockers, state.affectedNodes)
			}
			roundTrip, err := operationParams(storage.OperationTypeDatabaseAdmin, mustJSON(state.desired))
			if err != nil || !sameJSON(roundTrip, mustJSON(test.change)) {
				t.Fatalf("params=%s err=%v", roundTrip, err)
			}

			envs, payload, playbook, err := handler.operationInputs(
				context.Background(), store.cluster, storage.OperationTypeDatabaseAdmin, mustJSON(state.desired),
			)
			if err != nil {
				t.Fatal(err)
			}
			var inputs map[string]any
			if json.Unmarshal(payload, &inputs) != nil || playbook != databaseAdminPlaybook ||
				!containsString(envs, "ANSIBLE_RUN_TAGS="+test.tag+",database_admin") ||
				inputs["database_admin_primary"] != "postgresql-1" ||
				inputs["cloud_provider"] != "" || strings.Contains(string(payload), "database_admin_password") {
				t.Fatalf("envs=%v playbook=%q inputs=%v", envs, playbook, inputs)
			}
			items, ok := inputs[test.list].([]any)
			if !ok || len(items) != 1 {
				t.Fatalf("%s=%#v", test.list, inputs[test.list])
			}
			item, ok := items[0].(map[string]any)
			if !ok || test.flag != "" && item["flags"] != test.flag {
				t.Fatalf("item=%#v expected flag=%q", items[0], test.flag)
			}
		})
	}
}

func TestDatabaseAdminRejectsUnsafeOrAmbiguousChanges(t *testing.T) {
	invalid := []databaseAdminParams{
		{Action: userPresent, Role: `app_user";drop role postgres;--`},
		{Action: databaseAbsent, Database: "app_db", Owner: "unexpected"},
		{
			Action: grantPresent, Database: "app_db", Role: "app_user", ObjectType: "table",
			Objects: []string{"orders"}, Schema: "public", Privileges: []string{"ALL"},
		},
		{
			Action: grantPresent, Database: "app_db", Role: "app_user", ObjectType: "function",
			Objects: []string{"run"}, Schema: "public", Privileges: []string{"EXECUTE"},
		},
	}
	for _, change := range invalid {
		if validDatabaseAdminParams(change) {
			t.Errorf("accepted unsafe change: %+v", change)
		}
	}
}
