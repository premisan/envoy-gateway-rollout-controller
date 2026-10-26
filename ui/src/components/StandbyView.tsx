import React from 'react';
import { Server, ArrowUpRight, CheckCircle2, Shield, Radio, Layers, AlertCircle } from 'lucide-react';
import { useI18n } from '../i18n';
import { EnvoyGatewayStatus } from '../types';

interface Props {
  egStatus: EnvoyGatewayStatus | null;
  onOpenUpgradeModal: () => void;
}

export const StandbyView: React.FC<Props> = ({ egStatus, onOpenUpgradeModal }) => {
  const { t } = useI18n();

  const isInstalled = egStatus?.installed ?? false;
  const currentVersion = egStatus?.version || 'Unknown';
  const namespace = egStatus?.namespace || 'envoy-gateway-system';
  const readyReplicas = egStatus?.readyReplicas ?? 0;

  return (
    <div className="space-y-6">
      {/* Top Banner Card */}
      <div className="bg-gradient-to-r from-sky-900 to-indigo-900 rounded-2xl p-6 sm:p-8 text-white shadow-md relative overflow-hidden">
        <div className="absolute right-0 top-0 bottom-0 opacity-10 flex items-center pr-10 pointer-events-none">
          <Server className="w-64 h-64" />
        </div>

        <div className="relative z-10 max-w-3xl">
          <div className="inline-flex items-center gap-2 px-3 py-1 rounded-full bg-white/10 backdrop-blur-md border border-white/20 text-xs font-medium text-sky-200 mb-4">
            <span className="w-2 h-2 rounded-full bg-emerald-400 animate-pulse" />
            {t('standbyTitle')}
          </div>

          <h2 className="text-2xl sm:text-3xl font-extrabold tracking-tight mb-2">
            {t('standbyTitle')}
          </h2>
          <p className="text-sky-100 text-sm sm:text-base leading-relaxed mb-6">
            {t('standbySubtitle')}
          </p>

          <div className="flex flex-wrap items-center gap-3">
            <button
              onClick={onOpenUpgradeModal}
              className="inline-flex items-center gap-2 px-5 py-2.5 rounded-xl bg-white text-indigo-900 font-bold text-sm shadow hover:bg-sky-50 transition-all hover:scale-[1.02] active:scale-[0.98]"
            >
              <ArrowUpRight className="w-4 h-4 text-indigo-600" />
              {t('btnStartUpgrade')}
            </button>
          </div>
        </div>
      </div>

      {/* Cluster Envoy Gateway Detection Status Grid */}
      <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
        {/* Detection Card */}
        <div className="bg-white border border-slate-200 rounded-xl p-5 shadow-sm">
          <div className="flex items-center justify-between mb-3">
            <span className="text-xs font-semibold text-slate-500 uppercase tracking-wider">
              Envoy Gateway
            </span>
            {isInstalled ? (
              <span className="inline-flex items-center gap-1.5 px-2.5 py-0.5 rounded-full text-xs font-semibold bg-emerald-50 text-emerald-700 border border-emerald-200">
                <CheckCircle2 className="w-3.5 h-3.5" />
                {t('egDetected')}
              </span>
            ) : (
              <span className="inline-flex items-center gap-1.5 px-2.5 py-0.5 rounded-full text-xs font-semibold bg-amber-50 text-amber-700 border border-amber-200">
                <AlertCircle className="w-3.5 h-3.5" />
                {t('egNotDetected')}
              </span>
            )}
          </div>
          <div className="text-2xl font-black font-mono text-slate-800 tracking-tight">
            {isInstalled ? currentVersion : 'Not Found'}
          </div>
          <p className="text-xs text-slate-500 mt-1">
            {t('clusterEgVersion')}
          </p>
        </div>

        {/* Namespace & Deployment Card */}
        <div className="bg-white border border-slate-200 rounded-xl p-5 shadow-sm">
          <div className="flex items-center justify-between mb-3">
            <span className="text-xs font-semibold text-slate-500 uppercase tracking-wider">
              {t('namespace')}
            </span>
            <span className="text-xs font-mono font-medium text-slate-400">Namespace</span>
          </div>
          <div className="text-xl font-bold font-mono text-slate-800 truncate">
            {namespace}
          </div>
          <p className="text-xs text-slate-500 mt-1">
            Deployment: <code className="text-slate-700 font-mono font-medium">{egStatus?.deploymentName || 'envoy-gateway'}</code>
          </p>
        </div>

        {/* Replicas & Health Card */}
        <div className="bg-white border border-slate-200 rounded-xl p-5 shadow-sm">
          <div className="flex items-center justify-between mb-3">
            <span className="text-xs font-semibold text-slate-500 uppercase tracking-wider">
              {t('clusterReplicas')}
            </span>
            <span className="inline-flex items-center gap-1 text-xs font-semibold text-emerald-600">
              <span className="w-1.5 h-1.5 rounded-full bg-emerald-500" />
              Healthy
            </span>
          </div>
          <div className="text-2xl font-black font-mono text-slate-800">
            {readyReplicas} / {readyReplicas > 0 ? readyReplicas : 1} Ready
          </div>
          <p className="text-xs text-slate-500 mt-1">
            Control Plane Pods Status
          </p>
        </div>
      </div>

      {/* How Zero-Downtime Rollout Works Section */}
      <div className="bg-white border border-slate-200 rounded-xl p-6 shadow-sm">
        <h3 className="text-sm font-bold text-slate-900 uppercase tracking-wider mb-4 flex items-center gap-2">
          <Shield className="w-4 h-4 text-sky-600" />
          {t('howItWorks')}
        </h3>

        <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
          <div className="p-4 rounded-xl bg-slate-50 border border-slate-100 flex flex-col gap-2">
            <div className="w-8 h-8 rounded-lg bg-sky-100 text-sky-700 flex items-center justify-center font-bold text-sm">
              1
            </div>
            <div className="font-bold text-slate-800 text-sm flex items-center gap-1.5">
              <Layers className="w-4 h-4 text-sky-600" />
              CRD & Helm Sync
            </div>
            <p className="text-xs text-slate-600 leading-relaxed">
              {t('howItWorksStep1')}
            </p>
          </div>

          <div className="p-4 rounded-xl bg-slate-50 border border-slate-100 flex flex-col gap-2">
            <div className="w-8 h-8 rounded-lg bg-indigo-100 text-indigo-700 flex items-center justify-center font-bold text-sm">
              2
            </div>
            <div className="font-bold text-slate-800 text-sm flex items-center gap-1.5">
              <Radio className="w-4 h-4 text-indigo-600" />
              Ephemeral Canary Prometheus
            </div>
            <p className="text-xs text-slate-600 leading-relaxed">
              {t('howItWorksStep2')}
            </p>
          </div>

          <div className="p-4 rounded-xl bg-slate-50 border border-slate-100 flex flex-col gap-2">
            <div className="w-8 h-8 rounded-lg bg-emerald-100 text-emerald-700 flex items-center justify-center font-bold text-sm">
              3
            </div>
            <div className="font-bold text-slate-800 text-sm flex items-center gap-1.5">
              <Shield className="w-4 h-4 text-emerald-600" />
              Node-level Traffic Canary
            </div>
            <p className="text-xs text-slate-600 leading-relaxed">
              {t('howItWorksStep3')}
            </p>
          </div>
        </div>
      </div>
    </div>
  );
};
