import { ReactNode } from 'react';

export interface ConnectionInfoProps {
  clusterId?: number;
  connectionInfo?: {
    address?: string | string[] | Record<string, string | string[]>;
    port?: string | number | Record<string, string | number>;
    superuser?: string;
    password?: string;
  };
  /** Fallback server list when connection_info is not set (e.g. imported clusters) */
  servers?: { ip?: string; name?: string; role?: string }[];
}

export interface ConnectionInfoRowContainerProps {
  children: ReactNode;
}
