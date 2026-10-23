import React, { useEffect, useState } from 'react';
import { X, Sparkles, AlertCircle, PackageCheck, Loader2 } from 'lucide-react';

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
          // Default to latest version if different from current
          if (data[0].version !== currentVersion) {
            setSelectedVersion(data[0].version);
          } else if (data.length > 1) {
            setSelectedVersion(data[1].version);
          }
        }
      }
    } catch (e) {
      // Retain fallback list
    } finally {
      setLoadingReleases(false);
    }
  };

  if (!isOpen) return null;

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!selectedVersion.trim()) return;

    if (selectedVersion === currentVersion) {
      setError(`Selected version ${selectedVersion} is already the active version.`);
      return;
    }

    setLoading(true);
    setError('');
    try {
      await onConfirmUpgrade(selectedVersion.trim());
      onClose();
    } catch (err: any) {
      setError(err.message || 'Upgrade request failed');
    } finally {
      setLoading(false);
    }
  };

  const selectedRelease = releases.find((r) => r.version === selectedVersion);

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
            <p className="text-xs text-slate-400">Select an official Envoy Gateway release</p>
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
                value={currentVersion || 'v1.8.5'}
                className="w-full bg-slate-950 border border-slate-800 rounded-lg px-3 py-2 text-sm font-mono text-emerald-400 font-bold cursor-not-allowed"
              />
            </div>

            <div>
              <div className="flex items-center justify-between mb-1">
                <label className="block text-xs font-semibold text-slate-300">
                  Target Envoy Gateway Release
                </label>
                {loadingReleases && (
                  <span className="text-[11px] text-sky-400 flex items-center gap-1">
                    <Loader2 className="w-3 h-3 animate-spin" /> Fetching releases...
                  </span>
                )}
              </div>

              <select
                value={selectedVersion}
                onChange={(e) => {
                  setSelectedVersion(e.target.value);
                  setError('');
                }}
                className="w-full bg-slate-950 border border-sky-600/50 focus:border-sky-500 focus:outline-none rounded-lg px-3 py-2 text-sm font-mono text-white cursor-pointer"
              >
                {releases.map((rel, idx) => (
                  <option key={rel.version} value={rel.version} className="bg-slate-900 py-1">
                    {rel.version} {idx === 0 ? '(Latest Stable)' : ''} {rel.version === currentVersion ? '(Current Active)' : ''}
                  </option>
                ))}
              </select>

              {/* Helm OCI Information Box */}
              <div className="mt-2.5 p-3 rounded-lg bg-slate-950/80 border border-slate-800 text-[11px] text-slate-400 font-mono">
                <div className="flex items-center gap-1.5 text-sky-300 mb-1 font-semibold">
                  <PackageCheck className="w-3.5 h-3.5" />
                  Official Helm OCI Chart:
                </div>
                <div className="text-slate-300 break-all select-all">
                  {selectedRelease?.helmChart || `oci://docker.io/envoyproxy/gateway-helm:${selectedVersion}`}
                </div>
              </div>
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
              disabled={loading || selectedVersion === currentVersion}
              className="px-5 py-2 text-xs font-semibold rounded-lg bg-sky-600 hover:bg-sky-500 text-white transition-colors shadow-lg shadow-sky-600/20 disabled:opacity-40 disabled:cursor-not-allowed"
            >
              {loading ? 'Starting...' : `Launch Canary to ${selectedVersion}`}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
};
