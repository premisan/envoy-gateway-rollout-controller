import React from 'react';
import { RolloutItem } from '../types';
import { ArrowRight, Clock, Cpu, Server, CheckCircle2, AlertTriangle, PauseCircle, Flame } from 'lucide-react';
import { useI18n } from '../i18n';
import { useCountdownTimer } from '../hooks/useCountdownTimer';

interface Props {
  rollout: RolloutItem;
}

export const RolloutHeader: React.FC<Props> = ({ rollout }) => {
  const { t } = useI18n();
  const status = rollout.status;
  const phase = status?.phase || 'Pending';
  const isPaused = phase === 'Paused' || rollout.spec.control?.paused;
  const isObserving = phase === 'Observing' && !isPaused;

  const displayTimeRemaining = useCountdownTimer(status?.stepTimeRemaining, isObserving);

  const getPhaseBadge = (p: string) => {
    switch (p) {
      case 'Completed':
        return <span className="flex items-center gap-1.5 px-2.5 py-0.5 rounded-full text-xs font-semibold bg-emerald-50 text-emerald-700 border border-emerald-200"><CheckCircle2 className="w-3.5 h-3.5" /> {t('statusCompleted')}</span>;
      case 'Observing':
      case 'CanaryRunning':
        return <span className="flex items-center gap-1.5 px-2.5 py-0.5 rounded-full text-xs font-semibold bg-sky-50 text-sky-700 border border-sky-200 animate-pulse"><Flame className="w-3.5 h-3.5" /> {p} ({t('statusActive')})</span>;
      case 'Paused':
        return <span className="flex items-center gap-1.5 px-2.5 py-0.5 rounded-full text-xs font-semibold bg-amber-50 text-amber-700 border border-amber-200"><PauseCircle className="w-3.5 h-3.5" /> {t('statusPaused')}</span>;
      case 'Aborted':
      case 'RollingBack':
        return <span className="flex items-center gap-1.5 px-2.5 py-0.5 rounded-full text-xs font-semibold bg-rose-50 text-rose-700 border border-rose-200"><AlertTriangle className="w-3.5 h-3.5" /> {t('statusDegraded')} ({p})</span>;
      default:
        return <span className="flex items-center gap-1.5 px-2.5 py-0.5 rounded-full text-xs font-semibold bg-slate-100 text-slate-700 border border-slate-200">{p}</span>;
    }
  };

  return (
    <div className="bg-white border border-slate-200 rounded-xl p-5 shadow-sm mb-5">
      <div className="flex flex-col lg:flex-row lg:items-center justify-between gap-4 pb-4 border-b border-slate-100">
        <div>
          <div className="flex items-center gap-2.5 mb-1">
            <h2 className="text-lg font-bold text-slate-900 font-mono">{rollout.metadata.name}</h2>
            {getPhaseBadge(phase)}
          </div>
          <p className="text-xs text-slate-500 flex items-center gap-2">
            {t('targetGateway')}: <span className="text-slate-800 font-mono font-medium">{rollout.spec.gateway.name}</span>
            <span>•</span>
            {t('namespace')}: <span className="text-slate-800 font-mono font-medium">{rollout.spec.targetNamespace || 'envoy-gateway-system'}</span>
          </p>
        </div>

        {/* Version Transition Box */}
        <div className="flex items-center gap-3 bg-slate-50 px-3.5 py-2 rounded-lg border border-slate-200">
          <div className="text-right">
            <div className="text-[10px] text-slate-400 uppercase font-semibold">{t('activeVersion')}</div>
            <div className="font-mono text-sm font-bold text-emerald-600">{status?.currentVersion || 'None'}</div>
          </div>
          <ArrowRight className="w-4 h-4 text-slate-400" />
          <div>
            <div className="text-[10px] text-slate-400 uppercase font-semibold">{t('targetVersion')}</div>
            <div className="font-mono text-sm font-bold text-sky-600">{status?.targetVersion || rollout.spec.version}</div>
          </div>
        </div>
      </div>

      {/* Metric & Progress Summary Cards */}
      <div className="grid grid-cols-1 md:grid-cols-4 gap-3 pt-4">
        <div className="bg-slate-50 p-3.5 rounded-lg border border-slate-200/80">
          <div className="flex items-center justify-between text-xs text-slate-500 mb-1">
            <span>{t('currentStep')}</span>
            <Server className="w-4 h-4 text-sky-600" />
          </div>
          <div className="text-xl font-bold text-slate-900 font-mono">
            {status?.currentStep || 1} <span className="text-xs font-normal text-slate-400">/ {status?.totalSteps || rollout.spec.strategy.steps.length}</span>
          </div>
          <div className="text-[11px] text-slate-500 mt-0.5">
            {rollout.spec.strategy.steps[(status?.currentStep || 1) - 1]?.nodes || '1'} nodes
          </div>
        </div>

        <div className="bg-slate-50 p-3.5 rounded-lg border border-slate-200/80">
          <div className="flex items-center justify-between text-xs text-slate-500 mb-1">
            <span>{t('timeRemaining')}</span>
            <Clock className={`w-4 h-4 ${phase === 'Completed' ? 'text-emerald-500' : 'text-amber-500'}`} />
          </div>
          <div className="text-xl font-bold text-slate-900 font-mono">
            {phase === 'Completed' ? '0s' : displayTimeRemaining}
          </div>
          <div className="text-[11px] text-slate-400 mt-0.5">
            {phase === 'Completed' ? t('statusCompleted') : isObserving ? 'Timer' : 'Standby'}
          </div>
        </div>

        <div className="bg-slate-50 p-3.5 rounded-lg border border-slate-200/80">
          <div className="flex items-center justify-between text-xs text-slate-500 mb-1">
            <span>{t('ephemeralPrometheus')}</span>
            <Cpu className="w-4 h-4 text-purple-600" />
          </div>
          <div className="text-xs font-semibold text-slate-900 font-mono truncate">
            {status?.ephemeralPrometheusUrl ? (
              <span className="text-emerald-600">Attached</span>
            ) : (
              <span className="text-slate-400">Standby</span>
            )}
          </div>
          <div className="text-[11px] text-slate-400 mt-0.5 truncate font-mono">
            {status?.ephemeralPrometheusUrl ? 'Active' : 'On-demand'}
          </div>
        </div>

        <div className="bg-slate-50 p-3.5 rounded-lg border border-slate-200/80">
          <div className="flex items-center justify-between text-xs text-slate-500 mb-1">
            <span>Failures</span>
            <AlertTriangle className={`w-4 h-4 ${status?.consecutiveFailures ? 'text-rose-500' : 'text-slate-400'}`} />
          </div>
          <div className="text-xl font-bold font-mono">
            <span className={status?.consecutiveFailures ? 'text-rose-600' : 'text-emerald-600'}>
              {status?.consecutiveFailures || 0}
            </span>
            <span className="text-xs font-normal text-slate-400"> / {rollout.spec.analysis.failureLimit || 3}</span>
          </div>
          <div className="text-[11px] text-slate-400 mt-0.5">
            Limit
          </div>
        </div>
      </div>

      {status?.message && (
        <div className="mt-3 px-3 py-2 rounded-lg bg-sky-50 border border-sky-200 text-xs text-sky-800 flex items-center gap-2">
          <span className="w-1.5 h-1.5 rounded-full bg-sky-500 animate-ping" />
          <span><strong>Status:</strong> {status.message}</span>
        </div>
      )}
    </div>
  );
};
