package cluster

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"postgresql-cluster-console/internal/storage"
	clusterapi "postgresql-cluster-console/restapi/operations/cluster"
)

type routingStorage struct {
	storage.IStorage
	cluster *storage.Cluster
	updates int
}

func (s *routingStorage) GetCluster(context.Context, int64) (*storage.Cluster, error) {
	return s.cluster, nil
}

func (s *routingStorage) UpdateCluster(_ context.Context, req *storage.UpdateClusterReq) (*storage.Cluster, error) {
	s.updates++
	copy := *s.cluster
	copy.ConnectionInfo = req.ConnectionInfo
	s.cluster = &copy
	return s.cluster, nil
}

func (*routingStorage) GetProject(context.Context, int64) (*storage.Project, error) {
	return &storage.Project{Name: "project"}, nil
}

func (*routingStorage) GetEnvironment(context.Context, int64) (*storage.Environment, error) {
	return &storage.Environment{Name: "production"}, nil
}

func (*routingStorage) GetClusterServers(context.Context, int64) ([]storage.Server, error) {
	return nil, nil
}

func TestPatchClusterRoutingPreservesMetadataAndFeedsPrimaryPreflight(t *testing.T) {
	store := &routingStorage{cluster: &storage.Cluster{
		ID: 5, ProjectID: 3, EnvironmentID: 4, CreatedAt: time.Now(),
		ConnectionInfo: map[string]any{
			"address":  map[string]any{"primary": "old-primary.internal", "replica": "replica.internal"},
			"port":     int64(5432),
			"password": "database-secret",
			"note":     "preserve-me",
		},
	}}
	handler := NewPatchClusterRoutingHandler(store)
	request := httptest.NewRequest("PATCH", "/clusters/5/routing", nil)
	response := handler.Handle(clusterapi.PatchClustersIDRoutingParams{
		ID: 5, HTTPRequest: request,
		Body: map[string]any{
			"primary": map[string]any{
				"addresses": []any{"PRIMARY.internal", "10.0.4.2", "10.0.4.2"},
				"port":      float64(5000),
			},
			"replica": nil,
		},
	})

	ok, valid := response.(*clusterapi.PatchClustersIDRoutingOK)
	if !valid || store.updates != 1 {
		t.Fatalf("response=%#v updates=%d", response, store.updates)
	}
	info := healthObject(store.cluster.ConnectionInfo)
	if info["note"] != "preserve-me" || info["password"] != "database-secret" {
		t.Fatalf("unrelated metadata changed: %#v", info)
	}
	addresses, _ := info["address"].(map[string]any)
	ports, _ := info["port"].(map[string]any)
	if addresses["primary"] != "primary.internal, 10.0.4.2" || addresses["replica"] != nil ||
		ports["primary"] != float64(5000) || ports["replica"] != nil {
		t.Fatalf("routing=%#v ports=%#v", addresses, ports)
	}
	targets := primaryRoutingTargets(store.cluster.ConnectionInfo)
	if len(targets) != 2 || targets[0].Port != 5000 || targets[1].Port != 5000 {
		t.Fatalf("primary targets=%#v", targets)
	}
	payload, _ := json.Marshal(ok.Payload)
	if strings.Contains(string(payload), "database-secret") || strings.Contains(string(payload), `"password"`) {
		t.Fatalf("response leaked password: %s", payload)
	}
}

func TestPatchClusterRoutingRejectsInvalidOrMissingPrimary(t *testing.T) {
	cases := []map[string]any{
		{"writer": map[string]any{"addresses": []any{"primary.internal"}, "port": float64(5000)}},
		{"primary": map[string]any{"addresses": []any{"https://primary.internal"}, "port": float64(5000)}},
		{"primary": map[string]any{"addresses": []any{"[[::1]]"}, "port": float64(5000)}},
		{"primary": map[string]any{"addresses": []any{"primary.internal"}, "port": float64(70000)}},
		{"replica": map[string]any{"addresses": []any{"replica.internal"}, "port": float64(5001)}},
	}
	for _, body := range cases {
		store := &routingStorage{cluster: &storage.Cluster{
			ID: 5, ProjectID: 3, EnvironmentID: 4, CreatedAt: time.Now(),
		}}
		response := NewPatchClusterRoutingHandler(store).Handle(clusterapi.PatchClustersIDRoutingParams{
			ID: 5, HTTPRequest: httptest.NewRequest("PATCH", "/clusters/5/routing", nil), Body: body,
		})
		if _, valid := response.(*clusterapi.PatchClustersIDRoutingBadRequest); !valid || store.updates != 0 {
			t.Fatalf("body=%#v response=%#v updates=%d", body, response, store.updates)
		}
	}
}

func TestPatchClusterRoutingAddsPrimaryToImportedCluster(t *testing.T) {
	store := &routingStorage{cluster: &storage.Cluster{
		ID: 5, ProjectID: 3, EnvironmentID: 4, CreatedAt: time.Now(),
	}}
	response := NewPatchClusterRoutingHandler(store).Handle(clusterapi.PatchClustersIDRoutingParams{
		ID: 5, HTTPRequest: httptest.NewRequest("PATCH", "/clusters/5/routing", nil),
		Body: map[string]any{
			"primary": map[string]any{"addresses": []any{"10.0.4.2"}, "port": float64(5000)},
		},
	})
	if _, valid := response.(*clusterapi.PatchClustersIDRoutingOK); !valid {
		t.Fatalf("response=%#v", response)
	}
	targets := primaryRoutingTargets(store.cluster.ConnectionInfo)
	if len(targets) != 1 || targets[0].Address != "10.0.4.2" || targets[0].Port != 5000 {
		t.Fatalf("primary targets=%#v", targets)
	}
}
