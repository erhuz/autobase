import { FC, useEffect, useState } from 'react';
import { Alert, Box, Chip, Divider, Grid, LinearProgress, Paper, Stack, Typography } from '@mui/material';
import { useGetOperationsByIdLogQuery, useGetOperationsByIdQuery } from '@shared/api/api/operations.ts';
import { useParams } from 'react-router-dom';
import { LazyLog } from 'react-lazylog';
import { useAppSelector } from '@app/redux/store/hooks.ts';
import { selectPollingInterval } from '@app/redux/slices/pollingIntervalSlice/pollingIntervalSlice.ts';
import RefreshIntervalSelect from '@features/refresh-interval-select';
import { useTranslation } from 'react-i18next';

const JsonValue: FC<{ value?: object }> = ({ value }) => (
  <Box
    component="pre"
    sx={{ m: 0, p: 1, bgcolor: 'action.hover', borderRadius: 1, overflow: 'auto', whiteSpace: 'pre-wrap' }}>
    {JSON.stringify(value ?? {}, null, 2)}
  </Box>
);

const OperationLog: FC = () => {
  const { t } = useTranslation('operations');
  const { operationId } = useParams();
  const id = Number(operationId);
  const [isStopRequest, setIsStopRequest] = useState(false);
  const pollingInterval = useAppSelector(selectPollingInterval('operationLogs'));
  const detail = useGetOperationsByIdQuery(
    { id },
    { skip: !Number.isFinite(id), pollingInterval: isStopRequest ? 0 : pollingInterval },
  );

  const log = useGetOperationsByIdLogQuery(
    { id },
    { skip: !Number.isFinite(id), pollingInterval: isStopRequest ? 0 : pollingInterval },
  );

  useEffect(() => {
    setIsStopRequest(!!log.data?.isComplete);
  }, [log.data?.isComplete]);

  return (
    <Stack width="100%" gap={2}>
      <Paper sx={{ p: 2 }}>
        {detail.isFetching && <LinearProgress sx={{ mb: 2 }} />}
        {detail.isError ? (
          <Alert severity="error">{t('detailLoadError')}</Alert>
        ) : (
          <>
            <Stack direction={{ xs: 'column', sm: 'row' }} justifyContent="space-between" gap={1} mb={2}>
              <Typography variant="h6">
                {t('operation')} #{detail.data?.id ?? operationId}
              </Typography>
              <Chip label={detail.data?.status ?? '—'} />
            </Stack>
            <Grid container spacing={2}>
              <Grid size={{ xs: 6, md: 3 }}>
                <Typography variant="caption" color="text.secondary">
                  {t('type')}
                </Typography>
                <Typography>{detail.data?.type ?? '—'}</Typography>
              </Grid>
              <Grid size={{ xs: 6, md: 3 }}>
                <Typography variant="caption" color="text.secondary">
                  {t('actor')}
                </Typography>
                <Typography>{detail.data?.actor ?? '—'}</Typography>
              </Grid>
              <Grid size={{ xs: 6, md: 3 }}>
                <Typography variant="caption" color="text.secondary">
                  {t('started')}
                </Typography>
                <Typography>{detail.data?.started ? new Date(detail.data.started).toLocaleString() : '—'}</Typography>
              </Grid>
              <Grid size={{ xs: 6, md: 3 }}>
                <Typography variant="caption" color="text.secondary">
                  {t('finished')}
                </Typography>
                <Typography>{detail.data?.finished ? new Date(detail.data.finished).toLocaleString() : '—'}</Typography>
              </Grid>
              <Grid size={{ xs: 12, md: 6 }}>
                <Typography variant="subtitle2">{t('sanitizedParameters')}</Typography>
                <JsonValue value={detail.data?.sanitized_params} />
              </Grid>
              <Grid size={{ xs: 12, md: 6 }}>
                <Typography variant="subtitle2">{t('preflightSnapshot')}</Typography>
                <JsonValue value={detail.data?.preflight_snapshot} />
              </Grid>
              <Grid size={{ xs: 12, md: 6 }}>
                <Typography variant="subtitle2">{t('plan')}</Typography>
                <Box component="ol" mt={0}>
                  {detail.data?.plan?.map((step, index) => (
                    <li key={`${index}-${step}`}>{step}</li>
                  ))}
                </Box>
              </Grid>
              <Grid size={{ xs: 12, md: 6 }}>
                <Typography variant="subtitle2">{t('affectedNodes')}</Typography>
                <Stack direction="row" flexWrap="wrap" gap={1} mt={1}>
                  {detail.data?.affected_nodes?.map((node) => (
                    <Chip key={node} size="small" label={node} />
                  ))}
                </Stack>
              </Grid>
              <Grid size={{ xs: 12 }}>
                <Typography variant="subtitle2">{t('finalVerification')}</Typography>
                <JsonValue value={detail.data?.final_verification} />
              </Grid>
            </Grid>
            {detail.data?.safe_next_action && (
              <Alert severity="warning" sx={{ mt: 2 }}>
                <Typography fontWeight="bold">{t('safeNextAction')}</Typography>
                {detail.data.safe_next_action}
              </Alert>
            )}
          </>
        )}
      </Paper>

      <Divider />
      <Box>
        <Stack direction="row" justifyContent="flex-end" alignItems="center" gap="8px" pb={1}>
          <Typography variant="h6" sx={{ mr: 'auto' }}>
            {t('log')}
          </Typography>
          <RefreshIntervalSelect context="operationLogs" />
        </Stack>
        <Box height="55vh">
          <LazyLog
            follow
            scrollToAlignment="end"
            text={log.data?.log ?? '\t'}
            extraLines={1}
            overscanRowCount={10}
            caseInsensitive
            selectableLines
            enableSearch
          />
        </Box>
      </Box>
    </Stack>
  );
};

export default OperationLog;
