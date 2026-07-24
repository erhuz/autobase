export interface ClusterInfoProps {
  clusterId?: number;
  secretId?: number | null;
  postgresVersion?: number;
  clusterName?: string;
  description?: string;
  environment?: string;
  location?: string;
}
