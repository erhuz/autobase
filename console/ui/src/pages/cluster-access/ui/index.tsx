import { FC } from 'react';
import { Grid } from '@mui/material';
import { useOutletContext } from 'react-router-dom';
import ConnectionInfo from '@entities/cluster/connection-info';
import ClusterInfo from '@entities/cluster/cluster-info';
import { ClusterDetailsContext } from '@pages/cluster-details';

const ClusterAccess: FC = () => {
  const { cluster } = useOutletContext<ClusterDetailsContext>();

  return (
    <Grid container spacing={2} sx={{ p: { xs: 1, sm: 2 } }} alignItems="stretch">
      <Grid size={{ xs: 12, lg: 6 }}>
        <ConnectionInfo clusterId={cluster.id} connectionInfo={cluster.connection_info} servers={cluster.servers} />
      </Grid>
      <Grid size={{ xs: 12, lg: 6 }}>
        <ClusterInfo
          clusterId={cluster.id}
          secretId={cluster.secret_id}
          automationCredentials={cluster.automation_credentials}
        />
      </Grid>
    </Grid>
  );
};

export default ClusterAccess;
