import { FC } from 'react';
import { useParams } from 'react-router-dom';
import { useGetClustersByIdQuery } from '@shared/api/api/clusters.ts';
import { Grid } from '@mui/material';
import ClusterOverviewTable from '@widgets/cluster-overview-table';
import ConnectionInfo from '@entities/cluster/connection-info';
import ClusterInfo from '@entities/cluster/cluster-info';
import { useAppSelector } from '@app/redux/store/hooks.ts';
import { selectPollingInterval } from '@app/redux/slices/pollingIntervalSlice/pollingIntervalSlice.ts';
import Spinner from '@shared/ui/spinner';
import QueryPerformance from '@widgets/query-performance';

const OverviewCluster: FC = () => {
  const { clusterId } = useParams();
  const pollingInterval = useAppSelector(selectPollingInterval('clusterOverview'));

  const cluster = useGetClustersByIdQuery({ id: Number(clusterId) }, { pollingInterval });

  const connectionInfo = cluster.data?.connection_info;

  return cluster.isLoading ? (
    <Spinner />
  ) : (
    <Grid container spacing={2} padding={1}>
      <Grid size={{ xs: 12 }}>
        <ClusterOverviewTable
          clusterName={cluster.data?.name}
          items={cluster.data?.servers ?? []}
          isLoading={cluster.isFetching}
        />
      </Grid>
      <Grid size={{ xs: 6 }}>
        <ConnectionInfo connectionInfo={connectionInfo} servers={cluster.data?.servers} />
      </Grid>
      <Grid size={{ xs: 6 }}>
        <ClusterInfo
          postgresVersion={cluster.data?.postgres_version}
          clusterName={cluster.data?.name}
          description={cluster.data?.description}
          environment={cluster.data?.environment}
          location={cluster.data?.cluster_location}
        />
      </Grid>
      <Grid size={{ xs: 12 }}>
        <QueryPerformance clusterId={Number(clusterId)} />
      </Grid>
    </Grid>
  );
};

export default OverviewCluster;
