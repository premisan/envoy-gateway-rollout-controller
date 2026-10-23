export interface NodeStatus {
  nodeName: string;
  nodeIP: string;
  phase: 'Stable' | 'Canary' | 'Draining' | 'Completed' | 'RolledBack';
  activeVersion: string;
  canaryVersion?: string;
  drainProgress: number;
  activeSessions: number;
  errorCount5xx: number;
}

export interface MetricRule {
  name: string;
  threshold: number;
  query: string;
}

export interface RolloutItem {
  metadata: {
    name: string;
    namespace: string;
    creationTimestamp: string;
  };
  spec: {
    version: string;
    targetNamespace: string;
    gateway: {
      name: string;
    };
    strategy: {
      steps: Array<{
        nodes: string;
        pauseDuration: string;
      }>;
    };
    analysis: {
      ephemeralPrometheus?: {
        enabled: boolean;
        scrapeInterval: string;
      };
      prometheusUrl?: string;
      failureLimit: number;
      metrics?: MetricRule[];
    };
    control?: {
      paused?: boolean;
      abort?: boolean;
      forcePromote?: boolean;
    };
  };
  status?: {
    phase: string;
    currentVersion: string;
    targetVersion: string;
    currentStep: number;
    totalSteps: number;
    stepTimeRemaining: string;
    ephemeralPrometheusUrl?: string;
    consecutiveFailures: number;
    message: string;
    nodes?: NodeStatus[];
    lastMetricSnapshot?: Record<string, number>;
  };
}
