import { FC, useMemo, useState } from 'react';
import { MRT_ColumnDef, MRT_RowData, MRT_TableOptions } from 'material-react-table';
import { OPERATIONS_TABLE_COLUMN_NAMES, operationTableColumns } from '@widgets/operations-table/model/constants.ts';
import { useTranslation } from 'react-i18next';
import { OperationsTableValues } from '@widgets/operations-table/model/types.ts';
import OperationsTableButtons from '@features/operations-table-buttons';
import OperationsTableRowActions from '@features/operations-table-row-actions';
import { useGetOperationsQuery } from '@shared/api/api/operations.ts';
import { useAppSelector } from '@app/redux/store/hooks.ts';
import { selectCurrentProject } from '@app/redux/slices/projectSlice/projectSelectors.ts';
import { subDays } from 'date-fns/subDays';
import {
  formatOperationsDate,
  getOperationsDateRangeVariants,
} from '@features/operations-table-buttons/lib/functions.ts';
import { PAGINATION_LIMIT_OPTIONS } from '@shared/config/constants.ts';
import { selectPollingInterval } from '@app/redux/slices/pollingIntervalSlice/pollingIntervalSlice.ts';
import { useGetOperationsTableData } from '@widgets/operations-table/lib/hooks.tsx';
import { manageSortingOrder } from '@shared/lib/functions.ts';
import DefaultTable from '@shared/ui/default-table';
import RowActionsMenu from '@features/row-actions-menu/ui';
import { FormControl, InputLabel, MenuItem, Select, Stack, TextField } from '@mui/material';

type OperationFilters = {
  clusterName: string;
  type: string;
  status: string;
};

export const operationFilterParams = (filters: OperationFilters) => ({
  clusterName: filters.clusterName || undefined,
  type: filters.type || undefined,
  status: filters.status || undefined,
});

const operationTypes = [
  'deploy',
  'switchover',
  'reload',
  'rolling_restart',
  'replica_reinit',
  'backup_full',
  'backup_diff',
  'query_analytics_enable',
  'query_analytics_disable',
  'node_add',
  'node_remove',
  'config_update',
];
const operationStatuses = ['queued', 'running', 'succeeded', 'failed', 'cancelled'];
const openEndedDate = '9999-12-31T23:59:59.999Z';

const OperationsTable: FC = () => {
  const { t } = useTranslation(['operations', 'shared']);

  const currentProject = useAppSelector(selectCurrentProject);
  const pollingInterval = useAppSelector(selectPollingInterval('operations'));

  const [sorting, setSorting] = useState([
    {
      id: OPERATIONS_TABLE_COLUMN_NAMES.ID,
      desc: true,
    },
  ]);
  const [pagination, setPagination] = useState({
    pageIndex: 0,
    pageSize: PAGINATION_LIMIT_OPTIONS[1].value,
  });
  const [filters, setFilters] = useState<OperationFilters>({ clusterName: '', type: '', status: '' });

  const [startDate, setStartDate] = useState({
    name: getOperationsDateRangeVariants(t)[0].value,
    value: formatOperationsDate(subDays(new Date(), 1)),
  });

  const operationsList = useGetOperationsQuery(
    {
      projectId: Number(currentProject),
      startDate: startDate.value,
      endDate: openEndedDate,
      ...operationFilterParams(filters),
      offset: pagination.pageIndex * pagination.pageSize,
      limit: pagination.pageSize,
      ...(sorting?.[0] ? { sortBy: manageSortingOrder(sorting[0]) } : {}),
    },
    { pollingInterval },
  );

  const columns = useMemo<MRT_ColumnDef<OperationsTableValues>[]>(() => operationTableColumns(t), [t]);

  const data = useGetOperationsTableData(operationsList.data?.data);
  const updateFilter = (field: keyof OperationFilters, value: string) => {
    setFilters((current) => ({ ...current, [field]: value }));
    setPagination((current) => ({ ...current, pageIndex: 0 }));
  };

  const tableConfig: MRT_TableOptions<MRT_RowData> = {
    columns,
    data,
    enablePagination: true,
    showGlobalFilter: true,
    manualSorting: true,
    manualPagination: true,
    enableRowActions: true,
    enableStickyHeader: true,
    enableMultiSort: false,
    onPaginationChange: setPagination,
    onSortingChange: setSorting,
    rowCount: operationsList.data?.meta?.count ?? 0,
    state: {
      isLoading: operationsList.isFetching,
      pagination,
      sorting,
    },
    renderRowActions: ({ row }) => <RowActionsMenu row={row} ActionsComponent={OperationsTableRowActions} />,
  };

  return (
    <>
      <Stack direction={{ xs: 'column', lg: 'row' }} justifyContent="space-between" gap={1} mb={1}>
        <Stack direction={{ xs: 'column', sm: 'row' }} gap={1}>
          <TextField
            size="small"
            label={t('clusterFilter')}
            value={filters.clusterName}
            onChange={(event) => updateFilter('clusterName', event.target.value)}
          />
          <FormControl size="small" sx={{ minWidth: 190 }}>
            <InputLabel id="operation-type-filter-label">{t('type')}</InputLabel>
            <Select
              labelId="operation-type-filter-label"
              label={t('type')}
              value={filters.type}
              onChange={(event) => updateFilter('type', event.target.value)}>
              <MenuItem value="">{t('allTypes')}</MenuItem>
              {operationTypes.map((type) => (
                <MenuItem key={type} value={type}>
                  {type}
                </MenuItem>
              ))}
            </Select>
          </FormControl>
          <FormControl size="small" sx={{ minWidth: 150 }}>
            <InputLabel id="operation-status-filter-label">{t('status', { ns: 'shared' })}</InputLabel>
            <Select
              labelId="operation-status-filter-label"
              label={t('status', { ns: 'shared' })}
              value={filters.status}
              onChange={(event) => updateFilter('status', event.target.value)}>
              <MenuItem value="">{t('allStatuses')}</MenuItem>
              {operationStatuses.map((status) => (
                <MenuItem key={status} value={status}>
                  {status}
                </MenuItem>
              ))}
            </Select>
          </FormControl>
        </Stack>
        <OperationsTableButtons refetch={operationsList.refetch} startDate={startDate} setStartDate={setStartDate} />
      </Stack>
      <DefaultTable tableConfig={tableConfig} />
    </>
  );
};

export default OperationsTable;
