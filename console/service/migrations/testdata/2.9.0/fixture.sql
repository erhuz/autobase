insert into public.projects (project_name, project_description)
values ('migration-fixture', 'Stock 2.9.0 migration fixture');

insert into public.environments (environment_name, environment_description)
values ('migration-fixture', 'Stock 2.9.0 migration fixture');

select add_secret(
  (select project_id from public.projects where project_name = 'migration-fixture'),
  'password',
  'migration-fixture',
  '{"username":"fixture-operator","password":"fixture-only-password"}',
  'fixture-only-encryption-key'
);

insert into public.settings (setting_name, setting_value)
values ('migration-fixture', '{"enabled":true,"owner":"fixture-operator"}');

insert into public.clusters (
  project_id,
  environment_id,
  secret_id,
  cluster_name,
  cluster_status,
  cluster_description,
  cluster_location,
  connection_info,
  extra_vars,
  inventory,
  postgres_version,
  flags
)
select
  p.project_id,
  e.environment_id,
  s.secret_id,
  'migration-fixture',
  'healthy',
  'Stock 2.9.0 migration fixture',
  'fixture-region',
  '{"host":"db.example.test","port":5432}',
  '{"patroni_cluster_name":"migration-fixture","postgresql_exists":true}',
  '{"all":{"hosts":{"postgresql-1":{"ansible_host":"192.0.2.10"}}}}',
  16,
  9
from public.projects p, public.environments e, public.secrets s
where p.project_name = 'migration-fixture'
  and e.environment_name = 'migration-fixture'
  and s.secret_name = 'migration-fixture';

insert into public.servers (
  cluster_id,
  server_name,
  server_location,
  server_role,
  server_status,
  ip_address,
  timeline,
  lag,
  tags,
  pending_restart
)
select
  cluster_id,
  'postgresql-1',
  'fixture-region',
  'primary',
  'running',
  '192.0.2.10',
  7,
  0,
  '{"nofailover":false}',
  false
from public.clusters
where cluster_name = 'migration-fixture';

insert into public.operations (
  project_id,
  cluster_id,
  docker_code,
  cid,
  operation_type,
  operation_status,
  operation_log,
  created_at,
  updated_at
)
select
  project_id,
  cluster_id,
  'migration-fixture',
  '00000000-0000-0000-0000-000000000001',
  'deploy',
  'success',
  'fixture operation completed',
  '2026-07-01 12:00:00+00',
  '2026-07-01 12:01:00+00'
from public.clusters
where cluster_name = 'migration-fixture';
