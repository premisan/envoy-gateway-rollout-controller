import React, { useEffect, useState } from 'react';
import { X, Sparkles, AlertCircle, PackageCheck, Loader2 } from 'lucide-react';
import { useI18n } from '../i18n';

interface ReleaseInfo {
  version: string;
  helmChart: string;
  prerelease?: boolean;
  publishedAt?: string;
}

interface Props {
  isOpen: boolean;
  onClose: () => void;
  currentVersion: string;
  onConfirmUpgrade: (targetVersion: string) => Promise<void>;
}

const DEFAULT_RELEASES: ReleaseInfo[] = [
  { version: 'v1.9.2', helmChart: 'oci://docker.io/envoyproxy/gateway-helm:v1.9.2' },
  { version: 'v1.9.1', helmChart: 'oci://docker.io/envoyproxy/gateway-helm:v1.9.1' },
  { version: 'v1.9.0', helmChart: 'oci://docker.io/envoyproxy/gateway-helm:v1.9.0' },
  { version: 'v1.8.5', helmChart: 'oci://docker.io/envoyproxy/gateway-helm:v1.8.5' },
  { version: 'v1.8.4', helmChart: 'oci://docker.io/envoyproxy/gateway-helm:v1.8.4' },
  { version: 'v1.8.0', helmChart: 'oci://docker.io/envoyproxy/gateway-helm:v1.8.0' },
  { version: 'v1.7.2', helmChart: 'oci://docker.io/envoyproxy/gateway-helm:v1.7.2' },
  { version: 'v1.7.0', helmChart: 'oci://docker.io/envoyproxy/gateway-helm:v1.7.0' },
  { version: 'v1.6.1', helmChart: 'oci://docker.io/envoyproxy/gateway-helm:v1.6.1' },
];

export const UpgradeModal: React.FC<Props> = ({
  isOpen,
  onClose,
  currentVersion,
  onConfirmUpgrade,
}) => {
  const { t } = useI18n();
  const [releases, setReleases] = useState<ReleaseInfo[]>(DEFAULT_RELEASES);
  const [selectedVersion, setSelectedVersion] = useState<string>('v1.9.2');
  const [loadingReleases, setLoadingReleases] = useState<boolean>(false);
  const [loading, setLoading] = useState<boolean>(false);
  const [error, setError] = useState<string>('');

  useEffect(() => {
    if (isOpen) {
      fetchReleases();
    }
  }, [isOpen]);

  const fetchReleases = async () => {
    setLoadingReleases(true);
    try {
      const res = await fetch('/api/releases');
      if (res.ok) {
        const data: ReleaseInfo[] = await res.json();
        if (Array.isArray(data) && data.length > 0) {
          setReleases(data);
          if (data[0].version !== currentVersion) {
            setSelectedVersion(data[0].version);
          } else if (data.length > 1) {
            setSelectedVersion(data[1].version);
          }
        }
      }
    } catch {
      // Keep defaults
    } finally {
      setLoadingReleases(false);
    }
  };

  if (!isOpen) return null;

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!selectedVersion || selectedVersion === currentVersion) {
      setError('Please select a different target release version.');
      return;
    }

    setLoading(true);
    setError('');
    try {
      await onConfirmUpgrade(selectedVersion);
      onClose();
    } catch (err: any) {
      setError(err?.message || 'Failed to trigger upgrade');
    } finally {
      setLoading(false);
    }
  };

  const selectedRelease = releases.find((r) => r.version === selectedVersion);

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-slate-900/50 backdrop-blur-sm">
      <div className="bg-white border border-slate-200 rounded-xl max-w-md w-full p-5 shadow-xl relative">
        <button
          onClick={onClose}
          className="absolute top-4 right-4 text-slate-400 hover:text-slate-600 p-1 rounded-lg hover:bg-slate-100 transition-colors"
        >
          <X className="w-4 h-4" />
        </button>

        <div className="flex items-center gap-2.5 mb-4">
          <div className="p-2 bg-sky-50 border border-sky-100 rounded-lg text-sky-600">
            <Sparkles className="w-4 h-4" />
          </div>
          <h3 className="font-bold text-slate-900 text-sm">{t('modalTitle')}</h3>
        </div>

        <form onSubmit={handleSubmit} className="space-y-3.5">
          <div>
            <label className="block text-xs font-semibold text-slate-700 mb-1">
              {t('currentVersionLabel')}
            </label>
            <input
              type="text"
              disabled
              value={currentVersion || 'v1.8.5'}
              className="w-full bg-slate-50 border border-slate-200 rounded-lg px-3 py-1.5 text-xs font-mono text-emerald-700 font-bold cursor-not-allowed"
            />
          </div>

          <div>
            <div className="flex items-center justify-between mb-1">
              <label className="block text-xs font-semibold text-slate-700">
                {t('selectVersionLabel')}
              </label>
              {loadingReleases && (
                <span className="text-[11px] text-sky-600 flex items-center gap-1">
                  <Loader2 className="w-3 h-3 animate-spin" /> {t('fetchingReleases')}
                </span>
              )}
            </div>

            <select
              value={selectedVersion}
              onChange={(e) => {
                setSelectedVersion(e.target.value);
                setError('');
              }}
              className="w-full bg-white border border-slate-300 focus:border-sky-500 focus:ring-1 focus:ring-sky-500 rounded-lg px-3 py-1.5 text-xs font-mono text-slate-900 cursor-pointer"
            >
              {releases.map((rel, idx) => (
                <option key={rel.version} value={rel.version}>
                  {rel.version} {idx === 0 ? `(${t('latestBadge')})` : ''} {rel.version === currentVersion ? `(${t('activeVersion')})` : ''}
                </option>
              ))}
            </select>

            {/* Helm OCI Information Box */}
            <div className="mt-2 p-2.5 rounded-lg bg-slate-50 border border-slate-200 text-[11px] text-slate-600 font-mono">
              <div className="flex items-center gap-1 text-sky-700 mb-0.5 font-semibold">
                <PackageCheck className="w-3.5 h-3.5" />
                {t('helmChartPreview')}
              </div>
              <div className="text-slate-800 break-all select-all text-[10px]">
                {selectedRelease?.helmChart || `oci://docker.io/envoyproxy/gateway-helm:${selectedVersion}`}
              </div>
            </div>
          </div>

          {error && (
            <div className="p-2.5 rounded-lg bg-rose-50 border border-rose-200 text-rose-700 text-xs flex items-center gap-1.5">
              <AlertCircle className="w-3.5 h-3.5 flex-shrink-0" />
              <span>{error}</span>
            </div>
          )}

          <div className="flex items-center justify-end gap-2 pt-2 border-t border-slate-100">
            <button
              type="button"
              onClick={onClose}
              className="px-3 py-1.5 text-xs font-semibold text-slate-600 hover:text-slate-900 transition-colors"
            >
              {t('btnCancel')}
            </button>
            <button
              type="submit"
              disabled={loading || selectedVersion === currentVersion}
              className="px-4 py-1.5 text-xs font-semibold rounded-lg bg-sky-600 hover:bg-sky-500 text-white transition-colors disabled:opacity-40 disabled:cursor-not-allowed shadow-sm"
            >
              {loading ? t('btnLaunching') : `${t('btnLaunch')} ${selectedVersion}`}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
};
