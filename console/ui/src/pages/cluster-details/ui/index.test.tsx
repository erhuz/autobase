import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import '@testing-library/jest-dom/vitest';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { describe, expect, it, vi } from 'vitest';
import '@shared/i18n/i18n';
import ClusterDetails from './index';

vi.mock('@app/redux/store/hooks.ts', () => ({ useAppSelector: () => 0 }));
vi.mock('@shared/api/api/clusters.ts', () => ({
  useGetClustersByIdQuery: () => ({
    data: {
      id: 5,
      name: 'production',
      description: 'Primary customer workload',
      status: 'healthy',
      environment: 'prod',
      cluster_location: 'eu-north',
      postgres_version: 16,
    },
    isLoading: false,
    isFetching: false,
    isError: false,
    refetch: vi.fn(),
  }),
}));

describe('cluster details navigation', () => {
  it('keeps cluster context while navigating route-backed tabs', async () => {
    const user = userEvent.setup();
    render(
      <MemoryRouter initialEntries={['/clusters/5/overview']}>
        <Routes>
          <Route path="/clusters/:clusterId" element={<ClusterDetails />}>
            <Route path="overview" element={<div>Overview content</div>} />
            <Route path="query-performance" element={<div>Query content</div>} />
            <Route path="access" element={<div>Access content</div>} />
          </Route>
        </Routes>
      </MemoryRouter>,
    );

    expect(screen.getByRole('heading', { name: 'production' })).toBeInTheDocument();
    expect(screen.getByText('Availability: healthy')).toBeInTheDocument();
    expect(screen.getByRole('tab', { name: 'Overview' })).toHaveAttribute('aria-selected', 'true');
    expect(screen.getByRole('tab', { name: 'Query performance' })).toHaveAttribute(
      'href',
      '/clusters/5/query-performance',
    );

    await user.click(screen.getByRole('tab', { name: 'Query performance' }));

    expect(screen.getByText('Query content')).toBeInTheDocument();
    expect(screen.getByRole('tab', { name: 'Query performance' })).toHaveAttribute('aria-selected', 'true');
  });
});
