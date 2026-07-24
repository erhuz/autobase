import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import '@testing-library/jest-dom/vitest';
import { MemoryRouter } from 'react-router-dom';
import { describe, expect, it, vi } from 'vitest';
import '@shared/i18n/i18n';
import ClusterInfo from './index';

const attachCredential = vi.fn(() => ({ unwrap: () => Promise.resolve({}) }));

vi.mock('@app/redux/store/hooks.ts', () => ({ useAppSelector: () => 3 }));
vi.mock('@shared/api/api/secrets.ts', () => ({
  useGetSecretsQuery: () => ({
    data: {
      data: [
        { id: 7, name: 'operator-key', type: 'ssh_key' },
        { id: 8, name: 'cloud-token', type: 'hetzner' },
      ],
    },
  }),
}));
vi.mock('@shared/api/api/clusters.ts', () => ({
  usePutClustersByIdCredentialMutation: () => [attachCredential, { isLoading: false }],
}));

describe('cluster management credential', () => {
  it('offers only management credentials and attaches the selection', async () => {
    const user = userEvent.setup();
    render(
      <MemoryRouter>
        <ClusterInfo clusterId={5} clusterName="production" />
      </MemoryRouter>,
    );

    await user.click(screen.getByLabelText('Management credential'));
    await user.click(screen.getByRole('option', { name: 'operator-key' }));
    expect(screen.queryByRole('option', { name: 'cloud-token' })).not.toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Attach' }));

    await waitFor(() =>
      expect(attachCredential).toHaveBeenCalledWith({
        id: 5,
        requestClusterCredential: { secret_id: 7 },
      }),
    );
  });
});
