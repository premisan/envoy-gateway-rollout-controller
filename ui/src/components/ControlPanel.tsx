import React, { useState } from 'react';
import { Play, Pause, FastForward, RotateCcw, Sparkles } from 'lucide-react';
import { RolloutItem } from '../types';
import { useI18n } from '../i18n';

interface Props {
  rollout: RolloutItem;
  onPause: () => Promise<void>;
  onResume: () => Promise<void>;
  onForcePromote: () => Promise<void>;
  onAbort: () => Promise<void>;
  onOpenUpgradeModal: () => void;
}

export const ControlPanel: React.FC<Props> = ({
  rollout,
  onPause,
  onResume,
  onForcePromote,
  onAbort,
  onOpenUpgradeModal,
}) => {
  const { t } = useI18n();
  const [loadingAction, setLoadingAction] = useState<string | null>(null);
  const [confirmAbort, setConfirmAbort] = useState(false);

  const phase = rollout.status?.phase || 'Pending';
  const isPaused = phase === 'Paused' || rollout.spec.control?.paused;
  const isRolloutActive = phase === 'Observing' || phase === 'CanaryRunning' || phase === 'Deploying' || isPaused;

  const handleAction = async (actionName: string, actionFn: () => Promise<void>) => {
    setLoadingAction(actionName);
    try {
      await actionFn();
    } finally {
      setLoadingAction(null);
    }
  };

  return (
    <div className="bg-slate-900 border border-slate-800 rounded-2xl p-6 shadow-xl mb-6">
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
        <div>
          <h3 className="font-bold text-white text-base">{t('controlPanelTitle')}</h3>
          <p className="text-xs text-slate-400 mt-0.5">
            {t('controlPanelSubtitle')}
          </p>
        </div>

        <div className="flex flex-wrap items-center gap-3">
          {/* Upgrade Button */}
          <button
            onClick={onOpenUpgradeModal}
            disabled={isRolloutActive}
            className="flex items-center gap-2 px-4 py-2.5 rounded-xl text-xs font-semibold bg-sky-600 hover:bg-sky-500 text-white transition-colors shadow-lg shadow-sky-600/20 disabled:opacity-40 disabled:cursor-not-allowed"
          >
            <Sparkles className="w-4 h-4" />
            {t('btnUpgrade')}
          </button>

          {/* Pause / Resume */}
          {isRolloutActive && (
            isPaused ? (
              <button
                onClick={() => handleAction('resume', onResume)}
                disabled={loadingAction !== null}
                className="flex items-center gap-2 px-4 py-2.5 rounded-xl text-xs font-semibold bg-emerald-600 hover:bg-emerald-500 text-white transition-colors shadow-lg shadow-emerald-600/20"
              >
                <Play className="w-4 h-4 fill-current" />
                {t('btnResume')}
              </button>
            ) : (
              <button
                onClick={() => handleAction('pause', onPause)}
                disabled={loadingAction !== null}
                className="flex items-center gap-2 px-4 py-2.5 rounded-xl text-xs font-semibold bg-amber-600 hover:bg-amber-500 text-white transition-colors shadow-lg shadow-amber-600/20"
              >
                <Pause className="w-4 h-4 fill-current" />
                {t('btnPause')}
              </button>
            )
          )}

          {/* Force Promote */}
          {isRolloutActive && !isPaused && (
            <button
              onClick={() => handleAction('promote', onForcePromote)}
              disabled={loadingAction !== null}
              className="flex items-center gap-2 px-4 py-2.5 rounded-xl text-xs font-semibold bg-slate-800 hover:bg-slate-700 text-sky-300 border border-slate-700 transition-colors"
            >
              <FastForward className="w-4 h-4" />
              {t('btnForcePromote')}
            </button>
          )}

          {/* Immediate Rollback */}
          {isRolloutActive && (
            confirmAbort ? (
              <div className="flex items-center gap-2 bg-rose-950/60 p-1 rounded-xl border border-rose-800">
                <span className="text-[11px] text-rose-300 font-semibold px-2">{t('confirmAbort')}</span>
                <button
                  onClick={() => handleAction('abort', onAbort)}
                  className="px-3 py-1.5 rounded-lg text-xs font-bold bg-rose-600 hover:bg-rose-500 text-white shadow-lg shadow-rose-600/30"
                >
                  {t('btnAbort')}
                </button>
                <button
                  onClick={() => setConfirmAbort(false)}
                  className="px-2.5 py-1.5 text-xs text-slate-400 hover:text-white"
                >
                  {t('btnCancel')}
                </button>
              </div>
            ) : (
              <button
                onClick={() => setConfirmAbort(true)}
                className="flex items-center gap-2 px-4 py-2.5 rounded-xl text-xs font-semibold bg-rose-500/10 hover:bg-rose-500/20 text-rose-400 border border-rose-500/30 transition-colors"
              >
                <RotateCcw className="w-4 h-4" />
                {t('btnAbort')}
              </button>
            )
          )}
        </div>
      </div>
    </div>
  );
};
