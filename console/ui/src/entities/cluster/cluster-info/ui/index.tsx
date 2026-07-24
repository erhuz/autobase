import { FC, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Accordion,
  AccordionDetails,
  AccordionSummary,
  Button,
  Divider,
  MenuItem,
  Stack,
  TextField,
  Typography,
} from '@mui/material';
import ExpandMoreIcon from '@mui/icons-material/ExpandMore';
import { ClusterInfoProps } from '@entities/cluster/cluster-info/model/types.ts';
import EditNoteOutlinedIcon from '@mui/icons-material/EditNoteOutlined';
import { useGetClusterInfoConfig } from '@entities/cluster/cluster-info/lib/hooks.tsx';
import InfoCardBody from '@shared/ui/info-card-body';
import { useAppSelector } from '@app/redux/store/hooks.ts';
import { selectCurrentProject } from '@app/redux/slices/projectSlice/projectSelectors.ts';
import { useGetSecretsQuery } from '@shared/api/api/secrets.ts';
import { usePutClustersByIdCredentialMutation } from '@shared/api/api/clusters.ts';
import RouterPaths from '@app/router/routerPathsConfig';
import { Link } from 'react-router-dom';
import { toast } from 'react-toastify';

const ClusterInfo: FC<ClusterInfoProps> = ({
  clusterId,
  secretId,
  postgresVersion,
  clusterName,
  description,
  environment,
  location,
}) => {
  const { t } = useTranslation(['clusters', 'shared']);
  const projectId = useAppSelector(selectCurrentProject);
  const secrets = useGetSecretsQuery({ projectId: Number(projectId) }, { skip: !projectId });
  const [attachCredential, attachState] = usePutClustersByIdCredentialMutation();
  const [selectedCredential, setSelectedCredential] = useState<number | ''>('');
  const managementCredentials = (secrets.data?.data ?? []).filter(
    (secret) => secret.type === 'ssh_key' || secret.type === 'password',
  );

  useEffect(() => setSelectedCredential(secretId ?? ''), [secretId]);

  const config = useGetClusterInfoConfig({
    postgresVersion,
    clusterName,
    description,
    environment,
    location,
  });

  const attach = async () => {
    if (!clusterId || !selectedCredential) return;
    await attachCredential({
      id: clusterId,
      requestClusterCredential: { secret_id: selectedCredential },
    }).unwrap();
    toast.success(t('managementCredentialAttached'));
  };

  return (
    <Accordion defaultExpanded>
      <AccordionSummary expandIcon={<ExpandMoreIcon />}>
        <EditNoteOutlinedIcon />
        <Typography>{t('clusterInfo')}</Typography>
      </AccordionSummary>
      <AccordionDetails>
        <InfoCardBody config={config} />
        <Divider sx={{ my: 2 }} />
        <Stack gap={1}>
          <Typography variant="subtitle2">{t('managementCredential')}</Typography>
          <Stack direction={{ xs: 'column', sm: 'row' }} gap={1}>
            <TextField
              select
              fullWidth
              size="small"
              label={t('managementCredential')}
              value={selectedCredential}
              onChange={(event) => setSelectedCredential(Number(event.target.value))}>
              {managementCredentials.map((secret) => (
                <MenuItem key={secret.id} value={secret.id}>
                  {secret.name}
                </MenuItem>
              ))}
            </TextField>
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
          <Typography variant="caption" color={secretId ? 'success.main' : 'warning.main'}>
            {secretId ? t('managementCredentialReady') : t('managementCredentialRequired')}
          </Typography>
        </Stack>
      </AccordionDetails>
    </Accordion>
  );
};

export default ClusterInfo;
