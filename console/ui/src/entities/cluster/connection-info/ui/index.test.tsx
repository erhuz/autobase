import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import '@testing-library/jest-dom/vitest';
import { MemoryRouter } from 'react-router-dom';
import { describe, expect, it } from 'vitest';
import '@shared/i18n/i18n';
import ConnectionInfo from './index';

describe('cluster connection details', () => {
  it('uses labelled keyboard-native controls for protected values', async () => {
    const user = userEvent.setup();
    render(
      <MemoryRouter>
        <ConnectionInfo
          connectionInfo={{
            address: 'primary.internal',
            port: '5432',
            superuser: 'postgres',
            password: 'secret',
          }}
        />
      </MemoryRouter>,
    );

    expect(screen.queryByText('secret')).not.toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Show password' }));
    expect(screen.getByText('secret')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Hide password' })).toBeInTheDocument();
    expect(screen.getAllByRole('button', { name: 'Copy to clipboard' })).toHaveLength(4);
  });
});
