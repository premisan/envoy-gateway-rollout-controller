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
    <div className="bg-white border border-slate-200 rounded-xl p-4 shadow-sm mb-5">
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3">
        <h3 className="font-bold text-slate-900 text-sm">{t('controlPanelTitle')}</h3>

        <div className="flex flex-wrap items-center gap-2">
          {/* Upgrade Button */}
          <button
            onClick={onOpenUpgradeModal}
            disabled={isRolloutActive}
            className="flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-semibold bg-sky-600 hover:bg-sky-500 text-white transition-colors disabled:opacity-40 disabled:cursor-not-allowed shadow-sm"
          >
            <Sparkles className="w-3.5 h-3.5" />
            {t('btnUpgrade')}
          </button>

          {/* Pause / Resume */}
          {isRolloutActive && (
            isPaused ? (
              <button
                onClick={() => handleAction('resume', onResume)}
                disabled={loadingAction !== null}
                className="flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-semibold bg-emerald-600 hover:bg-emerald-500 text-white transition-colors shadow-sm"
              >
                <Play className="w-3.5 h-3.5 fill-current" />
                {t('btnResume')}
              </button>
            ) : (
              <button
                onClick={() => handleAction('pause', onPause)}
                disabled={loadingAction !== null}
                className="flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-semibold bg-amber-500 hover:bg-amber-600 text-white transition-colors shadow-sm"
              >
                <Pause className="w-3.5 h-3.5 fill-current" />
                {t('btnPause')}
              </button>
            )
          )}

          {/* Force Promote */}
          {isRolloutActive && !isPaused && (
            <button
              onClick={() => handleAction('promote', onForcePromote)}
              disabled={loadingAction !== null}
              className="flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-semibold bg-slate-100 hover:bg-slate-200 text-slate-800 border border-slate-300 transition-colors"
            >
              <FastForward className="w-3.5 h-3.5" />
              {t('btnForcePromote')}
            </button>
          )}

          {/* Immediate Rollback */}
          {isRolloutActive && (
            confirmAbort ? (
              <div className="flex items-center gap-1.5 bg-rose-50 p-1 rounded-lg border border-rose-200">
                <span className="text-[11px] text-rose-700 font-medium px-1.5">{t('confirmAbort')}</span>
                <button
                  onClick={() => handleAction('abort', onAbort)}
                  className="px-2.5 py-1 rounded text-xs font-bold bg-rose-600 hover:bg-rose-500 text-white shadow-sm"
                >
                  {t('btnAbort')}
                </button>
                <button
                  onClick={() => setConfirmAbort(false)}
                  className="px-2 py-1 text-xs text-slate-500 hover:text-slate-700"
                >
                  {t('btnCancel')}
                </button>
              </div>
            ) : (
              <button
                onClick={() => setConfirmAbort(true)}
                className="flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-semibold bg-rose-50 hover:bg-rose-100 text-rose-600 border border-rose-200 transition-colors"
              >
                <RotateCcw className="w-3.5 h-3.5" />
                {t('btnAbort')}
              </button>
            )
          )}
        </div>
      </div>
    </div>
  );
};
