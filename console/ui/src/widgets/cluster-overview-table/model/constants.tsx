import { createMRTColumnHelper } from 'material-react-table';
import { TFunction } from 'i18next';
import { Chip } from '@mui/material';
import type { ClusterOverviewTableValues } from '@widgets/cluster-overview-table/model/types.ts';

export const CLUSTER_OVERVIEW_TABLE_COLUMN_NAMES = Object.freeze({
  NAME: 'name',
  HOST: 'host',
  ROLE: 'role',
  STATE: 'state',
  TIMELINE: 'timeline',
  LAG_IN_MB: 'lagInMb',
  PENDING_RESTART: 'pendingRestart',
  TAGS: 'tags',
  ID: 'id',
});

const columnHelper = createMRTColumnHelper<ClusterOverviewTableValues>();

const stateColor = (state?: string): 'success' | 'warning' | 'error' | 'default' => {
  if (['healthy', 'running', 'ready', 'streaming'].includes(state ?? '')) return 'success';
  if (['failed', 'unhealthy', 'unavailable'].includes(state ?? '')) return 'error';
  if (['degraded', 'warning'].includes(state ?? '')) return 'warning';
  return 'default';
};

export const clusterOverviewTableColumns = (t: TFunction) => [
  columnHelper.accessor(CLUSTER_OVERVIEW_TABLE_COLUMN_NAMES.NAME, {
    header: t('name', { ns: 'shared' }),
  }),
  columnHelper.accessor(CLUSTER_OVERVIEW_TABLE_COLUMN_NAMES.HOST, {
    header: t('host', { ns: 'clusters' }),
    size: 70,
  }),
  columnHelper.accessor(CLUSTER_OVERVIEW_TABLE_COLUMN_NAMES.ROLE, {
    header: t('role', { ns: 'clusters' }),
    size: 120,
  }),
  columnHelper.accessor(CLUSTER_OVERVIEW_TABLE_COLUMN_NAMES.STATE, {
    header: t('state', { ns: 'clusters' }),
    size: 110,
    Cell: ({ cell }) => {
      const state = cell.getValue();
      return state ? <Chip size="small" color={stateColor(state)} label={state.replace(/_/g, ' ')} /> : '—';
    },
  }),
  columnHelper.accessor(CLUSTER_OVERVIEW_TABLE_COLUMN_NAMES.TIMELINE, {
    header: t('timeline', { ns: 'clusters' }),
    size: 80,
  }),
  columnHelper.accessor(CLUSTER_OVERVIEW_TABLE_COLUMN_NAMES.LAG_IN_MB, {
    header: t('lagInMb', { ns: 'clusters' }),
    size: 140,
  }),
  columnHelper.accessor(CLUSTER_OVERVIEW_TABLE_COLUMN_NAMES.PENDING_RESTART, {
    header: t('pendingRestart', { ns: 'clusters' }),
    size: 140,
    Cell: ({ cell }) =>
      cell.getValue() ? <Chip size="small" color="warning" label={t('pendingRestart', { ns: 'clusters' })} /> : '—',
  }),
  columnHelper.accessor(CLUSTER_OVERVIEW_TABLE_COLUMN_NAMES.TAGS, {
    header: t('tags', { ns: 'clusters' }),
  }),
];
