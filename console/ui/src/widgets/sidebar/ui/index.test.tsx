import { render, screen } from '@testing-library/react';
import '@testing-library/jest-dom/vitest';
import { MemoryRouter } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import '@shared/i18n/i18n';
import Sidebar from './index';

vi.mock('@mui/material', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@mui/material')>();
  return { ...actual, useMediaQuery: () => true };
});

describe('sidebar responsive width', () => {
  beforeEach(() => localStorage.setItem('isSidebarCollapsed', 'false'));

  it('V57 forces the compact drawer at phone width', () => {
    render(
      <MemoryRouter initialEntries={['/clusters/5/overview']}>
        <Sidebar />
      </MemoryRouter>,
    );

    expect(document.querySelector('.MuiDrawer-paper')).toHaveStyle({ width: '60px' });
    expect(screen.queryByRole('button', { name: 'Collapse sidebar' })).not.toBeInTheDocument();
  });
});
