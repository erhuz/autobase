-- +goose Up
create table cluster_backup_evidence (
    cluster_id bigint primary key references clusters(cluster_id) on delete cascade,
    observed_at timestamptz not null,
    repository_reachable boolean not null,
    latest_full timestamptz,
    latest_differential timestamptz,
    retention jsonb not null default '{}'::jsonb,
    wal_continuous boolean,
    locks jsonb not null default '[]'::jsonb,
    scheduler_owners jsonb not null default '[]'::jsonb,
    freshness_seconds bigint not null check (freshness_seconds > 0),
    restore_tested_at timestamptz,
    updated_at timestamptz not null default current_timestamp
);

-- +goose Down
drop table cluster_backup_evidence;
