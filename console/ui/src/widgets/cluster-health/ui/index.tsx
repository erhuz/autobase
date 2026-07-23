import { FC, ReactNode } from 'react';
import { Alert, Box, Button, Chip, Divider, Grid, Link, LinearProgress, Paper, Stack, Typography } from '@mui/material';
import { Link as RouterLink } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { useGetClustersByIdHealthQuery, HealthMember, HealthOperation } from '@shared/api/api/clusters.ts';
import { useAppSelector } from '@app/redux/store/hooks.ts';
import { selectPollingInterval } from '@app/redux/slices/pollingIntervalSlice/pollingIntervalSlice.ts';

type ClusterHealthProps = { clusterId: number };

const humanize = (value?: string) => value?.replaceAll('_', ' ') || '—';
const formatDate = (value?: string | null) => (value ? new Date(value).toLocaleString() : '—');

const stateColor = (state?: string): 'success' | 'warning' | 'error' | 'default' => {
  if (['healthy', 'ready', 'running', 'streaming', 'succeeded'].includes(state ?? '')) return 'success';
  if (['failed', 'unhealthy', 'unavailable'].includes(state ?? '')) return 'error';
  if (['degraded', 'configured_not_observed', 'not_observed', 'queued'].includes(state ?? '')) return 'warning';
  return 'default';
};

const Observed: FC<{ value?: boolean | null }> = ({ value }) => {
  const { t } = useTranslation('clusters');
  return <>{value == null ? t('healthNotObserved') : value ? t('yes') : t('no')}</>;
};

const Section: FC<{ title: string; state?: string; children: ReactNode }> = ({ title, state, children }) => (
  <Paper variant="outlined" sx={{ p: 2, height: '100%' }}>
    <Stack direction="row" justifyContent="space-between" alignItems="center" mb={1}>
      <Typography variant="subtitle1" fontWeight="bold">
        {title}
      </Typography>
      {state && <Chip size="small" color={stateColor(state)} label={humanize(state)} />}
    </Stack>
    {children}
  </Paper>
);

const Member: FC<{ member?: HealthMember | null }> = ({ member }) =>
  member ? (
    <Stack direction="row" justifyContent="space-between" gap={1}>
      <Typography>{member.name || '—'}</Typography>
      <Typography color="text.secondary">
        {[
          member.role,
          member.state,
          member.timeline != null ? `timeline ${member.timeline}` : '',
          member.lag != null ? `lag ${member.lag}` : '',
        ]
          .filter(Boolean)
          .join(' · ')}
        {member.pending_restart ? ' · pending restart' : ''}
      </Typography>
    </Stack>
  ) : (
    <Typography color="text.secondary">—</Typography>
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
        {operation.safe_next_action && <Alert severity="warning">{operation.safe_next_action}</Alert>}
      </>
    ) : (
      <Typography>—</Typography>
    )}
  </Stack>
);

const ClusterHealth: FC<ClusterHealthProps> = ({ clusterId }) => {
  const { t } = useTranslation('clusters');
  const pollingInterval = useAppSelector(selectPollingInterval('clusterOverview'));
  const health = useGetClustersByIdHealthQuery(
    { id: clusterId },
    { skip: !Number.isFinite(clusterId), pollingInterval },
  );

  if (health.isError) {
    return (
      <Paper sx={{ p: 2 }}>
        <Alert severity="error" action={<Button onClick={() => void health.refetch()}>{t('retry')}</Button>}>
          {t('healthLoadError')}
        </Alert>
      </Paper>
    );
  }

  const data = health.data;
  const topology = data?.topology;
  const dcs = data?.dcs;
  const routing = data?.routing;
  const backup = data?.backup;
  const operation = data?.operation;
  const recoverability = data?.recoverability;

  return (
    <Paper sx={{ p: 2 }}>
      <Stack direction={{ xs: 'column', sm: 'row' }} justifyContent="space-between" gap={1} mb={2}>
        <Box>
          <Typography variant="h6">{t('healthTitle')}</Typography>
          <Typography variant="body2" color="text.secondary">
            {t('healthObservedAt', { value: formatDate(data?.observed_at) })}
          </Typography>
        </Box>
        <Chip
          color={recoverability?.state === 'healthy' ? 'success' : 'warning'}
          label={t('healthRecoverabilityState', { state: humanize(recoverability?.state) })}
        />
      </Stack>
      {health.isFetching && <LinearProgress sx={{ mb: 2 }} />}
      {recoverability?.state === 'degraded' && (
        <Alert severity="warning" sx={{ mb: 2 }}>
          {t('healthRecoverabilityDegraded')}
          {(recoverability.reasons?.length ?? 0) > 0 && (
            <Box component="ul" mb={0}>
              {recoverability.reasons?.map((reason) => (
                <li key={reason}>{humanize(reason)}</li>
              ))}
            </Box>
          )}
        </Alert>
      )}

      <Grid container spacing={2}>
        <Grid size={{ xs: 12, lg: 6 }}>
          <Section title={t('healthTopology')} state={topology?.state}>
            <Typography variant="caption" color="text.secondary">
              {t('healthPatroniReachable')}: <Observed value={topology?.patroni_reachable} />
            </Typography>
            <Typography variant="subtitle2" mt={1}>
              {t('healthLeader')}
            </Typography>
            <Member member={topology?.leader} />
            <Divider sx={{ my: 1 }} />
            <Typography variant="subtitle2">{t('healthReplicas')}</Typography>
            <Stack gap={0.5}>
              {topology?.replicas?.length ? (
                topology.replicas.map((member) => <Member key={member.name} member={member} />)
              ) : (
                <Typography>—</Typography>
              )}
            </Stack>
          </Section>
        </Grid>

        <Grid size={{ xs: 12, md: 6, lg: 3 }}>
          <Section title={t('healthDcs')} state={dcs?.state}>
            <Typography>
              {t('healthType')}: {humanize(dcs?.type)}
            </Typography>
            <Typography>
              {t('healthReachable')}: <Observed value={dcs?.reachable} />
            </Typography>
            <Typography>
              {t('healthMembers')}: {dcs?.members?.join(', ') || '—'}
            </Typography>
          </Section>
        </Grid>

        <Grid size={{ xs: 12, md: 6, lg: 3 }}>
          <Section title={t('healthRouting')} state={routing?.state}>
            <Stack gap={1}>
              {routing?.targets?.length ? (
                routing.targets.map((target, index) => (
                  <Box key={`${target.role}-${target.address}-${index}`}>
                    <Typography>
                      {humanize(target.role)} · {target.address || '—'}
                      {target.port ? `:${target.port}` : ''}
                    </Typography>
                    <Typography variant="caption" color="text.secondary">
                      {t('healthReachable')}: <Observed value={target.reachable} /> · {t('healthRoleMatches')}:{' '}
                      <Observed value={target.role_matches} />
                    </Typography>
                  </Box>
                ))
              ) : (
                <Typography>—</Typography>
              )}
            </Stack>
          </Section>
        </Grid>

        <Grid size={{ xs: 12, lg: 6 }}>
          <Section title={t('healthBackup')} state={backup?.state}>
            <Grid container spacing={1}>
              <Grid size={{ xs: 6 }}>
                <Typography>
                  {t('healthRepository')}: <Observed value={backup?.repository_reachable} />
                </Typography>
              </Grid>
              <Grid size={{ xs: 6 }}>
                <Typography>
                  {t('healthWalContinuous')}: <Observed value={backup?.wal_continuous} />
                </Typography>
              </Grid>
              <Grid size={{ xs: 6 }}>
                <Typography>
                  {t('healthLatestFull')}: {formatDate(backup?.latest_full)}
                </Typography>
              </Grid>
              <Grid size={{ xs: 6 }}>
                <Typography>
                  {t('healthLatestDifferential')}: {formatDate(backup?.latest_differential)}
                </Typography>
              </Grid>
              <Grid size={{ xs: 6 }}>
                <Typography>
                  {t('healthFresh')}: <Observed value={backup?.fresh} />
                </Typography>
              </Grid>
              <Grid size={{ xs: 6 }}>
                <Typography>
                  {t('healthFreshnessPolicy')}: {backup?.freshness_policy || '—'}
                </Typography>
              </Grid>
              <Grid size={{ xs: 6 }}>
                <Typography>
                  {t('healthRetention')}: {backup?.retention ? JSON.stringify(backup.retention) : '—'}
                </Typography>
              </Grid>
              <Grid size={{ xs: 6 }}>
                <Typography>
                  {t('healthRestoreTested')}: {formatDate(backup?.restore_tested_at)}
                </Typography>
              </Grid>
              <Grid size={{ xs: 6 }}>
                <Typography>
                  {t('healthScheduler')}: {backup?.scheduler_owner || '—'}
                </Typography>
              </Grid>
              <Grid size={{ xs: 6 }}>
                <Typography>
                  {t('healthLocks')}: {backup?.locks?.join(', ') || '—'}
                </Typography>
              </Grid>
            </Grid>
          </Section>
        </Grid>

        <Grid size={{ xs: 12, lg: 6 }}>
          <Section title={t('healthOperations')}>
            <Stack gap={1}>
              <Operation label={t('healthActiveOperation')} operation={operation?.active} />
              <Operation label={t('healthLatestOperation')} operation={operation?.latest} />
              <Operation label={t('healthUnresolvedOperation')} operation={operation?.unresolved} />
            </Stack>
          </Section>
        </Grid>
      </Grid>
    </Paper>
  );
};

export default ClusterHealth;
