package cluster

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"testing"
	"time"

	"postgresql-cluster-console/internal/configuration"
	"postgresql-cluster-console/internal/storage"
	"postgresql-cluster-console/internal/xdocker"
	"postgresql-cluster-console/models"
	"postgresql-cluster-console/pkg/tracer"
	clusterapi "postgresql-cluster-console/restapi/operations/cluster"

	"github.com/rs/zerolog"
)

type blockedPreflightStorage struct {
	storage.IStorage
	cluster   *storage.Cluster
	servers   []storage.Server
	preflight *storage.CreateOperationPreflightReq
}

func (s *blockedPreflightStorage) GetCluster(context.Context, int64) (*storage.Cluster, error) {
	return s.cluster, nil
}

func (s *blockedPreflightStorage) GetClusterServers(context.Context, int64) ([]storage.Server, error) {
	return s.servers, nil
}

func (s *blockedPreflightStorage) HasActiveOperation(context.Context, int64) (bool, error) {
	return false, nil
}

func (s *blockedPreflightStorage) CreateOperationPreflight(_ context.Context, req *storage.CreateOperationPreflightReq) (*storage.OperationPreflight, error) {
	s.preflight = req
	return &storage.OperationPreflight{
		ID: 1, ClusterID: req.ClusterID, Type: req.Type, Observed: req.Observed, Desired: req.Desired,
		Checks: req.Checks, Blockers: req.Blockers, Plan: req.Plan, AffectedNodes: req.AffectedNodes,
		Confirmation: req.Confirmation, TopologyHash: req.TopologyHash, ExpiresAt: req.ExpiresAt,
	}, nil
}

type blockedPreflightWatcher struct{}

func (blockedPreflightWatcher) Run()                                            {}
func (blockedPreflightWatcher) Stop()                                           {}
func (blockedPreflightWatcher) HandleCluster(context.Context, *storage.Cluster) {}

type guardedOperationStorage struct {
	storage.IStorage
	cluster     *storage.Cluster
	servers     []storage.Server
	preflight   *storage.OperationPreflight
	reserved    *storage.CreateOperationReq
	updates     []*storage.UpdateOperationReq
	consumeOK   bool
	preflightID int64
}

func (s *guardedOperationStorage) GetCluster(context.Context, int64) (*storage.Cluster, error) {
	return s.cluster, nil
}

func (s *guardedOperationStorage) GetClusterServers(context.Context, int64) ([]storage.Server, error) {
	return s.servers, nil
}

func (s *guardedOperationStorage) HasActiveOperation(context.Context, int64) (bool, error) {
	return false, nil
}

func (s *guardedOperationStorage) CreateOperationPreflight(_ context.Context, req *storage.CreateOperationPreflightReq) (*storage.OperationPreflight, error) {
	s.preflightID++
	s.preflight = &storage.OperationPreflight{
		ID: s.preflightID, ClusterID: req.ClusterID, Type: req.Type, Observed: req.Observed, Desired: req.Desired,
		Checks: req.Checks, Blockers: req.Blockers, Plan: req.Plan, AffectedNodes: req.AffectedNodes,
		Confirmation: req.Confirmation, TopologyHash: req.TopologyHash, ExpiresAt: req.ExpiresAt,
	}
	return s.preflight, nil
}

func (s *guardedOperationStorage) GetOperationPreflight(context.Context, int64) (*storage.OperationPreflight, error) {
	if s.preflight == nil {
		return nil, errors.New("preflight not found")
	}
	return s.preflight, nil
}

func (s *guardedOperationStorage) ReserveOperation(_ context.Context, req *storage.CreateOperationReq) (*storage.Operation, error) {
	s.reserved = req
	return &storage.Operation{ID: 9}, nil
}

func (s *guardedOperationStorage) ConsumeOperationPreflight(context.Context, int64) (bool, error) {
	return s.consumeOK, nil
}

func (s *guardedOperationStorage) UpdateOperation(_ context.Context, req *storage.UpdateOperationReq) (*storage.Operation, error) {
	s.updates = append(s.updates, req)
	return &storage.Operation{ID: req.ID}, nil
}

type operationDocker struct {
	xdocker.IManager
	calls  int
	config *xdocker.ManageClusterConfig
	err    error
}

func (d *operationDocker) ManageCluster(_ context.Context, config *xdocker.ManageClusterConfig) (xdocker.InstanceID, error) {
	d.calls++
	d.config = config
	return "container-1", d.err
}

type operationLogs struct{ calls int }

func (l *operationLogs) StoreInDb(int64, xdocker.InstanceID, string) { l.calls++ }
func (l *operationLogs) PrintToConsole(xdocker.InstanceID, string)   {}
func (l *operationLogs) Stop()                                       {}

func TestQueryAnalyticsPlanAndTopologyHash(t *testing.T) {
	timeline := int64(4)
	servers := []storage.Server{
		{Name: "postgresql-3", Role: "replica", Status: "streaming", Timeline: &timeline},
		{Name: "postgresql-1", Role: "leader", Status: "running", Timeline: &timeline},
		{Name: "postgresql-2", Role: "replica", Status: "streaming", Timeline: &timeline},
	}
	nodes, leaders, healthy := operationTopology(servers)
	if leaders != 1 || healthy != 3 || nodes[0].Name != "postgresql-1" {
		t.Fatalf("topology = %+v, leaders=%d healthy=%d", nodes, leaders, healthy)
	}
	plan, affected := queryAnalyticsPlan(nodes)
	if len(plan) != 5 || len(affected) != 3 || affected[2] != "postgresql-1" {
		t.Fatalf("plan=%v affected=%v", plan, affected)
	}
	before, err := topologyHash(nodes)
	if err != nil {
		t.Fatal(err)
	}
	nodes[1].Role = "leader"
	after, err := topologyHash(nodes)
	if err != nil {
		t.Fatal(err)
	}
	if before == after {
		t.Fatal("role change did not invalidate topology hash")
	}
	now := time.Now()
	for i := range servers {
		servers[i].UpdatedAt = &now
	}
	if !topologyFresh(servers, now.Add(-time.Second)) {
		t.Fatal("fresh topology was rejected")
	}
	servers[0].UpdatedAt = nil
	if topologyFresh(servers, now.Add(-time.Second)) {
		t.Fatal("stale topology was accepted")
	}
}

func TestQueryAnalyticsPreflightReportsAndBlocksManagementDrift(t *testing.T) {
	store := &blockedPreflightStorage{cluster: &storage.Cluster{
		ID: 5, Status: storage.ClusterStatusReady, PostgreVersion: 13,
	}}
	operationType := storage.OperationTypeQueryAnalyticsEnable
	response := NewGuardedOperationsHandler(
		store, nil, nil, blockedPreflightWatcher{}, &configuration.Config{}, zerolog.Nop(),
	).HandlePreflight(clusterapi.PostClustersIDPreflightsParams{
		ID: 5, HTTPRequest: httptest.NewRequest("POST", "/clusters/5/preflights", nil),
		Body: &models.RequestOperationPreflight{Type: &operationType},
	})
	if _, ok := response.(*clusterapi.PostClustersIDPreflightsCreated); !ok || store.preflight == nil {
		t.Fatalf("response=%#v preflight=%+v", response, store.preflight)
	}

	var blockers []string
	if err := json.Unmarshal(store.preflight.Blockers, &blockers); err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"PostgreSQL 14-18", "at least three healthy nodes", "exactly one leader",
		"topology names resolved", "topology refreshed now",
	} {
		found := false
		for _, blocker := range blockers {
			found = found || blocker == required
		}
		if !found {
			t.Errorf("missing blocker %q: %v", required, blockers)
		}
	}
}

func TestGuardedOperationRejectsUnsupportedAndChangedObservedState(t *testing.T) {
	now := time.Now().UTC()
	store := &guardedOperationStorage{
		cluster: &storage.Cluster{ID: 5, ProjectID: 3, PostgreVersion: 16},
		servers: []storage.Server{
			{Name: "postgresql-1", Role: "leader", Status: "running", UpdatedAt: &now},
			{Name: "postgresql-2", Role: "replica", Status: "streaming", UpdatedAt: &now},
			{Name: "postgresql-3", Role: "replica", Status: "streaming", UpdatedAt: &now},
		},
	}
	handler := NewGuardedOperationsHandler(store, nil, nil, blockedPreflightWatcher{}, &configuration.Config{}, zerolog.Nop())
	unsupported := "rolling_restart"
	response := handler.HandlePreflight(clusterapi.PostClustersIDPreflightsParams{
		ID: 5, HTTPRequest: httptest.NewRequest("POST", "/clusters/5/preflights", nil),
		Body: &models.RequestOperationPreflight{Type: &unsupported},
	})
	if _, ok := response.(*clusterapi.PostClustersIDPreflightsBadRequest); !ok || store.preflight != nil {
		t.Fatalf("unsupported response=%#v preflight=%+v", response, store.preflight)
	}

	operationType := storage.OperationTypeQueryAnalyticsEnable
	response = handler.HandlePreflight(clusterapi.PostClustersIDPreflightsParams{
		ID: 5, HTTPRequest: httptest.NewRequest("POST", "/clusters/5/preflights", nil),
		Body: &models.RequestOperationPreflight{Type: &operationType},
	})
	if _, ok := response.(*clusterapi.PostClustersIDPreflightsCreated); !ok {
		t.Fatalf("preflight response=%#v", response)
	}
	store.cluster.PostgreVersion = 17
	request := httptest.NewRequest("POST", "/clusters/5/operations", nil)
	request = request.WithContext(context.WithValue(request.Context(), tracer.CtxCidKey{}, "test-cid"))
	wrongConfirmation := "WRONG"
	response = handler.HandleOperation(clusterapi.PostClustersIDOperationsParams{
		ID: 5, HTTPRequest: request,
		Body: &models.RequestOperationStart{PreflightID: &store.preflight.ID, Confirmation: &wrongConfirmation},
	})
	if _, ok := response.(*clusterapi.PostClustersIDOperationsBadRequest); !ok || store.reserved != nil {
		t.Fatalf("confirmation response=%#v reserved=%+v", response, store.reserved)
	}
	response = handler.HandleOperation(clusterapi.PostClustersIDOperationsParams{
		ID: 5, HTTPRequest: request,
		Body: &models.RequestOperationStart{PreflightID: &store.preflight.ID, Confirmation: &store.preflight.Confirmation},
	})
	if _, ok := response.(*clusterapi.PostClustersIDOperationsBadRequest); !ok || store.reserved != nil {
		t.Fatalf("changed-state response=%#v reserved=%+v", response, store.reserved)
	}
}

func TestGuardedOperationRecordsLaunchFailure(t *testing.T) {
	now := time.Now().UTC()
	store := &guardedOperationStorage{
		cluster: &storage.Cluster{
			ID: 5, ProjectID: 3, Name: "cluster-1", PostgreVersion: 16,
			QueryAnalyticsManaged: true, QueryAnalyticsDesired: true,
		},
		servers: []storage.Server{
			{Name: "postgresql-1", Role: "leader", Status: "running", UpdatedAt: &now},
			{Name: "postgresql-2", Role: "replica", Status: "streaming", UpdatedAt: &now},
			{Name: "postgresql-3", Role: "replica", Status: "streaming", UpdatedAt: &now},
		},
		consumeOK: true,
	}
	docker := &operationDocker{err: errors.New("automation unavailable")}
	logs := &operationLogs{}
	handler := NewGuardedOperationsHandler(store, docker, logs, blockedPreflightWatcher{}, &configuration.Config{}, zerolog.Nop())
	operationType := storage.OperationTypeQueryAnalyticsDisable
	response := handler.HandlePreflight(clusterapi.PostClustersIDPreflightsParams{
		ID: 5, HTTPRequest: httptest.NewRequest("POST", "/clusters/5/preflights", nil),
		Body: &models.RequestOperationPreflight{Type: &operationType},
	})
	if _, ok := response.(*clusterapi.PostClustersIDPreflightsCreated); !ok {
		t.Fatalf("preflight response=%#v", response)
	}
	request := httptest.NewRequest("POST", "/clusters/5/operations", nil)
	request = request.WithContext(context.WithValue(request.Context(), tracer.CtxCidKey{}, "test-cid"))
	response = handler.HandleOperation(clusterapi.PostClustersIDOperationsParams{
		ID: 5, HTTPRequest: request,
		Body: &models.RequestOperationStart{PreflightID: &store.preflight.ID, Confirmation: &store.preflight.Confirmation},
	})
	if _, ok := response.(*clusterapi.PostClustersIDOperationsBadRequest); !ok {
		t.Fatalf("operation response=%#v", response)
	}
	if store.reserved == nil || docker.calls != 1 || logs.calls != 0 || len(store.updates) != 1 ||
		store.updates[0].Status == nil || *store.updates[0].Status != storage.OperationStatusFailed ||
		store.updates[0].SafeNextAction == nil {
		t.Fatalf("reserved=%+v docker=%d logs=%d updates=%+v", store.reserved, docker.calls, logs.calls, store.updates)
	}
}

func TestGuardedOperationLaunchesFixedAutomation(t *testing.T) {
	now := time.Now().UTC()
	store := &guardedOperationStorage{
		cluster: &storage.Cluster{
			ID: 5, ProjectID: 3, Name: "cluster-1", PostgreVersion: 16,
			QueryAnalyticsManaged: true, QueryAnalyticsDesired: true,
			ExtraVars: []byte(`{"playbook":"untrusted.yml","query_analytics_state":"enabled","enable_pg_stat_monitor":true}`),
		},
		servers: []storage.Server{
			{Name: "postgresql-1", Role: "leader", Status: "running", UpdatedAt: &now},
			{Name: "postgresql-2", Role: "replica", Status: "streaming", UpdatedAt: &now},
			{Name: "postgresql-3", Role: "replica", Status: "streaming", UpdatedAt: &now},
		},
		consumeOK: true,
	}
	docker := &operationDocker{}
	logs := &operationLogs{}
	handler := NewGuardedOperationsHandler(store, docker, logs, blockedPreflightWatcher{}, &configuration.Config{}, zerolog.Nop())
	operationType := storage.OperationTypeQueryAnalyticsDisable
	response := handler.HandlePreflight(clusterapi.PostClustersIDPreflightsParams{
		ID: 5, HTTPRequest: httptest.NewRequest("POST", "/clusters/5/preflights", nil),
		Body: &models.RequestOperationPreflight{Type: &operationType},
	})
	if _, ok := response.(*clusterapi.PostClustersIDPreflightsCreated); !ok {
		t.Fatalf("preflight response=%#v", response)
	}
	request := httptest.NewRequest("POST", "/clusters/5/operations", nil)
	request = request.WithContext(context.WithValue(request.Context(), tracer.CtxCidKey{}, "test-cid"))
	response = handler.HandleOperation(clusterapi.PostClustersIDOperationsParams{
		ID: 5, HTTPRequest: request,
		Body: &models.RequestOperationStart{PreflightID: &store.preflight.ID, Confirmation: &store.preflight.Confirmation},
	})
	if _, ok := response.(*clusterapi.PostClustersIDOperationsAccepted); !ok {
		t.Fatalf("operation response=%#v", response)
	}
	var extraVars map[string]any
	if err := json.Unmarshal([]byte(docker.config.ExtraVars), &extraVars); err != nil {
		t.Fatal(err)
	}
	if docker.config.Playbook != queryAnalyticsPlaybook || extraVars["query_analytics_state"] != "disabled" ||
		extraVars["enable_pg_stat_monitor"] != false || docker.calls != 1 || logs.calls != 1 ||
		len(store.updates) != 1 || store.updates[0].Status == nil ||
		*store.updates[0].Status != storage.OperationStatusRunning {
		t.Fatalf("config=%+v vars=%+v docker=%d logs=%d updates=%+v", docker.config, extraVars, docker.calls, logs.calls, store.updates)
	}
}
