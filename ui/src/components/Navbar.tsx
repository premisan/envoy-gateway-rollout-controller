import React from 'react';
import { Network, Languages, Terminal } from 'lucide-react';
import { useI18n } from '../i18n';

interface Props {
  connected: boolean;
  activeRolloutsCount: number;
}

export const Navbar: React.FC<Props> = ({ connected }) => {
  const { t, language, setLanguage } = useI18n();

  return (
    <header className="bg-white border-b border-slate-200 px-6 py-3.5 flex items-center justify-between sticky top-0 z-50 shadow-sm">
      <div className="flex items-center gap-3">
        <div className="p-2 bg-sky-50 border border-sky-100 rounded-lg text-sky-600">
          <Network className="w-5 h-5" />
        </div>
        <div className="flex items-center gap-2">
          <h1 className="font-bold text-base text-slate-900 tracking-tight">
            {t('appTitle')}
          </h1>
          <span className="text-[11px] px-2 py-0.5 rounded-full bg-sky-50 border border-sky-200 text-sky-700 font-mono font-bold">
            EGRC v1.0.9
          </span>
        </div>
      </div>

      <div className="flex items-center gap-3 text-xs">
        {/* Language Selector */}
        <div className="flex items-center bg-slate-100 border border-slate-200 rounded-lg p-0.5 text-slate-700">
          <Languages className="w-3.5 h-3.5 ml-2 mr-1 text-slate-500" />
          <button
            onClick={() => setLanguage('en')}
            className={`px-2 py-0.5 rounded text-xs font-semibold transition-colors ${
              language === 'en'
                ? 'bg-white text-slate-900 shadow-sm'
                : 'text-slate-500 hover:text-slate-900'
            }`}
          >
            EN
          </button>
          <button
            onClick={() => setLanguage('ko')}
            className={`px-2 py-0.5 rounded text-xs font-semibold transition-colors ${
              language === 'ko'
                ? 'bg-white text-slate-900 shadow-sm'
                : 'text-slate-500 hover:text-slate-900'
            }`}
          >
            KO
          </button>
        </div>

        <div className="flex items-center gap-1.5 px-2.5 py-1 rounded-lg bg-slate-100 border border-slate-200 text-slate-700">
          <Terminal className="w-3.5 h-3.5 text-slate-500" />
          <span>{t('namespace')}: <strong className="text-slate-900 font-mono font-medium">envoy-gateway-system</strong></span>
        </div>

        <div className="flex items-center gap-2 px-2.5 py-1 rounded-lg bg-slate-100 border border-slate-200">
          <span className={`w-2 h-2 rounded-full ${connected ? 'bg-emerald-500' : 'bg-rose-500'}`} />
          <span className="text-slate-700 font-medium text-[11px]">
            {connected ? t('sseLive') : t('sseReconnecting')}
          </span>
        </div>
      </div>
    </header>
  );
};
