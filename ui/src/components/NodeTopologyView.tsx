import React from 'react';
import { NodeStatus } from '../types';
import { Server, Activity, ArrowDownCircle, AlertCircle, ShieldCheck } from 'lucide-react';

interface Props {
  nodes?: NodeStatus[];
}

export const NodeTopologyView: React.FC<Props> = ({ nodes = [] }) => {
  const getNodeBorder = (phase: string) => {
    switch (phase) {
      case 'Canary':
        return 'border-sky-500 bg-sky-950/20 shadow-sky-500/10 shadow-lg';
      case 'Draining':
        return 'border-purple-500 bg-purple-950/20 shadow-purple-500/10 shadow-lg';
      case 'Completed':
      case 'Stable':
        return 'border-emerald-500/50 bg-slate-900';
      case 'RolledBack':
        return 'border-rose-500 bg-rose-950/20';
      default:
        return 'border-slate-800 bg-slate-900';
    }
  };

  const getStatusBadge = (phase: string) => {
    switch (phase) {
      case 'Canary':
        return <span className="px-2 py-0.5 rounded text-[10px] font-bold bg-sky-500/20 text-sky-300 border border-sky-500/40 animate-pulse">CANARY SERVING</span>;
      case 'Draining':
        return <span className="px-2 py-0.5 rounded text-[10px] font-bold bg-purple-500/20 text-purple-300 border border-purple-500/40">DRAINING</span>;
      case 'Completed':
        return <span className="px-2 py-0.5 rounded text-[10px] font-bold bg-emerald-500/20 text-emerald-300 border border-emerald-500/40">UPGRADED</span>;
      case 'Stable':
        return <span className="px-2 py-0.5 rounded text-[10px] font-bold bg-emerald-500/10 text-emerald-400 border border-emerald-500/20">STABLE</span>;
      case 'RolledBack':
        return <span className="px-2 py-0.5 rounded text-[10px] font-bold bg-rose-500/20 text-rose-300 border border-rose-500/40">ROLLED BACK</span>;
      default:
        return <span className="px-2 py-0.5 rounded text-[10px] font-bold bg-slate-800 text-slate-400">{phase}</span>;
    }
  };

  return (
    <div className="bg-slate-900 border border-slate-800 rounded-2xl p-6 shadow-xl mb-6">
      <div className="flex items-center justify-between pb-4 mb-6 border-b border-slate-800">
        <div>
          <h3 className="font-bold text-white text-base flex items-center gap-2">
            <Server className="w-5 h-5 text-sky-400" />
            Node Topology & DataPlane Pod Grid
          </h3>
          <p className="text-xs text-slate-400 mt-0.5">
            Real-time status of LB worker nodes running Envoy Proxy DaemonSet
          </p>
        </div>

        {/* Legend */}
        <div className="flex items-center gap-3 text-[11px] text-slate-400">
          <div className="flex items-center gap-1.5"><span className="w-2.5 h-2.5 rounded-full bg-emerald-400" /> Stable</div>
          <div className="flex items-center gap-1.5"><span className="w-2.5 h-2.5 rounded-full bg-sky-400" /> Canary</div>
          <div className="flex items-center gap-1.5"><span className="w-2.5 h-2.5 rounded-full bg-purple-400" /> Draining</div>
          <div className="flex items-center gap-1.5"><span className="w-2.5 h-2.5 rounded-full bg-rose-400" /> Rolled Back</div>
        </div>
      </div>

      {nodes.length === 0 ? (
        <div className="py-12 text-center text-slate-500 text-sm">
          No active worker nodes found. Controller is discovering nodes matching NodeSelector...
        </div>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
          {nodes.map((node) => (
            <div
              key={node.nodeName}
              className={`rounded-xl border p-4 transition-all duration-300 ${getNodeBorder(node.phase)}`}
            >
              <div className="flex items-start justify-between mb-3">
                <div>
                  <div className="font-mono text-sm font-bold text-white flex items-center gap-2">
                    {node.nodeName}
                  </div>
                  <div className="text-[11px] font-mono text-slate-400">{node.nodeIP}</div>
                </div>
                {getStatusBadge(node.phase)}
              </div>

              {/* Version Info */}
              <div className="bg-slate-950/70 rounded-lg p-2.5 mb-3 border border-slate-800/80 text-xs font-mono">
                <div className="flex justify-between items-center mb-1">
                  <span className="text-slate-400">Active Envoy:</span>
                  <span className="text-emerald-400 font-semibold">{node.activeVersion}</span>
                </div>
                {node.canaryVersion && (
                  <div className="flex justify-between items-center text-sky-400">
                    <span>Canary Shadow:</span>
                    <span className="font-semibold">{node.canaryVersion}</span>
                  </div>
                )}
              </div>

              {/* Draining Progress Bar (if draining) */}
              {node.phase === 'Draining' && (
                <div className="mb-3">
                  <div className="flex justify-between text-[11px] text-purple-300 mb-1">
                    <span className="flex items-center gap-1"><ArrowDownCircle className="w-3 h-3" /> Admin API Drain</span>
                    <span>{node.drainProgress}%</span>
                  </div>
                  <div className="w-full bg-slate-950 rounded-full h-1.5 overflow-hidden">
                    <div
                      className="bg-purple-500 h-1.5 rounded-full transition-all duration-500"
                      style={{ width: `${node.drainProgress}%` }}
                    />
                  </div>
                </div>
              )}

              {/* Node Stats */}
              <div className="grid grid-cols-2 gap-2 text-xs">
                <div className="bg-slate-950/40 p-2 rounded border border-slate-800/60">
                  <div className="text-[10px] text-slate-400">Active Sessions</div>
                  <div className="font-mono font-bold text-slate-200 mt-0.5">{node.activeSessions}</div>
                </div>
                <div className="bg-slate-950/40 p-2 rounded border border-slate-800/60">
                  <div className="text-[10px] text-slate-400">5xx Error Count</div>
                  <div className={`font-mono font-bold mt-0.5 ${node.errorCount5xx > 0 ? 'text-rose-400' : 'text-slate-400'}`}>
                    {node.errorCount5xx}
                  </div>
                </div>
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  );
};
