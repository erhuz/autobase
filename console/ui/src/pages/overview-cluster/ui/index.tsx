import { FC } from 'react';
import { useOutletContext, useParams } from 'react-router-dom';
import { Box } from '@mui/material';
import ClusterOverviewTable from '@widgets/cluster-overview-table';
import ClusterHealth from '@widgets/cluster-health';
import { ClusterDetailsContext } from '@pages/cluster-details';

const OverviewCluster: FC = () => {
  const { clusterId } = useParams();
  const { cluster, isFetching } = useOutletContext<ClusterDetailsContext>();

  return (
    <Box sx={{ p: { xs: 1, sm: 2 } }}>
      <ClusterHealth clusterId={Number(clusterId)}>
        <ClusterOverviewTable
          items={cluster.servers ?? []}
          isLoading={isFetching}
        />
      </ClusterHealth>
    </Box>
  );
};

export default OverviewCluster;
