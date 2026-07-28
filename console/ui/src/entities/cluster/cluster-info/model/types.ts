import type { ClusterInfoAutomationCredentials } from '@shared/api/api/clusters.ts';

export interface ClusterInfoProps {
  clusterId?: number;
  secretId?: number | null;
  automationCredentials?: ClusterInfoAutomationCredentials | null;
}
