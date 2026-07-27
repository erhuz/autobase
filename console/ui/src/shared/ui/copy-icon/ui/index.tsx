import { FC } from 'react';
import { CopyIconProps } from '@shared/ui/copy-icon/model/types.ts';
import { useCopyToClipboard } from '@shared/lib/hooks.tsx';
import CopyValueIcon from '@mui/icons-material/ContentCopyOutlined';
import { IconButton, SxProps, Theme } from '@mui/material';
import { useTranslation } from 'react-i18next';

const CopyIcon: FC<CopyIconProps & { sx?: SxProps<Theme> }> = ({ valueToCopy, sx }) => {
  const { t } = useTranslation('shared');
  const copyFunction = useCopyToClipboard()[1];

  return (
    <IconButton
      size="small"
      aria-label={t('copyToClipboard')}
      disabled={!valueToCopy}
      onClick={() => valueToCopy && copyFunction(valueToCopy)}
      sx={{ p: 0.5, ...sx }}>
      <CopyValueIcon sx={{ fontSize: 'inherit' }} />
    </IconButton>
  );
};

export default CopyIcon;
