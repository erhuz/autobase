package cluster

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"postgresql-cluster-console/internal/configuration"
	"postgresql-cluster-console/internal/storage"
	"postgresql-cluster-console/models"
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
	response := NewQueryAnalyticsOperationsHandler(
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
