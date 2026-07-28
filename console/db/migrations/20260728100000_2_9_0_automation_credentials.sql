-- +goose Up

alter table public.clusters
  add column postgres_superuser_secret_id bigint references public.secrets (secret_id),
  add column postgres_replication_secret_id bigint references public.secrets (secret_id),
  add column patroni_restapi_secret_id bigint references public.secrets (secret_id);

create index clusters_postgres_superuser_secret_id_idx
  on public.clusters (postgres_superuser_secret_id);
create index clusters_postgres_replication_secret_id_idx
  on public.clusters (postgres_replication_secret_id);
create index clusters_patroni_restapi_secret_id_idx
  on public.clusters (patroni_restapi_secret_id);

create or replace view public.v_secrets_list as
select
  s.project_id,
  s.secret_id,
  s.secret_name,
  s.secret_type,
  s.created_at,
  s.updated_at,
  count(c.cluster_name) > 0 as used,
  coalesce(string_agg(distinct c.cluster_name, ', '), '') as used_by_clusters
from
  public.secrets s
  left join lateral (
    select cl.cluster_name
    from public.clusters cl
    where cl.project_id = s.project_id
      and s.secret_id in (
        cl.secret_id,
        cl.postgres_superuser_secret_id,
        cl.postgres_replication_secret_id,
        cl.patroni_restapi_secret_id
      )
  ) c on true
group by
  s.project_id,
  s.secret_id,
  s.secret_name,
  s.secret_type,
  s.created_at,
  s.updated_at;

-- +goose Down

create or replace view public.v_secrets_list as
select
  s.project_id,
  s.secret_id,
  s.secret_name,
  s.secret_type,
  s.created_at,
  s.updated_at,
  case when count(c.secret_id) > 0 then
    true
  else
    false
  end as used,
  coalesce(string_agg(distinct c.cluster_name, ', '), '') as used_by_clusters
from
  public.secrets s
  left join lateral (
    select
      cluster_name,
      secret_id
    from
      public.clusters
    where
      secret_id = s.secret_id
      and project_id = s.project_id
  ) c on true
group by
  s.project_id,
  s.secret_id,
  s.secret_name,
  s.secret_type,
  s.created_at,
  s.updated_at;

drop index if exists public.clusters_patroni_restapi_secret_id_idx;
drop index if exists public.clusters_postgres_replication_secret_id_idx;
drop index if exists public.clusters_postgres_superuser_secret_id_idx;

alter table public.clusters
  drop column if exists patroni_restapi_secret_id,
  drop column if exists postgres_replication_secret_id,
  drop column if exists postgres_superuser_secret_id;
