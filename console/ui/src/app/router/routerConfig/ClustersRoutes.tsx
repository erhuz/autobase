import { lazy } from 'react';
import { Navigate, Route } from 'react-router-dom';
import RouterPaths from '@app/router/routerPathsConfig';

const Clusters = lazy(() => import('@pages/clusters'));
const AddCluster = lazy(() => import('@pages/add-cluster'));
const ClusterDetails = lazy(() => import('@pages/cluster-details'));
const OverviewCluster = lazy(() => import('@pages/overview-cluster'));
const ClusterQueryPerformance = lazy(() => import('@pages/cluster-query-performance'));
const ClusterAccess = lazy(() => import('@pages/cluster-access'));

const ClustersRoutes = () => (
  <Route>
    {/*redirects to "clusters" when opening homepage*/}
    <Route path="" element={<Navigate to={RouterPaths.clusters.absolutePath} />} />
    <Route
      path={RouterPaths.clusters.absolutePath}
      handle={{
        breadcrumb: { label: 'clusters', ns: 'clusters' },
      }}>
      <Route path="" element={<Clusters />} />
      <Route
        path={RouterPaths.clusters.add.relativePath}
        handle={{
          breadcrumb: { label: 'createCluster', ns: 'clusters' },
        }}
        element={<AddCluster />}
      />
      <Route path=":clusterId" element={<ClusterDetails />}>
        <Route
          path={RouterPaths.clusters.overview.relativePath}
          handle={{
            breadcrumb: { label: 'overview', ns: 'shared' },
          }}
          element={<OverviewCluster />}
        />
        <Route
          path={RouterPaths.clusters.queryPerformance.relativePath}
          handle={{
            breadcrumb: { label: 'queryPerformance', ns: 'clusters' },
          }}
          element={<ClusterQueryPerformance />}
        />
        <Route
          path={RouterPaths.clusters.access.relativePath}
          handle={{
            breadcrumb: { label: 'connectionAccess', ns: 'clusters' },
          }}
          element={<ClusterAccess />}
        />
      </Route>
    </Route>
  </Route>
);

export default ClustersRoutes;
