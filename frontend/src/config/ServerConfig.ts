import _ from 'lodash';
import { ServerConfig } from '../types/ServerConfig';
import { parseHealthConfig } from './HealthConfig';
import { MeshCluster } from '../types/Mesh';

export type Durations = { [key: number]: string };

export type ComputedServerConfig = ServerConfig & {
  durations: Durations;
};

function getHomeCluster(cfg: ServerConfig): MeshCluster | undefined {
  return Object.values(cfg.clusters).find(cluster => cluster.isKialiHome);
}

export const humanDurations = (cfg: ComputedServerConfig, prefix?: string, suffix?: string): Durations =>
  _.mapValues(cfg.durations, v => _.reject([prefix, v, suffix], _.isEmpty).join(' '));

const toDurations = (tupleArray: [number, string][]): Durations => {
  const obj: Duration = {};
  tupleArray.forEach(tuple => {
    obj[tuple[0]] = tuple[1];
  });
  return obj;
};

const durationsTuples: [number, string][] = [
  [60, '1m'],
  [120, '2m'],
  [300, '5m'],
  [600, '10m'],
  [1800, '30m'],
  [3600, '1h'],
  [10800, '3h'],
  [21600, '6h'],
  [43200, '12h'],
  [86400, '1d'],
  [604800, '7d'],
  [2592000, '30d']
];

const computeValidDurations = (cfg: ComputedServerConfig): void => {
  const tsdbRetention = cfg.prometheus.storageTsdbRetention;
  const scrapeInterval = cfg.prometheus.globalScrapeInterval;
  let filtered = durationsTuples.filter(
    d => (!tsdbRetention || d[0] <= tsdbRetention!) && (!scrapeInterval || d[0] >= scrapeInterval * 2)
  );
  // Make sure we keep at least one item, even if it's silly
  if (filtered.length === 0) {
    filtered = [durationsTuples[0]];
  }
  cfg.durations = toDurations(filtered);
};

// Set some reasonable defaults. Initial values should be valid for fields
// than may not be providedby/set on the server.
const defaultServerConfig: ComputedServerConfig = {
  ambientEnabled: false,
  authStrategy: '',
  clusters: {},
  clusterWideAccess: true,
  controlPlanes: { Kubernetes: 'istio-system' },
  durations: {},
  gatewayAPIClasses: [],
  gatewayAPIEnabled: false,
  logLevel: '',
  healthConfig: {
    rate: []
  },
  deployment: {
    viewOnlyMode: false
  },
  ignoreHomeCluster: false,
  installationTag: 'Kiali Console',
  istioAnnotations: {
    ambientAnnotation: 'ambient.istio.io/redirection',
    ambientAnnotationEnabled: 'enabled',
    istioInjectionAnnotation: 'sidecar.istio.io/inject'
  },
  istioIdentityDomain: 'svc.cluster.local',
  istioLabels: {
    ambientNamespaceLabel: 'istio.io/dataplane-mode',
    ambientNamespaceLabelValue: 'ambient',
    ambientWaypointGatewayLabel: 'gateway.networking.k8s.io/gateway-name',
    ambientWaypointLabel: 'gateway.istio.io/managed',
    ambientWaypointLabelValue: 'istio.io-mesh-controller',
    appLabelName: '',
    injectionLabelName: 'istio-injection',
    injectionLabelRev: 'istio.io/rev',
    versionLabelName: ''
  },
  kialiFeatureFlags: {
    disabledFeatures: [],
    istioInjectionAction: true,
    istioAnnotationAction: true,
    istioUpgradeAction: false,
    uiDefaults: {
      graph: {
        findOptions: [],
        hideOptions: [],
        settings: {
          animation: 'point'
        },
        traffic: {
          ambient: 'total',
          grpc: 'requests',
          http: 'requests',
          tcp: 'sent'
        }
      },
      i18n: {
        language: 'en',
        showSelector: false
      },
      list: {
        includeHealth: true,
        includeIstioResources: true,
        includeValidations: true,
        showIncludeToggles: false
      },
      mesh: {
        findOptions: [],
        hideOptions: []
      },
      tracing: { limit: 100 }
    }
  },
  prometheus: {
    globalScrapeInterval: 15,
    storageTsdbRetention: 21600
  }
};

// Overwritten with real server config on user login. Also used for tests.
let serverConfig = defaultServerConfig;
computeValidDurations(serverConfig);
export { serverConfig };

let homeCluster = getHomeCluster(serverConfig);
let isMultiCluster = isMC();
export { homeCluster, isMultiCluster };

// set when serverConfig is set
let appLabelNames: string[] = [];
let versionLabelNames: string[] = [];

export const toValidDuration = (duration: number): number => {
  // Check if valid
  if (serverConfig.durations[duration]) {
    return duration;
  }
  // Get closest duration
  const validDurations = durationsTuples.filter(d => serverConfig.durations[d[0]]);
  for (let i = validDurations.length - 1; i > 0; i--) {
    if (duration > durationsTuples[i][0]) {
      return validDurations[i][0];
    }
  }
  return validDurations[0][0];
};

export const setServerConfig = (cfg: ServerConfig): void => {
  serverConfig = {
    ...defaultServerConfig,
    ...cfg
  };

  serverConfig.healthConfig = cfg.healthConfig ? parseHealthConfig(cfg.healthConfig) : serverConfig.healthConfig;
  computeValidDurations(serverConfig);

  homeCluster = getHomeCluster(serverConfig);
  isMultiCluster = isMC();
  if (!serverConfig.ambientEnabled) {
    serverConfig.kialiFeatureFlags.uiDefaults.graph.traffic.ambient = 'none';
  }

  // set the current app and version label name possibilities
  if (cfg.istioLabels.appLabelName !== '' && cfg.istioLabels.versionLabelName !== '') {
    appLabelNames = [cfg.istioLabels.appLabelName];
    versionLabelNames = [cfg.istioLabels.versionLabelName];
  } else {
    appLabelNames = ['service.istio.io/canonical-name', 'app.kubernetes.io/name', 'app'];
    versionLabelNames = ['service.istio.io/canonical-revision', 'app.kubernetes.io/version', 'version'];
  }
};

export const isIstioControlPlane = (cluster: string, namespace: string): boolean => {
  return serverConfig.controlPlanes[cluster] === namespace;
};

export const isIstioNamespace = (namespace: string): boolean => {
  return Object.values(serverConfig.controlPlanes).some(cpNamespace => cpNamespace === namespace);
};

export const istioNamespaces = (): string[] => {
  const cpNamespaces = Object.values(serverConfig.controlPlanes);
  return [...new Set(cpNamespaces)];
};

export const isHomeCluster = (cluster: string): boolean => {
  return !isMultiCluster || cluster === homeCluster?.name;
};

// Return true if the cluster is configured for this Kiali instance
export const isConfiguredCluster = (cluster: string): boolean => {
  return Object.keys(serverConfig.clusters).includes(cluster);
};

function isMC(): boolean {
  // If there is only one cluster, it is not a multi-cluster deployment.
  // If there are multiple clusters but only one is accessible, it is not a multi-cluster deployment.
  return (
    Object.keys(serverConfig.clusters).length > 1 &&
    Object.values(serverConfig.clusters).filter(c => c.accessible).length > 1
  );
}

// getAppLabelName returns the app label name found in the labels, or undefined
export const getAppLabelName = (labels?: { [key: string]: string }): string | undefined => {
  if (labels) {
    return appLabelNames.find(labelName => labels[labelName] !== undefined);
  }
  return undefined;
};

// getVersionLabelName returns the version label name found in the labels, or undefined
export const getVersionLabelName = (labels?: { [key: string]: string }): string | undefined => {
  if (labels) {
    return versionLabelNames.find(labelName => labels[labelName] !== undefined);
  }
  return undefined;
};
