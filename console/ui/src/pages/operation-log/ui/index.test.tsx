import { render, screen } from '@testing-library/react';
import '@testing-library/jest-dom/vitest';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { describe, expect, it, vi } from 'vitest';
import '@shared/i18n/i18n';
import OperationLog from './index';

vi.mock('@app/redux/store/hooks.ts', () => ({
  useAppDispatch: () => vi.fn(),
  useAppSelector: () => 0,
}));
vi.mock('react-lazylog', () => ({ LazyLog: ({ text }: { text: string }) => <pre>{text}</pre> }));
vi.mock('@shared/api/api/operations.ts', () => ({
  useGetOperationsByIdQuery: () => ({
    data: {
      id: 17,
      cluster_id: 5,
      type: 'query_analytics_enable',
      status: 'failed',
      actor: 'api-token',
      sanitized_params: { state: 'enabled', password: '[REDACTED]' },
      preflight_snapshot: { id: 11, checks: [{ name: 'all nodes healthy', ok: true }] },
      plan: ['configure replica', 'verify topology'],
      affected_nodes: ['postgresql-2'],
      final_verification: { automation_summary: true, verified: false },
      safe_next_action: 'Run a fresh preflight.',
      started: '2026-07-23T10:00:00Z',
      finished: '2026-07-23T10:05:00Z',
    },
    isFetching: false,
    isError: false,
  }),
  useGetOperationsByIdLogQuery: () => ({
    data: { log: 'task output without credentials', isComplete: true },
    refetch: vi.fn(),
  }),
}));

describe('operation center detail', () => {
  it('shows durable audit, terminal outcome, redacted input, failure guidance, and log', () => {
    render(
      <MemoryRouter initialEntries={['/operations/17/log']}>
        <Routes>
          <Route path="/operations/:operationId/log" element={<OperationLog />} />
        </Routes>
      </MemoryRouter>,
    );

    expect(screen.getByText('Operation #17')).toBeInTheDocument();
    expect(screen.getByText('failed')).toBeInTheDocument();
    expect(screen.getByText(/REDACTED/)).toBeInTheDocument();
    expect(screen.getByText('configure replica')).toBeInTheDocument();
    expect(screen.getByText('postgresql-2')).toBeInTheDocument();
    expect(screen.getByText('Run a fresh preflight.')).toBeInTheDocument();
    expect(screen.getByText('task output without credentials')).toBeInTheDocument();
  });
});
