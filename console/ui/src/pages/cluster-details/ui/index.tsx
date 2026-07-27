import { FC } from 'react';
import { Alert, Box, Button, Chip, Stack, Tab, Tabs, Typography } from '@mui/material';
import { generatePath, Link, Outlet, useLocation, useParams } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import RouterPaths from '@app/router/routerPathsConfig';
import { useAppSelector } from '@app/redux/store/hooks.ts';
import { selectPollingInterval } from '@app/redux/slices/pollingIntervalSlice/pollingIntervalSlice.ts';
import { ClusterInfo, useGetClustersByIdQuery } from '@shared/api/api/clusters.ts';
import Spinner from '@shared/ui/spinner';

export type ClusterDetailsContext = {
  cluster: ClusterInfo;
  isFetching: boolean;
};

const humanize = (value?: string) => value?.replace(/_/g, ' ') || '—';

const statusColor = (status?: string): 'success' | 'warning' | 'error' | 'default' => {
  if (['healthy', 'running', 'ready'].includes(status ?? '')) return 'success';
  if (['failed', 'unhealthy', 'unavailable'].includes(status ?? '')) return 'error';
  if (['degraded', 'warning'].includes(status ?? '')) return 'warning';
  return 'default';
};

const ClusterDetails: FC = () => {
  const { t } = useTranslation(['clusters', 'shared']);
  const { clusterId: clusterIdParam } = useParams();
  const location = useLocation();
  const clusterId = Number(clusterIdParam);
  const pollingInterval = useAppSelector(selectPollingInterval('clusterOverview'));
  const cluster = useGetClustersByIdQuery(
    { id: clusterId },
    { skip: !Number.isFinite(clusterId), pollingInterval },
  );

  if (!Number.isFinite(clusterId)) {
    return <Alert severity="error">{t('clusterLoadError')}</Alert>;
  }

  if (cluster.isLoading) return <Spinner />;

  if (cluster.isError || !cluster.data) {
    return (
      <Alert
        severity="error"
        action={<Button onClick={() => void cluster.refetch()}>{t('retry')}</Button>}>
        {t('clusterLoadError')}
      </Alert>
    );
  }

  const tabs = [
    {
      label: t('overview', { ns: 'shared' }),
      path: generatePath(RouterPaths.clusters.overview.absolutePath, { clusterId: String(clusterId) }),
    },
    {
      label: t('queryPerformance'),
      path: generatePath(RouterPaths.clusters.queryPerformance.absolutePath, { clusterId: String(clusterId) }),
    },
    {
      label: t('connectionAccess'),
      path: generatePath(RouterPaths.clusters.access.absolutePath, { clusterId: String(clusterId) }),
    },
  ];
  const metadata = [
    cluster.data.environment &&
      `${t('environment', { ns: 'shared' })}: ${cluster.data.environment}`,
    cluster.data.cluster_location && `${t('location')}: ${cluster.data.cluster_location}`,
    cluster.data.postgres_version && `${t('postgresVersion')}: ${cluster.data.postgres_version}`,
  ].filter(Boolean);

  return (
    <Stack>
      <Box component="header" sx={{ px: { xs: 1, sm: 2 }, pt: 2 }}>
        <Stack direction={{ xs: 'column', sm: 'row' }} justifyContent="space-between" gap={2}>
          <Box>
            <Typography component="h1" variant="h4" fontWeight={700} letterSpacing="-0.02em">
              {cluster.data.name || t('cluster')}
            </Typography>
            {cluster.data.description && (
              <Typography color="text.secondary" mt={0.5} maxWidth="75ch">
                {cluster.data.description}
              </Typography>
            )}
            {metadata.length > 0 && (
              <Typography variant="body2" color="text.secondary" mt={1}>
                {metadata.join(' · ')}
              </Typography>
            )}
          </Box>
          <Chip
            size="small"
            color={statusColor(cluster.data.status)}
            label={t('clusterAvailability', { state: humanize(cluster.data.status) })}
            sx={{ alignSelf: { xs: 'flex-start', sm: 'center' } }}
          />
        </Stack>
      </Box>

      <Box sx={{ borderBottom: 1, borderColor: 'divider', px: { xs: 0, sm: 1 }, mt: 2 }}>
        <Tabs
          value={location.pathname}
          variant="scrollable"
          scrollButtons="auto"
          aria-label={t('clusterNavigation')}>
          {tabs.map((tab) => (
            <Tab
              key={tab.path}
              component={Link}
              label={tab.label}
              value={tab.path}
              to={tab.path}
              sx={{ textTransform: 'none', minHeight: 48 }}
            />
          ))}
        </Tabs>
      </Box>

      <Outlet context={{ cluster: cluster.data, isFetching: cluster.isFetching } satisfies ClusterDetailsContext} />
    </Stack>
  );
};

export default ClusterDetails;
