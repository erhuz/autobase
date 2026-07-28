import { cleanup, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import '@testing-library/jest-dom/vitest';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, describe, expect, it, vi } from 'vitest';
import '@shared/i18n/i18n';
import ClusterInfo from './index';

const attachCredential = vi.fn(() => ({ unwrap: () => Promise.resolve({}) }));
const saveAutomationCredentials = vi.fn(() => ({ unwrap: () => Promise.resolve({}) }));

vi.mock('@app/redux/store/hooks.ts', () => ({ useAppSelector: () => 3 }));
vi.mock('@shared/api/api/secrets.ts', () => ({
  useGetSecretsQuery: () => ({
    data: {
      data: [
        { id: 7, name: 'operator-key', type: 'ssh_key' },
        { id: 8, name: 'cloud-token', type: 'hetzner' },
        { id: 11, name: 'postgres-superuser', type: 'password' },
        { id: 12, name: 'postgres-replication', type: 'password' },
        { id: 13, name: 'patroni-restapi', type: 'password' },
      ],
    },
  }),
}));
vi.mock('@shared/api/api/clusters.ts', () => ({
  usePutClustersByIdCredentialMutation: () => [attachCredential, { isLoading: false }],
  usePutClustersByIdAutomationCredentialsMutation: () => [
    saveAutomationCredentials,
    { isLoading: false },
  ],
}));

afterEach(cleanup);

describe('cluster management credential', () => {
  it('offers only management credentials and attaches the selection', async () => {
    const user = userEvent.setup();
    render(
      <MemoryRouter>
        <ClusterInfo clusterId={5} />
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

  it('saves all three automation credential bindings atomically', async () => {
    const user = userEvent.setup();
    render(
      <MemoryRouter>
        <ClusterInfo clusterId={5} />
      </MemoryRouter>,
    );

    for (const [label, option] of [
      ['PostgreSQL superuser', 'postgres-superuser'],
      ['PostgreSQL replication', 'postgres-replication'],
      ['Patroni REST API', 'patroni-restapi'],
    ]) {
      await user.click(screen.getByLabelText(label));
      await user.click(screen.getByRole('option', { name: option }));
    }
    await user.click(screen.getByRole('button', { name: 'Save automation credentials' }));

    await waitFor(() =>
      expect(saveAutomationCredentials).toHaveBeenCalledWith({
        id: 5,
        requestClusterAutomationCredentials: {
          postgres_superuser_secret_id: 11,
          postgres_replication_secret_id: 12,
          patroni_restapi_secret_id: 13,
        },
      }),
    );
  });
});
