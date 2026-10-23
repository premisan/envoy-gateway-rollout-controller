import React from 'react';
import { NodeStatus } from '../types';
import { Server, ArrowDownCircle } from 'lucide-react';
import { useI18n } from '../i18n';

interface Props {
  nodes?: NodeStatus[];
}

export const NodeTopologyView: React.FC<Props> = ({ nodes = [] }) => {
  const { t } = useI18n();

  const getNodeBorder = (phase: string) => {
    switch (phase) {
      case 'Canary':
        return 'border-sky-300 bg-sky-50/40 shadow-sm';
      case 'Draining':
        return 'border-purple-300 bg-purple-50/40 shadow-sm';
      case 'Completed':
      case 'Stable':
        return 'border-slate-200 bg-white shadow-sm';
      case 'RolledBack':
        return 'border-rose-300 bg-rose-50/40';
      default:
        return 'border-slate-200 bg-slate-50';
    }
  };

  const getStatusBadge = (phase: string) => {
    switch (phase) {
      case 'Canary':
        return <span className="px-2 py-0.5 rounded text-[10px] font-bold bg-sky-100 text-sky-700 border border-sky-200 animate-pulse">CANARY</span>;
      case 'Draining':
        return <span className="px-2 py-0.5 rounded text-[10px] font-bold bg-purple-100 text-purple-700 border border-purple-200">DRAINING</span>;
      case 'Completed':
        return <span className="px-2 py-0.5 rounded text-[10px] font-bold bg-emerald-100 text-emerald-700 border border-emerald-200">DONE</span>;
      case 'Stable':
        return <span className="px-2 py-0.5 rounded text-[10px] font-bold bg-slate-100 text-slate-700 border border-slate-200">STABLE</span>;
      case 'RolledBack':
        return <span className="px-2 py-0.5 rounded text-[10px] font-bold bg-rose-100 text-rose-700 border border-rose-200">ROLLED BACK</span>;
      default:
        return <span className="px-2 py-0.5 rounded text-[10px] font-bold bg-slate-100 text-slate-600">{phase}</span>;
    }
  };

  return (
    <div className="bg-white border border-slate-200 rounded-xl p-5 shadow-sm mb-5">
      <div className="flex items-center justify-between pb-3.5 mb-4 border-b border-slate-100">
        <h3 className="font-bold text-slate-900 text-sm flex items-center gap-2">
          <Server className="w-4 h-4 text-sky-600" />
          {t('nodeTopologyTitle')}
        </h3>

        {/* Legend */}
        <div className="flex items-center gap-3 text-[11px] text-slate-500">
          <div className="flex items-center gap-1.5"><span className="w-2 h-2 rounded-full bg-emerald-500" /> {t('nodeFilterStable')}</div>
          <div className="flex items-center gap-1.5"><span className="w-2 h-2 rounded-full bg-sky-500" /> {t('nodeFilterCanary')}</div>
          <div className="flex items-center gap-1.5"><span className="w-2 h-2 rounded-full bg-purple-500" /> {t('nodeFilterDraining')}</div>
        </div>
      </div>

      {nodes.length === 0 ? (
        <div className="py-8 text-center text-slate-400 text-xs">
          No worker nodes found.
        </div>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-3">
          {nodes.map((node) => (
            <div
              key={node.nodeName}
              className={`rounded-lg border p-3.5 transition-all ${getNodeBorder(node.phase)}`}
            >
              <div className="flex items-start justify-between mb-2.5">
                <div>
                  <div className="font-mono text-xs font-bold text-slate-900">
                    {node.nodeName}
                  </div>
                  <div className="text-[11px] font-mono text-slate-400">{node.nodeIP}</div>
                </div>
                {getStatusBadge(node.phase)}
              </div>

              {/* Version Info */}
              <div className="bg-slate-50 rounded-lg p-2 mb-2.5 border border-slate-200/80 text-xs font-mono">
                <div className="flex justify-between items-center text-[11px]">
                  <span className="text-slate-500">{t('activeVersion')}:</span>
                  <span className="text-emerald-700 font-semibold">{node.activeVersion}</span>
                </div>
                {node.canaryVersion && (
                  <div className="flex justify-between items-center text-sky-700 text-[11px] mt-0.5">
                    <span>Canary:</span>
                    <span className="font-semibold">{node.canaryVersion}</span>
                  </div>
                )}
              </div>

              {/* Draining Progress Bar (if draining) */}
              {node.phase === 'Draining' && (
                <div className="mb-2.5">
                  <div className="flex justify-between text-[10px] text-purple-700 mb-1">
                    <span className="flex items-center gap-1"><ArrowDownCircle className="w-3 h-3" /> Drain</span>
                    <span>{node.drainProgress}%</span>
                  </div>
                  <div className="w-full bg-slate-200 rounded-full h-1.5 overflow-hidden">
                    <div
                      className="bg-purple-600 h-1.5 rounded-full transition-all duration-500"
                      style={{ width: `${node.drainProgress}%` }}
                    />
                  </div>
                </div>
              )}

              {/* Node Stats */}
              <div className="grid grid-cols-2 gap-2 text-xs">
                <div className="bg-slate-50 p-2 rounded border border-slate-200">
                  <div className="text-[10px] text-slate-400">{t('activeSessions')}</div>
                  <div className="font-mono font-bold text-slate-800 mt-0.5">{node.activeSessions}</div>
                </div>
                <div className="bg-slate-50 p-2 rounded border border-slate-200">
                  <div className="text-[10px] text-slate-400">5xx Errors</div>
                  <div className={`font-mono font-bold mt-0.5 ${node.errorCount5xx > 0 ? 'text-rose-600' : 'text-slate-600'}`}>
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
