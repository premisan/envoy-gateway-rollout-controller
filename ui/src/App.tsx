import React, { useEffect, useState } from 'react';
import { Navbar } from './components/Navbar';
import { RolloutHeader } from './components/RolloutHeader';
import { RolloutPipelineView } from './components/RolloutPipelineView';
import { NodeTopologyView } from './components/NodeTopologyView';
import { MetricCharts } from './components/MetricCharts';
import { ControlPanel } from './components/ControlPanel';
import { UpgradeModal } from './components/UpgradeModal';
import { StandbyView } from './components/StandbyView';
import { RolloutItem, EnvoyGatewayStatus } from './types';
import { useI18n } from './i18n';

export function App() {
  const { t } = useI18n();
  const [rollouts, setRollouts] = useState<RolloutItem[]>([]);
  const [selectedRolloutName, setSelectedRolloutName] = useState<string>('eg');
  const [connected, setConnected] = useState<boolean>(false);
  const [isUpgradeModalOpen, setIsUpgradeModalOpen] = useState<boolean>(false);
  const [egStatus, setEgStatus] = useState<EnvoyGatewayStatus | null>(null);

  const selectedRollout = rollouts.find((r) => r.metadata.name === selectedRolloutName) || (rollouts.length > 0 ? rollouts[0] : null);

  useEffect(() => {
    // Initial fetches
    fetchRollouts();
    fetchEgStatus();

    // Setup SSE connection
    const eventSource = new EventSource('/api/stream');

    eventSource.onopen = () => {
      setConnected(true);
    };

    eventSource.onmessage = (event) => {
      try {
        const data: RolloutItem[] = JSON.parse(event.data);
        if (Array.isArray(data)) {
          setRollouts(data);
          if (data.length > 0) {
            setSelectedRolloutName((prev) => {
              if (data.some((r) => r.metadata.name === prev)) return prev;
              return data[0].metadata.name;
            });
          }
        }
      } catch (err) {
        console.error('Failed to parse SSE payload', err);
      }
    };

    eventSource.onerror = () => {
      setConnected(false);
    };

    // Periodically re-check cluster Envoy Gateway status every 15s
    const statusInterval = setInterval(() => {
      fetchEgStatus();
    }, 15000);

    return () => {
      eventSource.close();
      clearInterval(statusInterval);
    };
  }, []);

  const fetchRollouts = async () => {
    try {
      const res = await fetch('/api/rollouts');
      if (res.ok) {
        const data = await res.json();
        if (Array.isArray(data)) {
          setRollouts(data);
          if (data.length > 0) {
            setSelectedRolloutName((prev) => {
              if (data.some((r) => r.metadata.name === prev)) return prev;
              return data[0].metadata.name;
            });
          }
        }
      }
    } catch (e) {
      console.warn('Backend not ready or API unreachable', e);
    }
  };

  const fetchEgStatus = async () => {
    try {
      const res = await fetch('/api/envoy-gateway/status');
      if (res.ok) {
        const data: EnvoyGatewayStatus = await res.json();
        setEgStatus(data);
      }
    } catch (e) {
      console.warn('Failed to fetch Envoy Gateway cluster status', e);
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

  const handleStartUpgrade = async (targetVersion: string, customYaml?: string) => {
    if (selectedRollout && !customYaml) {
      // Existing rollout resource present and no custom YAML override: perform upgrade action
      await handleAction('upgrade', { targetVersion });
    } else {
      // Bootstrap or apply custom YAML
      const ns = egStatus?.namespace || 'envoy-gateway-system';
      const res = await fetch('/api/rollouts/bootstrap', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ targetVersion, namespace: ns, yaml: customYaml }),
      });
      if (!res.ok) {
        const txt = await res.text();
        throw new Error(txt || 'Failed to apply rollout manifest');
      }
      await fetchRollouts();
      await fetchEgStatus();
    }
  };

  const detectedVersion = selectedRollout?.status?.currentVersion || selectedRollout?.spec?.version || egStatus?.version || 'v1.7.4';

  return (
    <div className="min-h-screen bg-slate-50 text-slate-900 flex flex-col">
      <Navbar connected={connected} activeRolloutsCount={rollouts.length} />

      <main className="flex-1 max-w-7xl w-full mx-auto p-5">
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

            <RolloutPipelineView rollout={selectedRollout} />

            <NodeTopologyView nodes={selectedRollout.status?.nodes} />

            <MetricCharts
              metricsSnapshot={selectedRollout.status?.lastMetricSnapshot}
              failureLimit={selectedRollout.spec.analysis?.failureLimit}
              consecutiveFailures={selectedRollout.status?.consecutiveFailures}
              definedMetrics={selectedRollout.spec.analysis?.metrics}
            />
          </>
        ) : (
          <StandbyView
            egStatus={egStatus}
            onOpenUpgradeModal={() => setIsUpgradeModalOpen(true)}
          />
        )}

        <UpgradeModal
          isOpen={isUpgradeModalOpen}
          onClose={() => setIsUpgradeModalOpen(false)}
          currentVersion={detectedVersion}
          onConfirmUpgrade={handleStartUpgrade}
        />
      </main>

      <footer className="border-t border-slate-200 py-3 text-center text-xs text-slate-500 bg-white">
        {t('footerText')}
      </footer>
    </div>
  );
}
