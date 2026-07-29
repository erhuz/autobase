import { FC, useEffect, useMemo, useState } from 'react';
import { Alert, Box, Button, Divider, Paper, Stack, TextField, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router-dom';
import StorageIcon from '@mui/icons-material/Storage';
import { toast } from 'react-toastify';
import { ConnectionInfoProps } from '@entities/cluster/connection-info/model/types.ts';
import { useGetConnectionInfoConfig } from '@entities/cluster/connection-info/lib/hooks.tsx';
import InfoCardBody from '@shared/ui/info-card-body';
import { DBDESK_URL } from '@shared/config/constants.ts';
import RouterPaths from '@app/router/routerPathsConfig';
import {
  RequestClusterRouting,
  RequestClusterRoutingTarget,
  usePatchClustersByIdRoutingMutation,
} from '@shared/api/api/clusters.ts';

const routingRoles = [
  { key: 'primary', label: 'routingPrimary' },
  { key: 'replica', label: 'routingReplica' },
  { key: 'replica_sync', label: 'routingReplicaSync' },
  { key: 'replica_async', label: 'routingReplicaAsync' },
] as const;

type RoutingRole = (typeof routingRoles)[number]['key'];
type RoutingFields = { addresses: string; port: string };
type RoutingForm = Record<RoutingRole, RoutingFields>;

const emptyRoutingForm = (): RoutingForm => ({
  primary: { addresses: '', port: '' },
  replica: { addresses: '', port: '' },
  replica_sync: { addresses: '', port: '' },
  replica_async: { addresses: '', port: '' },
});

const isRecord = (value: unknown): value is Record<string, unknown> =>
  typeof value === 'object' && value !== null && !Array.isArray(value);

const addressesFrom = (value: unknown): string[] => {
  if (Array.isArray(value)) return value.flatMap(addressesFrom);
  if (typeof value !== 'string') return [];
  return value
    .split(/[\n,]/)
    .map((address) => address.trim())
    .filter(Boolean);
};

const portFrom = (value: unknown): string =>
  typeof value === 'string' || typeof value === 'number' ? String(value) : '';

const formFromConnectionInfo = (connectionInfo: ConnectionInfoProps['connectionInfo']): RoutingForm => {
  const form = emptyRoutingForm();
  const addresses = connectionInfo?.address;
  const ports = connectionInfo?.port;
  const addressMap = isRecord(addresses) ? addresses : undefined;
  const portMap = isRecord(ports) ? ports : undefined;

  if (!addressMap && !portMap) return form;
  for (const { key } of routingRoles) {
    const roleAddresses = addressesFrom(addressMap ? addressMap[key] : addresses);
    const rolePort = portFrom(portMap ? portMap[key] : ports);
    if (roleAddresses.length > 0 && rolePort) {
      form[key] = { addresses: roleAddresses.join('\n'), port: rolePort };
    }
  }
  return form;
};

const targetFrom = (fields: RoutingFields): RequestClusterRoutingTarget | null => {
  const addresses = addressesFrom(fields.addresses);
  const port = Number(fields.port);
  if (addresses.length === 0 && fields.port === '') return null;
  if (addresses.length === 0 || !Number.isInteger(port) || port < 1 || port > 65535) return null;
  return { addresses, port };
};

const isPartialTarget = (fields: RoutingFields): boolean =>
  (addressesFrom(fields.addresses).length > 0 || fields.port !== '') && targetFrom(fields) === null;

const routingPatch = (saved: RoutingForm, current: RoutingForm): RequestClusterRouting => {
  const patch: RequestClusterRouting = {};
  for (const { key } of routingRoles) {
    const before = targetFrom(saved[key]);
    const after = targetFrom(current[key]);
    if (JSON.stringify(before) !== JSON.stringify(after)) patch[key] = after;
  }
  return patch;
};

const ConnectionInfo: FC<ConnectionInfoProps> = ({ clusterId, connectionInfo, servers }) => {
  const { t } = useTranslation(['clusters', 'shared']);
  const navigate = useNavigate();
  const config = useGetConnectionInfoConfig({ connectionInfo });
  const initialForm = useMemo(() => formFromConnectionInfo(connectionInfo), [connectionInfo]);
  const [savedForm, setSavedForm] = useState<RoutingForm>(initialForm);
  const [form, setForm] = useState<RoutingForm>(initialForm);
  const [saveRouting, saveState] = usePatchClustersByIdRoutingMutation();
  const hasConnectionData = connectionInfo?.address || connectionInfo?.superuser || (servers?.length ?? 0) > 0;
  const patch = routingPatch(savedForm, form);
  const primary = targetFrom(form.primary);
  const hasPartialRole = routingRoles.some(({ key }) => isPartialTarget(form[key]));
  const canSave = Boolean(clusterId && primary && !hasPartialRole && Object.keys(patch).length > 0);

  useEffect(() => {
    setSavedForm(initialForm);
    setForm(initialForm);
  }, [initialForm]);

  const updateField = (role: RoutingRole, field: keyof RoutingFields, value: string) => {
    setForm((current) => ({ ...current, [role]: { ...current[role], [field]: value } }));
  };

  const save = async () => {
    if (!clusterId || !canSave) return;
    try {
      await saveRouting({ id: clusterId, requestClusterRouting: patch }).unwrap();
      setSavedForm(form);
      toast.success(t('routingSaved'));
    } catch {
      toast.error(t('routingSaveError'));
    }
  };

  return (
    <Paper variant="outlined" sx={{ p: { xs: 2, sm: 2.5 }, height: '100%', boxShadow: 'none' }}>
      <Stack gap={2}>
        <Box>
          <Typography variant="h6">{t('connectionDetails')}</Typography>
          <Typography variant="body2" color="text.secondary">
            {t('connectionDetailsDescription')}
          </Typography>
        </Box>
        <InfoCardBody config={config} />
        <Divider />
        <Stack gap={0.5}>
          <Typography variant="subtitle1">{t('routingConfiguration')}</Typography>
          <Typography variant="body2" color="text.secondary">
            {t('routingConfigurationDescription')}
          </Typography>
        </Stack>
        {routingRoles.map(({ key, label }) => {
          const invalid = isPartialTarget(form[key]);
          return (
            <Stack key={key} gap={1}>
              <Typography variant="subtitle2">{t(label)}</Typography>
              <Stack direction={{ xs: 'column', sm: 'row' }} gap={1}>
                <TextField
                  fullWidth
                  multiline
                  minRows={2}
                  size="small"
                  label={t('routingAddresses', { role: t(label) })}
                  value={form[key].addresses}
                  disabled={saveState.isLoading}
                  error={invalid}
                  helperText={
                    invalid
                      ? t('routingInvalid')
                      : key === 'primary'
                        ? t('routingPrimaryRequired')
                        : t('routingOptional')
                  }
                  onChange={(event) => updateField(key, 'addresses', event.target.value)}
                />
                <TextField
                  type="number"
                  size="small"
                  label={t('routingPort', { role: t(label) })}
                  value={form[key].port}
                  disabled={saveState.isLoading}
                  error={invalid}
                  slotProps={{ htmlInput: { min: 1, max: 65535 } }}
                  onChange={(event) => updateField(key, 'port', event.target.value)}
                  sx={{ minWidth: { sm: 150 } }}
                />
              </Stack>
            </Stack>
          );
        })}
        {saveState.isError && <Alert severity="error">{t('routingSaveError')}</Alert>}
        <Button variant="contained" disabled={!canSave || saveState.isLoading} onClick={save}>
          {t('saveRouting')}
        </Button>
        {DBDESK_URL && hasConnectionData && (
          <Button
            variant="outlined"
            size="small"
            startIcon={<StorageIcon />}
            onClick={() => {
              navigate(RouterPaths.sqlEditor.absolutePath);
            }}
            sx={{ mt: 2 }}>
            {t('openInSqlEditor', { ns: 'shared' })}
          </Button>
        )}
      </Stack>
    </Paper>
  );
};

export default ConnectionInfo;
