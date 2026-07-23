import { render, screen } from '@testing-library/react';
import '@testing-library/jest-dom/vitest';
import { MemoryRouter } from 'react-router-dom';
import { describe, expect, it, vi } from 'vitest';
import '@shared/i18n/i18n';
import ClusterHealth from './index';

vi.mock('@app/redux/store/hooks.ts', () => ({ useAppSelector: () => 0 }));
vi.mock('@shared/api/api/clusters.ts', () => ({
  useGetClustersByIdHealthQuery: () => ({
    data: {
      observed_at: '2026-07-23T10:00:00Z',
      topology: {
        state: 'healthy',
        patroni_reachable: true,
        leader: { name: 'postgresql-1', role: 'leader', state: 'running', timeline: 7, lag: 0 },
        replicas: [{ name: 'postgresql-2', role: 'replica', state: 'streaming', timeline: 7, lag: 0 }],
      },
      dcs: { state: 'configured_not_observed', type: 'etcd', reachable: null, members: ['dcs-1', 'dcs-2'] },
      routing: {
        state: 'configured_not_observed',
        targets: [{ role: 'primary', address: 'primary.internal', port: 5000, reachable: null, role_matches: null }],
      },
      backup: {
        state: 'not_observed',
        repository_reachable: null,
        retention: { full: 2 },
        wal_continuous: null,
        locks: [],
        freshness_policy: '24h',
      },
      operation: {
        active: { id: 9, type: 'query_analytics_enable', status: 'running' },
        latest: { id: 8, type: 'reload', status: 'succeeded', finished: '2026-07-23T09:00:00Z' },
        unresolved: { id: 7, type: 'reload', status: 'failed', safe_next_action: 'Run a fresh preflight.' },
      },
      recoverability: {
        state: 'degraded',
        reasons: ['backup_not_observed', 'wal_continuity_not_observed', 'restore_evidence_missing'],
      },
    },
    isError: false,
    isFetching: false,
    refetch: vi.fn(),
  }),
}));

describe('cluster health UI', () => {
  it('keeps healthy topology distinct from degraded recoverability and missing backup evidence', () => {
    render(
      <MemoryRouter>
        <ClusterHealth clusterId={5} />
      </MemoryRouter>,
    );

    expect(screen.getByText('postgresql-1')).toBeInTheDocument();
    expect(screen.getByText(/Members: dcs-1, dcs-2/)).toBeInTheDocument();
    expect(screen.getAllByText('not observed').length).toBeGreaterThan(0);
    expect(screen.getByText('backup not observed')).toBeInTheDocument();
    expect(screen.getByText(/Retention:.*"full":2/)).toBeInTheDocument();
    expect(screen.getByText('Freshness policy: 24h')).toBeInTheDocument();
    expect(screen.getByRole('link', { name: /#9/ })).toHaveAttribute('href', '/operations/9/log');
    expect(screen.getByText('Run a fresh preflight.')).toBeInTheDocument();
  });
});
