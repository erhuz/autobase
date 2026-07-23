package cluster

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"postgresql-cluster-console/internal/storage"

	"github.com/go-openapi/strfmt"
)

func TestClusterHealthIncludesTopologyRoutingAndOperations(t *testing.T) {
	now := time.Now().UTC()
	observed := now.Add(-time.Minute)
	timeline, lag, restart := int64(7), int64(0), false
	finished := now.Add(-2 * time.Minute)
	safeNext := "retry after topology review"
	cluster := &storage.Cluster{
		Status:    storage.ClusterStatusHealthy,
		Flags:     1,
		UpdatedAt: &observed,
		ExtraVars: []byte(`{"dcs_type":"etcd"}`),
		Inventory: []byte(`{"all":{"children":{"etcd_cluster":{"hosts":{"dcs-2":{},"dcs-1":{}}}}}}`),
		ConnectionInfo: map[string]any{
			"address": map[string]any{"replica": "replica.internal", "primary": "primary.internal"},
			"port":    map[string]any{"primary": 5000, "replica": 5001},
		},
	}
	servers := []storage.Server{
		{Name: "postgresql-2", Role: "replica", Status: "streaming", Timeline: &timeline, Lag: &lag, PendingRestart: &restart, UpdatedAt: &observed},
		{Name: "postgresql-1", Role: "leader", Status: "running", Timeline: &timeline, Lag: &lag, PendingRestart: &restart, UpdatedAt: &observed},
	}
	operations := []storage.ClusterHealthOperation{
		{ID: 1, Type: "reload", Status: storage.OperationStatusFailed, CreatedAt: now.Add(-4 * time.Minute), UpdatedAt: &finished, SafeNextAction: &safeNext},
		{ID: 2, Type: "reload", Status: storage.OperationStatusSucceeded, CreatedAt: now.Add(-3 * time.Minute), UpdatedAt: &finished},
		{ID: 3, Type: "rolling_restart", Status: storage.OperationStatusRunning, CreatedAt: now.Add(-time.Minute), UpdatedAt: &now},
	}

	health := clusterHealthModel(cluster, servers, operations, nil, now)
	if health.Topology.Leader.Name != "postgresql-1" || len(health.Topology.Replicas) != 1 || len(health.Topology.Members) != 2 {
		t.Fatalf("topology = %+v", health.Topology)
	}
	if health.Topology.PatroniReachable == nil || !*health.Topology.PatroniReachable {
		t.Fatalf("patroni reachability = %v", health.Topology.PatroniReachable)
	}
	if health.Dcs.State != "configured_not_observed" || strings.Join(health.Dcs.Members, ",") != "dcs-1,dcs-2" {
		t.Fatalf("dcs = %+v", health.Dcs)
	}
	if health.Dcs.Reachable != nil {
		t.Fatalf("unobserved DCS reported reachability = %v", *health.Dcs.Reachable)
	}
	if len(health.Routing.Targets) != 2 || health.Routing.Targets[0].Role != "primary" || *health.Routing.Targets[0].Port != 5000 {
		t.Fatalf("routing = %+v", health.Routing)
	}
	for _, target := range health.Routing.Targets {
		if target.Reachable != nil || target.RoleMatches != nil {
			t.Fatalf("unobserved routing target reported health = %+v", target)
		}
	}
	if health.Operation.Active.ID != 3 || health.Operation.Active.Finished != nil ||
		health.Operation.Latest.ID != 2 || health.Operation.Unresolved.ID != 1 {
		t.Fatalf("operations = %+v", health.Operation)
	}
	if err := health.Validate(strfmt.Default); err != nil {
		t.Fatal(err)
	}
}

func TestClusterHealthDegradesWithoutBackupEvidence(t *testing.T) {
	health := clusterHealthModel(&storage.Cluster{}, nil, nil, nil, time.Now().UTC())
	if health.Backup.State != "not_observed" || health.Recoverability.State != "degraded" {
		t.Fatalf("backup=%+v recoverability=%+v", health.Backup, health.Recoverability)
	}
	if strings.Join(health.Recoverability.Reasons, ",") != "backup_not_observed,wal_continuity_not_observed,restore_evidence_missing" {
		t.Fatalf("reasons = %v", health.Recoverability.Reasons)
	}
}

func TestClusterHealthRequiresRecoverableBackupEvidence(t *testing.T) {
	now := time.Now().UTC()
	latestFull, latestDiff, restored := now.Add(-2*time.Hour), now.Add(-time.Hour), now.Add(-24*time.Hour)
	walContinuous := true
	evidence := &storage.BackupEvidence{
		ObservedAt: now, RepositoryReachable: true,
		LatestFull: &latestFull, LatestDifferential: &latestDiff,
		Retention: []byte(`{"full":7,"differential":6}`), WalContinuous: &walContinuous,
		Locks: []byte(`[]`), SchedulerOwners: []byte(`["postgresql-1"]`),
		FreshnessSeconds: 86400, RestoreTestedAt: &restored,
	}
	health := clusterHealthModel(&storage.Cluster{}, nil, nil, evidence, now)
	if health.Backup.State != "healthy" || health.Recoverability.State != "healthy" ||
		health.Backup.SchedulerOwner == nil || *health.Backup.SchedulerOwner != "postgresql-1" ||
		health.Backup.Fresh == nil || !*health.Backup.Fresh {
		t.Fatalf("backup=%+v recoverability=%+v", health.Backup, health.Recoverability)
	}

	evidence.SchedulerOwners = []byte(`["postgresql-1","postgresql-2"]`)
	evidence.Locks = []byte(`["backup.lock"]`)
	health = clusterHealthModel(&storage.Cluster{}, nil, nil, evidence, now)
	if health.Backup.State != "degraded" ||
		!strings.Contains(strings.Join(health.Recoverability.Reasons, ","), "duplicate_scheduler_owners") ||
		!strings.Contains(strings.Join(health.Recoverability.Reasons, ","), "backup_lock_active") {
		t.Fatalf("backup=%+v recoverability=%+v", health.Backup, health.Recoverability)
	}
}

func TestClusterHealthOmitsStoredSecrets(t *testing.T) {
	cluster := &storage.Cluster{
		ExtraVars: []byte(`{"query_analytics_monitor_password":"extra-secret"}`),
		Inventory: []byte(`{"all":{"vars":{"ansible_password":"inventory-secret"}}}`),
		ConnectionInfo: map[string]any{
			"address": map[string]any{
				"primary":  "postgresql://health-user:route-secret@postgres.internal:5432/postgres",
				"password": "map-secret",
			},
			"port":      5432,
			"superuser": "postgres", "password": "connection-secret",
		},
	}
	payload, err := json.Marshal(clusterHealthModel(cluster, nil, nil, nil, time.Now().UTC()))
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"extra-secret", "inventory-secret", "connection-secret", "route-secret", "map-secret", "health-user", "superuser", "password"} {
		if strings.Contains(string(payload), secret) {
			t.Fatalf("health response leaked %q: %s", secret, payload)
		}
	}
	if !strings.Contains(string(payload), `"address":"postgres.internal"`) {
		t.Fatalf("sanitized routing target missing: %s", payload)
	}
}
