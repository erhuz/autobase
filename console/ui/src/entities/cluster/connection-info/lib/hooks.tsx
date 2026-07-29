import { useTranslation } from 'react-i18next';
import { Typography } from '@mui/material';

import CopyIcon from '@shared/ui/copy-icon';
import ConnectionInfoRowContainer from '@entities/cluster/connection-info/ui/ConnectionInfoRowConteiner.tsx';
import { ConnectionInfoProps } from '@entities/cluster/connection-info/model/types.ts';

export const useGetConnectionInfoConfig = ({
  connectionInfo,
}: {
  connectionInfo: ConnectionInfoProps['connectionInfo'];
}): { title: string; children: React.ReactNode }[] => {
  const { t } = useTranslation(['clusters', 'shared']);

  const iconFontSize = '16px';

  const renderCollection = (collection: string | number | object, defaultLabel: string) => {
    if (typeof collection === 'string' || typeof collection === 'number') {
      return [
        {
          title: defaultLabel,
          children: (
            <ConnectionInfoRowContainer>
              <Typography>{collection}</Typography>
              <CopyIcon valueToCopy={collection} sx={{ fontSize: iconFontSize }} />
            </ConnectionInfoRowContainer>
          ),
        },
      ];
    }

    if (typeof collection === 'object' && collection !== null) {
      return Object.entries(collection).map(([key, value]) => ({
        title: `${defaultLabel} ${key}`,
        children: (
          <ConnectionInfoRowContainer>
            <Typography>{value}</Typography>
            <CopyIcon valueToCopy={value} sx={{ fontSize: iconFontSize }} />
          </ConnectionInfoRowContainer>
        ),
      }));
    }

    return [];
  };

  return [
    ...(connectionInfo?.address ? renderCollection(connectionInfo.address, t('address', { ns: 'shared' })) : []),
    ...(connectionInfo?.port ? renderCollection(connectionInfo.port, t('port', { ns: 'clusters' })) : []),
  ];
};
