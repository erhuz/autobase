package watcher

import (
	"context"
	"errors"
	"fmt"
	"postgresql-cluster-console/internal/configuration"
	"postgresql-cluster-console/internal/storage"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

const (
	queryAnalyticsApplication = "autobase-query-collector"
	queryAnalyticsVersion     = "2.3.2"
)

type QueryAnalyticsWatcher interface {
	Run()
	Stop()
}

type queryAnalyticsWatcher struct {
	db  storage.IStorage
	cfg *configuration.Config
	log zerolog.Logger

	ctx         context.Context
	cancel      context.CancelFunc
	wg          sync.WaitGroup
	lastCleanup time.Time
}

func NewQueryAnalyticsWatcher(db storage.IStorage, cfg *configuration.Config) QueryAnalyticsWatcher {
	return &queryAnalyticsWatcher{
		db: db, cfg: cfg,
		log: log.Logger.With().Str("module", "query_analytics_watcher").Logger(),
	}
}

func (w *queryAnalyticsWatcher) Run() {
	if w.cancel != nil {
		return
	}
	w.ctx, w.cancel = context.WithCancel(context.Background())
	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		w.loop()
	}()
}

func (w *queryAnalyticsWatcher) Stop() {
	if w.cancel == nil {
		return
	}
	w.cancel()
	w.wg.Wait()
	w.cancel = nil
}

func (w *queryAnalyticsWatcher) loop() {
	ticker := time.NewTicker(w.cfg.QueryAnalytics.RunEvery)
	defer ticker.Stop()
	w.doWork()
	for {
		select {
		case <-w.ctx.Done():
			return
		case <-ticker.C:
			w.doWork()
		}
	}
}

func (w *queryAnalyticsWatcher) doWork() {
	now := time.Now().UTC()
	if w.lastCleanup.IsZero() || now.Sub(w.lastCleanup) >= time.Hour {
		if _, err := w.db.PurgeQueryAnalyticsBefore(w.ctx, now.Add(-w.cfg.QueryAnalytics.Retention)); err != nil {
			w.log.Error().Err(err).Msg("query analytics retention failed")
		} else {
			w.lastCleanup = now
		}
	}

	limit := int64(1000)
	projects, _, err := w.db.GetProjects(w.ctx, &limit, nil)
	if err != nil {
		w.log.Error().Err(err).Msg("query analytics project scan failed")
		return
	}
	for _, project := range projects {
		clusters, _, err := w.db.GetClusters(w.ctx, &storage.GetClustersReq{ProjectID: project.ID, Limit: &limit})
		if err != nil {
			w.log.Error().Err(err).Int64("project_id", project.ID).Msg("query analytics cluster scan failed")
			continue
		}
		for i := range clusters {
			w.collectCluster(&clusters[i])
		}
	}
}

func (w *queryAnalyticsWatcher) collectCluster(cluster *storage.Cluster) {
	if !cluster.QueryAnalyticsDesired {
		return
	}
	servers, err := w.db.GetClusterServers(w.ctx, cluster.ID)
	if err != nil {
		w.log.Error().Err(err).Int64("cluster_id", cluster.ID).Msg("query analytics server scan failed")
		return
	}
	password, err := w.db.GetQueryAnalyticsCredential(w.ctx, cluster.ID, w.cfg.EncryptionKey)
	if err != nil {
		for i := range servers {
			w.markSource(cluster.ID, servers[i].ID, "unknown", "credential_missing", nil, nil)
		}
		return
	}
	for i := range servers {
		if servers[i].Status != "running" && servers[i].Status != "streaming" {
			w.markSource(cluster.ID, servers[i].ID, "unreachable", "server_unhealthy", nil, nil)
			continue
		}
		w.collectServer(cluster, &servers[i], password)
	}
}

func (w *queryAnalyticsWatcher) collectServer(cluster *storage.Cluster, server *storage.Server, password string) {
	connectConfig, err := pgx.ParseConfig("sslmode=" + w.cfg.QueryAnalytics.SSLMode)
	if err != nil {
		w.markSource(cluster.ID, server.ID, "unsupported", "sslmode_invalid", nil, nil)
		return
	}
	connectConfig.Host = server.IpAddress.String()
	connectConfig.Port = w.cfg.QueryAnalytics.Port
	connectConfig.Database = "postgres"
	connectConfig.User = w.cfg.QueryAnalytics.Username
	connectConfig.Password = password
	connectConfig.ConnectTimeout = w.cfg.QueryAnalytics.ConnectTimeout
	connectConfig.RuntimeParams["application_name"] = queryAnalyticsApplication

	ctx, cancel := context.WithTimeout(w.ctx, w.cfg.QueryAnalytics.QueryTimeout)
	defer cancel()
	conn, err := pgx.ConnectConfig(ctx, connectConfig)
	if err != nil {
		w.markSource(cluster.ID, server.ID, "unreachable", "connect_failed", nil, nil)
		return
	}
	defer conn.Close(context.Background()) //nolint:errcheck

	capability, err := readQueryAnalyticsCapability(ctx, conn)
	if err != nil {
		code := "capability_query_failed"
		if errors.Is(err, pgx.ErrNoRows) {
			code = "extension_missing"
		}
		w.markSource(cluster.ID, server.ID, "unsupported", code, nil, nil)
		return
	}
	if capability.ExtensionVersion != queryAnalyticsVersion || !capability.PrivacySafe {
		code := "extension_version_mismatch"
		if !capability.PrivacySafe {
			code = "privacy_drift"
		}
		w.markSource(cluster.ID, server.ID, "unsupported", code, &capability.NodeBootTime, &capability.ExtensionVersion)
		return
	}

	buckets, err := readQueryAnalyticsBuckets(ctx, conn, cluster.ID, server.ID, capability.NodeBootTime, capability.PostgresVersion)
	if err != nil {
		w.markSource(cluster.ID, server.ID, "unreachable", "collection_query_failed", &capability.NodeBootTime, &capability.ExtensionVersion)
		return
	}
	for i := range buckets {
		if err = w.db.IngestQueryAnalyticsBucket(ctx, &buckets[i]); err != nil {
			w.markSource(cluster.ID, server.ID, "unreachable", "ingest_failed", &capability.NodeBootTime, &capability.ExtensionVersion)
			return
		}
	}
	w.markSource(cluster.ID, server.ID, "healthy", "", &capability.NodeBootTime, &capability.ExtensionVersion)
}

func (w *queryAnalyticsWatcher) markSource(clusterID, serverID int64, status, errorCode string, bootTime *time.Time, version *string) {
	var code *string
	var collectedAt *time.Time
	if errorCode != "" {
		code = &errorCode
	}
	if status == "healthy" {
		now := time.Now().UTC()
		collectedAt = &now
	}
	if err := w.db.UpsertQueryAnalyticsSource(w.ctx, &storage.QueryAnalyticsSource{
		ClusterID: clusterID, ServerID: serverID, NodeBootTime: bootTime, Status: status,
		ExtensionVersion: version, LastCollectedAt: collectedAt, LastErrorCode: code,
	}); err != nil {
		w.log.Error().Err(err).Int64("cluster_id", clusterID).Int64("server_id", serverID).Msg("query analytics source update failed")
	}
}

type queryAnalyticsCapability struct {
	ExtensionVersion string
	NodeBootTime     time.Time
	PostgresVersion  int
	PrivacySafe      bool
}

func readQueryAnalyticsCapability(ctx context.Context, conn *pgx.Conn) (*queryAnalyticsCapability, error) {
	result := queryAnalyticsCapability{}
	var normalized, applicationNames, queryID, utility, planning, comments, plans string
	err := conn.QueryRow(ctx, `
		select extversion, pg_postmaster_start_time(), current_setting('server_version_num')::int / 10000,
		       current_setting('pg_stat_monitor.pgsm_normalized_query', true),
		       current_setting('pg_stat_monitor.pgsm_track_application_names', true),
		       current_setting('pg_stat_monitor.pgsm_enable_pgsm_query_id', true),
		       current_setting('pg_stat_monitor.pgsm_track_utility', true),
		       current_setting('pg_stat_monitor.pgsm_track_planning', true),
		       current_setting('pg_stat_monitor.pgsm_extract_comments', true),
		       current_setting('pg_stat_monitor.pgsm_enable_query_plan', true)
		from pg_extension where extname = 'pg_stat_monitor'`).Scan(
		&result.ExtensionVersion, &result.NodeBootTime, &result.PostgresVersion,
		&normalized, &applicationNames, &queryID, &utility, &planning, &comments, &plans,
	)
	if err != nil {
		return nil, err
	}
	result.PrivacySafe = queryAnalyticsPrivacySafe(normalized, applicationNames, queryID, utility, planning, comments, plans)
	return &result, nil
}

func queryAnalyticsPrivacySafe(normalized, applicationNames, queryID, utility, planning, comments, plans string) bool {
	return normalized == "on" && applicationNames == "on" && queryID == "on" &&
		utility == "off" && planning == "off" && comments == "off" && plans == "off"
}

func readQueryAnalyticsBuckets(ctx context.Context, conn *pgx.Conn, clusterID, serverID int64, nodeBootTime time.Time, postgresVersion int) ([]storage.QueryAnalyticsBucket, error) {
	roleExpression := "coalesce(username, '')"
	readTimeExpression := "coalesce(blk_read_time, 0) + coalesce(temp_blk_read_time, 0)"
	writeTimeExpression := "coalesce(blk_write_time, 0) + coalesce(temp_blk_write_time, 0)"
	if postgresVersion == 15 {
		roleExpression = `coalesce("user"::text, '')`
	}
	if postgresVersion >= 17 {
		readTimeExpression = "coalesce(shared_blk_read_time, 0) + coalesce(local_blk_read_time, 0) + coalesce(temp_blk_read_time, 0)"
		writeTimeExpression = "coalesce(shared_blk_write_time, 0) + coalesce(local_blk_write_time, 0) + coalesce(temp_blk_write_time, 0)"
	}

	query := fmt.Sprintf(queryAnalyticsCollectionSQL, roleExpression, readTimeExpression, writeTimeExpression)
	rows, err := conn.Query(ctx, query, queryAnalyticsApplication)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	buckets := make([]storage.QueryAnalyticsBucket, 0)
	byBucket := make(map[string]int)
	sampleIndexes := make([]map[string]int, 0)
	for rows.Next() {
		var bucketID int64
		var bucketStart time.Time
		var sample storage.QueryAnalyticsSample
		if err = rows.Scan(
			&bucketID, &bucketStart,
			&sample.FingerprintID, &sample.NormalizedQuery, &sample.DatabaseName, &sample.RoleName, &sample.ApplicationName,
			&sample.Calls, &sample.TotalExecTimeMs, &sample.MinExecTimeMs, &sample.MaxExecTimeMs, &sample.MeanExecTimeMs,
			&sample.Rows, &sample.SharedBlocksHit, &sample.SharedBlocksRead, &sample.TempBlocksRead, &sample.TempBlocksWritten,
			&sample.ReadTimeMs, &sample.WriteTimeMs, &sample.WALBytes, &sample.LatencyHistogram,
		); err != nil {
			return nil, err
		}
		key := fmt.Sprintf("%d/%s", bucketID, bucketStart.Format(time.RFC3339Nano))
		index, ok := byBucket[key]
		if !ok {
			bucket := storage.QueryAnalyticsBucket{
				ClusterID: clusterID, ServerID: serverID, NodeBootTime: nodeBootTime,
				BucketID: bucketID, BucketStart: bucketStart, BucketEnd: bucketStart.Add(time.Minute), Complete: true,
			}
			buckets = append(buckets, bucket)
			sampleIndexes = append(sampleIndexes, make(map[string]int))
			index = len(buckets) - 1
			byBucket[key] = index
		}
		sample.ClusterID, sample.ServerID, sample.NodeBootTime = clusterID, serverID, nodeBootTime
		sample.BucketID = bucketID
		mergeQueryAnalyticsSample(&buckets[index], sampleIndexes[index], sample)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	for i := range buckets {
		finalizeQueryAnalyticsBucket(&buckets[i])
	}
	return buckets, nil
}

func mergeQueryAnalyticsSample(bucket *storage.QueryAnalyticsBucket, indexes map[string]int, sample storage.QueryAnalyticsSample) {
	bucket.Calls += sample.Calls
	bucket.TotalExecTimeMs += sample.TotalExecTimeMs
	bucket.MaxExecTimeMs = max(bucket.MaxExecTimeMs, sample.MaxExecTimeMs)
	bucket.Rows += sample.Rows
	bucket.SharedBlocksHit += sample.SharedBlocksHit
	bucket.SharedBlocksRead += sample.SharedBlocksRead
	bucket.TempBlocksRead += sample.TempBlocksRead
	bucket.TempBlocksWritten += sample.TempBlocksWritten
	bucket.ReadTimeMs += sample.ReadTimeMs
	bucket.WriteTimeMs += sample.WriteTimeMs
	bucket.WALBytes += sample.WALBytes

	key := sample.FingerprintID + "\x00" + sample.DatabaseName + "\x00" + sample.RoleName + "\x00" + sample.ApplicationName
	index, ok := indexes[key]
	if !ok {
		indexes[key] = len(bucket.Samples)
		bucket.Samples = append(bucket.Samples, sample)
		return
	}
	current := &bucket.Samples[index]
	current.Calls += sample.Calls
	current.TotalExecTimeMs += sample.TotalExecTimeMs
	current.MinExecTimeMs = min(current.MinExecTimeMs, sample.MinExecTimeMs)
	current.MaxExecTimeMs = max(current.MaxExecTimeMs, sample.MaxExecTimeMs)
	current.Rows += sample.Rows
	current.SharedBlocksHit += sample.SharedBlocksHit
	current.SharedBlocksRead += sample.SharedBlocksRead
	current.TempBlocksRead += sample.TempBlocksRead
	current.TempBlocksWritten += sample.TempBlocksWritten
	current.ReadTimeMs += sample.ReadTimeMs
	current.WriteTimeMs += sample.WriteTimeMs
	current.WALBytes += sample.WALBytes
	if len(current.LatencyHistogram) < len(sample.LatencyHistogram) {
		current.LatencyHistogram = append(current.LatencyHistogram, make([]string, len(sample.LatencyHistogram)-len(current.LatencyHistogram))...)
	}
	for i, value := range sample.LatencyHistogram {
		left, _ := strconv.ParseInt(current.LatencyHistogram[i], 10, 64)
		right, _ := strconv.ParseInt(value, 10, 64)
		current.LatencyHistogram[i] = strconv.FormatInt(left+right, 10)
	}
}

func finalizeQueryAnalyticsBucket(bucket *storage.QueryAnalyticsBucket) {
	for i := range bucket.Samples {
		if bucket.Samples[i].Calls > 0 {
			bucket.Samples[i].MeanExecTimeMs = bucket.Samples[i].TotalExecTimeMs / float64(bucket.Samples[i].Calls)
		}
	}
	totalOrder := make([]int, len(bucket.Samples))
	latencyOrder := make([]int, len(bucket.Samples))
	for i := range bucket.Samples {
		totalOrder[i], latencyOrder[i] = i, i
	}
	sort.Slice(totalOrder, func(i, j int) bool {
		return bucket.Samples[totalOrder[i]].TotalExecTimeMs > bucket.Samples[totalOrder[j]].TotalExecTimeMs
	})
	sort.Slice(latencyOrder, func(i, j int) bool {
		return bucket.Samples[latencyOrder[i]].MaxExecTimeMs > bucket.Samples[latencyOrder[j]].MaxExecTimeMs
	})
	for i := 0; i < len(bucket.Samples) && i < 100; i++ {
		bucket.Samples[totalOrder[i]].TopTotalTime = true
		bucket.Samples[latencyOrder[i]].TopMaxLatency = true
	}
	retained := bucket.Samples[:0]
	for _, sample := range bucket.Samples {
		if sample.TopTotalTime || sample.TopMaxLatency {
			retained = append(retained, sample)
		}
	}
	bucket.Samples = retained
}

const queryAnalyticsCollectionSQL = `
select bucket, bucket_start_time, pgsm_query_id::text as fingerprint_id,
       left(query, 2048) as normalized_query, coalesce(datname, '') as database_name,
       %s as role_name, coalesce(application_name, '') as application_name,
       calls, total_exec_time, min_exec_time, max_exec_time, mean_exec_time, rows,
       shared_blks_hit, shared_blks_read, temp_blks_read, temp_blks_written,
       %s as read_time_ms, %s as write_time_ms, coalesce(wal_bytes, 0)::bigint as wal_bytes,
       coalesce(resp_calls, '{}') as latency_histogram
from pg_stat_monitor
where bucket_done = true and application_name is distinct from $1
  and pgsm_query_id is not null and query is not null and query <> ''
order by bucket_start_time, bucket, pgsm_query_id`
