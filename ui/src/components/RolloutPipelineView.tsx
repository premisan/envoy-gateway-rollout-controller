import React from 'react';
import { RolloutItem } from '../types';
import { useI18n } from '../i18n';
import { GitCommit, Activity, ArrowDownCircle, FastForward, CheckCircle2, Clock, Server, ShieldCheck } from 'lucide-react';

interface Props {
  rollout: RolloutItem;
}

export const RolloutPipelineView: React.FC<Props> = ({ rollout }) => {
  const { t } = useI18n();
  const status = rollout.status;
  const phase = status?.phase || 'Pending';
  const currentStep = status?.currentStep || 1;
  const totalSteps = status?.totalSteps || rollout.spec.strategy.steps.length || 3;

  const getStageState = (stageIndex: number): 'completed' | 'active' | 'pending' | 'aborted' => {
    if (phase === 'Aborted' || phase === 'RollingBack') {
      return stageIndex === 2 ? 'aborted' : 'completed';
    }
    if (phase === 'Completed') {
      return 'completed';
    }

    if (stageIndex === 1) return 'completed';
    if (stageIndex === 2) {
      if (phase === 'Observing' || phase === 'CanaryRunning') return 'active';
      if (phase === 'Draining' || phase === 'Completed') return 'completed';
      return 'pending';
    }
    if (stageIndex === 3) {
      if (phase === 'Draining') return 'active';
      if (phase === 'Completed') return 'completed';
      return 'pending';
    }
    if (stageIndex === 4) {
      if (phase === 'Observing' || phase === 'CanaryRunning' || phase === 'Draining') {
        return currentStep > 1 ? 'active' : 'pending';
      }
      if (phase === 'Completed') return 'completed';
      return 'pending';
    }
    if (stageIndex === 5) {
      if (phase === 'Completed') return 'completed';
      return 'pending';
    }

    return 'pending';
  };

  const stages = [
    { idx: 1, title: t('step1Title'), desc: t('step1Desc'), icon: GitCommit },
    { idx: 2, title: t('step2Title'), desc: t('step2Desc'), icon: Activity },
    { idx: 3, title: t('step3Title'), desc: t('step3Desc'), icon: ArrowDownCircle },
    { idx: 4, title: t('step4Title'), desc: t('step4Desc'), icon: FastForward },
    { idx: 5, title: t('step5Title'), desc: t('step5Desc'), icon: CheckCircle2 },
  ];

  return (
    <div className="bg-white border border-slate-200 rounded-xl p-5 shadow-sm mb-5">
      <div className="flex items-center justify-between pb-3.5 mb-4 border-b border-slate-100">
        <h3 className="font-bold text-slate-900 text-sm flex items-center gap-2">
          <Server className="w-4 h-4 text-sky-600" />
          {t('pipelineTitle')}
        </h3>
        <span className="text-xs font-mono px-2 py-0.5 rounded-full bg-slate-100 border border-slate-200 text-slate-700">
          {t('currentStep')}: <strong className="text-slate-900">{currentStep} / {totalSteps}</strong>
        </span>
      </div>

      {/* Stepper Timeline Grid */}
      <div className="grid grid-cols-1 md:grid-cols-5 gap-2.5 mb-4">
        {stages.map((stage) => {
          const state = getStageState(stage.idx);
          const Icon = stage.icon;

          let badgeColor = 'bg-slate-50 border-slate-200 text-slate-600';
          let iconColor = 'text-slate-400 bg-white';
          let statusText = 'Pending';

          if (state === 'completed') {
            badgeColor = 'bg-emerald-50/60 border-emerald-200 text-emerald-800';
            iconColor = 'text-emerald-600 bg-emerald-100/60';
            statusText = 'Done';
          } else if (state === 'active') {
            badgeColor = 'bg-sky-50 border-sky-300 text-sky-900 shadow-sm ring-1 ring-sky-300';
            iconColor = 'text-sky-600 bg-sky-100 animate-pulse';
            statusText = 'Active';
          } else if (state === 'aborted') {
            badgeColor = 'bg-rose-50 border-rose-200 text-rose-800';
            iconColor = 'text-rose-600 bg-rose-100';
            statusText = 'Aborted';
          }

          return (
            <div
              key={stage.idx}
              className={`p-3.5 rounded-lg border transition-all flex flex-col justify-between ${badgeColor}`}
            >
              <div>
                <div className="flex items-center justify-between mb-2">
                  <div className={`p-1.5 rounded-md border border-slate-200/60 ${iconColor}`}>
                    <Icon className="w-3.5 h-3.5" />
                  </div>
                  <span className={`text-[10px] font-mono px-1.5 py-0.5 rounded font-bold uppercase ${
                    state === 'active' ? 'bg-sky-200/60 text-sky-800' :
                    state === 'completed' ? 'bg-emerald-200/60 text-emerald-800' :
                    state === 'aborted' ? 'bg-rose-200/60 text-rose-800' : 'bg-slate-200/60 text-slate-600'
                  }`}>
                    {statusText}
                  </span>
                </div>
                <h4 className="text-xs font-bold text-slate-900 mb-1 line-clamp-1">{stage.title}</h4>
                <p className="text-[11px] text-slate-500 leading-snug">{stage.desc}</p>
              </div>

              <div className="mt-3 pt-2 border-t border-slate-200/60 flex items-center justify-between text-[10px] font-mono text-slate-400">
                <span>0{stage.idx}</span>
                {state === 'active' && status?.stepTimeRemaining && (
                  <span className="text-amber-600 font-bold flex items-center gap-1">
                    <Clock className="w-3 h-3" /> {status.stepTimeRemaining}
                  </span>
                )}
              </div>
            </div>
          );
        })}
      </div>

      {/* Current Execution Detail Callout */}
      <div className="p-3 rounded-lg bg-slate-50 border border-slate-200 flex flex-col md:flex-row md:items-center justify-between gap-2 text-xs">
        <div className="flex items-center gap-2">
          <ShieldCheck className="w-4 h-4 text-sky-600 flex-shrink-0" />
          <span className="text-slate-500 font-medium">{t('currentActionDetails')}:</span>
          <span className="text-slate-900 font-medium">
            {status?.message || 'Stable'}
          </span>
        </div>

        <div className="flex items-center gap-2 font-mono text-[11px] text-slate-500">
          <span className="px-2 py-0.5 rounded bg-white border border-slate-200">
            PromQL: <strong className="text-emerald-600">15s</strong>
          </span>
          <span className="px-2 py-0.5 rounded bg-white border border-slate-200">
            Drain: <strong className="text-sky-600">/drain_listeners</strong>
          </span>
        </div>
      </div>
    </div>
  );
};
