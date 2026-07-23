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

func switchoverFixture() (*guardedOperationStorage, string) {
	now := time.Now().UTC()
	leaderLag, candidateLag, replicaLag := int64(0), int64(10), int64(2)
	flags := storage.SetPatroniConnectStatus(0, 1)
	store := &guardedOperationStorage{
		cluster: &storage.Cluster{
			ID: 5, ProjectID: 3, Name: "cluster-1", Status: storage.ClusterStatusHealthy, Flags: *flags,
			ExtraVars: []byte(`{"dcs_type":"etcd","patroni_maximum_lag_on_failover":100}`),
			Inventory: []byte(`{"all":{"children":{"etcd_cluster":{"hosts":{"dcs-1":{},"dcs-2":{},"dcs-3":{}}}}}}`),
			ConnectionInfo: map[string]any{
				"address": "primary.internal",
				"port":    map[string]any{"primary": float64(5432)},
			},
		},
		servers: []storage.Server{
			{Name: "postgresql-2", Role: "replica", Status: "streaming", Lag: &candidateLag, UpdatedAt: &now},
			{Name: "postgresql-1", Role: "leader", Status: "running", Lag: &leaderLag, UpdatedAt: &now},
			{Name: "postgresql-3", Role: "replica", Status: "streaming", Lag: &replicaLag, UpdatedAt: &now},
		},
		consumeOK: true,
	}
	return store, "postgresql-2"
}

func switchoverPreflight(t *testing.T, handler *guardedOperationsHandler, store *guardedOperationStorage, target string) {
	t.Helper()
	operationType := storage.OperationTypeSwitchover
	response := handler.HandlePreflight(clusterapi.PostClustersIDPreflightsParams{
		ID: 5, HTTPRequest: httptest.NewRequest("POST", "/clusters/5/preflights", nil),
		Body: &models.RequestOperationPreflight{Type: &operationType, Target: target},
	})
	if _, ok := response.(*clusterapi.PostClustersIDPreflightsCreated); !ok {
		t.Fatalf("preflight response=%#v", response)
	}
	if store.preflight == nil {
		t.Fatal("preflight was not stored")
	}
}

func TestSwitchoverPreflightBindsHealthyCandidateAndRouting(t *testing.T) {
	store, target := switchoverFixture()
	handler := NewGuardedOperationsHandler(store, nil, nil, blockedPreflightWatcher{}, &configuration.Config{}, zerolog.Nop())
	switchoverPreflight(t, handler, store, target)

	var blockers []string
	if err := json.Unmarshal(store.preflight.Blockers, &blockers); err != nil {
		t.Fatal(err)
	}
	var desired switchoverDesired
	if err := json.Unmarshal(store.preflight.Desired, &desired); err != nil {
		t.Fatal(err)
	}
	if len(blockers) != 0 || desired.Target != target || desired.PreviousLeader != "postgresql-1" ||
		desired.MaxCandidateLag != 100 || len(desired.Routing) != 1 ||
		store.preflight.Confirmation != "SWITCHOVER postgresql-1 TO postgresql-2" ||
		!strings.Contains(string(store.preflight.Plan), "primary.internal:5432") {
		t.Fatalf("blockers=%v desired=%+v confirmation=%q plan=%s", blockers, desired, store.preflight.Confirmation, store.preflight.Plan)
	}
}

func TestSwitchoverPreflightBlocksCandidateOverLagPolicy(t *testing.T) {
	store, target := switchoverFixture()
	*store.servers[0].Lag = 101
	handler := NewGuardedOperationsHandler(store, nil, nil, blockedPreflightWatcher{}, &configuration.Config{}, zerolog.Nop())
	switchoverPreflight(t, handler, store, target)
	if !strings.Contains(string(store.preflight.Blockers), "candidate lag within policy") {
		t.Fatalf("blockers=%s", store.preflight.Blockers)
	}
}

func TestSwitchoverRejectsRoleChangeAfterConfirmation(t *testing.T) {
	store, target := switchoverFixture()
	handler := NewGuardedOperationsHandler(store, nil, nil, blockedPreflightWatcher{}, &configuration.Config{}, zerolog.Nop())
	switchoverPreflight(t, handler, store, target)

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

func TestSwitchoverLaunchesFixedAutomation(t *testing.T) {
	store, target := switchoverFixture()
	docker := &operationDocker{}
	logs := &operationLogs{}
	handler := NewGuardedOperationsHandler(store, docker, logs, blockedPreflightWatcher{}, &configuration.Config{}, zerolog.Nop())
	switchoverPreflight(t, handler, store, target)

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
	if store.reserved == nil || store.reserved.Type != storage.OperationTypeSwitchover ||
		docker.config.Playbook != switchoverPlaybook ||
		extraVars["patroni_switchover_candidate_name"] != target ||
		len(extraVars["switchover_primary_routing_targets"].([]any)) != 1 ||
		docker.calls != 1 || logs.calls != 1 {
		t.Fatalf("reserved=%+v config=%+v vars=%+v docker=%d logs=%d", store.reserved, docker.config, extraVars, docker.calls, logs.calls)
	}
}
