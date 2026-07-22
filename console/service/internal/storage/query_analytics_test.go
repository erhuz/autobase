package storage

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func validQueryAnalyticsBucket() QueryAnalyticsBucket {
	start := time.Date(2026, 7, 22, 12, 0, 0, 0, time.UTC)
	return QueryAnalyticsBucket{
		ClusterID: 1, ServerID: 2, NodeBootTime: start.Add(-time.Hour), BucketID: 3,
		BucketStart: start, BucketEnd: start.Add(time.Minute), Complete: true,
		Calls: 3, TotalExecTimeMs: 12, MaxExecTimeMs: 7, Rows: 4,
		SharedBlocksHit: 5, SharedBlocksRead: 2, ReadTimeMs: 1.5, WALBytes: 20,
		Samples: []QueryAnalyticsSample{{
			ClusterID: 1, ServerID: 2, NodeBootTime: start.Add(-time.Hour), BucketID: 3,
			FingerprintID: "42", NormalizedQuery: "select * from users where id = $1",
			DatabaseName: "postgres", RoleName: "app", ApplicationName: "api",
			Calls: 3, TotalExecTimeMs: 12, MinExecTimeMs: 1, MaxExecTimeMs: 7, MeanExecTimeMs: 4,
			Rows: 4, SharedBlocksHit: 5, SharedBlocksRead: 2, ReadTimeMs: 1.5, WALBytes: 20,
			LatencyHistogram: []string{"1", "2"}, TopTotalTime: true,
		}},
	}
}

func TestValidateQueryAnalyticsBucket(t *testing.T) {
	bucket := validQueryAnalyticsBucket()
	if err := validateQueryAnalyticsBucket(&bucket); err != nil {
		t.Fatalf("valid bucket rejected: %v", err)
	}

	bucket.Complete = false
	if err := validateQueryAnalyticsBucket(&bucket); err == nil {
		t.Fatal("active bucket accepted")
	}

	bucket = validQueryAnalyticsBucket()
	bucket.Samples[0].ServerID++
	if err := validateQueryAnalyticsBucket(&bucket); err == nil {
		t.Fatal("sample from another source accepted")
	}

	bucket = validQueryAnalyticsBucket()
	bucket.Samples = make([]QueryAnalyticsSample, maxQueryAnalyticsSamplesPerBucket+1)
	if err := validateQueryAnalyticsBucket(&bucket); err == nil {
		t.Fatal("oversized sample accepted")
	}
}

func TestQueryAnalyticsDefaultRequiresCompatiblePostgres(t *testing.T) {
	for version, want := range map[int]bool{13: false, 14: true, 18: true, 19: false} {
		if got := queryAnalyticsEnabledByDefault(version); got != want {
			t.Errorf("PostgreSQL %d default = %t, want %t", version, got, want)
		}
	}
}

func TestQueryAnalyticsStatusTracksCoverage(t *testing.T) {
	cluster := &Cluster{PostgreVersion: 16, QueryAnalyticsManaged: true, QueryAnalyticsDesired: true}
	coverage := []QueryAnalyticsCoverage{{ServerStatus: "running", CollectionStatus: "healthy"}, {ServerStatus: "streaming", CollectionStatus: "unreachable"}}
	status := queryAnalyticsStatus(cluster, coverage)
	if status.State != "degraded" || status.ExpectedNodeCount != 2 || status.CollectedNodeCount != 1 {
		t.Fatalf("status = %+v", status)
	}
	cluster.QueryAnalyticsManaged = false
	if got := queryAnalyticsStatus(cluster, coverage).State; got != "rollout_required" {
		t.Fatalf("unmanaged state = %q", got)
	}
	cluster.PostgreVersion = 13
	if got := queryAnalyticsStatus(cluster, coverage).State; got != "unsupported" {
		t.Fatalf("unsupported state = %q", got)
	}
}

func TestQueryAnalyticsMigrationContract(t *testing.T) {
	paths, err := filepath.Glob("../../../db/migrations/*query_analytics.sql")
	if err != nil || len(paths) != 1 {
		t.Fatalf("find migration: %v (%v)", err, paths)
	}
	data, err := os.ReadFile(paths[0])
	if err != nil {
		t.Fatal(err)
	}
	sql := string(data)
	for _, required := range []string{
		"set query_analytics_managed = false", "set default false",
		"primary key (cluster_id, server_id, node_boot_time, bucket_id)",
		"on delete cascade", "query_analytics_buckets_cluster_time_idx",
	} {
		if !strings.Contains(sql, required) {
			t.Errorf("migration missing %q", required)
		}
	}
	for _, forbidden := range []string{"client_ip", "comments text", "query_plan", "error_message"} {
		if strings.Contains(sql, forbidden) {
			t.Errorf("migration persists forbidden field %q", forbidden)
		}
	}
}
