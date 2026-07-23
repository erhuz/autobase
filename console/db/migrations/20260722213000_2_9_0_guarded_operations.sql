-- +goose Up

alter table public.operations drop constraint operations_operation_status_check;
alter table public.operations disable trigger handle_updated_at;
update public.operations set operation_status = 'running' where operation_status = 'in_progress';
update public.operations set operation_status = 'succeeded' where operation_status = 'success';
alter table public.operations enable trigger handle_updated_at;
alter table public.operations
  add constraint operations_operation_status_check
  check (operation_status in ('queued', 'running', 'succeeded', 'failed', 'cancelled'));

alter table public.operations
  add column actor text not null default 'api-token',
  add column sanitized_params jsonb not null default '{}',
  add column preflight_snapshot jsonb not null default '{}',
  add column plan jsonb not null default '[]',
  add column affected_nodes jsonb not null default '[]',
  add column final_verification jsonb not null default '{}',
  add column safe_next_action text;

create table public.operation_preflights (
  id bigserial primary key,
  cluster_id bigint not null references public.clusters (cluster_id) on delete cascade,
  operation_type text not null check (operation_type in ('query_analytics_enable', 'query_analytics_disable')),
  observed jsonb not null,
  desired jsonb not null,
  checks jsonb not null,
  blockers jsonb not null,
  plan jsonb not null,
  affected_nodes jsonb not null,
  confirmation text not null,
  topology_hash text not null,
  expires_at timestamp with time zone not null,
  consumed_at timestamp with time zone,
  created_at timestamp with time zone not null default current_timestamp
);

create index operation_preflights_cluster_created_idx
  on public.operation_preflights (cluster_id, created_at desc);

-- Separate lock table works for both plain PostgreSQL and a Timescale operations hypertable.
create table public.cluster_operation_locks (
  cluster_id bigint primary key references public.clusters (cluster_id) on delete cascade,
  operation_id bigint not null,
  created_at timestamp with time zone not null default current_timestamp
);

-- +goose StatementBegin
create function public.release_cluster_operation_lock() returns trigger as $$
begin
  if new.operation_status in ('succeeded', 'failed', 'cancelled') then
    delete from public.cluster_operation_locks
    where cluster_id = new.cluster_id and operation_id = new.id;
  end if;
  return new;
end;
$$ language plpgsql;
-- +goose StatementEnd

create trigger release_cluster_operation_lock
  after update of operation_status on public.operations
  for each row execute function public.release_cluster_operation_lock();

-- +goose StatementBegin
create function public.keep_terminal_operation_immutable() returns trigger as $$
begin
  if old.operation_status in ('succeeded', 'failed', 'cancelled') then
    if new.operation_status is distinct from old.operation_status
      or (to_jsonb(new) - 'operation_log' - 'updated_at') is distinct from
         (to_jsonb(old) - 'operation_log' - 'updated_at')
      or (old.operation_log is not null and new.operation_log not like old.operation_log || '%') then
      raise exception 'terminal operation % is immutable', old.id;
    end if;
  end if;
  return new;
end;
$$ language plpgsql;
-- +goose StatementEnd

create trigger keep_terminal_operation_immutable
  before update on public.operations
  for each row execute function public.keep_terminal_operation_immutable();

-- +goose Down

drop trigger if exists keep_terminal_operation_immutable on public.operations;
drop function if exists public.keep_terminal_operation_immutable();
drop trigger if exists release_cluster_operation_lock on public.operations;
drop function if exists public.release_cluster_operation_lock();
drop table if exists public.cluster_operation_locks;
drop table if exists public.operation_preflights;

alter table public.operations
  drop column if exists safe_next_action,
  drop column if exists final_verification,
  drop column if exists affected_nodes,
  drop column if exists plan,
  drop column if exists preflight_snapshot,
  drop column if exists sanitized_params,
  drop column if exists actor;

alter table public.operations drop constraint operations_operation_status_check;
alter table public.operations disable trigger handle_updated_at;
update public.operations set operation_status = 'in_progress' where operation_status in ('queued', 'running');
update public.operations set operation_status = 'success' where operation_status = 'succeeded';
update public.operations set operation_status = 'failed' where operation_status = 'cancelled';
alter table public.operations enable trigger handle_updated_at;
alter table public.operations
  add constraint operations_operation_status_check
  check (operation_status in ('in_progress', 'success', 'failed'));
