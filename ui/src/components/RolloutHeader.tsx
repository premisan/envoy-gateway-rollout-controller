import React from 'react';
import { RolloutItem } from '../types';
import { ArrowRight, Clock, Cpu, Server, CheckCircle2, AlertTriangle, PauseCircle, Flame } from 'lucide-react';

interface Props {
  rollout: RolloutItem;
}

export const RolloutHeader: React.FC<Props> = ({ rollout }) => {
  const status = rollout.status;
  const phase = status?.phase || 'Pending';

  const getPhaseBadge = (p: string) => {
    switch (p) {
      case 'Completed':
        return <span className="flex items-center gap-1.5 px-3 py-1 rounded-full text-xs font-semibold bg-emerald-500/10 text-emerald-400 border border-emerald-500/30"><CheckCircle2 className="w-3.5 h-3.5" /> Completed</span>;
      case 'Observing':
      case 'CanaryRunning':
        return <span className="flex items-center gap-1.5 px-3 py-1 rounded-full text-xs font-semibold bg-sky-500/10 text-sky-400 border border-sky-500/30 animate-pulse"><Flame className="w-3.5 h-3.5" /> {p} (Active)</span>;
      case 'Paused':
        return <span className="flex items-center gap-1.5 px-3 py-1 rounded-full text-xs font-semibold bg-amber-500/10 text-amber-400 border border-amber-500/30"><PauseCircle className="w-3.5 h-3.5" /> Paused</span>;
      case 'Aborted':
      case 'RollingBack':
        return <span className="flex items-center gap-1.5 px-3 py-1 rounded-full text-xs font-semibold bg-rose-500/10 text-rose-400 border border-rose-500/30"><AlertTriangle className="w-3.5 h-3.5" /> {p}</span>;
      default:
        return <span className="flex items-center gap-1.5 px-3 py-1 rounded-full text-xs font-semibold bg-slate-800 text-slate-300 border border-slate-700">{p}</span>;
    }
  };

  return (
    <div className="bg-slate-900 border border-slate-800 rounded-2xl p-6 shadow-xl mb-6">
      <div className="flex flex-col lg:flex-row lg:items-center justify-between gap-4 pb-6 border-b border-slate-800">
        <div>
          <div className="flex items-center gap-3 mb-1">
            <h2 className="text-xl font-bold text-white font-mono">{rollout.metadata.name}</h2>
            {getPhaseBadge(phase)}
          </div>
          <p className="text-xs text-slate-400 flex items-center gap-2">
            Target Gateway: <span className="text-slate-200 font-mono font-medium">{rollout.spec.gateway.name}</span>
            <span>•</span>
            Namespace: <span className="text-slate-200 font-mono font-medium">{rollout.spec.targetNamespace || 'envoy-gateway-system'}</span>
          </p>
        </div>

        {/* Version Transition Box */}
        <div className="flex items-center gap-3 bg-slate-950/80 px-4 py-2.5 rounded-xl border border-slate-800">
          <div className="text-right">
            <div className="text-[10px] text-slate-400 uppercase font-semibold">Active Version</div>
            <div className="font-mono text-sm font-bold text-emerald-400">{status?.currentVersion || 'None'}</div>
          </div>
          <ArrowRight className="w-4 h-4 text-slate-500" />
          <div>
            <div className="text-[10px] text-slate-400 uppercase font-semibold">Target Version</div>
            <div className="font-mono text-sm font-bold text-sky-400">{rollout.spec.version}</div>
          </div>
        </div>
      </div>

      {/* Metric & Progress Summary Cards */}
      <div className="grid grid-cols-1 md:grid-cols-4 gap-4 pt-6">
        <div className="bg-slate-950/50 p-4 rounded-xl border border-slate-800/80">
          <div className="flex items-center justify-between text-xs text-slate-400 mb-1">
            <span>Rollout Step</span>
            <Server className="w-4 h-4 text-sky-400" />
          </div>
          <div className="text-2xl font-bold text-white font-mono">
            {status?.currentStep || 1} <span className="text-sm font-normal text-slate-500">/ {status?.totalSteps || rollout.spec.strategy.steps.length}</span>
          </div>
          <div className="text-[11px] text-slate-400 mt-1">
            {rollout.spec.strategy.steps[(status?.currentStep || 1) - 1]?.nodes || '1'} nodes in this stage
          </div>
        </div>

        <div className="bg-slate-950/50 p-4 rounded-xl border border-slate-800/80">
          <div className="flex items-center justify-between text-xs text-slate-400 mb-1">
            <span>Step Remaining Time</span>
            <Clock className="w-4 h-4 text-amber-400" />
          </div>
          <div className="text-2xl font-bold text-white font-mono">
            {status?.stepTimeRemaining || '0s'}
          </div>
          <div className="text-[11px] text-slate-400 mt-1">
            Observation timer
          </div>
        </div>

        <div className="bg-slate-950/50 p-4 rounded-xl border border-slate-800/80">
          <div className="flex items-center justify-between text-xs text-slate-400 mb-1">
            <span>Ephemeral Prometheus</span>
            <Cpu className="w-4 h-4 text-purple-400" />
          </div>
          <div className="text-sm font-semibold text-white font-mono truncate">
            {status?.ephemeralPrometheusUrl ? (
              <span className="text-emerald-400">Attached (In-Cluster)</span>
            ) : (
              <span className="text-slate-500">Standby</span>
            )}
          </div>
          <div className="text-[11px] text-slate-400 mt-1 truncate font-mono">
            {status?.ephemeralPrometheusUrl || 'Spawns on rollout'}
          </div>
        </div>

        <div className="bg-slate-950/50 p-4 rounded-xl border border-slate-800/80">
          <div className="flex items-center justify-between text-xs text-slate-400 mb-1">
            <span>Consecutive Failures</span>
            <AlertTriangle className={`w-4 h-4 ${status?.consecutiveFailures ? 'text-rose-400' : 'text-slate-500'}`} />
          </div>
          <div className="text-2xl font-bold font-mono">
            <span className={status?.consecutiveFailures ? 'text-rose-400' : 'text-emerald-400'}>
              {status?.consecutiveFailures || 0}
            </span>
            <span className="text-sm font-normal text-slate-500"> / {rollout.spec.analysis.failureLimit || 3}</span>
          </div>
          <div className="text-[11px] text-slate-400 mt-1">
            Auto-rollback trigger limit
          </div>
        </div>
      </div>

      {status?.message && (
        <div className="mt-4 px-4 py-2.5 rounded-lg bg-sky-950/40 border border-sky-800/50 text-xs text-sky-200 flex items-center gap-2">
          <span className="w-1.5 h-1.5 rounded-full bg-sky-400 animate-ping" />
          <span><strong>Status:</strong> {status.message}</span>
        </div>
      )}
    </div>
  );
};
