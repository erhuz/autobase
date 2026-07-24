-- +goose Up

alter table public.operation_preflights
  drop constraint operation_preflights_operation_type_check,
  add constraint operation_preflights_operation_type_check
  check (operation_type in (
    'switchover',
    'reload',
    'rolling_restart',
    'replica_reinit',
    'backup_full',
    'backup_diff',
    'query_analytics_enable',
    'query_analytics_disable',
    'node_add',
    'node_remove',
    'config_update',
    'rolling_update',
    'postgresql_upgrade',
    'emergency_failover',
    'restore',
    'pitr',
    'database_admin',
    'extension_admin',
    'pgbouncer_admin'
  ));

-- +goose Down

alter table public.operation_preflights
  drop constraint operation_preflights_operation_type_check,
  add constraint operation_preflights_operation_type_check
  check (operation_type in ('query_analytics_enable', 'query_analytics_disable'))
  not valid;
