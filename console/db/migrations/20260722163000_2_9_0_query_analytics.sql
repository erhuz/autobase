-- +goose Up

-- Existing clusters remain unmanaged; cluster creation opts compatible versions in.
alter table public.clusters
  add column query_analytics_managed boolean,
  add column query_analytics_desired boolean;

update public.clusters
set query_analytics_managed = false,
    query_analytics_desired = false;

alter table public.clusters
  alter column query_analytics_managed set default false,
  alter column query_analytics_managed set not null,
  alter column query_analytics_desired set default false,
  alter column query_analytics_desired set not null;

create table public.query_analytics_sources (
  cluster_id bigint not null references public.clusters (cluster_id) on delete cascade,
  server_id bigint not null references public.servers (server_id) on delete cascade,
  node_boot_time timestamp with time zone,
  status text not null check (status in ('unknown', 'healthy', 'unreachable', 'unsupported', 'disabled')),
  extension_version text,
  last_bucket_start timestamp with time zone,
  last_collected_at timestamp with time zone,
  last_error_code text,
  primary key (cluster_id, server_id)
);

create table public.query_analytics_fingerprints (
  cluster_id bigint not null references public.clusters (cluster_id) on delete cascade,
  fingerprint_id text not null,
  normalized_query text not null,
  first_seen timestamp with time zone not null,
  last_seen timestamp with time zone not null,
  primary key (cluster_id, fingerprint_id)
);

create table public.query_analytics_buckets (
  cluster_id bigint not null references public.clusters (cluster_id) on delete cascade,
  server_id bigint not null references public.servers (server_id) on delete cascade,
  node_boot_time timestamp with time zone not null,
  bucket_id bigint not null,
  bucket_start timestamp with time zone not null,
  bucket_end timestamp with time zone not null,
  calls bigint not null,
  total_exec_time_ms double precision not null,
  max_exec_time_ms double precision not null,
  rows bigint not null,
  shared_blocks_hit bigint not null,
  shared_blocks_read bigint not null,
  temp_blocks_read bigint not null,
  temp_blocks_written bigint not null,
  read_time_ms double precision not null,
  write_time_ms double precision not null,
  wal_bytes bigint not null,
  collected_at timestamp with time zone not null default current_timestamp,
  primary key (cluster_id, server_id, node_boot_time, bucket_id),
  check (bucket_end > bucket_start)
);

create table public.query_analytics_samples (
  cluster_id bigint not null,
  server_id bigint not null,
  node_boot_time timestamp with time zone not null,
  bucket_id bigint not null,
  bucket_start timestamp with time zone not null,
  fingerprint_id text not null,
  database_name text not null,
  role_name text not null,
  application_name text not null,
  calls bigint not null,
  total_exec_time_ms double precision not null,
  min_exec_time_ms double precision not null,
  max_exec_time_ms double precision not null,
  mean_exec_time_ms double precision not null,
  rows bigint not null,
  shared_blocks_hit bigint not null,
  shared_blocks_read bigint not null,
  temp_blocks_read bigint not null,
  temp_blocks_written bigint not null,
  read_time_ms double precision not null,
  write_time_ms double precision not null,
  wal_bytes bigint not null,
  latency_histogram text[] not null default '{}',
  top_total_time boolean not null,
  top_max_latency boolean not null,
  primary key (
    cluster_id, server_id, node_boot_time, bucket_id, fingerprint_id,
    database_name, role_name, application_name
  ),
  foreign key (cluster_id, server_id, node_boot_time, bucket_id)
    references public.query_analytics_buckets (cluster_id, server_id, node_boot_time, bucket_id)
    on delete cascade,
  foreign key (cluster_id, fingerprint_id)
    references public.query_analytics_fingerprints (cluster_id, fingerprint_id)
    on delete cascade,
  check (top_total_time or top_max_latency)
);

create index query_analytics_buckets_cluster_time_idx
  on public.query_analytics_buckets (cluster_id, bucket_start desc);
create index query_analytics_samples_cluster_fingerprint_time_idx
  on public.query_analytics_samples (cluster_id, fingerprint_id, bucket_start desc);
create index query_analytics_samples_cluster_database_time_idx
  on public.query_analytics_samples (cluster_id, database_name, bucket_start desc);
create index query_analytics_samples_cluster_role_time_idx
  on public.query_analytics_samples (cluster_id, role_name, bucket_start desc);
create index query_analytics_samples_cluster_application_time_idx
  on public.query_analytics_samples (cluster_id, application_name, bucket_start desc);

-- +goose Down

drop table if exists public.query_analytics_samples;
drop table if exists public.query_analytics_buckets;
drop table if exists public.query_analytics_fingerprints;
drop table if exists public.query_analytics_sources;
alter table public.clusters
  drop column if exists query_analytics_desired,
  drop column if exists query_analytics_managed;
