import { renderHook } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { OPERATIONS_TABLE_COLUMN_NAMES } from '../model/constants.ts';
import { useGetOperationsTableData } from './hooks.tsx';

describe('operation table state', () => {
  it('shows finished only for terminal operations', () => {
    const { result } = renderHook(() =>
      useGetOperationsTableData([
        { id: 1, status: 'running', finished: '2026-07-23T10:00:00Z' },
        { id: 2, status: 'failed', finished: '2026-07-23T10:05:00Z' },
      ]),
    );

    expect(result.current[0][OPERATIONS_TABLE_COLUMN_NAMES.FINISHED]).toBe('-');
    expect(result.current[1][OPERATIONS_TABLE_COLUMN_NAMES.FINISHED]).toBe('2026-07-23T10:05:00Z');
  });
});
