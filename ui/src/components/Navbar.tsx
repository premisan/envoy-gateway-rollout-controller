import React from 'react';
import { Network, Languages, Terminal } from 'lucide-react';
import { useI18n } from '../i18n';

interface Props {
  connected: boolean;
  activeRolloutsCount: number;
}

export const Navbar: React.FC<Props> = ({ connected, activeRolloutsCount }) => {
  const { t, language, setLanguage } = useI18n();

  return (
    <header className="bg-slate-950/80 border-b border-slate-800 px-6 py-4 flex items-center justify-between sticky top-0 z-50 backdrop-blur-md">
      <div className="flex items-center gap-3">
        <div className="p-2 bg-sky-500/10 border border-sky-500/30 rounded-xl text-sky-400">
          <Network className="w-6 h-6" />
        </div>
        <div>
          <div className="flex items-center gap-2">
            <h1 className="font-bold text-lg text-white tracking-wide">
              {t('appTitle')}
            </h1>
            <span className="text-xs px-2 py-0.5 rounded-full bg-sky-950 border border-sky-700 text-sky-300 font-mono font-bold">
              EGRC
            </span>
          </div>
          <p className="text-xs text-slate-400">
            {t('appSubtitle')}
          </p>
        </div>
      </div>

      <div className="flex items-center gap-3 text-xs">
        {/* Language Selector */}
        <div className="flex items-center bg-slate-900 border border-slate-800 rounded-lg p-0.5 text-slate-300">
          <Languages className="w-3.5 h-3.5 ml-2 mr-1 text-slate-400" />
          <button
            onClick={() => setLanguage('en')}
            className={`px-2 py-1 rounded text-xs font-semibold transition-colors ${
              language === 'en'
                ? 'bg-sky-600 text-white shadow-sm'
                : 'text-slate-400 hover:text-white'
            }`}
          >
            EN
          </button>
          <button
            onClick={() => setLanguage('ko')}
            className={`px-2 py-1 rounded text-xs font-semibold transition-colors ${
              language === 'ko'
                ? 'bg-sky-600 text-white shadow-sm'
                : 'text-slate-400 hover:text-white'
            }`}
          >
            한국어
          </button>
        </div>

        <div className="flex items-center gap-1.5 px-3 py-1.5 rounded-lg bg-slate-900 border border-slate-800 text-slate-300">
          <Terminal className="w-3.5 h-3.5 text-slate-400" />
          <span>{t('namespace')}: <strong className="text-sky-300 font-mono">envoy-gateway-system</strong></span>
        </div>

        <div className="flex items-center gap-2 px-3 py-1.5 rounded-lg bg-slate-900 border border-slate-800">
          <span className={`w-2 h-2 rounded-full ${connected ? 'bg-emerald-400 animate-pulse' : 'bg-red-400'}`} />
          <span className="text-slate-300">
            {connected ? t('sseLive') : t('sseReconnecting')}
          </span>
        </div>
      </div>
    </header>
  );
};
