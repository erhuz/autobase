import { FC, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button, Divider, MenuItem, Paper, Stack, TextField, Typography } from '@mui/material';
import { ClusterInfoProps } from '@entities/cluster/cluster-info/model/types.ts';
import { useAppSelector } from '@app/redux/store/hooks.ts';
import { selectCurrentProject } from '@app/redux/slices/projectSlice/projectSelectors.ts';
import { useGetSecretsQuery } from '@shared/api/api/secrets.ts';
import {
  usePutClustersByIdAutomationCredentialsMutation,
  usePutClustersByIdCredentialMutation,
} from '@shared/api/api/clusters.ts';
import type { RequestClusterAutomationCredentials } from '@shared/api/api/clusters.ts';
import RouterPaths from '@app/router/routerPathsConfig';
import { Link } from 'react-router-dom';
import { toast } from 'react-toastify';

type AutomationCredentialKey = keyof RequestClusterAutomationCredentials;
type AutomationCredentialSelection = Record<AutomationCredentialKey, number | ''>;

const emptyAutomationCredentials: AutomationCredentialSelection = {
  postgres_superuser_secret_id: '',
  postgres_replication_secret_id: '',
  patroni_restapi_secret_id: '',
};

const ClusterInfo: FC<ClusterInfoProps> = ({ clusterId, secretId, automationCredentials }) => {
  const { t } = useTranslation(['clusters', 'shared']);
  const projectId = useAppSelector(selectCurrentProject);
  const secrets = useGetSecretsQuery({ projectId: Number(projectId) }, { skip: !projectId });
  const [attachCredential, attachState] = usePutClustersByIdCredentialMutation();
  const [saveAutomationCredentials, automationState] = usePutClustersByIdAutomationCredentialsMutation();
  const [selectedCredential, setSelectedCredential] = useState<number | ''>('');
  const [selectedAutomationCredentials, setSelectedAutomationCredentials] =
    useState<AutomationCredentialSelection>(emptyAutomationCredentials);
  const managementCredentials = (secrets.data?.data ?? []).filter(
    (secret) => secret.type === 'ssh_key' || secret.type === 'password',
  );
  const passwordCredentials = (secrets.data?.data ?? []).filter((secret) => secret.type === 'password');
  const selectedValue =
    selectedCredential === '' || managementCredentials.some((secret) => secret.id === selectedCredential)
      ? selectedCredential
      : '';
  const savedAutomationCredentials: AutomationCredentialSelection = {
    postgres_superuser_secret_id: automationCredentials?.postgres_superuser_secret_id ?? '',
    postgres_replication_secret_id: automationCredentials?.postgres_replication_secret_id ?? '',
    patroni_restapi_secret_id: automationCredentials?.patroni_restapi_secret_id ?? '',
  };
  const automationChanged = Object.keys(savedAutomationCredentials).some(
    (key) =>
      selectedAutomationCredentials[key as AutomationCredentialKey] !==
      savedAutomationCredentials[key as AutomationCredentialKey],
  );
  const automationFields: Array<{ key: AutomationCredentialKey; label: string }> = [
    { key: 'postgres_superuser_secret_id', label: t('postgresSuperuserCredential') },
    { key: 'postgres_replication_secret_id', label: t('postgresReplicationCredential') },
    { key: 'patroni_restapi_secret_id', label: t('patroniRestapiCredential') },
  ];

  useEffect(() => setSelectedCredential(secretId ?? ''), [secretId]);
  useEffect(
    () =>
      setSelectedAutomationCredentials({
        postgres_superuser_secret_id: automationCredentials?.postgres_superuser_secret_id ?? '',
        postgres_replication_secret_id: automationCredentials?.postgres_replication_secret_id ?? '',
        patroni_restapi_secret_id: automationCredentials?.patroni_restapi_secret_id ?? '',
      }),
    [automationCredentials],
  );

  const attach = async () => {
    if (!clusterId || !selectedCredential) return;
    try {
      await attachCredential({
        id: clusterId,
        requestClusterCredential: { secret_id: selectedCredential },
      }).unwrap();
      toast.success(t('managementCredentialAttached'));
    } catch {
      toast.error(t('managementCredentialAttachError'));
    }
  };

  const saveAutomation = async () => {
    if (!clusterId) return;
    try {
      await saveAutomationCredentials({
        id: clusterId,
        requestClusterAutomationCredentials: {
          postgres_superuser_secret_id: selectedAutomationCredentials.postgres_superuser_secret_id || null,
          postgres_replication_secret_id: selectedAutomationCredentials.postgres_replication_secret_id || null,
          patroni_restapi_secret_id: selectedAutomationCredentials.patroni_restapi_secret_id || null,
        },
      }).unwrap();
      toast.success(t('automationCredentialsSaved'));
    } catch {
      toast.error(t('automationCredentialsSaveError'));
    }
  };

  return (
    <Paper variant="outlined" sx={{ p: { xs: 2, sm: 2.5 }, height: '100%', boxShadow: 'none' }}>
      <Stack gap={2}>
        <Stack gap={0.5}>
          <Typography variant="h6">{t('managementAccess')}</Typography>
          <Typography variant="body2" color="text.secondary">
            {t('managementAccessDescription')}
          </Typography>
        </Stack>

        <TextField
          select
          fullWidth
          size="small"
          label={t('managementCredential')}
          value={selectedValue}
          disabled={secrets.isFetching || attachState.isLoading}
          onChange={(event) =>
            setSelectedCredential(event.target.value === '' ? '' : Number(event.target.value))
          }>
          <MenuItem value="">
            <em>{t('selectManagementCredential')}</em>
          </MenuItem>
          {managementCredentials.map((secret) => (
            <MenuItem key={secret.id} value={secret.id}>
              {secret.name}
            </MenuItem>
          ))}
        </TextField>

        <Typography variant="body2" color={secretId ? 'success.main' : 'warning.main'}>
          {secretId ? t('managementCredentialReady') : t('managementCredentialRequired')}
        </Typography>

        <Stack direction={{ xs: 'column', sm: 'row' }} gap={1}>
          <Button
            variant="contained"
            disabled={!selectedCredential || selectedCredential === secretId || attachState.isLoading}
            onClick={attach}>
            {t('attachCredential')}
          </Button>
        </Stack>

        <Divider />

        <Stack gap={0.5}>
          <Typography variant="subtitle1">{t('automationCredentials')}</Typography>
          <Typography variant="body2" color="text.secondary">
            {t('automationCredentialsDescription')}
          </Typography>
        </Stack>

        {automationFields.map((field) => {
          const selected = selectedAutomationCredentials[field.key];
          const value =
            selected === '' || passwordCredentials.some((secret) => secret.id === selected) ? selected : '';
          return (
            <Stack key={field.key} gap={0.5}>
              <TextField
                select
                fullWidth
                size="small"
                label={field.label}
                value={value}
                disabled={secrets.isFetching || automationState.isLoading}
                onChange={(event) =>
                  setSelectedAutomationCredentials((current) => ({
                    ...current,
                    [field.key]: event.target.value === '' ? '' : Number(event.target.value),
                  }))
                }>
                <MenuItem value="">
                  <em>{t('selectPasswordCredential')}</em>
                </MenuItem>
                {passwordCredentials.map((secret) => (
                  <MenuItem key={secret.id} value={secret.id}>
                    {secret.name}
                  </MenuItem>
                ))}
              </TextField>
              <Typography variant="caption" color={value ? 'success.main' : 'warning.main'}>
                {value ? t('automationCredentialReady') : t('automationCredentialRequired')}
              </Typography>
            </Stack>
          );
        })}

        {passwordCredentials.length === 0 && (
          <Typography variant="body2" color="warning.main">
            {t('noPasswordCredentials')}
          </Typography>
        )}

        <Stack direction={{ xs: 'column', sm: 'row' }} gap={1}>
          <Button
            variant="contained"
            disabled={!clusterId || !automationChanged || automationState.isLoading}
            onClick={saveAutomation}>
            {t('saveAutomationCredentials')}
          </Button>
          <Button component={Link} to={RouterPaths.settings.secrets.absolutePath}>
            {t('manageCredentials')}
          </Button>
        </Stack>
      </Stack>
    </Paper>
  );
};

export default ClusterInfo;
