import { describe, expect, it } from 'vitest';
import { operationFilterParams } from './index';

describe('operation center filters', () => {
  it('passes cluster, type, and state filters to the operations API and omits empty values', () => {
    expect(operationFilterParams({ clusterName: 'cluster-1', type: 'reload', status: 'running' })).toEqual({
      clusterName: 'cluster-1',
      type: 'reload',
      status: 'running',
    });
    expect(operationFilterParams({ clusterName: '', type: '', status: '' })).toEqual({
      clusterName: undefined,
      type: undefined,
      status: undefined,
    });
  });
});
