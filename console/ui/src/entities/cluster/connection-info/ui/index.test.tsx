import { cleanup, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import '@testing-library/jest-dom/vitest';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, describe, expect, it, vi } from 'vitest';
import '@shared/i18n/i18n';
import ConnectionInfo from './index';

const saveRouting = vi.fn(() => ({ unwrap: () => Promise.resolve({}) }));

vi.mock('@shared/api/api/clusters.ts', () => ({
  usePatchClustersByIdRoutingMutation: () => [saveRouting, { isLoading: false }],
}));

afterEach(() => {
  cleanup();
  saveRouting.mockClear();
});

describe('cluster routing configuration', () => {
  it('adds primary routing for an imported cluster without exposing a password', async () => {
    const user = userEvent.setup();
    render(
      <MemoryRouter>
        <ConnectionInfo clusterId={5} connectionInfo={{ password: 'must-not-render' }} servers={[{ ip: '10.0.4.2' }]} />
      </MemoryRouter>,
    );

    await user.type(screen.getByLabelText('Primary addresses'), '10.0.4.2, 10.0.4.3');
    await user.type(screen.getByLabelText('Primary port'), '5000');
    await user.click(screen.getByRole('button', { name: 'Save routing' }));

    await waitFor(() =>
      expect(saveRouting).toHaveBeenCalledWith({
        id: 5,
        requestClusterRouting: {
          primary: { addresses: ['10.0.4.2', '10.0.4.3'], port: 5000 },
        },
      }),
    );
    expect(screen.queryByText('must-not-render')).not.toBeInTheDocument();
    expect(screen.queryByLabelText(/password/i)).not.toBeInTheDocument();
  });

  it('normalizes legacy routing and clears an optional role explicitly', async () => {
    const user = userEvent.setup();
    render(
      <MemoryRouter>
        <ConnectionInfo
          clusterId={5}
          connectionInfo={{
            address: '10.0.4.2, 10.0.4.3',
            port: { primary: '5000', replica: '5001' },
          }}
        />
      </MemoryRouter>,
    );

    expect(screen.getByLabelText('Primary addresses')).toHaveValue('10.0.4.2\n10.0.4.3');
    await user.clear(screen.getByLabelText('Replica addresses'));
    await user.clear(screen.getByLabelText('Replica port'));
    await user.click(screen.getByRole('button', { name: 'Save routing' }));

    await waitFor(() =>
      expect(saveRouting).toHaveBeenCalledWith({
        id: 5,
        requestClusterRouting: { replica: null },
      }),
    );
  });
});
