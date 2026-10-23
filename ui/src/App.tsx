import React, { useEffect, useState } from 'react';
import { Navbar } from './components/Navbar';
import { RolloutHeader } from './components/RolloutHeader';
import { NodeTopologyView } from './components/NodeTopologyView';
import { MetricCharts } from './components/MetricCharts';
import { ControlPanel } from './components/ControlPanel';
import { UpgradeModal } from './components/UpgradeModal';
import { RolloutItem } from './types';
import { RefreshCw, PlusCircle, AlertCircle } from 'lucide-react';

const SAMPLE_FALLBACK_ROLLOUT: RolloutItem = {
  metadata: {
    name: 'eg-rollout-prod',
    namespace: 'envoy-gateway-system',
    creationTimestamp: new Date().toISOString(),
  },
  spec: {
    version: 'v1.9.2',
    targetNamespace: 'envoy-gateway-system',
    gateway: {
      name: 'eg',
    },
    strategy: {
      steps: [
        { nodes: '1', pauseDuration: '3m' },
        { nodes: '3', pauseDuration: '5m' },
        { nodes: '100%', pauseDuration: '5m' },
      ],
    },
    analysis: {
      ephemeralPrometheus: {
        enabled: true,
        scrapeInterval: '5s',
      },
      failureLimit: 3,
    },
    control: {
      paused: false,
      abort: false,
      forcePromote: false,
    },
  },
  status: {
    phase: 'Observing',
    currentVersion: 'v1.8.5',
    targetVersion: 'v1.9.2',
    currentStep: 1,
    totalSteps: 3,
    stepTimeRemaining: '2m 14s',
    ephemeralPrometheusUrl: 'http://egr-prom-eg-rollout-prod.envoy-gateway-system.svc:9090',
    consecutiveFailures: 0,
    message: 'Observing Step 1/3 (1 node canary). Prometheus metrics within safe threshold.',
    nodes: [
      {
        nodeName: 'ingress-lb-node-01',
        nodeIP: '10.244.1.11',
        phase: 'Canary',
        activeVersion: 'v1.8.5',
        canaryVersion: 'v1.9.2',
        drainProgress: 0,
        activeSessions: 142,
        errorCount5xx: 0,
      },
      {
        nodeName: 'ingress-lb-node-02',
        nodeIP: '10.244.1.12',
        phase: 'Stable',
        activeVersion: 'v1.8.5',
        drainProgress: 0,
        activeSessions: 268,
        errorCount5xx: 0,
      },
      {
        nodeName: 'ingress-lb-node-03',
        nodeIP: '10.244.1.13',
        phase: 'Stable',
        activeVersion: 'v1.8.5',
        drainProgress: 0,
        activeSessions: 285,
        errorCount5xx: 0,
      },
    ],
    lastMetricSnapshot: {
      'http-5xx-rate': 0.04,
      'connection-errors': 0.0,
      'p99-latency': 12.4,
    },
  },
};

export function App() {
  const [rollouts, setRollouts] = useState<RolloutItem[]>([SAMPLE_FALLBACK_ROLLOUT]);
  const [selectedRolloutName, setSelectedRolloutName] = useState<string>('eg-rollout-prod');
  const [connected, setConnected] = useState<boolean>(false);
  const [isUpgradeModalOpen, setIsUpgradeModalOpen] = useState<boolean>(false);

  const selectedRollout = rollouts.find((r) => r.metadata.name === selectedRolloutName) || rollouts[0];

  useEffect(() => {
    // Initial fetch
    fetchRollouts();

    // Setup SSE connection
    const eventSource = new EventSource('/api/stream');

    eventSource.onopen = () => {
      setConnected(true);
    };

    eventSource.onmessage = (event) => {
      try {
        const data: RolloutItem[] = JSON.parse(event.data);
        if (Array.isArray(data) && data.length > 0) {
          setRollouts(data);
        }
      } catch (err) {
        console.error('Failed to parse SSE payload', err);
      }
    };

    eventSource.onerror = () => {
      setConnected(false);
    };

    return () => {
      eventSource.close();
    };
  }, []);

  const fetchRollouts = async () => {
    try {
      const res = await fetch('/api/rollouts');
      if (res.ok) {
        const data = await res.json();
        if (Array.isArray(data) && data.length > 0) {
          setRollouts(data);
          if (!data.some((r) => r.metadata.name === selectedRolloutName)) {
            setSelectedRolloutName(data[0].metadata.name);
          }
        }
      }
    } catch (e) {
      // Backend not running yet or sandbox, fallback retained
    }
  };

  const handleAction = async (action: string, body?: any) => {
    if (!selectedRollout) return;
    const ns = selectedRollout.spec.targetNamespace || selectedRollout.metadata.namespace || 'envoy-gateway-system';
    const res = await fetch(`/api/rollouts/${selectedRollout.metadata.name}/${action}?namespace=${ns}`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: body ? JSON.stringify(body) : undefined,
    });
    if (!res.ok) {
      const txt = await res.text();
      throw new Error(txt || `Failed to execute ${action}`);
    }
    await fetchRollouts();
  };

  return (
    <div className="min-h-screen bg-slate-950 text-slate-100 flex flex-col">
      <Navbar connected={connected} activeRolloutsCount={rollouts.length} />

      <main className="flex-1 max-w-7xl w-full mx-auto p-6">
        {selectedRollout ? (
          <>
            <ControlPanel
              rollout={selectedRollout}
              onPause={() => handleAction('pause')}
              onResume={() => handleAction('resume')}
              onForcePromote={() => handleAction('force-promote')}
              onAbort={() => handleAction('abort')}
              onOpenUpgradeModal={() => setIsUpgradeModalOpen(true)}
            />

            <RolloutHeader rollout={selectedRollout} />

            <NodeTopologyView nodes={selectedRollout.status?.nodes} />

            <MetricCharts
              metricsSnapshot={selectedRollout.status?.lastMetricSnapshot}
              failureLimit={selectedRollout.spec.analysis?.failureLimit}
              consecutiveFailures={selectedRollout.status?.consecutiveFailures}
            />

            <UpgradeModal
              isOpen={isUpgradeModalOpen}
              onClose={() => setIsUpgradeModalOpen(false)}
              currentVersion={selectedRollout.status?.currentVersion || selectedRollout.spec.version}
              onConfirmUpgrade={(ver) => handleAction('upgrade', { targetVersion: ver })}
            />
          </>
        ) : (
          <div className="py-20 text-center">
            <p className="text-slate-400">No Envoy Gateway Rollout resources detected.</p>
          </div>
        )}
      </main>

      <footer className="border-t border-slate-900 py-4 text-center text-xs text-slate-500">
        Envoy Gateway Zero-Downtime Rollout Controller &bull; envoy-gateway-system
      </footer>
    </div>
  );
}
