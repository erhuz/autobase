import { expect, test } from '@playwright/test';

const token = 'management-v1-token';

test('management v1 shows recoverability, guards analytics, and preserves audit evidence', async ({ page }) => {
  let operationRequest: unknown;

  await page.addInitScript((value) => localStorage.setItem('token', value), token);
  await page.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname.replace(/^\/api\/v1/, '');

    if (path === '/projects') {
      return route.fulfill({ json: { data: [{ id: 1, name: 'Management' }] } });
    }
    if (path === '/clusters/5') {
      return route.fulfill({
        json: {
          id: 5,
          name: 'management-fixture',
          status: 'healthy',
          postgres_version: 16,
          environment: 'test',
          cluster_location: 'fixture-region',
          servers: [
            { id: 1, name: 'postgresql-1', role: 'primary', status: 'running', timeline: 7, lag: 0 },
            { id: 2, name: 'postgresql-2', role: 'replica', status: 'streaming', timeline: 7, lag: 0 },
          ],
        },
      });
    }
    if (path === '/clusters/5/health') {
      return route.fulfill({
        json: {
          observed_at: '2026-07-23T10:00:00Z',
          topology: {
            state: 'healthy',
            patroni_reachable: true,
            leader: { name: 'postgresql-1', role: 'leader', state: 'running', timeline: 7, lag: 0 },
            replicas: [{ name: 'postgresql-2', role: 'replica', state: 'streaming', timeline: 7, lag: 0 }],
          },
          dcs: { state: 'healthy', type: 'etcd', reachable: true, members: ['dcs-1', 'dcs-2', 'dcs-3'] },
          routing: {
            state: 'healthy',
            targets: [{ role: 'primary', address: 'primary.internal', port: 5000, reachable: true, role_matches: true }],
          },
          backup: {
            state: 'degraded',
            repository_reachable: true,
            latest_full: '2026-07-23T02:00:00Z',
            latest_differential: '2026-07-23T08:00:00Z',
            retention: { full: 2, differential: 6 },
            wal_continuous: true,
            locks: [],
            scheduler_owner: 'postgresql-2',
            fresh: true,
            freshness_policy: '24h',
          },
          operation: {
            unresolved: { id: 17, type: 'query_analytics_enable', status: 'failed', safe_next_action: 'Run a fresh preflight.' },
          },
          recoverability: { state: 'degraded', reasons: ['restore_evidence_missing'] },
        },
      });
    }
    if (path === '/clusters/5/query-performance/42') {
      return route.fulfill({
        json: {
          fingerprint: { normalized_query: 'select * from users where id = $1' },
          series: [{ bucket_start: '2026-07-23T09:00:00Z', metrics: { total_exec_time_ms: 12 } }],
          histogram: [1, 3, 1],
        },
      });
    }
    if (path === '/clusters/5/query-performance') {
      return route.fulfill({
        json: {
          status: { state: 'enabled', collected_node_count: 2, expected_node_count: 2 },
          coverage: [
            { server_id: 1, server_name: 'postgresql-1', collection_status: 'healthy' },
            { server_id: 2, server_name: 'postgresql-2', collection_status: 'healthy' },
          ],
          summary: { calls: 3, total_exec_time_ms: 12, mean_exec_time_ms: 4, max_exec_time_ms: 7 },
          series: [{ bucket_start: '2026-07-23T09:00:00Z', metrics: { total_exec_time_ms: 12 } }],
          queries: [
            {
              fingerprint_id: '42',
              normalized_query: 'select * from users where id = $1',
              metrics: { calls: 3, total_exec_time_ms: 12, max_exec_time_ms: 7 },
            },
          ],
          filters: { databases: ['postgres'], roles: ['app'], applications: ['portal'] },
        },
      });
    }
    if (path === '/clusters/5/preflights' && request.method() === 'POST') {
      return route.fulfill({
        json: {
          id: 11,
          confirmation: 'DISABLE QUERY ANALYTICS',
          blockers: [],
          affected_nodes: ['postgresql-2', 'postgresql-1'],
          plan: ['configure and verify replica', 'controlled switchover'],
        },
      });
    }
    if (path === '/clusters/5/operations' && request.method() === 'POST') {
      operationRequest = request.postDataJSON();
      return route.fulfill({ json: { operation_id: 17, status: 'running' } });
    }
    if (path === '/operations/17/log') {
      return route.fulfill({
        body: 'task output without credentials',
        headers: { 'content-type': 'text/plain', 'x-log-completed': 'true' },
      });
    }
    if (path === '/operations/17') {
      return route.fulfill({
        json: {
          id: 17,
          cluster_id: 5,
          type: 'query_analytics_enable',
          status: 'failed',
          actor: 'api-token',
          sanitized_params: { state: 'enabled', password: '[REDACTED]' },
          preflight_snapshot: { id: 11, checks: [{ name: 'all nodes healthy', ok: true }] },
          plan: ['configure replica', 'verify topology'],
          affected_nodes: ['postgresql-2'],
          final_verification: { verified: false },
          safe_next_action: 'Run a fresh preflight.',
          started: '2026-07-23T10:00:00Z',
          finished: '2026-07-23T10:05:00Z',
        },
      });
    }

    return route.fulfill({ status: 404, json: { path } });
  });

  await page.goto('/clusters/5/overview');
  await expect(page.getByText('Database availability does not prove recoverability.')).toBeVisible();
  await expect(page.getByText('restore evidence missing')).toBeVisible();
  await expect(page.getByText(/Scheduler owner: postgresql-2/)).toBeVisible();
  await expect(page.getByText('select * from users where id = $1')).toBeVisible();

  await page.getByRole('button', { name: 'Details' }).click();
  await expect(page.getByRole('img', { name: 'Latency distribution' })).toBeVisible();

  await page.getByRole('button', { name: 'Disable analytics' }).click();
  const start = page.getByRole('button', { name: 'Start operation' });
  await expect(start).toBeDisabled();
  await page.getByLabel('Confirmation phrase').fill('DISABLE QUERY ANALYTICS');
  await start.click();
  expect(operationRequest).toEqual({ preflight_id: 11, confirmation: 'DISABLE QUERY ANALYTICS' });

  await page.goto('/operations/17/log');
  await expect(page.getByText('Operation #17')).toBeVisible();
  await expect(page.getByText(/REDACTED/)).toBeVisible();
  await expect(page.getByText('Run a fresh preflight.')).toBeVisible();
  await expect(page.getByText('postgresql-2')).toBeVisible();
});
