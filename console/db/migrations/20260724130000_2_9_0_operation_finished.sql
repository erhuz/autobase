-- +goose Up

create or replace view public.v_operations as
select
  op.project_id,
  op.cluster_id,
  op.id,
  op.created_at as "started",
  case when op.operation_status in ('succeeded', 'failed', 'cancelled') then op.updated_at end as "finished",
  op.operation_type as "type",
  op.operation_status as "status",
  cl.cluster_name as "cluster",
  env.environment_name as "environment"
from
  public.operations op
  join public.clusters cl on op.cluster_id = cl.cluster_id
  join public.projects pr on op.project_id = pr.project_id
  join public.environments env on cl.environment_id = env.environment_id;

-- +goose Down

create or replace view public.v_operations as
select
  op.project_id,
  op.cluster_id,
  op.id,
  op.created_at as "started",
  op.updated_at as "finished",
  op.operation_type as "type",
  op.operation_status as "status",
  cl.cluster_name as "cluster",
  env.environment_name as "environment"
from
  public.operations op
  join public.clusters cl on op.cluster_id = cl.cluster_id
  join public.projects pr on op.project_id = pr.project_id
  join public.environments env on cl.environment_id = env.environment_id;
