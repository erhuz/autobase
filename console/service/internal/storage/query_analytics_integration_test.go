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
	const monitorPassword = "collector-password"
	if err = store.SetQueryAnalyticsCredential(ctx, clusterID, monitorPassword, "test-key"); err != nil {
		t.Fatal(err)
	}
	decrypted, err := store.GetQueryAnalyticsCredential(ctx, clusterID, "test-key")
	if err != nil || decrypted != monitorPassword {
		t.Fatalf("credential round trip = %q, %v", decrypted, err)
	}
	var storedPlaintext bool
	if err = pool.QueryRow(ctx, `
		select password_ciphertext = convert_to($2, 'UTF8')
		from query_analytics_credentials where cluster_id = $1`, clusterID, monitorPassword).Scan(&storedPlaintext); err != nil {
		t.Fatal(err)
	}
	if storedPlaintext {
		t.Fatal("credential was stored as plaintext")
	}

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
	if _, err = pool.Exec(ctx, "update servers set server_status = 'running', server_role = 'primary' where server_id = $1", serverID); err != nil {
		t.Fatal(err)
	}
	filter := &QueryAnalyticsFilter{From: now.Add(-time.Minute), To: now.Add(2 * time.Minute)}
	overview, err := store.GetQueryAnalyticsOverview(ctx, clusterID, filter)
	if err != nil {
		t.Fatal(err)
	}
	if overview.Status.State != "enabled" || overview.Status.CollectedNodeCount != 1 {
		t.Fatalf("overview status = %+v", overview.Status)
	}
	if overview.Summary.Calls != 3 || overview.Summary.TotalExecTimeMs != 12 || len(overview.Queries) != 1 {
		t.Fatalf("overview metrics = %+v, queries = %d", overview.Summary, len(overview.Queries))
	}
	if len(overview.Filters.Databases) != 1 || overview.Filters.Databases[0] != "postgres" {
		t.Fatalf("overview filters = %+v", overview.Filters)
	}
	detail, err := store.GetQueryAnalyticsDetail(ctx, clusterID, "42", filter)
	if err != nil {
		t.Fatal(err)
	}
	if detail == nil || detail.Fingerprint.Calls != 3 || len(detail.Histogram) != 2 || detail.Histogram[1] != "2" {
		t.Fatalf("detail = %+v", detail)
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
