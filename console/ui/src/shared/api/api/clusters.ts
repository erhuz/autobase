import { baseApi as api } from '../baseApi.ts';

const injectedRtkApi = api.injectEndpoints({
  endpoints: (build) => ({
    postClusters: build.mutation<PostClustersApiResponse, PostClustersApiArg>({
      query: (queryArg) => ({ url: `/clusters`, method: 'POST', body: queryArg.requestClusterCreate }),
      invalidatesTags: () => [{ type: 'Clusters', id: 'LIST' }],
    }),
    getClusters: build.query<GetClustersApiResponse, GetClustersApiArg>({
      query: (queryArg) => ({
        url: `/clusters`,
        params: {
          offset: queryArg.offset,
          limit: queryArg.limit,
          project_id: queryArg.projectId,
          name: queryArg.name,
          status: queryArg.status,
          location: queryArg.location,
          environment: queryArg.environment,
          server_count: queryArg.serverCount,
          postgres_version: queryArg.postgresVersion,
          created_at_from: queryArg.createdAtFrom,
          created_at_to: queryArg.createdAtTo,
          sort_by: queryArg.sortBy,
        },
      }),
      providesTags: (result) =>
        result?.data
          ? [...result.data.map(({ id }) => ({ type: 'Clusters', id })), { type: 'Clusters', id: 'LIST' }]
          : [{ type: 'Clusters', id: 'LIST' }],
    }),
    getClustersDefaultName: build.query<GetClustersDefaultNameApiResponse, GetClustersDefaultNameApiArg>({
      query: () => ({ url: `/clusters/default_name` }),
      keepUnusedDataFor: 0,
    }),
    getClustersById: build.query<GetClustersByIdApiResponse, GetClustersByIdApiArg>({
      query: (queryArg) => ({ url: `/clusters/${queryArg.id}` }),
      providesTags: (result, error, { id }) => [{ type: 'Clusters', id }],
    }),
    getClustersByIdHealth: build.query<GetClustersByIdHealthApiResponse, GetClustersByIdHealthApiArg>({
      query: (queryArg) => ({ url: `/clusters/${queryArg.id}/health` }),
      providesTags: (result, error, { id }) => [{ type: 'Clusters', id }],
    }),
    getClustersByIdQueryPerformance: build.query<
      GetClustersByIdQueryPerformanceApiResponse,
      GetClustersByIdQueryPerformanceApiArg
    >({
      query: (queryArg) => ({
        url: `/clusters/${queryArg.id}/query-performance`,
        params: queryPerformanceParams(queryArg),
      }),
    }),
    getClustersByIdQueryPerformanceFingerprintId: build.query<
      GetClustersByIdQueryPerformanceFingerprintIdApiResponse,
      GetClustersByIdQueryPerformanceFingerprintIdApiArg
    >({
      query: (queryArg) => ({
        url: `/clusters/${queryArg.id}/query-performance/${encodeURIComponent(queryArg.fingerprintId)}`,
        params: queryPerformanceParams(queryArg),
      }),
    }),
    postClustersByIdPreflights: build.mutation<PostClustersByIdPreflightsApiResponse, PostClustersByIdPreflightsApiArg>(
      {
        query: (queryArg) => ({
          url: `/clusters/${queryArg.id}/preflights`,
          method: 'POST',
          body: queryArg.requestOperationPreflight,
        }),
      },
    ),
    postClustersByIdOperations: build.mutation<PostClustersByIdOperationsApiResponse, PostClustersByIdOperationsApiArg>(
      {
        query: (queryArg) => ({
          url: `/clusters/${queryArg.id}/operations`,
          method: 'POST',
          body: queryArg.requestOperationStart,
        }),
      },
    ),
    deleteClustersById: build.mutation<DeleteClustersByIdApiResponse, DeleteClustersByIdApiArg>({
      query: (queryArg) => ({ url: `/clusters/${queryArg.id}`, method: 'DELETE' }),
      invalidatesTags: () => [{ type: 'Clusters', id: 'LIST' }],
    }),
    postClustersByIdRefresh: build.mutation<PostClustersByIdRefreshApiResponse, PostClustersByIdRefreshApiArg>({
      query: (queryArg) => ({ url: `/clusters/${queryArg.id}/refresh`, method: 'POST' }),
      invalidatesTags: (result, error, { id }) => [{ type: 'Clusters', id }],
    }),
    postClustersByIdReinit: build.mutation<PostClustersByIdReinitApiResponse, PostClustersByIdReinitApiArg>({
      query: (queryArg) => ({
        url: `/clusters/${queryArg.id}/reinit`,
        method: 'POST',
        body: queryArg.requestClusterReinit,
      }),
      invalidatesTags: (result, error, { id }) => [{ type: 'Clusters', id }],
    }),
    postClustersByIdReload: build.mutation<PostClustersByIdReloadApiResponse, PostClustersByIdReloadApiArg>({
      query: (queryArg) => ({
        url: `/clusters/${queryArg.id}/reload`,
        method: 'POST',
        body: queryArg.requestClusterReload,
      }),
      invalidatesTags: (result, error, { id }) => [{ type: 'Clusters', id }],
    }),
    postClustersByIdRestart: build.mutation<PostClustersByIdRestartApiResponse, PostClustersByIdRestartApiArg>({
      query: (queryArg) => ({
        url: `/clusters/${queryArg.id}/restart`,
        method: 'POST',
        body: queryArg.requestClusterRestart,
      }),
      invalidatesTags: (result, error, { id }) => [{ type: 'Clusters', id }],
    }),
    postClustersByIdStop: build.mutation<PostClustersByIdStopApiResponse, PostClustersByIdStopApiArg>({
      query: (queryArg) => ({
        url: `/clusters/${queryArg.id}/stop`,
        method: 'POST',
        body: queryArg.requestClusterStop,
      }),
      invalidatesTags: (result, error, { id }) => [{ type: 'Clusters', id }],
    }),
    postClustersByIdStart: build.mutation<PostClustersByIdStartApiResponse, PostClustersByIdStartApiArg>({
      query: (queryArg) => ({
        url: `/clusters/${queryArg.id}/start`,
        method: 'POST',
        body: queryArg.requestClusterStart,
      }),
      invalidatesTags: (result, error, { id }) => [{ type: 'Clusters', id }],
    }),
    postClustersByIdRemove: build.mutation<PostClustersByIdRemoveApiResponse, PostClustersByIdRemoveApiArg>({
      query: (queryArg) => ({
        url: `/clusters/${queryArg.id}/remove`,
        method: 'POST',
        body: queryArg.requestClusterRemove,
      }),
      invalidatesTags: () => [{ type: 'Clusters', id: 'LIST' }],
    }),
  }),
  overrideExisting: false,
});
export { injectedRtkApi as clustersApi };
export type PostClustersApiResponse = /** status 200 OK */ ResponseClusterCreate;
export type PostClustersApiArg = {
  requestClusterCreate: RequestClusterCreate;
};
export type GetClustersApiResponse = /** status 200 OK */ ResponseClustersInfo;
export type GetClustersApiArg = {
  offset?: number;
  limit?: number;
  projectId: number;
  /** Filter by name */
  name?: string;
  /** Filter by status */
  status?: string;
  /** Filter by location */
  location?: string;
  /** Filter by environment */
  environment?: string;
  /** Filter by server_count */
  serverCount?: number;
  /** Filter by postgres_version */
  postgresVersion?: number;
  /** Created at after this date */
  createdAtFrom?: string;
  /** Created at till this date */
  createdAtTo?: string;
  /** Sort by fields. Example: sort_by=id,-name,created_at,updated_at
   Supported values:
   - id
   - name
   - created_at
   - updated_at
   - environment
   - project
   - status
   - location
   - server_count
   - postgres_version
   */
  sortBy?: string;
};
export type GetClustersDefaultNameApiResponse = /** status 200 OK */ ResponseClusterDefaultName;
export type GetClustersDefaultNameApiArg = void;
export type GetClustersByIdApiResponse = /** status 200 OK */ ClusterInfo;
export type GetClustersByIdApiArg = {
  id: number;
};
export type GetClustersByIdHealthApiResponse = ResponseClusterHealth;
export type GetClustersByIdHealthApiArg = {
  id: number;
};
export type QueryPerformanceApiArg = {
  id: number;
  from?: string;
  to?: string;
  serverId?: number;
  database?: string;
  role?: string;
  application?: string;
};
export type GetClustersByIdQueryPerformanceApiResponse = ResponseQueryPerformance;
export type GetClustersByIdQueryPerformanceApiArg = QueryPerformanceApiArg;
export type GetClustersByIdQueryPerformanceFingerprintIdApiResponse = ResponseQueryPerformanceDetail;
export type GetClustersByIdQueryPerformanceFingerprintIdApiArg = QueryPerformanceApiArg & {
  fingerprintId: string;
};
export type PostClustersByIdPreflightsApiResponse = ResponseOperationPreflight;
export type PostClustersByIdPreflightsApiArg = { id: number; requestOperationPreflight: RequestOperationPreflight };
export type PostClustersByIdOperationsApiResponse = ResponseOperationStart;
export type PostClustersByIdOperationsApiArg = { id: number; requestOperationStart: RequestOperationStart };
export type DeleteClustersByIdApiResponse = /** status 204 OK */ void;
export type DeleteClustersByIdApiArg = {
  id: number;
};
export type PostClustersByIdRefreshApiResponse = /** status 200 OK */ ClusterInfo;
export type PostClustersByIdRefreshApiArg = {
  id: number;
};
export type PostClustersByIdReinitApiResponse = /** status 200 OK */ ResponseClusterCreate;
export type PostClustersByIdReinitApiArg = {
  id: number;
  requestClusterReinit: RequestClusterReinit;
};
export type PostClustersByIdReloadApiResponse = /** status 200 OK */ ResponseClusterCreate;
export type PostClustersByIdReloadApiArg = {
  id: number;
  requestClusterReload: RequestClusterReload;
};
export type PostClustersByIdRestartApiResponse = /** status 200 OK */ ResponseClusterCreate;
export type PostClustersByIdRestartApiArg = {
  id: number;
  requestClusterRestart: RequestClusterRestart;
};
export type PostClustersByIdStopApiResponse = /** status 200 OK */ ResponseClusterCreate;
export type PostClustersByIdStopApiArg = {
  id: number;
  requestClusterStop: RequestClusterStop;
};
export type PostClustersByIdStartApiResponse = /** status 200 OK */ ResponseClusterCreate;
export type PostClustersByIdStartApiArg = {
  id: number;
  requestClusterStart: RequestClusterStart;
};
export type PostClustersByIdRemoveApiResponse = /** status 204 OK */ void;
export type PostClustersByIdRemoveApiArg = {
  id: number;
  requestClusterRemove: RequestClusterRemove;
};
export type ResponseClusterCreate = {
  /** unique code for cluster */
  cluster_id?: number;
};
export type ErrorObject = {
  code?: number;
  title?: string;
  description?: string;
};
export type RequestClusterCreate = {
  name?: string;
  /** Info about cluster */
  description?: string;
  /** Info for deployment system authorization */
  auth_info?: {
    secret_id?: number;
  };
  /** Project for new cluster */
  project_id?: number;
  /** Project environment */
  environment_id?: number;
  envs?: string[];
  extra_vars?: string[];
  existing_cluster?: boolean;
  query_analytics_enabled?: boolean;
};
export type QueryPerformanceMetrics = {
  calls?: number;
  total_exec_time_ms?: number;
  max_exec_time_ms?: number;
  mean_exec_time_ms?: number;
  rows?: number;
  shared_blocks_hit?: number;
  shared_blocks_read?: number;
  temp_blocks_read?: number;
  temp_blocks_written?: number;
  read_time_ms?: number;
  write_time_ms?: number;
  wal_bytes?: number;
};
export type QueryPerformanceStatus = {
  state?: 'unsupported' | 'rollout_required' | 'disabled' | 'collecting' | 'enabled' | 'degraded';
  managed?: boolean;
  desired?: boolean;
  postgres_version?: number;
  expected_node_count?: number;
  collected_node_count?: number;
};
export type QueryPerformanceCoverage = {
  server_id?: number;
  server_name?: string;
  server_role?: string;
  server_status?: string;
  collection_status?: string;
  extension_version?: string | null;
  last_bucket_start?: string | null;
  last_collected_at?: string | null;
  last_error_code?: string | null;
};
export type QueryPerformancePoint = {
  bucket_start?: string;
  metrics?: QueryPerformanceMetrics;
};
export type QueryPerformanceQuery = {
  fingerprint_id?: string;
  normalized_query?: string;
  metrics?: QueryPerformanceMetrics;
  top_total_time?: boolean;
  top_max_latency?: boolean;
};
export type QueryPerformanceFilters = {
  databases?: string[];
  roles?: string[];
  applications?: string[];
};
export type ResponseQueryPerformance = {
  status?: QueryPerformanceStatus;
  coverage?: QueryPerformanceCoverage[];
  summary?: QueryPerformanceMetrics;
  series?: QueryPerformancePoint[];
  queries?: QueryPerformanceQuery[];
  filters?: QueryPerformanceFilters;
};
export type ResponseQueryPerformanceDetail = {
  fingerprint?: QueryPerformanceQuery;
  series?: QueryPerformancePoint[];
  histogram?: number[];
};
export type RequestOperationPreflight = {
  type: 'switchover' | 'reload' | 'rolling_restart' | 'query_analytics_enable' | 'query_analytics_disable';
  target?: string;
};
export type RequestOperationStart = { preflight_id: number; confirmation: string };
export type ResponsePreflightCheck = { name?: string; ok?: boolean };
export type ResponseOperationPreflight = {
  id?: number;
  type?: string;
  observed?: object;
  desired?: object;
  checks?: ResponsePreflightCheck[];
  blockers?: string[];
  plan?: string[];
  affected_nodes?: string[];
  confirmation?: string;
  expires_at?: string;
};
export type ResponseOperationStart = { operation_id?: number; status?: string };
export type HealthMember = {
  name?: string;
  role?: string;
  state?: string;
  timeline?: number | null;
  lag?: number | null;
  pending_restart?: boolean | null;
};
export type HealthTopology = {
  state?: string;
  observed_at?: string | null;
  patroni_reachable?: boolean | null;
  leader?: HealthMember | null;
  replicas?: HealthMember[];
  members?: HealthMember[];
};
export type HealthDcs = {
  state?: string;
  type?: string;
  reachable?: boolean | null;
  members?: string[];
};
export type HealthRoutingTarget = {
  role?: string;
  address?: string;
  port?: number | null;
  reachable?: boolean | null;
  role_matches?: boolean | null;
};
export type HealthRouting = {
  state?: string;
  targets?: HealthRoutingTarget[];
};
export type HealthBackup = {
  state?: string;
  repository_reachable?: boolean | null;
  latest_full?: string | null;
  latest_differential?: string | null;
  retention?: object;
  wal_continuous?: boolean | null;
  locks?: string[];
  scheduler_owner?: string | null;
  fresh?: boolean | null;
  freshness_policy?: string | null;
  restore_tested_at?: string | null;
};
export type HealthOperation = {
  id?: number;
  type?: string;
  status?: string;
  started?: string;
  finished?: string | null;
  safe_next_action?: string | null;
};
export type HealthOperationSummary = {
  active?: HealthOperation | null;
  latest?: HealthOperation | null;
  unresolved?: HealthOperation | null;
};
export type HealthRecoverability = {
  state?: 'healthy' | 'degraded';
  reasons?: string[];
};
export type ResponseClusterHealth = {
  observed_at?: string;
  topology?: HealthTopology;
  dcs?: HealthDcs;
  routing?: HealthRouting;
  backup?: HealthBackup;
  operation?: HealthOperationSummary;
  recoverability?: HealthRecoverability;
};
export type ClusterInfoInstance = {
  id?: number;
  name?: string;
  ip?: string;
  status?: string;
  role?: string;
  timeline?: number | null;
  lag?: number | null;
  tags?: object;
  pending_restart?: boolean | null;
};
export type ClusterInfo = {
  id?: number;
  name?: string;
  description?: string;
  status?: string;
  creation_time?: string;
  environment?: string;
  servers?: ClusterInfoInstance[];
  postgres_version?: number;
  /** Code of location */
  cluster_location?: string;
  /** Project for cluster */
  project_name?: string;
  connection_info?: object;
};
export type PaginationInfoForListRequests = {
  offset?: number | null;
  limit?: number | null;
  count?: number | null;
};
export type ResponseClustersInfo = {
  data?: ClusterInfo[];
  meta?: PaginationInfoForListRequests;
};
export type ResponseClusterDefaultName = {
  name?: string;
};
export type RequestClusterReinit = object;
export type RequestClusterReload = object;
export type RequestClusterRestart = object;
export type RequestClusterStop = object;
export type RequestClusterStart = object;
export type RequestClusterRemove = object;
export const {
  usePostClustersMutation,
  useGetClustersQuery,
  useLazyGetClustersQuery,
  useGetClustersDefaultNameQuery,
  useLazyGetClustersDefaultNameQuery,
  useGetClustersByIdQuery,
  useLazyGetClustersByIdQuery,
  useGetClustersByIdHealthQuery,
  useLazyGetClustersByIdHealthQuery,
  useGetClustersByIdQueryPerformanceQuery,
  useLazyGetClustersByIdQueryPerformanceQuery,
  useGetClustersByIdQueryPerformanceFingerprintIdQuery,
  useLazyGetClustersByIdQueryPerformanceFingerprintIdQuery,
  usePostClustersByIdPreflightsMutation,
  usePostClustersByIdOperationsMutation,
  useDeleteClustersByIdMutation,
  usePostClustersByIdRefreshMutation,
  usePostClustersByIdReinitMutation,
  usePostClustersByIdReloadMutation,
  usePostClustersByIdRestartMutation,
  usePostClustersByIdStopMutation,
  usePostClustersByIdStartMutation,
  usePostClustersByIdRemoveMutation,
} = injectedRtkApi;

const queryPerformanceParams = (queryArg: QueryPerformanceApiArg) => ({
  from: queryArg.from,
  to: queryArg.to,
  server_id: queryArg.serverId,
  database: queryArg.database,
  role: queryArg.role,
  application: queryArg.application,
});
