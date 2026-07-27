import { CLUSTER_OVERVIEW_TABLE_COLUMN_NAMES } from '@widgets/cluster-overview-table/model/constants.tsx';
import type { ClusterInfoInstance } from '@shared/api/api/clusters.ts';
import type { ReactNode } from 'react';

export interface ClusterOverviewTableProps {
  items?: ClusterInfoInstance[];
  isLoading?: boolean;
}

export interface ClusterOverviewTableValues {
  [CLUSTER_OVERVIEW_TABLE_COLUMN_NAMES.NAME]?: string;
  [CLUSTER_OVERVIEW_TABLE_COLUMN_NAMES.HOST]?: string;
  [CLUSTER_OVERVIEW_TABLE_COLUMN_NAMES.ROLE]?: string;
  [CLUSTER_OVERVIEW_TABLE_COLUMN_NAMES.STATE]?: string;
  [CLUSTER_OVERVIEW_TABLE_COLUMN_NAMES.TIMELINE]?: number | null;
  [CLUSTER_OVERVIEW_TABLE_COLUMN_NAMES.LAG_IN_MB]?: number | null;
  [CLUSTER_OVERVIEW_TABLE_COLUMN_NAMES.PENDING_RESTART]: boolean;
  [CLUSTER_OVERVIEW_TABLE_COLUMN_NAMES.TAGS]?: ReactNode;
  [CLUSTER_OVERVIEW_TABLE_COLUMN_NAMES.ID]?: number;
}
