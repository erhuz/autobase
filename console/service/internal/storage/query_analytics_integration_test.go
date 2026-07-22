package storage

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestQueryAnalyticsDuplicateIngestAndRetention(t *testing.T) {
	dsn := os.Getenv("PG_CONSOLE_TEST_DSN")
	if dsn == "" {
		t.Skip("PG_CONSOLE_TEST_DSN is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	name := fmt.Sprintf("query-analytics-test-%d", time.Now().UnixNano())
	var projectID, environmentID, serverID int64
	err = pool.QueryRow(ctx, "select project_id from projects where project_name = 'default'").Scan(&projectID)
	if err != nil {
		t.Fatal(err)
	}
	err = pool.QueryRow(ctx, "select environment_id from environments where environment_name = 'test' limit 1").Scan(&environmentID)
	if err != nil {
		t.Fatal(err)
	}
	store := &dbStorage{db: pool}
	cluster, err := store.CreateCluster(ctx, &CreateClusterReq{
		ProjectID: projectID, EnvironmentID: environmentID, Name: name, PostgreSqlVersion: 16,
	})
	if err != nil {
		t.Fatal(err)
	}
	clusterID := cluster.ID
	if !cluster.QueryAnalyticsManaged || !cluster.QueryAnalyticsDesired {
		t.Fatal("compatible new cluster did not default query analytics on")
	}
	defer pool.Exec(context.Background(), "delete from clusters where cluster_id = $1", clusterID) //nolint:errcheck

	err = pool.QueryRow(ctx, `
		insert into servers (cluster_id, server_name, ip_address)
		values ($1, 'postgresql-1', '10.255.255.1') returning server_id`, clusterID).Scan(&serverID)
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC().Truncate(time.Minute)
	bucket := validQueryAnalyticsBucket()
	bucket.ClusterID, bucket.ServerID = clusterID, serverID
	bucket.NodeBootTime, bucket.BucketStart, bucket.BucketEnd = now.Add(-time.Hour), now, now.Add(time.Minute)
	bucket.Samples[0].ClusterID, bucket.Samples[0].ServerID = clusterID, serverID
	bucket.Samples[0].NodeBootTime = bucket.NodeBootTime

	if err = store.IngestQueryAnalyticsBucket(ctx, &bucket); err != nil {
		t.Fatal(err)
	}
	if err = store.IngestQueryAnalyticsBucket(ctx, &bucket); err != nil {
		t.Fatal(err)
	}

	for table, want := range map[string]int64{"query_analytics_buckets": 1, "query_analytics_samples": 1} {
		var got int64
		if err = pool.QueryRow(ctx, "select count(*) from "+table+" where cluster_id = $1", clusterID).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("%s count after retry = %d, want %d", table, got, want)
		}
	}

	old := bucket
	old.BucketID++
	old.BucketStart, old.BucketEnd = now.Add(-8*24*time.Hour), now.Add(-8*24*time.Hour+time.Minute)
	old.Samples = append([]QueryAnalyticsSample(nil), bucket.Samples...)
	old.Samples[0].BucketID, old.Samples[0].FingerprintID = old.BucketID, "expired"
	if err = store.IngestQueryAnalyticsBucket(ctx, &old); err != nil {
		t.Fatal(err)
	}
	deleted, err := store.PurgeQueryAnalyticsBefore(ctx, now.Add(-7*24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if deleted != 1 {
		t.Fatalf("deleted buckets = %d, want 1", deleted)
	}
}
