import { FC } from 'react';
import { Box, Button, Paper, Stack, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router-dom';
import StorageIcon from '@mui/icons-material/Storage';
import { ConnectionInfoProps } from '@entities/cluster/connection-info/model/types.ts';
import { useGetConnectionInfoConfig } from '@entities/cluster/connection-info/lib/hooks.tsx';
import InfoCardBody from '@shared/ui/info-card-body';
import { DBDESK_URL } from '@shared/config/constants.ts';
import RouterPaths from '@app/router/routerPathsConfig';

const ConnectionInfo: FC<ConnectionInfoProps> = ({ connectionInfo, servers }) => {
  const { t } = useTranslation(['clusters', 'shared']);
  const navigate = useNavigate();

  const config = useGetConnectionInfoConfig({ connectionInfo });

  const hasConnectionData = connectionInfo?.address || connectionInfo?.superuser || (servers?.length ?? 0) > 0;

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
