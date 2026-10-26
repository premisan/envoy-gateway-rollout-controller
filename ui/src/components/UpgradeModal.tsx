import React, { useEffect, useState } from 'react';
import {
  X,
  Sparkles,
  AlertCircle,
  PackageCheck,
  Loader2,
  Copy,
  Check,
  FileCode2,
  GitBranch,
  RotateCcw,
  ExternalLink,
  ShieldCheck,
  Layers,
  Server,
} from 'lucide-react';
import { useI18n } from '../i18n';

interface ReleaseInfo {
  version: string;
  helmChart: string;
  prerelease?: boolean;
  publishedAt?: string;
}

interface DiscoveredConfig {
  gatewayName: string;
  envoyProxyName: string;
  nodeSelector?: Record<string, string>;
  tolerations?: any[];
  annotations?: Record<string, string>;
}

interface Props {
  isOpen: boolean;
  onClose: () => void;
  currentVersion: string;
  onConfirmUpgrade: (targetVersion: string, customYaml?: string) => Promise<void>;
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
  const [selectedVersion, setSelectedVersion] = useState<string>('');
  const [loadingReleases, setLoadingReleases] = useState<boolean>(false);
  const [loadingTemplate, setLoadingTemplate] = useState<boolean>(false);
  const [loading, setLoading] = useState<boolean>(false);
  const [error, setError] = useState<string>('');

  // YAML & Customization State
  const [yamlContent, setYamlContent] = useState<string>('');
  const [originalTemplate, setOriginalTemplate] = useState<string>('');
  const [discovered, setDiscovered] = useState<DiscoveredConfig | null>(null);
  const [activeTab, setActiveTab] = useState<'yaml' | 'gitops'>('yaml');
  const [copied, setCopied] = useState<boolean>(false);

  useEffect(() => {
    if (isOpen) {
      fetchReleases();
    }
  }, [isOpen]);

  useEffect(() => {
    if (selectedVersion) {
      fetchTemplate(selectedVersion);
    }
  }, [selectedVersion]);

  const fetchReleases = async () => {
    setLoadingReleases(true);
    try {
      const res = await fetch('/api/releases');
      if (res.ok) {
        const data: ReleaseInfo[] = await res.json();
        if (Array.isArray(data) && data.length > 0) {
          setReleases(data);
          const candidate = data.find((r) => r.version !== currentVersion) || data[0];
          setSelectedVersion(candidate.version);
          return;
        }
      }
    } catch {
      // Fallback
    } finally {
      setLoadingReleases(false);
    }

    const fallbackCandidate = DEFAULT_RELEASES.find((r) => r.version !== currentVersion) || DEFAULT_RELEASES[0];
    setSelectedVersion(fallbackCandidate.version);
  };

  const fetchTemplate = async (ver: string) => {
    setLoadingTemplate(true);
    try {
      const res = await fetch(`/api/rollouts/template?targetVersion=${ver}`);
      if (res.ok) {
        const data = await res.json();
        if (data.yaml) {
          setYamlContent(data.yaml);
          setOriginalTemplate(data.yaml);
        }
        if (data.discovered) {
          setDiscovered(data.discovered);
        }
      }
    } catch {
      // Ignore template fetch error
    } finally {
      setLoadingTemplate(false);
    }
  };

  const handleCopyYaml = async () => {
    try {
      await navigator.clipboard.writeText(yamlContent);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch {
      // Clipboard fallback
    }
  };

  const handleResetTemplate = () => {
    if (originalTemplate) {
      setYamlContent(originalTemplate);
      setError('');
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
      await onConfirmUpgrade(selectedVersion, yamlContent);
      onClose();
    } catch (err: any) {
      setError(err?.message || 'Failed to trigger upgrade');
    } finally {
      setLoading(false);
    }
  };

  const selectedRelease = releases.find((r) => r.version === selectedVersion);
  const nodeSelectorCount = discovered?.nodeSelector ? Object.keys(discovered.nodeSelector).length : 0;
  const tolerationCount = discovered?.tolerations ? discovered.tolerations.length : 0;

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-slate-900/60 backdrop-blur-sm">
      <div className="bg-white border border-slate-200 rounded-2xl max-w-3xl w-full p-6 shadow-2xl relative max-h-[92vh] flex flex-col">
        {/* Close Button */}
        <button
          onClick={onClose}
          className="absolute top-5 right-5 text-slate-400 hover:text-slate-600 p-1.5 rounded-lg hover:bg-slate-100 transition-colors"
        >
          <X className="w-4 h-4" />
        </button>

        {/* Modal Header */}
        <div className="flex items-center gap-3 mb-4 pb-3 border-b border-slate-100">
          <div className="p-2.5 bg-sky-50 border border-sky-100 rounded-xl text-sky-600">
            <Sparkles className="w-5 h-5" />
          </div>
          <div>
            <h3 className="font-bold text-slate-900 text-base">{t('modalTitle')}</h3>
            <p className="text-xs text-slate-500">{t('modalSubtitle')}</p>
          </div>
        </div>

        {/* Version Selection Grid */}
        <div className="grid grid-cols-1 md:grid-cols-2 gap-3.5 mb-4 bg-slate-50/80 p-3.5 rounded-xl border border-slate-200/80">
          <div>
            <label className="block text-xs font-semibold text-slate-700 mb-1">
              {t('currentVersionLabel')}
            </label>
            <div className="flex items-center gap-2 bg-white border border-slate-200 rounded-lg px-3 py-1.5 text-xs font-mono text-emerald-700 font-bold">
              <span className="w-2 h-2 rounded-full bg-emerald-500 animate-pulse" />
              {currentVersion || 'v1.7.4'}
            </div>
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
              className="w-full bg-white border border-slate-300 focus:border-sky-500 focus:ring-1 focus:ring-sky-500 rounded-lg px-3 py-1.5 text-xs font-mono text-slate-900 font-semibold cursor-pointer shadow-sm"
            >
              {releases.map((rel, idx) => (
                <option key={rel.version} value={rel.version}>
                  {rel.version} {idx === 0 ? `(${t('latestBadge')})` : ''} {rel.version === currentVersion ? `(${t('activeVersion')})` : ''}
                </option>
              ))}
            </select>
          </div>

          <div className="md:col-span-2 flex items-center justify-between text-[11px] text-slate-600 bg-white border border-slate-200/70 rounded-lg px-3 py-1.5">
            <div className="flex items-center gap-1.5 text-sky-700 font-medium">
              <PackageCheck className="w-3.5 h-3.5 flex-shrink-0" />
              <span>{t('helmChartPreview')}</span>
              <span className="font-mono text-slate-800 select-all font-normal">
                {selectedRelease?.helmChart || `oci://docker.io/envoyproxy/gateway-helm:${selectedVersion}`}
              </span>
            </div>
          </div>
        </div>

        {/* Auto-Discovered Cluster Infrastructure Badges */}
        {discovered && (
          <div className="mb-3.5 px-3 py-2 bg-emerald-50/70 border border-emerald-200/80 rounded-xl text-xs">
            <div className="flex items-center gap-1.5 text-emerald-800 font-semibold mb-1">
              <ShieldCheck className="w-4 h-4 text-emerald-600" />
              <span>{t('discoveredConfigTitle')}</span>
            </div>
            <div className="flex flex-wrap items-center gap-2 text-[11px] text-slate-700">
              <span className="inline-flex items-center gap-1 bg-white px-2 py-0.5 rounded-md border border-emerald-200 font-mono">
                <Layers className="w-3 h-3 text-emerald-600" />
                {t('detectedGateway')}: <b className="text-slate-900">{discovered.gatewayName}</b>
              </span>
              <span className="inline-flex items-center gap-1 bg-white px-2 py-0.5 rounded-md border border-emerald-200 font-mono">
                <Server className="w-3 h-3 text-emerald-600" />
                {t('detectedEnvoyProxy')}: <b className="text-slate-900">{discovered.envoyProxyName}</b>
              </span>
              {nodeSelectorCount > 0 && (
                <span className="inline-flex items-center gap-1 bg-white px-2 py-0.5 rounded-md border border-emerald-200">
                  {t('detectedNodeSelector')}: <b className="text-emerald-700">{nodeSelectorCount} rules</b>
                </span>
              )}
              {tolerationCount > 0 && (
                <span className="inline-flex items-center gap-1 bg-white px-2 py-0.5 rounded-md border border-emerald-200">
                  {t('detectedTolerations')}: <b className="text-emerald-700">{tolerationCount} rules</b>
                </span>
              )}
            </div>
          </div>
        )}

        {/* Tab Navigation */}
        <div className="flex items-center justify-between border-b border-slate-200 mb-2">
          <div className="flex items-center gap-2">
            <button
              type="button"
              onClick={() => setActiveTab('yaml')}
              className={`flex items-center gap-1.5 px-3 py-2 text-xs font-semibold border-b-2 transition-colors ${
                activeTab === 'yaml'
                  ? 'border-sky-600 text-sky-700'
                  : 'border-transparent text-slate-500 hover:text-slate-800'
              }`}
            >
              <FileCode2 className="w-3.5 h-3.5" />
              {t('tabYamlEdit')}
            </button>
            <button
              type="button"
              onClick={() => setActiveTab('gitops')}
              className={`flex items-center gap-1.5 px-3 py-2 text-xs font-semibold border-b-2 transition-colors ${
                activeTab === 'gitops'
                  ? 'border-sky-600 text-sky-700'
                  : 'border-transparent text-slate-500 hover:text-slate-800'
              }`}
            >
              <GitBranch className="w-3.5 h-3.5" />
              {t('tabGitOps')}
            </button>
          </div>

          <div className="flex items-center gap-2 pb-1">
            {activeTab === 'yaml' && (
              <button
                type="button"
                onClick={handleResetTemplate}
                className="flex items-center gap-1 text-[11px] text-slate-500 hover:text-slate-700 px-2 py-1 rounded hover:bg-slate-100 transition-colors"
                title={t('resetTemplate')}
              >
                <RotateCcw className="w-3 h-3" />
                <span>{t('resetTemplate')}</span>
              </button>
            )}
            <button
              type="button"
              onClick={handleCopyYaml}
              className="flex items-center gap-1 text-[11px] font-semibold text-sky-700 bg-sky-50 hover:bg-sky-100 border border-sky-200 px-2.5 py-1 rounded-lg transition-colors"
            >
              {copied ? (
                <>
                  <Check className="w-3 h-3 text-emerald-600" />
                  <span className="text-emerald-700">{t('copied')}</span>
                </>
              ) : (
                <>
                  <Copy className="w-3 h-3" />
                  <span>{t('copyYaml')}</span>
                </>
              )}
            </button>
          </div>
        </div>

        {/* Tab Content */}
        <div className="flex-1 overflow-y-auto mb-3">
          {activeTab === 'yaml' ? (
            <div className="relative">
              {loadingTemplate ? (
                <div className="h-64 flex flex-col items-center justify-center text-slate-500 text-xs gap-2 bg-slate-900 rounded-xl">
                  <Loader2 className="w-5 h-5 animate-spin text-sky-400" />
                  <span>{t('loadingTemplate')}</span>
                </div>
              ) : (
                <div className="relative">
                  <textarea
                    value={yamlContent}
                    onChange={(e) => {
                      setYamlContent(e.target.value);
                      setError('');
                    }}
                    rows={12}
                    spellCheck={false}
                    className="w-full bg-slate-900 text-emerald-400 font-mono text-xs p-3.5 rounded-xl border border-slate-700 focus:outline-none focus:ring-2 focus:ring-sky-500 leading-relaxed resize-y selection:bg-slate-700"
                  />
                  <div className="text-[10px] text-slate-400 mt-1 flex justify-between items-center">
                    <span>* Lines: {yamlContent.split('\n').length}</span>
                    <span>Direct editing supported</span>
                  </div>
                </div>
              )}
            </div>
          ) : (
            <div className="p-4 bg-slate-50 border border-slate-200 rounded-xl space-y-3 text-xs text-slate-700">
              <div className="flex items-start gap-2 text-sky-900">
                <GitBranch className="w-4 h-4 text-sky-600 flex-shrink-0 mt-0.5" />
                <div>
                  <h4 className="font-bold text-xs">{t('gitOpsGuideTitle')}</h4>
                  <p className="text-[11px] text-slate-600 mt-0.5 leading-relaxed">
                    {t('gitOpsGuideDesc')}
                  </p>
                </div>
              </div>

              <div>
                <label className="block text-[11px] font-semibold text-slate-700 mb-1">
                  kubectl Quick Apply
                </label>
                <div className="bg-slate-900 text-slate-200 p-2.5 rounded-lg font-mono text-[11px] select-all overflow-x-auto">
                  kubectl apply -f - &lt;&lt;EOF{'\n'}
                  {yamlContent}
                  EOF
                </div>
              </div>

              <div className="pt-2 border-t border-slate-200 flex items-center justify-between text-[11px]">
                <a
                  href="https://github.com/premisan/envoy-gateway-rollout-controller"
                  target="_blank"
                  rel="noopener noreferrer"
                  className="text-sky-600 hover:text-sky-800 flex items-center gap-1 font-semibold"
                >
                  <ExternalLink className="w-3.5 h-3.5" />
                  {t('officialDocs')}
                </a>
                <span className="text-slate-500 text-[10px]">
                  Recommended for production GitOps workflows
                </span>
              </div>
            </div>
          )}
        </div>

        {error && (
          <div className="mb-3 p-2.5 rounded-lg bg-rose-50 border border-rose-200 text-rose-700 text-xs flex items-center gap-1.5">
            <AlertCircle className="w-3.5 h-3.5 flex-shrink-0" />
            <span>{error}</span>
          </div>
        )}

        {/* Modal Actions */}
        <div className="flex items-center justify-between pt-3 border-t border-slate-100 mt-auto">
          <div className="text-[11px] text-slate-500">
            Target: <b className="font-mono text-slate-800">{selectedVersion}</b>
          </div>

          <div className="flex items-center gap-2">
            <button
              type="button"
              onClick={onClose}
              className="px-3.5 py-1.5 text-xs font-semibold text-slate-600 hover:text-slate-900 transition-colors"
            >
              {t('btnCancel')}
            </button>
            <button
              type="button"
              onClick={handleSubmit}
              disabled={loading || selectedVersion === currentVersion || !yamlContent}
              className="px-4 py-1.5 text-xs font-semibold rounded-lg bg-sky-600 hover:bg-sky-500 text-white transition-colors disabled:opacity-40 disabled:cursor-not-allowed shadow-sm flex items-center gap-1.5"
            >
              {loading && <Loader2 className="w-3 h-3 animate-spin" />}
              {loading ? t('btnLaunching') : t('applyCustomRollout')}
            </button>
          </div>
        </div>
      </div>
    </div>
  );
};