import { FC, useEffect, useMemo, useState } from 'react';
import {
  Alert,
  AlertColor,
  Box,
  Button,
  Chip,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  FormControl,
  Grid,
  InputLabel,
  LinearProgress,
  MenuItem,
  Paper,
  Select,
  Stack,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  TextField,
  Typography,
} from '@mui/material';
import { useTranslation } from 'react-i18next';
import {
  QueryPerformanceApiArg,
  QueryPerformanceMetrics,
  QueryPerformancePoint,
  QueryPerformanceQuery,
  QueryPerformanceStatus,
  ResponseOperationPreflight,
  useGetClustersByIdQueryPerformanceQuery,
  useLazyGetClustersByIdQueryPerformanceFingerprintIdQuery,
  usePostClustersByIdOperationsMutation,
  usePostClustersByIdPreflightsMutation,
} from '@shared/api/api/clusters.ts';

type QueryPerformanceProps = { clusterId: number };

export type QueryPerformanceSelection = {
  rangeHours: number;
  serverId?: number;
  database?: string;
  role?: string;
  application?: string;
};

const statusConfig: Record<
  NonNullable<QueryPerformanceStatus['state']> | 'unknown',
  { severity: AlertColor; key: string }
> = {
  enabled: { severity: 'success', key: 'queryPerformanceStatusEnabled' },
  collecting: { severity: 'info', key: 'queryPerformanceStatusCollecting' },
  degraded: { severity: 'warning', key: 'queryPerformanceStatusDegraded' },
  rollout_required: { severity: 'warning', key: 'queryPerformanceStatusRolloutRequired' },
  disabled: { severity: 'info', key: 'queryPerformanceStatusDisabled' },
  unsupported: { severity: 'info', key: 'queryPerformanceStatusUnsupported' },
  unknown: { severity: 'info', key: 'queryPerformanceStatusUnknown' },
};

export const queryPerformanceStatusConfig = (state?: QueryPerformanceStatus['state']) =>
  statusConfig[state ?? 'unknown'];

export const buildQueryPerformanceArgs = (
  clusterId: number,
  selection: QueryPerformanceSelection,
  now = Date.now(),
): QueryPerformanceApiArg => ({
  id: clusterId,
  from: new Date(now - selection.rangeHours * 60 * 60 * 1000).toISOString(),
  to: new Date(now).toISOString(),
  serverId: selection.serverId,
  database: selection.database,
  role: selection.role,
  application: selection.application,
});

export const normalizeBars = (values: number[]) => {
  const highest = Math.max(0, ...values);
  return values.map((value) => (highest === 0 ? 0 : (Math.max(0, value) / highest) * 100));
};

const numberFormatter = new Intl.NumberFormat(undefined, { maximumFractionDigits: 1 });

const formatNumber = (value?: number) => numberFormatter.format(value ?? 0);
const formatMilliseconds = (value?: number) => `${formatNumber(value)} ms`;

const MetricCard: FC<{ label: string; value: string }> = ({ label, value }) => (
  <Paper variant="outlined" sx={{ p: 2, height: '100%' }}>
    <Typography variant="caption" color="text.secondary">
      {label}
    </Typography>
    <Typography variant="h6">{value}</Typography>
  </Paper>
);

const Trend: FC<{ label: string; points: QueryPerformancePoint[] }> = ({ label, points }) => {
  const heights = normalizeBars(points.map((point) => point.metrics?.total_exec_time_ms ?? 0));
  return (
    <Box>
      <Typography variant="subtitle2" mb={1}>
        {label}
      </Typography>
      {points.length === 0 ? (
        <Typography color="text.secondary">—</Typography>
      ) : (
        <Box
          role="img"
          aria-label={label}
          display="flex"
          alignItems="flex-end"
          gap="2px"
          height={140}
          borderBottom={1}
          borderColor="divider">
          {points.map((point, index) => (
            <Box
              key={`${point.bucket_start}-${index}`}
              title={`${point.bucket_start ?? ''}: ${formatMilliseconds(point.metrics?.total_exec_time_ms)}`}
              bgcolor="primary.main"
              minWidth={3}
              flex={1}
              height={`${Math.max(2, heights[index])}%`}
            />
          ))}
        </Box>
      )}
    </Box>
  );
};

const QueryDetail: FC<{
  query?: QueryPerformanceQuery;
  series?: QueryPerformancePoint[];
  histogram?: number[];
  loading: boolean;
}> = ({ query, series = [], histogram = [], loading }) => {
  const { t } = useTranslation('clusters');
  const heights = normalizeBars(histogram);
  return (
    <Paper variant="outlined" sx={{ p: 2, mt: 2 }}>
      <Typography variant="subtitle1" fontWeight="bold">
        {t('queryPerformanceDetail')}
      </Typography>
      {loading && <LinearProgress sx={{ my: 1 }} />}
      {query && (
        <>
          <Typography component="code" display="block" sx={{ my: 1, overflowWrap: 'anywhere' }}>
            {query.normalized_query}
          </Typography>
          <Grid container spacing={2} mb={2}>
            <Grid size={{ xs: 12, md: 6 }}>
              <Trend label={t('queryPerformanceQueryTrend')} points={series} />
            </Grid>
            <Grid size={{ xs: 12, md: 6 }}>
              <Typography variant="subtitle2" mb={1}>
                {t('queryPerformanceHistogram')}
              </Typography>
              <Box
                role="img"
                aria-label={t('queryPerformanceHistogram')}
                display="flex"
                alignItems="flex-end"
                gap={1}
                height={140}
                borderBottom={1}
                borderColor="divider">
                {histogram.map((count, index) => (
                  <Box
                    key={index}
                    title={`${t('queryPerformanceBucket')} ${index + 1}: ${count}`}
                    bgcolor="secondary.main"
                    flex={1}
                    height={`${Math.max(2, heights[index])}%`}
                  />
                ))}
              </Box>
            </Grid>
          </Grid>
        </>
      )}
    </Paper>
  );
};

const QueryPerformance: FC<QueryPerformanceProps> = ({ clusterId }) => {
  const { t } = useTranslation('clusters');
  const [selection, setSelection] = useState<QueryPerformanceSelection>({ rangeHours: 1 });
  const [windowEnd, setWindowEnd] = useState(Date.now());
  const [selectedFingerprint, setSelectedFingerprint] = useState<string>();
  const [preflight, setPreflight] = useState<ResponseOperationPreflight>();
  const [confirmation, setConfirmation] = useState('');
  const [operationError, setOperationError] = useState('');
  const [acceptedOperation, setAcceptedOperation] = useState<number>();
  const args = useMemo(
    () => buildQueryPerformanceArgs(clusterId, selection, windowEnd),
    [clusterId, selection, windowEnd],
  );
  const overview = useGetClustersByIdQueryPerformanceQuery(args, { skip: !Number.isFinite(clusterId) });
  const [getDetail, detail] = useLazyGetClustersByIdQueryPerformanceFingerprintIdQuery();
  const [createPreflight, preflightRequest] = usePostClustersByIdPreflightsMutation();
  const [startOperation, operationRequest] = usePostClustersByIdOperationsMutation();

  useEffect(() => setSelectedFingerprint(undefined), [selection]);

  const updateSelection = (change: Partial<QueryPerformanceSelection>) => {
    setSelection((current) => ({ ...current, ...change }));
    setWindowEnd(Date.now());
  };
  const refresh = () => setWindowEnd((current) => Math.max(Date.now(), current + 1));
  const selectQuery = (fingerprintId?: string) => {
    if (!fingerprintId) return;
    setSelectedFingerprint(fingerprintId);
    void getDetail({ ...args, fingerprintId });
  };
  const beginOperation = async (type: 'query_analytics_enable' | 'query_analytics_disable') => {
    setOperationError('');
    try {
      const result = await createPreflight({ id: clusterId, requestOperationPreflight: { type } }).unwrap();
      setPreflight(result);
      setConfirmation('');
    } catch {
      setOperationError(t('queryPerformancePreflightError'));
    }
  };
  const launchOperation = async () => {
    if (!preflight?.id) return;
    setOperationError('');
    try {
      const result = await startOperation({
        id: clusterId,
        requestOperationStart: { preflight_id: preflight.id, confirmation },
      }).unwrap();
      setAcceptedOperation(result.operation_id);
      setPreflight(undefined);
      setConfirmation('');
      refresh();
    } catch {
      setOperationError(t('queryPerformanceOperationError'));
    }
  };

  if (overview.isError) {
    return (
      <Paper sx={{ p: 2 }}>
        <Alert severity="error" action={<Button onClick={refresh}>{t('retry')}</Button>}>
          {t('queryPerformanceLoadError')}
        </Alert>
      </Paper>
    );
  }

  const data = overview.data;
  const status = queryPerformanceStatusConfig(data?.status?.state);
  const summary: QueryPerformanceMetrics = data?.summary ?? {};
  const coverage = data?.coverage ?? [];
  const canEnable = data?.status?.state === 'disabled' || data?.status?.state === 'rollout_required';
  const canDisable = ['enabled', 'collecting', 'degraded'].includes(data?.status?.state ?? '');

  return (
    <Paper sx={{ p: 2 }}>
      <Stack direction={{ xs: 'column', sm: 'row' }} justifyContent="space-between" gap={1} mb={2}>
        <Box>
          <Typography variant="h6">{t('queryPerformance')}</Typography>
          <Typography variant="body2" color="text.secondary">
            {t('queryPerformanceDescription')}
          </Typography>
        </Box>
        <Stack direction="row" gap={1} alignItems="center">
          {canEnable && (
            <Button variant="outlined" onClick={() => void beginOperation('query_analytics_enable')}>
              {t('queryPerformanceEnable')}
            </Button>
          )}
          {canDisable && (
            <Button color="warning" variant="outlined" onClick={() => void beginOperation('query_analytics_disable')}>
              {t('queryPerformanceDisable')}
            </Button>
          )}
          <Button onClick={refresh} disabled={overview.isFetching}>
            {t('refresh')}
          </Button>
        </Stack>
      </Stack>

      {overview.isFetching && <LinearProgress sx={{ mb: 2 }} />}
      <Alert severity={status.severity} sx={{ mb: 2 }}>
        {t(status.key, {
          collected: data?.status?.collected_node_count ?? 0,
          expected: data?.status?.expected_node_count ?? 0,
          version: data?.status?.postgres_version ?? '',
        })}
      </Alert>
      {operationError && (
        <Alert severity="error" sx={{ mb: 2 }}>
          {operationError}
        </Alert>
      )}
      {acceptedOperation && (
        <Alert severity="info" sx={{ mb: 2 }}>
          {t('queryPerformanceOperationAccepted', { id: acceptedOperation })}
        </Alert>
      )}

      {coverage.length > 0 && (
        <Stack direction="row" flexWrap="wrap" gap={1} mb={2} aria-label={t('queryPerformanceCoverage')}>
          {coverage.map((source) => (
            <Chip
              key={source.server_id}
              size="small"
              color={source.collection_status === 'healthy' ? 'success' : 'warning'}
              label={`${source.server_name}: ${source.collection_status}${source.last_error_code ? ` (${source.last_error_code})` : ''}`}
            />
          ))}
        </Stack>
      )}

      <Stack direction={{ xs: 'column', md: 'row' }} gap={1} mb={1}>
        <FormControl size="small" sx={{ minWidth: 120 }}>
          <InputLabel id="query-performance-range-label">{t('queryPerformanceRange')}</InputLabel>
          <Select
            labelId="query-performance-range-label"
            id="query-performance-range"
            label={t('queryPerformanceRange')}
            value={selection.rangeHours}
            onChange={(event) => updateSelection({ rangeHours: Number(event.target.value) })}>
            {[1, 6, 24, 168].map((hours) => (
              <MenuItem key={hours} value={hours}>
                {hours === 168 ? t('queryPerformanceSevenDays') : t('queryPerformanceHours', { hours })}
              </MenuItem>
            ))}
          </Select>
        </FormControl>
        <FormControl size="small" sx={{ minWidth: 160 }}>
          <InputLabel id="query-performance-server-label">{t('server')}</InputLabel>
          <Select
            labelId="query-performance-server-label"
            id="query-performance-server"
            label={t('server')}
            value={selection.serverId ?? ''}
            onChange={(event) => updateSelection({ serverId: Number(event.target.value) || undefined })}>
            <MenuItem value="">{t('queryPerformanceAllServers')}</MenuItem>
            {coverage.map((source) => (
              <MenuItem key={source.server_id} value={source.server_id}>
                {source.server_name}
              </MenuItem>
            ))}
          </Select>
        </FormControl>
        {(
          [
            ['database', t('database'), data?.filters?.databases ?? []],
            ['role', t('role'), data?.filters?.roles ?? []],
            ['application', t('queryPerformanceApplication'), data?.filters?.applications ?? []],
          ] as const
        ).map(([field, label, values]) => (
          <FormControl key={field} size="small" sx={{ minWidth: 160 }}>
            <InputLabel id={`query-performance-${field}-label`}>{label}</InputLabel>
            <Select
              labelId={`query-performance-${field}-label`}
              id={`query-performance-${field}`}
              label={label}
              value={selection[field] ?? ''}
              onChange={(event) => updateSelection({ [field]: event.target.value || undefined })}>
              <MenuItem value="">{t('queryPerformanceAll')}</MenuItem>
              {values.map((value) => (
                <MenuItem key={value} value={value}>
                  {value}
                </MenuItem>
              ))}
            </Select>
          </FormControl>
        ))}
      </Stack>
      <Typography variant="caption" color="text.secondary">
        {t('queryPerformanceFilterScope')}
      </Typography>

      <Grid container spacing={2} my={1}>
        <Grid size={{ xs: 6, md: 3 }}>
          <MetricCard label={t('queryPerformanceCalls')} value={formatNumber(summary.calls)} />
        </Grid>
        <Grid size={{ xs: 6, md: 3 }}>
          <MetricCard label={t('queryPerformanceTotalTime')} value={formatMilliseconds(summary.total_exec_time_ms)} />
        </Grid>
        <Grid size={{ xs: 6, md: 3 }}>
          <MetricCard label={t('queryPerformanceMeanLatency')} value={formatMilliseconds(summary.mean_exec_time_ms)} />
        </Grid>
        <Grid size={{ xs: 6, md: 3 }}>
          <MetricCard label={t('queryPerformanceMaxLatency')} value={formatMilliseconds(summary.max_exec_time_ms)} />
        </Grid>
      </Grid>

      <Trend label={t('queryPerformanceTotalTimeTrend')} points={data?.series ?? []} />

      <Typography variant="subtitle1" fontWeight="bold" mt={3} mb={1}>
        {t('queryPerformanceTopQueries')}
      </Typography>
      <TableContainer>
        <Table size="small">
          <TableHead>
            <TableRow>
              <TableCell>{t('queryPerformanceQuery')}</TableCell>
              <TableCell align="right">{t('queryPerformanceCalls')}</TableCell>
              <TableCell align="right">{t('queryPerformanceTotalTime')}</TableCell>
              <TableCell align="right">{t('queryPerformanceMaxLatency')}</TableCell>
              <TableCell />
            </TableRow>
          </TableHead>
          <TableBody>
            {(data?.queries ?? []).map((query) => (
              <TableRow key={query.fingerprint_id} selected={selectedFingerprint === query.fingerprint_id}>
                <TableCell sx={{ maxWidth: 520 }}>
                  <Typography component="code" fontSize="0.8rem" sx={{ overflowWrap: 'anywhere' }}>
                    {query.normalized_query}
                  </Typography>
                  {query.top_max_latency && <Chip size="small" label={t('queryPerformanceSlowest')} sx={{ ml: 1 }} />}
                </TableCell>
                <TableCell align="right">{formatNumber(query.metrics?.calls)}</TableCell>
                <TableCell align="right">{formatMilliseconds(query.metrics?.total_exec_time_ms)}</TableCell>
                <TableCell align="right">{formatMilliseconds(query.metrics?.max_exec_time_ms)}</TableCell>
                <TableCell align="right">
                  <Button size="small" onClick={() => selectQuery(query.fingerprint_id)}>
                    {t('queryPerformanceDetails')}
                  </Button>
                </TableCell>
              </TableRow>
            ))}
            {(data?.queries?.length ?? 0) === 0 && (
              <TableRow>
                <TableCell colSpan={5} align="center">
                  {t('queryPerformanceNoData')}
                </TableCell>
              </TableRow>
            )}
          </TableBody>
        </Table>
      </TableContainer>

      {selectedFingerprint &&
        (detail.isError ? (
          <Alert severity="error" sx={{ mt: 2 }}>
            {t('queryPerformanceDetailError')}
          </Alert>
        ) : (
          <QueryDetail
            query={detail.data?.fingerprint}
            series={detail.data?.series}
            histogram={detail.data?.histogram}
            loading={detail.isFetching}
          />
        ))}

      <Dialog open={Boolean(preflight)} onClose={() => setPreflight(undefined)} fullWidth maxWidth="sm">
        <DialogTitle>{t('queryPerformanceOperationPreflight')}</DialogTitle>
        <DialogContent>
          {(preflight?.blockers?.length ?? 0) > 0 && (
            <Alert severity="error" sx={{ mb: 2 }}>
              {preflight?.blockers?.join(', ')}
            </Alert>
          )}
          <Typography variant="subtitle2">{t('queryPerformanceAffectedNodes')}</Typography>
          <Typography variant="body2" mb={2}>
            {preflight?.affected_nodes?.join(', ') || '—'}
          </Typography>
          <Typography variant="subtitle2">{t('queryPerformanceOperationPlan')}</Typography>
          <Box component="ol" mt={0} pl={3}>
            {preflight?.plan?.map((step) => (
              <li key={step}>{step}</li>
            ))}
          </Box>
          <TextField
            fullWidth
            label={t('queryPerformanceConfirmation')}
            helperText={t('queryPerformanceConfirmationHelp', { phrase: preflight?.confirmation })}
            value={confirmation}
            onChange={(event) => setConfirmation(event.target.value)}
            autoComplete="off"
          />
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setPreflight(undefined)}>{t('cancel')}</Button>
          <Button
            variant="contained"
            color="warning"
            disabled={
              preflightRequest.isLoading ||
              operationRequest.isLoading ||
              Boolean(preflight?.blockers?.length) ||
              confirmation !== preflight?.confirmation
            }
            onClick={() => void launchOperation()}>
            {t('queryPerformanceStartOperation')}
          </Button>
        </DialogActions>
      </Dialog>
    </Paper>
  );
};

export default QueryPerformance;
