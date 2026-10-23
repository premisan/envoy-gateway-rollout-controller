import React from 'react';
import { Network, ShieldCheck, Activity, Terminal } from 'lucide-react';

interface Props {
  connected: boolean;
  activeRolloutsCount: number;
}

export const Navbar: React.FC<Props> = ({ connected, activeRolloutsCount }) => {
  return (
    <header className="bg-slate-950/80 border-b border-slate-800 px-6 py-4 flex items-center justify-between sticky top-0 z-50 backdrop-blur-md">
      <div className="flex items-center gap-3">
        <div className="p-2 bg-sky-500/10 border border-sky-500/30 rounded-xl text-sky-400">
          <Network className="w-6 h-6" />
        </div>
        <div>
          <div className="flex items-center gap-2">
            <h1 className="font-bold text-lg text-white tracking-wide">
              Envoy Gateway Rollout Controller
            </h1>
            <span className="text-xs px-2 py-0.5 rounded-full bg-sky-950 border border-sky-700 text-sky-300 font-mono font-bold">
              EGRC
            </span>
          </div>
          <p className="text-xs text-slate-400">
            Node-level Canary Orchestration, Ephemeral Prometheus & Graceful Drain
          </p>
        </div>
      </div>

      <div className="flex items-center gap-4 text-xs">
        <div className="flex items-center gap-1.5 px-3 py-1.5 rounded-lg bg-slate-900 border border-slate-800 text-slate-300">
          <Terminal className="w-3.5 h-3.5 text-slate-400" />
          <span>Namespace: <strong className="text-sky-300 font-mono">envoy-gateway-system</strong></span>
        </div>

        <div className="flex items-center gap-2 px-3 py-1.5 rounded-lg bg-slate-900 border border-slate-800">
          <span className={`w-2 h-2 rounded-full ${connected ? 'bg-emerald-400 animate-pulse' : 'bg-red-400'}`} />
          <span className="text-slate-300">
            {connected ? 'SSE Stream Live' : 'Reconnecting...'}
          </span>
        </div>
      </div>
    </header>
  );
};
