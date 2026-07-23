package cluster

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"postgresql-cluster-console/internal/configuration"
	"postgresql-cluster-console/internal/storage"
	"postgresql-cluster-console/models"
	"postgresql-cluster-console/pkg/tracer"
	clusterapi "postgresql-cluster-console/restapi/operations/cluster"

	"github.com/rs/zerolog"
)

func replicaReinitPreflight(t *testing.T, handler *guardedOperationsHandler, store *guardedOperationStorage, target string) {
	t.Helper()
	operationType := storage.OperationTypeReplicaReinit
	response := handler.HandlePreflight(clusterapi.PostClustersIDPreflightsParams{
		ID: 5, HTTPRequest: httptest.NewRequest("POST", "/clusters/5/preflights", nil),
		Body: &models.RequestOperationPreflight{Type: &operationType, Target: target},
	})
	if _, ok := response.(*clusterapi.PostClustersIDPreflightsCreated); !ok || store.preflight == nil {
		t.Fatalf("preflight response=%#v preflight=%+v", response, store.preflight)
	}
}

func TestReplicaReinitPreflightBindsDataLossSourceAndMethod(t *testing.T) {
	store, target := switchoverFixture()
	handler := NewGuardedOperationsHandler(store, nil, nil, blockedPreflightWatcher{}, &configuration.Config{}, zerolog.Nop())
	replicaReinitPreflight(t, handler, store, target)

	var desired replicaReinitDesired
	if err := json.Unmarshal(store.preflight.Desired, &desired); err != nil {
		t.Fatal(err)
	}
	if string(store.preflight.Blockers) != "[]" || desired.Target != target ||
		desired.CloneSource != "postgresql-1" || desired.CloneMethod != replicaReinitCloneMethod ||
		desired.MaxLagBytes != 100 || !desired.DataLoss ||
		store.preflight.Confirmation != "REINITIALIZE REPLICA postgresql-2 DELETE LOCAL DATA" ||
		!strings.Contains(string(store.preflight.Plan), "clone postgresql-2 from leader postgresql-1 with pg_basebackup") {
		t.Fatalf("blockers=%s desired=%+v confirmation=%q plan=%s", store.preflight.Blockers, desired, store.preflight.Confirmation, store.preflight.Plan)
	}
}

func TestReplicaReinitPreflightRejectsLeaderAndMissingHealthyPeer(t *testing.T) {
	store, _ := switchoverFixture()
	handler := NewGuardedOperationsHandler(store, nil, nil, blockedPreflightWatcher{}, &configuration.Config{}, zerolog.Nop())
	replicaReinitPreflight(t, handler, store, "postgresql-1")
	if !strings.Contains(string(store.preflight.Blockers), "selected target is current replica") {
		t.Fatalf("leader target blockers=%s", store.preflight.Blockers)
	}

	store, target := switchoverFixture()
	store.servers[1].Status = "stopped"
	store.servers[2].Status = "stopped"
	handler = NewGuardedOperationsHandler(store, nil, nil, blockedPreflightWatcher{}, &configuration.Config{}, zerolog.Nop())
	replicaReinitPreflight(t, handler, store, target)
	if !strings.Contains(string(store.preflight.Blockers), "another healthy member") {
		t.Fatalf("peer blockers=%s", store.preflight.Blockers)
	}
}

func TestReplicaReinitRejectsRoleChangeAfterConfirmation(t *testing.T) {
	store, target := switchoverFixture()
	handler := NewGuardedOperationsHandler(store, nil, nil, blockedPreflightWatcher{}, &configuration.Config{}, zerolog.Nop())
	replicaReinitPreflight(t, handler, store, target)

	store.servers[0].Role = "leader"
	store.servers[1].Role = "replica"
	request := httptest.NewRequest("POST", "/clusters/5/operations", nil)
	request = request.WithContext(context.WithValue(request.Context(), tracer.CtxCidKey{}, "test-cid"))
	response := handler.HandleOperation(clusterapi.PostClustersIDOperationsParams{
		ID: 5, HTTPRequest: request,
		Body: &models.RequestOperationStart{
			PreflightID: &store.preflight.ID, Confirmation: &store.preflight.Confirmation,
		},
	})
	if _, ok := response.(*clusterapi.PostClustersIDOperationsBadRequest); !ok || store.reserved != nil {
		t.Fatalf("response=%#v reserved=%+v", response, store.reserved)
	}
}

func TestReplicaReinitLaunchesFixedAutomation(t *testing.T) {
	store, target := switchoverFixture()
	docker := &operationDocker{}
	logs := &operationLogs{}
	handler := NewGuardedOperationsHandler(store, docker, logs, blockedPreflightWatcher{}, &configuration.Config{}, zerolog.Nop())
	replicaReinitPreflight(t, handler, store, target)

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
	if store.reserved == nil || store.reserved.Type != storage.OperationTypeReplicaReinit ||
		docker.config.Playbook != replicaReinitPlaybook ||
		extraVars["replica_reinit_target_name"] != target ||
		extraVars["replica_reinit_clone_source"] != "postgresql-1" ||
		extraVars["replica_reinit_clone_method"] != replicaReinitCloneMethod ||
		extraVars["replica_reinit_max_lag_bytes"] != float64(100) ||
		docker.calls != 1 || logs.calls != 1 {
		t.Fatalf("reserved=%+v config=%+v vars=%+v docker=%d logs=%d", store.reserved, docker.config, extraVars, docker.calls, logs.calls)
	}
}
