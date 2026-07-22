package storage

import (
	"context"
	"errors"
	"strings"
	"time"
)

const maxQueryAnalyticsSamplesPerBucket = 200

func queryAnalyticsEnabledByDefault(postgresVersion int) bool {
	return postgresVersion >= 14 && postgresVersion <= 18
}

func validateQueryAnalyticsBucket(bucket *QueryAnalyticsBucket) error {
	if bucket == nil || !bucket.Complete {
		return errors.New("query analytics bucket must be complete")
	}
	if bucket.ClusterID < 1 || bucket.ServerID < 1 || bucket.NodeBootTime.IsZero() || bucket.BucketStart.IsZero() || !bucket.BucketEnd.After(bucket.BucketStart) {
		return errors.New("query analytics bucket identity is invalid")
	}
	if len(bucket.Samples) > maxQueryAnalyticsSamplesPerBucket {
		return errors.New("query analytics bucket exceeds sample limit")
	}
	for _, sample := range bucket.Samples {
		if sample.ClusterID != bucket.ClusterID || sample.ServerID != bucket.ServerID || !sample.NodeBootTime.Equal(bucket.NodeBootTime) || sample.BucketID != bucket.BucketID {
			return errors.New("query analytics sample does not match bucket")
		}
		if sample.FingerprintID == "" || sample.NormalizedQuery == "" || strings.ContainsRune(sample.NormalizedQuery, '\x00') || (!sample.TopTotalTime && !sample.TopMaxLatency) {
			return errors.New("query analytics sample is invalid")
		}
	}
	return nil
}

func (s *dbStorage) UpsertQueryAnalyticsSource(ctx context.Context, source *QueryAnalyticsSource) error {
	_, err := s.db.Exec(ctx, `
		insert into query_analytics_sources (
			cluster_id, server_id, node_boot_time, status, extension_version,
			last_bucket_start, last_collected_at, last_error_code
		) values ($1, $2, $3, $4, $5, $6, $7, $8)
		on conflict (cluster_id, server_id) do update set
			node_boot_time = coalesce(excluded.node_boot_time, query_analytics_sources.node_boot_time),
			status = excluded.status,
			extension_version = coalesce(excluded.extension_version, query_analytics_sources.extension_version),
			last_bucket_start = coalesce(excluded.last_bucket_start, query_analytics_sources.last_bucket_start),
			last_collected_at = coalesce(excluded.last_collected_at, query_analytics_sources.last_collected_at),
			last_error_code = excluded.last_error_code`,
		source.ClusterID, source.ServerID, source.NodeBootTime, source.Status, source.ExtensionVersion,
		source.LastBucketStart, source.LastCollectedAt, source.LastErrorCode)
	return err
}

func (s *dbStorage) IngestQueryAnalyticsBucket(ctx context.Context, bucket *QueryAnalyticsBucket) error {
	if err := validateQueryAnalyticsBucket(bucket); err != nil {
		return err
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	for _, sample := range bucket.Samples {
		_, err = tx.Exec(ctx, `
			insert into query_analytics_fingerprints (
				cluster_id, fingerprint_id, normalized_query, first_seen, last_seen
			) values ($1, $2, $3, $4, $4)
			on conflict (cluster_id, fingerprint_id) do update set
				normalized_query = excluded.normalized_query,
				last_seen = greatest(query_analytics_fingerprints.last_seen, excluded.last_seen)`,
			sample.ClusterID, sample.FingerprintID, sample.NormalizedQuery, bucket.BucketStart)
		if err != nil {
			return err
		}
	}

	_, err = tx.Exec(ctx, `
		insert into query_analytics_buckets (
			cluster_id, server_id, node_boot_time, bucket_id, bucket_start, bucket_end,
			calls, total_exec_time_ms, max_exec_time_ms, rows, shared_blocks_hit,
			shared_blocks_read, temp_blocks_read, temp_blocks_written, read_time_ms,
			write_time_ms, wal_bytes
		) values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
		on conflict (cluster_id, server_id, node_boot_time, bucket_id) do nothing`,
		bucket.ClusterID, bucket.ServerID, bucket.NodeBootTime, bucket.BucketID, bucket.BucketStart,
		bucket.BucketEnd, bucket.Calls, bucket.TotalExecTimeMs, bucket.MaxExecTimeMs, bucket.Rows,
		bucket.SharedBlocksHit, bucket.SharedBlocksRead, bucket.TempBlocksRead, bucket.TempBlocksWritten,
		bucket.ReadTimeMs, bucket.WriteTimeMs, bucket.WALBytes)
	if err != nil {
		return err
	}

	for _, sample := range bucket.Samples {
		histogram := sample.LatencyHistogram
		if histogram == nil {
			histogram = []string{}
		}
		_, err = tx.Exec(ctx, `
			insert into query_analytics_samples (
				cluster_id, server_id, node_boot_time, bucket_id, bucket_start, fingerprint_id,
				database_name, role_name, application_name, calls, total_exec_time_ms,
				min_exec_time_ms, max_exec_time_ms, mean_exec_time_ms, rows,
				shared_blocks_hit, shared_blocks_read, temp_blocks_read, temp_blocks_written,
				read_time_ms, write_time_ms, wal_bytes, latency_histogram,
				top_total_time, top_max_latency
			) values (
				$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12,
				$13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25
			)
			on conflict (cluster_id, server_id, node_boot_time, bucket_id, fingerprint_id, database_name, role_name, application_name) do nothing`,
			sample.ClusterID, sample.ServerID, sample.NodeBootTime, sample.BucketID, bucket.BucketStart, sample.FingerprintID,
			sample.DatabaseName, sample.RoleName, sample.ApplicationName, sample.Calls, sample.TotalExecTimeMs,
			sample.MinExecTimeMs, sample.MaxExecTimeMs, sample.MeanExecTimeMs, sample.Rows,
			sample.SharedBlocksHit, sample.SharedBlocksRead, sample.TempBlocksRead, sample.TempBlocksWritten,
			sample.ReadTimeMs, sample.WriteTimeMs, sample.WALBytes, histogram,
			sample.TopTotalTime, sample.TopMaxLatency)
		if err != nil {
			return err
		}
	}

	now := time.Now().UTC()
	_, err = tx.Exec(ctx, `
		insert into query_analytics_sources (
			cluster_id, server_id, node_boot_time, status, last_bucket_start, last_collected_at
		) values ($1, $2, $3, 'healthy', $4, $5)
		on conflict (cluster_id, server_id) do update set
			node_boot_time = excluded.node_boot_time,
			status = excluded.status,
			last_bucket_start = greatest(query_analytics_sources.last_bucket_start, excluded.last_bucket_start),
			last_collected_at = excluded.last_collected_at,
			last_error_code = null`,
		bucket.ClusterID, bucket.ServerID, bucket.NodeBootTime, bucket.BucketStart, now)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (s *dbStorage) PurgeQueryAnalyticsBefore(ctx context.Context, before time.Time) (int64, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	result, err := tx.Exec(ctx, "delete from query_analytics_buckets where bucket_start < $1", before)
	if err != nil {
		return 0, err
	}
	deleted := result.RowsAffected()
	if _, err = tx.Exec(ctx, "delete from query_analytics_fingerprints where last_seen < $1", before); err != nil {
		return 0, err
	}
	if err = tx.Commit(ctx); err != nil {
		return 0, err
	}
	return deleted, nil
}

func (s *dbStorage) SetQueryAnalyticsCredential(ctx context.Context, clusterID int64, password, encryptionKey string) error {
	_, err := s.db.Exec(ctx, `
		insert into query_analytics_credentials (cluster_id, password_ciphertext)
		values ($1, extensions.pgp_sym_encrypt($2, $3, 'cipher-algo=aes256'))
		on conflict (cluster_id) do update set
			password_ciphertext = excluded.password_ciphertext,
			updated_at = current_timestamp`, clusterID, password, encryptionKey)
	return err
}

func (s *dbStorage) GetQueryAnalyticsCredential(ctx context.Context, clusterID int64, encryptionKey string) (string, error) {
	return QueryRowToScalar[string](ctx, s.db, `
		select extensions.pgp_sym_decrypt(password_ciphertext, $2)
		from query_analytics_credentials where cluster_id = $1`, clusterID, encryptionKey)
}
