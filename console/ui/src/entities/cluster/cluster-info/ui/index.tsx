import { FC, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button, MenuItem, Paper, Stack, TextField, Typography } from '@mui/material';
import { ClusterInfoProps } from '@entities/cluster/cluster-info/model/types.ts';
import { useAppSelector } from '@app/redux/store/hooks.ts';
import { selectCurrentProject } from '@app/redux/slices/projectSlice/projectSelectors.ts';
import { useGetSecretsQuery } from '@shared/api/api/secrets.ts';
import { usePutClustersByIdCredentialMutation } from '@shared/api/api/clusters.ts';
import RouterPaths from '@app/router/routerPathsConfig';
import { Link } from 'react-router-dom';
import { toast } from 'react-toastify';

const ClusterInfo: FC<ClusterInfoProps> = ({ clusterId, secretId }) => {
  const { t } = useTranslation(['clusters', 'shared']);
  const projectId = useAppSelector(selectCurrentProject);
  const secrets = useGetSecretsQuery({ projectId: Number(projectId) }, { skip: !projectId });
  const [attachCredential, attachState] = usePutClustersByIdCredentialMutation();
  const [selectedCredential, setSelectedCredential] = useState<number | ''>('');
  const managementCredentials = (secrets.data?.data ?? []).filter(
    (secret) => secret.type === 'ssh_key' || secret.type === 'password',
  );
  const selectedValue =
    selectedCredential === '' || managementCredentials.some((secret) => secret.id === selectedCredential)
      ? selectedCredential
      : '';

  useEffect(() => setSelectedCredential(secretId ?? ''), [secretId]);

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
          <Button component={Link} to={RouterPaths.settings.secrets.absolutePath}>
            {t('manageCredentials')}
          </Button>
        </Stack>
      </Stack>
    </Paper>
  );
};

export default ClusterInfo;
