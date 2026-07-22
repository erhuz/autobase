package storage

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

func (s *dbStorage) GetQueryAnalyticsOverview(ctx context.Context, clusterID int64, filter *QueryAnalyticsFilter) (*QueryAnalyticsOverview, error) {
	cluster, err := s.GetCluster(ctx, clusterID)
	if err != nil {
		return nil, err
	}
	coverage, err := QueryRowsToStruct[QueryAnalyticsCoverage](ctx, s.db, `
		select sv.server_id, sv.server_name, sv.server_role, sv.server_status,
		       coalesce(src.status, 'unknown'), src.extension_version,
		       src.last_bucket_start, src.last_collected_at, src.last_error_code
		from servers sv left join query_analytics_sources src
		  on src.cluster_id = sv.cluster_id and src.server_id = sv.server_id
		where sv.cluster_id = $1 order by sv.server_name`, clusterID)
	if err != nil {
		return nil, err
	}

	status := queryAnalyticsStatus(cluster, coverage)
	summary, err := QueryRowToStruct[QueryAnalyticsMetrics](ctx, s.db, `
		select coalesce(sum(calls), 0)::bigint, coalesce(sum(total_exec_time_ms), 0)::double precision,
		       coalesce(max(max_exec_time_ms), 0)::double precision,
		       case when coalesce(sum(calls), 0) = 0 then 0 else sum(total_exec_time_ms) / sum(calls) end::double precision,
		       coalesce(sum(rows), 0)::bigint, coalesce(sum(shared_blocks_hit), 0)::bigint,
		       coalesce(sum(shared_blocks_read), 0)::bigint, coalesce(sum(temp_blocks_read), 0)::bigint,
		       coalesce(sum(temp_blocks_written), 0)::bigint, coalesce(sum(read_time_ms), 0)::double precision,
		       coalesce(sum(write_time_ms), 0)::double precision, coalesce(sum(wal_bytes), 0)::bigint
		from query_analytics_buckets
		where cluster_id = $1 and bucket_start >= $2 and bucket_start < $3
		  and ($4::bigint is null or server_id = $4)`, clusterID, filter.From, filter.To, filter.ServerID)
	if err != nil {
		return nil, err
	}

	series, err := QueryRowsToStruct[QueryAnalyticsSeriesPoint](ctx, s.db, `
		select date_trunc('minute', bucket_start), sum(calls)::bigint,
		       sum(total_exec_time_ms)::double precision, max(max_exec_time_ms)::double precision,
		       case when sum(calls) = 0 then 0 else sum(total_exec_time_ms) / sum(calls) end::double precision,
		       sum(rows)::bigint, sum(shared_blocks_hit)::bigint, sum(shared_blocks_read)::bigint,
		       sum(temp_blocks_read)::bigint, sum(temp_blocks_written)::bigint,
		       sum(read_time_ms)::double precision, sum(write_time_ms)::double precision, sum(wal_bytes)::bigint
		from query_analytics_buckets
		where cluster_id = $1 and bucket_start >= $2 and bucket_start < $3
		  and ($4::bigint is null or server_id = $4)
		group by date_trunc('minute', bucket_start) order by 1`, clusterID, filter.From, filter.To, filter.ServerID)
	if err != nil {
		return nil, err
	}

	queries, err := QueryRowsToStruct[QueryAnalyticsQuery](ctx, s.db, queryAnalyticsQueriesSQL,
		clusterID, filter.From, filter.To, filter.ServerID, filter.DatabaseName, filter.RoleName, filter.ApplicationName)
	if err != nil {
		return nil, err
	}
	options, err := QueryRowToStruct[QueryAnalyticsFilterOptions](ctx, s.db, `
		select coalesce(array_agg(distinct database_name order by database_name), '{}'),
		       coalesce(array_agg(distinct role_name order by role_name), '{}'),
		       coalesce(array_agg(distinct application_name order by application_name), '{}')
		from query_analytics_samples where cluster_id = $1 and bucket_start >= $2 and bucket_start < $3`,
		clusterID, filter.From, filter.To)
	if err != nil {
		return nil, err
	}

	return &QueryAnalyticsOverview{
		Status: status, Coverage: coverage, Summary: *summary, Series: series, Queries: queries, Filters: *options,
	}, nil
}

func (s *dbStorage) GetQueryAnalyticsDetail(ctx context.Context, clusterID int64, fingerprintID string, filter *QueryAnalyticsFilter) (*QueryAnalyticsDetail, error) {
	query, err := QueryRowToStruct[QueryAnalyticsQuery](ctx, s.db, queryAnalyticsDetailSQL,
		clusterID, fingerprintID, filter.From, filter.To, filter.ServerID, filter.DatabaseName, filter.RoleName, filter.ApplicationName)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	series, err := QueryRowsToStruct[QueryAnalyticsSeriesPoint](ctx, s.db, `
		select date_trunc('minute', bucket_start), sum(calls)::bigint,
		       sum(total_exec_time_ms)::double precision, max(max_exec_time_ms)::double precision,
		       case when sum(calls) = 0 then 0 else sum(total_exec_time_ms) / sum(calls) end::double precision,
		       sum(rows)::bigint, sum(shared_blocks_hit)::bigint, sum(shared_blocks_read)::bigint,
		       sum(temp_blocks_read)::bigint, sum(temp_blocks_written)::bigint,
		       sum(read_time_ms)::double precision, sum(write_time_ms)::double precision, sum(wal_bytes)::bigint
		from query_analytics_samples
		where cluster_id = $1 and fingerprint_id = $2 and bucket_start >= $3 and bucket_start < $4
		  and ($5::bigint is null or server_id = $5) and ($6::text is null or database_name = $6)
		  and ($7::text is null or role_name = $7) and ($8::text is null or application_name = $8)
		group by date_trunc('minute', bucket_start) order by 1`,
		clusterID, fingerprintID, filter.From, filter.To, filter.ServerID, filter.DatabaseName, filter.RoleName, filter.ApplicationName)
	if err != nil {
		return nil, err
	}
	histogram, err := QueryRowToScalar[[]string](ctx, s.db, `
		select latency_histogram from query_analytics_samples
		where cluster_id = $1 and fingerprint_id = $2 and bucket_start >= $3 and bucket_start < $4
		  and ($5::bigint is null or server_id = $5) and ($6::text is null or database_name = $6)
		  and ($7::text is null or role_name = $7) and ($8::text is null or application_name = $8)
		order by bucket_start desc limit 1`,
		clusterID, fingerprintID, filter.From, filter.To, filter.ServerID, filter.DatabaseName, filter.RoleName, filter.ApplicationName)
	if errors.Is(err, pgx.ErrNoRows) {
		histogram = []string{}
	} else if err != nil {
		return nil, err
	}
	return &QueryAnalyticsDetail{Fingerprint: *query, Series: series, Histogram: histogram}, nil
}

func queryAnalyticsStatus(cluster *Cluster, coverage []QueryAnalyticsCoverage) QueryAnalyticsStatus {
	status := QueryAnalyticsStatus{
		Managed: cluster.QueryAnalyticsManaged, Desired: cluster.QueryAnalyticsDesired, PostgresVersion: cluster.PostgreVersion,
	}
	if cluster.PostgreVersion < 14 || cluster.PostgreVersion > 18 {
		status.State = "unsupported"
		return status
	}
	if !cluster.QueryAnalyticsManaged {
		status.State = "rollout_required"
		return status
	}
	if !cluster.QueryAnalyticsDesired {
		status.State = "disabled"
		return status
	}
	for _, source := range coverage {
		if source.ServerStatus == "running" || source.ServerStatus == "streaming" {
			status.ExpectedNodeCount++
			if source.CollectionStatus == "healthy" {
				status.CollectedNodeCount++
			}
		}
	}
	if status.CollectedNodeCount == 0 && status.ExpectedNodeCount > 0 {
		status.State = "collecting"
	} else if status.ExpectedNodeCount > 0 && status.CollectedNodeCount == status.ExpectedNodeCount {
		status.State = "enabled"
	} else {
		status.State = "degraded"
	}
	return status
}

const queryAnalyticsQueriesSQL = `
select s.fingerprint_id, f.normalized_query, sum(s.calls)::bigint,
       sum(s.total_exec_time_ms)::double precision, max(s.max_exec_time_ms)::double precision,
       case when sum(s.calls) = 0 then 0 else sum(s.total_exec_time_ms) / sum(s.calls) end::double precision,
       sum(s.rows)::bigint, sum(s.shared_blocks_hit)::bigint, sum(s.shared_blocks_read)::bigint,
       sum(s.temp_blocks_read)::bigint, sum(s.temp_blocks_written)::bigint,
       sum(s.read_time_ms)::double precision, sum(s.write_time_ms)::double precision, sum(s.wal_bytes)::bigint,
       bool_or(s.top_total_time), bool_or(s.top_max_latency)
from query_analytics_samples s join query_analytics_fingerprints f
  on f.cluster_id = s.cluster_id and f.fingerprint_id = s.fingerprint_id
where s.cluster_id = $1 and s.bucket_start >= $2 and s.bucket_start < $3
  and ($4::bigint is null or s.server_id = $4) and ($5::text is null or s.database_name = $5)
  and ($6::text is null or s.role_name = $6) and ($7::text is null or s.application_name = $7)
group by s.fingerprint_id, f.normalized_query order by sum(s.total_exec_time_ms) desc limit 100`

const queryAnalyticsDetailSQL = `
select s.fingerprint_id, f.normalized_query, sum(s.calls)::bigint,
       sum(s.total_exec_time_ms)::double precision, max(s.max_exec_time_ms)::double precision,
       case when sum(s.calls) = 0 then 0 else sum(s.total_exec_time_ms) / sum(s.calls) end::double precision,
       sum(s.rows)::bigint, sum(s.shared_blocks_hit)::bigint, sum(s.shared_blocks_read)::bigint,
       sum(s.temp_blocks_read)::bigint, sum(s.temp_blocks_written)::bigint,
       sum(s.read_time_ms)::double precision, sum(s.write_time_ms)::double precision, sum(s.wal_bytes)::bigint,
       bool_or(s.top_total_time), bool_or(s.top_max_latency)
from query_analytics_samples s join query_analytics_fingerprints f
  on f.cluster_id = s.cluster_id and f.fingerprint_id = s.fingerprint_id
where s.cluster_id = $1 and s.fingerprint_id = $2 and s.bucket_start >= $3 and s.bucket_start < $4
  and ($5::bigint is null or s.server_id = $5) and ($6::text is null or s.database_name = $6)
  and ($7::text is null or s.role_name = $7) and ($8::text is null or s.application_name = $8)
group by s.fingerprint_id, f.normalized_query`
