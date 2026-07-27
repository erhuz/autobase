import { FC } from 'react';
import { Box } from '@mui/material';
import { useParams } from 'react-router-dom';
import QueryPerformance from '@widgets/query-performance';

const ClusterQueryPerformance: FC = () => {
  const { clusterId } = useParams();

  return (
    <Box sx={{ p: { xs: 1, sm: 2 } }}>
      <QueryPerformance clusterId={Number(clusterId)} />
    </Box>
  );
};

export default ClusterQueryPerformance;
