import React from 'react';
import { RolloutItem } from '../types';
import { useI18n } from '../i18n';
import { GitCommit, Activity, ArrowDownCircle, FastForward, CheckCircle2, AlertCircle, Clock, Server, ShieldCheck } from 'lucide-react';

interface Props {
  rollout: RolloutItem;
}

export const RolloutPipelineView: React.FC<Props> = ({ rollout }) => {
  const { t } = useI18n();
  const status = rollout.status;
  const phase = status?.phase || 'Pending';
  const currentStep = status?.currentStep || 1;
  const totalSteps = status?.totalSteps || rollout.spec.strategy.steps.length || 3;

  // Determine state of each pipeline stage based on rollout phase and steps
  const getStageState = (stageIndex: number): 'completed' | 'active' | 'pending' | 'aborted' => {
    if (phase === 'Aborted' || phase === 'RollingBack') {
      return stageIndex === 2 ? 'aborted' : 'completed';
    }
    if (phase === 'Completed') {
      return 'completed';
    }

    if (stageIndex === 1) {
      // Step 1: Canary Pod & xDS Sync
      return 'completed';
    }
    if (stageIndex === 2) {
      // Step 2: Metric Guardrail (Observing / CanaryRunning)
      if (phase === 'Observing' || phase === 'CanaryRunning') return 'active';
      if (phase === 'Draining' || phase === 'Completed') return 'completed';
      return 'pending';
    }
    if (stageIndex === 3) {
      // Step 3: Admin API Drain
      if (phase === 'Draining') return 'active';
      if (phase === 'Completed') return 'completed';
      return 'pending';
    }
    if (stageIndex === 4) {
      // Step 4: Progressive Node Cutover
      if (phase === 'Observing' || phase === 'CanaryRunning' || phase === 'Draining') {
        return currentStep > 1 ? 'active' : 'pending';
      }
      if (phase === 'Completed') return 'completed';
      return 'pending';
    }
    if (stageIndex === 5) {
      // Step 5: Completion & Teardown
      if (phase === 'Completed') return 'completed';
      return 'pending';
    }

    return 'pending';
  };

  const stages = [
    {
      idx: 1,
      title: t('step1Title'),
      desc: t('step1Desc'),
      icon: GitCommit,
    },
    {
      idx: 2,
      title: t('step2Title'),
      desc: t('step2Desc'),
      icon: Activity,
    },
    {
      idx: 3,
      title: t('step3Title'),
      desc: t('step3Desc'),
      icon: ArrowDownCircle,
    },
    {
      idx: 4,
      title: t('step4Title'),
      desc: t('step4Desc'),
      icon: FastForward,
    },
    {
      idx: 5,
      title: t('step5Title'),
      desc: t('step5Desc'),
      icon: CheckCircle2,
    },
  ];

  return (
    <div className="bg-slate-900 border border-slate-800 rounded-2xl p-6 shadow-xl mb-6">
      <div className="flex flex-col sm:flex-row sm:items-center justify-between pb-4 mb-6 border-b border-slate-800 gap-2">
        <div>
          <h3 className="font-bold text-white text-base flex items-center gap-2">
            <Server className="w-5 h-5 text-sky-400" />
            {t('pipelineTitle')}
          </h3>
          <p className="text-xs text-slate-400 mt-0.5">
            {t('pipelineSubtitle')}
          </p>
        </div>
        <div className="flex items-center gap-2">
          <span className="text-xs font-mono px-3 py-1 rounded-full bg-slate-950 border border-slate-800 text-sky-300">
            {t('currentStep')}: <strong className="text-white">{currentStep} / {totalSteps}</strong>
          </span>
        </div>
      </div>

      {/* Stepper Timeline Grid */}
      <div className="grid grid-cols-1 md:grid-cols-5 gap-3 mb-6">
        {stages.map((stage) => {
          const state = getStageState(stage.idx);
          const Icon = stage.icon;

          let badgeColor = 'bg-slate-950/60 border-slate-800 text-slate-400';
          let iconColor = 'text-slate-500';
          let statusText = 'Pending';

          if (state === 'completed') {
            badgeColor = 'bg-emerald-950/20 border-emerald-500/40 text-emerald-300';
            iconColor = 'text-emerald-400';
            statusText = 'Done';
          } else if (state === 'active') {
            badgeColor = 'bg-sky-950/40 border-sky-500 text-sky-200 shadow-lg shadow-sky-500/10 ring-1 ring-sky-500/50';
            iconColor = 'text-sky-400 animate-pulse';
            statusText = 'In Progress';
          } else if (state === 'aborted') {
            badgeColor = 'bg-rose-950/30 border-rose-500/50 text-rose-300';
            iconColor = 'text-rose-400';
            statusText = 'Aborted';
          }

          return (
            <div
              key={stage.idx}
              className={`p-4 rounded-xl border transition-all duration-300 flex flex-col justify-between ${badgeColor}`}
            >
              <div>
                <div className="flex items-center justify-between mb-2">
                  <div className={`p-2 rounded-lg bg-slate-900 border border-slate-800 ${iconColor}`}>
                    <Icon className="w-4 h-4" />
                  </div>
                  <span className={`text-[10px] font-mono px-2 py-0.5 rounded font-bold uppercase ${
                    state === 'active' ? 'bg-sky-500/20 text-sky-300 animate-pulse' :
                    state === 'completed' ? 'bg-emerald-500/20 text-emerald-400' :
                    state === 'aborted' ? 'bg-rose-500/20 text-rose-300' : 'bg-slate-800 text-slate-500'
                  }`}>
                    {statusText}
                  </span>
                </div>
                <h4 className="text-xs font-bold text-white mb-1.5 line-clamp-1">{stage.title}</h4>
                <p className="text-[11px] text-slate-400 leading-relaxed">{stage.desc}</p>
              </div>

              <div className="mt-4 pt-2 border-t border-slate-800/60 flex items-center justify-between text-[10px] font-mono text-slate-500">
                <span>Stage 0{stage.idx}</span>
                {state === 'active' && status?.stepTimeRemaining && (
                  <span className="text-amber-400 font-bold flex items-center gap-1">
                    <Clock className="w-3 h-3" /> {status.stepTimeRemaining}
                  </span>
                )}
              </div>
            </div>
          );
        })}
      </div>

      {/* Current Execution Detail Callout */}
      <div className="p-4 rounded-xl bg-slate-950/80 border border-slate-800 flex flex-col md:flex-row md:items-center justify-between gap-3 text-xs">
        <div className="flex items-start gap-3">
          <div className="p-2 rounded-lg bg-sky-500/10 border border-sky-500/30 text-sky-400 mt-0.5">
            <ShieldCheck className="w-4 h-4" />
          </div>
          <div>
            <div className="text-[11px] text-slate-400 uppercase font-semibold">
              {t('currentActionDetails')}
            </div>
            <div className="text-slate-200 font-medium mt-0.5">
              {status?.message || 'Rollout completed successfully. System in stable state.'}
            </div>
          </div>
        </div>

        <div className="flex flex-wrap items-center gap-2 font-mono text-[11px]">
          <span className="px-2.5 py-1 rounded bg-slate-900 border border-slate-800 text-slate-300">
            PromQL Analyzer: <strong className="text-emerald-400">15s Safe</strong>
          </span>
          <span className="px-2.5 py-1 rounded bg-slate-900 border border-slate-800 text-slate-300">
            Drain API: <strong className="text-sky-300">/drain_listeners?inboundonly</strong>
          </span>
        </div>
      </div>
    </div>
  );
};
