import React, { useState } from 'react';
import { X, Sparkles, AlertCircle } from 'lucide-react';

interface Props {
  isOpen: boolean;
  onClose: () => void;
  currentVersion: string;
  onConfirmUpgrade: (targetVersion: string) => Promise<void>;
}

export const UpgradeModal: React.FC<Props> = ({
  isOpen,
  onClose,
  currentVersion,
  onConfirmUpgrade,
}) => {
  const [targetVersion, setTargetVersion] = useState('v1.7.0');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');

  if (!isOpen) return null;

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!targetVersion.trim()) return;

    setLoading(true);
    setError('');
    try {
      await onConfirmUpgrade(targetVersion.trim());
      onClose();
    } catch (err: any) {
      setError(err.message || 'Upgrade request failed');
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-slate-950/80 backdrop-blur-sm">
      <div className="bg-slate-900 border border-slate-800 rounded-2xl w-full max-w-md p-6 shadow-2xl relative">
        <button
          onClick={onClose}
          className="absolute top-4 right-4 text-slate-400 hover:text-white"
        >
          <X className="w-5 h-5" />
        </button>

        <div className="flex items-center gap-3 mb-4">
          <div className="p-2.5 bg-sky-500/10 border border-sky-500/30 rounded-xl text-sky-400">
            <Sparkles className="w-5 h-5" />
          </div>
          <div>
            <h3 className="font-bold text-lg text-white">Start Canary Upgrade</h3>
            <p className="text-xs text-slate-400">Trigger zero-downtime rollout sequence</p>
          </div>
        </div>

        <form onSubmit={handleSubmit}>
          <div className="space-y-4 mb-6">
            <div>
              <label className="block text-xs font-semibold text-slate-300 mb-1">
                Current Active Version
              </label>
              <input
                type="text"
                disabled
                value={currentVersion || 'None'}
                className="w-full bg-slate-950 border border-slate-800 rounded-lg px-3 py-2 text-sm font-mono text-slate-400 cursor-not-allowed"
              />
            </div>

            <div>
              <label className="block text-xs font-semibold text-slate-300 mb-1">
                Target Envoy Gateway Version
              </label>
              <input
                type="text"
                required
                value={targetVersion}
                onChange={(e) => setTargetVersion(e.target.value)}
                placeholder="e.g. v1.7.0"
                className="w-full bg-slate-950 border border-sky-600/50 focus:border-sky-500 focus:outline-none rounded-lg px-3 py-2 text-sm font-mono text-white"
              />
              <span className="text-[11px] text-slate-500 mt-1 block">
                Will execute node-by-node rollout with ephemeral Prometheus metrics.
              </span>
            </div>

            {error && (
              <div className="p-3 rounded-lg bg-rose-500/10 border border-rose-500/30 text-rose-300 text-xs flex items-center gap-2">
                <AlertCircle className="w-4 h-4 flex-shrink-0" />
                <span>{error}</span>
              </div>
            )}
          </div>

          <div className="flex items-center justify-end gap-3">
            <button
              type="button"
              onClick={onClose}
              className="px-4 py-2 text-xs font-semibold text-slate-400 hover:text-white transition-colors"
            >
              Cancel
            </button>
            <button
              type="submit"
              disabled={loading}
              className="px-5 py-2 text-xs font-semibold rounded-lg bg-sky-600 hover:bg-sky-500 text-white transition-colors shadow-lg shadow-sky-600/20 disabled:opacity-50"
            >
              {loading ? 'Starting...' : 'Launch Canary Rollout'}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
};
