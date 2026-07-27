import { FC, ReactNode } from 'react';
import {
  Alert,
  AlertTitle,
  Box,
  Button,
  Chip,
  Divider,
  Grid,
  LinearProgress,
  Link,
  Paper,
  Stack,
  Typography,
} from '@mui/material';
import { Link as RouterLink } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { HealthMember, HealthOperation, useGetClustersByIdHealthQuery } from '@shared/api/api/clusters.ts';
import { useAppSelector } from '@app/redux/store/hooks.ts';
import { selectPollingInterval } from '@app/redux/slices/pollingIntervalSlice/pollingIntervalSlice.ts';

type ClusterHealthProps = {
  clusterId: number;
  children?: ReactNode;
};

const humanize = (value?: string) => value?.replace(/_/g, ' ') || '—';
const formatDate = (value?: string | null) => (value ? new Date(value).toLocaleString() : '—');

const stateColor = (state?: string): 'success' | 'warning' | 'error' | 'default' => {
  if (['healthy', 'ready', 'running', 'streaming', 'succeeded'].includes(state ?? '')) return 'success';
  if (['failed', 'unhealthy', 'unavailable'].includes(state ?? '')) return 'error';
  if (['degraded', 'configured_not_observed', 'not_observed', 'queued'].includes(state ?? '')) return 'warning';
  return 'default';
};

const formatMember = (member?: HealthMember | null) => {
  if (!member) return '—';
  const detail = [
    member.name,
    humanize(member.role),
    humanize(member.state),
    member.timeline != null ? `timeline ${member.timeline}` : '',
    member.lag != null ? `lag ${member.lag}` : '',
    member.pending_restart ? 'pending restart' : '',
  ].filter(Boolean);
  return detail.join(' · ');
};

const Observed: FC<{ value?: boolean | null }> = ({ value }) => {
  const { t } = useTranslation('clusters');
  return <>{value == null ? t('healthNotObserved') : value ? t('yes') : t('no')}</>;
};

const StatusItem: FC<{ label: string; state?: string; detail: ReactNode }> = ({ label, state, detail }) => (
  <Stack gap={0.75} minWidth={0}>
    <Stack direction="row" justifyContent="space-between" alignItems="center" gap={1}>
      <Typography variant="subtitle2">{label}</Typography>
      <Chip size="small" color={stateColor(state)} label={humanize(state)} />
    </Stack>
    <Typography variant="body2" color="text.secondary" noWrap>
      {detail}
    </Typography>
  </Stack>
);

const DetailRow: FC<{ label: string; children: ReactNode }> = ({ label, children }) => (
  <Stack
    component="div"
    direction={{ xs: 'column', sm: 'row' }}
    justifyContent="space-between"
    gap={{ xs: 0.25, sm: 2 }}
    py={0.75}>
    <Typography component="dt" variant="body2" color="text.secondary">
      {label}
    </Typography>
    <Typography component="dd" variant="body2" m={0} textAlign={{ sm: 'right' }} sx={{ overflowWrap: 'anywhere' }}>
      {children}
    </Typography>
  </Stack>
);

const Operation: FC<{ label: string; operation?: HealthOperation | null }> = ({ label, operation }) => (
  <Stack gap={0.25}>
    <Typography variant="caption" color="text.secondary">
      {label}
    </Typography>
    {operation?.id ? (
      <>
        <Link component={RouterLink} to={`/operations/${operation.id}/log`}>
          #{operation.id} · {humanize(operation.type)} · {humanize(operation.status)}
        </Link>
        {operation.safe_next_action && (
          <Typography variant="body2" color="warning.main">
            {operation.safe_next_action}
          </Typography>
        )}
      </>
    ) : (
      <Typography>—</Typography>
    )}
  </Stack>
);

const ClusterHealth: FC<ClusterHealthProps> = ({ clusterId, children }) => {
  const { t } = useTranslation('clusters');
  const pollingInterval = useAppSelector(selectPollingInterval('clusterOverview'));
  const health = useGetClustersByIdHealthQuery(
    { id: clusterId },
    { skip: !Number.isFinite(clusterId), pollingInterval },
  );

  if (health.isError) {
    return (
      <Alert severity="error" action={<Button onClick={() => void health.refetch()}>{t('retry')}</Button>}>
        {t('healthLoadError')}
      </Alert>
    );
  }

  const data = health.data;
  const topology = data?.topology;
  const dcs = data?.dcs;
  const routing = data?.routing;
  const backup = data?.backup;
  const operation = data?.operation;
  const recoverability = data?.recoverability;
  const needsAttention = recoverability?.state === 'degraded' || Boolean(operation?.unresolved?.id);

  return (
    <Stack gap={2}>
      <Paper variant="outlined" sx={{ p: { xs: 2, sm: 2.5 }, boxShadow: 'none' }}>
        <Stack direction={{ xs: 'column', sm: 'row' }} justifyContent="space-between" gap={1.5}>
          <Box>
            <Typography variant="h6">{t('healthTitle')}</Typography>
            <Typography variant="body2" color="text.secondary">
              {t('healthObserved')} {formatDate(data?.observed_at)}
            </Typography>
          </Box>
          <Chip
            size="small"
            color={recoverability?.state === 'healthy' ? 'success' : 'warning'}
            label={t('healthRecoverabilityState', { state: humanize(recoverability?.state) })}
            sx={{ alignSelf: { xs: 'flex-start', sm: 'center' } }}
          />
        </Stack>

        {health.isFetching && <LinearProgress sx={{ mt: 2 }} />}

        {needsAttention && (
          <Alert severity="warning" sx={{ mt: 2 }}>
            <AlertTitle>{t('healthNeedsAttention')}</AlertTitle>
            {recoverability?.state === 'degraded' && t('healthRecoverabilityDegraded')}
            {(recoverability?.reasons?.length ?? 0) > 0 && (
              <Box component="ul" mt={0.5} mb={operation?.unresolved?.safe_next_action ? 1 : 0} pl={2.5}>
                {recoverability?.reasons?.map((reason) => (
                  <li key={reason}>{humanize(reason)}</li>
                ))}
              </Box>
            )}
            {operation?.unresolved?.safe_next_action && (
              <Typography variant="body2">{operation.unresolved.safe_next_action}</Typography>
            )}
          </Alert>
        )}

        <Grid container spacing={2.5} mt={needsAttention ? 0.5 : 1}>
          <Grid size={{ xs: 12, sm: 6, lg: 3 }}>
            <StatusItem
              label={t('healthTopology')}
              state={topology?.state}
              detail={
                <>
                  {t('healthPatroniReachable')}: <Observed value={topology?.patroni_reachable} />
                </>
              }
            />
          </Grid>
          <Grid size={{ xs: 12, sm: 6, lg: 3 }}>
            <StatusItem
              label={t('healthDcs')}
              state={dcs?.state}
              detail={
                <>
                  {humanize(dcs?.type)} · {t('healthReachable').toLowerCase()}:{' '}
                  <Observed value={dcs?.reachable} />
                </>
              }
            />
          </Grid>
          <Grid size={{ xs: 12, sm: 6, lg: 3 }}>
            <StatusItem
              label={t('healthRouting')}
              state={routing?.state}
              detail={t('healthRoutingTargets', { count: routing?.targets?.length ?? 0 })}
            />
          </Grid>
          <Grid size={{ xs: 12, sm: 6, lg: 3 }}>
            <StatusItem
              label={t('healthBackup')}
              state={backup?.state}
              detail={
                <>
                  {t('healthFresh')}: <Observed value={backup?.fresh} />
                </>
              }
            />
          </Grid>
        </Grid>
      </Paper>

      {children}

      <Grid container spacing={2}>
        <Grid size={{ xs: 12, lg: 6 }}>
          <Paper variant="outlined" sx={{ p: { xs: 2, sm: 2.5 }, height: '100%', boxShadow: 'none' }}>
            <Typography variant="h6">{t('healthCoordinationRouting')}</Typography>
            <Box component="dl" m={0} mt={1}>
              <DetailRow label={t('healthPatroniReachable')}>
                <Observed value={topology?.patroni_reachable} />
              </DetailRow>
              <DetailRow label={t('healthLeader')}>{formatMember(topology?.leader)}</DetailRow>
              <DetailRow label={t('healthReplicas')}>
                {topology?.replicas?.length ? topology.replicas.map(formatMember).join('; ') : '—'}
              </DetailRow>
              <DetailRow label={t('healthType')}>{humanize(dcs?.type)}</DetailRow>
              <DetailRow label={t('healthReachable')}>
                <Observed value={dcs?.reachable} />
              </DetailRow>
              <DetailRow label={t('healthMembers')}>{dcs?.members?.join(', ') || '—'}</DetailRow>
            </Box>

            <Divider sx={{ my: 2 }} />
            <Typography variant="subtitle1" fontWeight={600}>
              {t('healthRouting')}
            </Typography>
            <Stack gap={1.25} mt={1}>
              {routing?.targets?.length ? (
                routing.targets.map((target, index) => (
                  <Stack key={`${target.role}-${target.address}-${index}`} gap={0.25}>
                    <Typography variant="body2">
                      {humanize(target.role)} · {target.address || '—'}
                      {target.port ? `:${target.port}` : ''}
                    </Typography>
                    <Typography variant="caption" color="text.secondary">
                      {t('healthReachable')}: <Observed value={target.reachable} /> · {t('healthRoleMatches')}:{' '}
                      <Observed value={target.role_matches} />
                    </Typography>
                  </Stack>
                ))
              ) : (
                <Typography color="text.secondary">—</Typography>
              )}
            </Stack>

            <Divider sx={{ my: 2 }} />
            <Typography variant="subtitle1" fontWeight={600} mb={1}>
              {t('healthOperations')}
            </Typography>
            <Grid container spacing={2}>
              <Grid size={{ xs: 12, sm: 6 }}>
                <Operation label={t('healthActiveOperation')} operation={operation?.active} />
              </Grid>
              <Grid size={{ xs: 12, sm: 6 }}>
                <Operation label={t('healthLatestOperation')} operation={operation?.latest} />
              </Grid>
              <Grid size={{ xs: 12 }}>
                <Operation label={t('healthUnresolvedOperation')} operation={operation?.unresolved} />
              </Grid>
            </Grid>
          </Paper>
        </Grid>

        <Grid size={{ xs: 12, lg: 6 }}>
          <Paper variant="outlined" sx={{ p: { xs: 2, sm: 2.5 }, height: '100%', boxShadow: 'none' }}>
            <Typography variant="h6">{t('healthBackup')}</Typography>
            <Box component="dl" m={0} mt={1}>
              <DetailRow label={t('healthRepository')}>
                <Observed value={backup?.repository_reachable} />
              </DetailRow>
              <DetailRow label={t('healthFresh')}>
                <Observed value={backup?.fresh} />
              </DetailRow>
              <DetailRow label={t('healthFreshnessPolicy')}>{backup?.freshness_policy || '—'}</DetailRow>
              <DetailRow label={t('healthLatestFull')}>{formatDate(backup?.latest_full)}</DetailRow>
              <DetailRow label={t('healthLatestDifferential')}>
                {formatDate(backup?.latest_differential)}
              </DetailRow>
              <DetailRow label={t('healthWalContinuous')}>
                <Observed value={backup?.wal_continuous} />
              </DetailRow>
              <DetailRow label={t('healthRetention')}>
                {backup?.retention ? JSON.stringify(backup.retention) : '—'}
              </DetailRow>
              <DetailRow label={t('healthRestoreTested')}>{formatDate(backup?.restore_tested_at)}</DetailRow>
              <DetailRow label={t('healthScheduler')}>{backup?.scheduler_owner || '—'}</DetailRow>
              <DetailRow label={t('healthLocks')}>{backup?.locks?.join(', ') || '—'}</DetailRow>
            </Box>
          </Paper>
        </Grid>
      </Grid>
    </Stack>
  );
};

export default ClusterHealth;
