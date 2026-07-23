package cluster

import (
	"testing"
	"time"

	"postgresql-cluster-console/internal/storage"
)

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
