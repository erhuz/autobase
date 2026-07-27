const routerClustersPathsConfig = {
  absolutePath: '/clusters',
  add: {
    absolutePath: '/clusters/add',
    relativePath: 'add',
  },
  overview: {
    absolutePath: '/clusters/:clusterId/overview',
    relativePath: 'overview',
  },
  queryPerformance: {
    absolutePath: '/clusters/:clusterId/query-performance',
    relativePath: 'query-performance',
  },
  access: {
    absolutePath: '/clusters/:clusterId/access',
    relativePath: 'access',
  },
};

export default routerClustersPathsConfig;
