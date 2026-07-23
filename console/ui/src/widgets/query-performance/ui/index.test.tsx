import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import '@testing-library/jest-dom/vitest';
import { describe, expect, it, vi } from 'vitest';
import '@shared/i18n/i18n';
import QueryPerformance, { buildQueryPerformanceArgs, normalizeBars, queryPerformanceStatusConfig } from './index';

const api = vi.hoisted(() => ({
  getDetail: vi.fn(),
  getOverview: vi.fn(),
  preflight: vi.fn(() => ({
    unwrap: () =>
      Promise.resolve({
        id: 11,
        confirmation: 'DISABLE QUERY ANALYTICS',
        blockers: [],
        affected_nodes: ['postgresql-2', 'postgresql-1'],
        plan: ['configure and verify replica postgresql-2', 'controlled switchover from postgresql-1'],
      }),
  })),
  start: vi.fn(() => ({ unwrap: () => Promise.resolve({ operation_id: 17, status: 'running' }) })),
}));

vi.mock('@shared/api/api/clusters.ts', () => ({
  useGetClustersByIdQueryPerformanceQuery: (args: unknown) => {
    api.getOverview(args);
    return {
      data: {
        status: { state: 'enabled', collected_node_count: 1, expected_node_count: 1 },
        coverage: [{ server_id: 2, server_name: 'postgresql-1', collection_status: 'healthy' }],
        summary: { calls: 3, total_exec_time_ms: 12, mean_exec_time_ms: 4, max_exec_time_ms: 7 },
        series: [{ bucket_start: '2026-07-22T12:00:00Z', metrics: { total_exec_time_ms: 12 } }],
        queries: [
          {
            fingerprint_id: '42',
            normalized_query: 'select * from users where id = $1',
            metrics: { calls: 3, total_exec_time_ms: 12, max_exec_time_ms: 7 },
          },
        ],
        filters: { databases: ['postgres'], roles: ['app'], applications: ['portal'] },
      },
      isError: false,
      isFetching: false,
      refetch: vi.fn(),
    };
  },
  useLazyGetClustersByIdQueryPerformanceFingerprintIdQuery: () => [
    api.getDetail,
    {
      data: {
        fingerprint: { normalized_query: 'select * from users where id = $1' },
        series: [{ bucket_start: '2026-07-22T12:00:00Z', metrics: { total_exec_time_ms: 12 } }],
        histogram: [1, 3, 1],
      },
      isError: false,
      isFetching: false,
    },
  ],
  usePostClustersByIdPreflightsMutation: () => [api.preflight, { isLoading: false }],
  usePostClustersByIdOperationsMutation: () => [api.start, { isLoading: false }],
}));

describe('query performance UI', () => {
  it('maps every collection status to an operator message', () => {
    for (const state of ['enabled', 'collecting', 'degraded', 'rollout_required', 'disabled', 'unsupported'] as const) {
      expect(queryPerformanceStatusConfig(state).key).toContain('queryPerformanceStatus');
    }
  });

  it('builds node, database, role, and application filters', () => {
    const args = buildQueryPerformanceArgs(
      7,
      { rangeHours: 6, serverId: 9, database: 'app', role: 'reader', application: 'portal' },
      Date.parse('2026-07-22T12:00:00Z'),
    );
    expect(args).toMatchObject({
      id: 7,
      from: '2026-07-22T06:00:00.000Z',
      to: '2026-07-22T12:00:00.000Z',
      serverId: 9,
      database: 'app',
      role: 'reader',
      application: 'portal',
    });
    expect(buildQueryPerformanceArgs(7, { rangeHours: 168 }, Date.parse('2026-07-22T12:00:00Z')).from).toBe(
      '2026-07-15T12:00:00.000Z',
    );
  });

  it('normalizes trend values without dividing by zero', () => {
    expect(normalizeBars([0, 5, 10])).toEqual([0, 50, 100]);
    expect(normalizeBars([0, 0])).toEqual([0, 0]);
  });

  it('normalizes detail histogram buckets', () => {
    expect(normalizeBars([1, 4, 2])).toEqual([25, 100, 50]);
  });

  it('renders status, filters, trend, and on-demand detail histogram', () => {
    const clock = vi.spyOn(Date, 'now').mockReturnValue(Date.parse('2026-07-22T12:00:00Z'));
    render(<QueryPerformance clusterId={7} />);

    expect(screen.getByText('Collecting from 1 of 1 database nodes.')).toBeInTheDocument();
    expect(screen.getByLabelText('Database')).toBeInTheDocument();
    expect(screen.getByRole('img', { name: 'Total execution time trend' })).toBeInTheDocument();

    clock.mockReturnValue(Date.parse('2026-07-22T12:01:00Z'));
    fireEvent.click(screen.getByRole('button', { name: 'Refresh' }));
    expect(api.getOverview).toHaveBeenLastCalledWith(expect.objectContaining({ to: '2026-07-22T12:01:00.000Z' }));

    fireEvent.click(screen.getByRole('button', { name: 'Details' }));
    expect(api.getDetail).toHaveBeenCalledWith(expect.objectContaining({ id: 7, fingerprintId: '42' }));
    expect(screen.getByRole('img', { name: 'Latency distribution' })).toBeInTheDocument();
    clock.mockRestore();
  });

  it('requires the exact preflight confirmation before launch', async () => {
    cleanup();
    const view = render(<QueryPerformance clusterId={7} />);
    fireEvent.click(view.getByRole('button', { name: 'Disable analytics' }));
    const confirmation = await view.findByLabelText('Confirmation phrase');
    expect(view.getByRole('button', { name: 'Start operation' })).toBeDisabled();
    fireEvent.change(confirmation, { target: { value: 'DISABLE QUERY ANALYTICS' } });
    fireEvent.click(view.getByRole('button', { name: 'Start operation' }));
    await waitFor(() =>
      expect(api.start).toHaveBeenCalledWith({
        id: 7,
        requestOperationStart: { preflight_id: 11, confirmation: 'DISABLE QUERY ANALYTICS' },
      }),
    );
  });
});
