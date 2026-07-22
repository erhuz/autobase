-- +goose Up

create table public.query_analytics_credentials (
  cluster_id bigint primary key references public.clusters (cluster_id) on delete cascade,
  password_ciphertext bytea not null,
  updated_at timestamp with time zone not null default current_timestamp
);

-- +goose Down

drop table if exists public.query_analytics_credentials;
