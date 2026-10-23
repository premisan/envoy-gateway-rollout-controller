import React from 'react';
import { Activity } from 'lucide-react';
import { useI18n } from '../i18n';

interface Props {
  metricsSnapshot?: Record<string, number>;
  failureLimit?: number;
  consecutiveFailures?: number;
}

export const MetricCharts: React.FC<Props> = ({
  metricsSnapshot = {},
  failureLimit = 3,
  consecutiveFailures = 0,
}) => {
  const { t } = useI18n();
  const error5xxRate = metricsSnapshot['http-5xx-rate'] ?? 0.08;
  const connErrors = metricsSnapshot['connection-errors'] ?? 0.0;
  const latencyP99 = metricsSnapshot['p99-latency'] ?? 14.2;

  const threshold5xx = 1.0; // 1%
  const thresholdConn = 5.0;

  return (
    <div className="bg-slate-900 border border-slate-800 rounded-2xl p-6 shadow-xl mb-6">
      <div className="flex items-center justify-between pb-4 mb-6 border-b border-slate-800">
        <div>
          <h3 className="font-bold text-white text-base flex items-center gap-2">
            <Activity className="w-5 h-5 text-emerald-400" />
            {t('metricAnalysisTitle')}
          </h3>
          <p className="text-xs text-slate-400 mt-0.5">
            {t('metricAnalysisSubtitle')}
          </p>
        </div>
        <div className="text-xs text-slate-400 font-mono">
          Evaluation Interval: <span className="text-sky-300">15s</span>
        </div>
      </div>

      <div className="grid grid-cols-1 md:grid-cols-3 gap-6">
        {/* Metric 1: HTTP 5xx Error Rate */}
        <div className="bg-slate-950/60 p-5 rounded-xl border border-slate-800/80">
          <div className="flex items-center justify-between mb-2">
            <span className="text-xs font-semibold text-slate-300">{t('metric5xx')}</span>
            <span className="text-[11px] font-mono text-rose-400 font-bold">Limit: 1.00%</span>
          </div>

          <div className="flex items-baseline gap-2 mb-3">
            <span className={`text-3xl font-extrabold font-mono ${error5xxRate > threshold5xx ? 'text-rose-400' : 'text-emerald-400'}`}>
              {error5xxRate.toFixed(2)}%
            </span>
            <span className="text-xs text-slate-500">Canary Nodes</span>
          </div>

          {/* Threshold Progress Bar with Red Line Indicator */}
          <div className="relative pt-2">
            <div className="w-full bg-slate-900 rounded-full h-2.5 overflow-hidden border border-slate-800">
              <div
                className={`h-2.5 rounded-full transition-all duration-500 ${
                  error5xxRate > threshold5xx ? 'bg-rose-500' : 'bg-emerald-500'
                }`}
                style={{ width: `${Math.min(100, (error5xxRate / 2.0) * 100)}%` }}
              />
            </div>
            {/* 1.0% Threshold Mark (middle at 50%) */}
            <div
              className="absolute top-1 bottom-0 w-0.5 bg-rose-500/80 border-r border-rose-400"
              style={{ left: '50%' }}
              title="1.0% Threshold"
            />
          </div>
          <div className="flex justify-between text-[10px] text-slate-500 font-mono mt-1.5">
            <span>0.0%</span>
            <span className="text-rose-400 font-semibold">1.0% (Limit)</span>
            <span>2.0%</span>
          </div>
        </div>

        {/* Metric 2: Connection Errors */}
        <div className="bg-slate-950/60 p-5 rounded-xl border border-slate-800/80">
          <div className="flex items-center justify-between mb-2">
            <span className="text-xs font-semibold text-slate-300">{t('metricConnError')}</span>
            <span className="text-[11px] font-mono text-amber-400 font-bold">Limit: 5/min</span>
          </div>

          <div className="flex items-baseline gap-2 mb-3">
            <span className={`text-3xl font-extrabold font-mono ${connErrors > thresholdConn ? 'text-rose-400' : 'text-slate-200'}`}>
              {connErrors.toFixed(0)}
            </span>
            <span className="text-xs text-slate-500">errors / min</span>
          </div>

          <div className="w-full bg-slate-900 rounded-full h-2.5 overflow-hidden border border-slate-800 mt-2">
            <div
              className="bg-sky-500 h-2.5 rounded-full transition-all duration-500"
              style={{ width: `${Math.min(100, (connErrors / 10.0) * 100)}%` }}
            />
          </div>
          <div className="flex justify-between text-[10px] text-slate-500 font-mono mt-1.5">
            <span>0</span>
            <span className="text-amber-400">5 (Limit)</span>
            <span>10+</span>
          </div>
        </div>

        {/* Metric 3: P99 Latency */}
        <div className="bg-slate-950/60 p-5 rounded-xl border border-slate-800/80">
          <div className="flex items-center justify-between mb-2">
            <span className="text-xs font-semibold text-slate-300">{t('metricLatency')}</span>
            <span className="text-[11px] font-mono text-emerald-400">Normal</span>
          </div>

          <div className="flex items-baseline gap-2 mb-3">
            <span className="text-3xl font-extrabold font-mono text-sky-400">
              {latencyP99.toFixed(1)}
            </span>
            <span className="text-xs text-slate-500">milliseconds</span>
          </div>

          <div className="w-full bg-slate-900 rounded-full h-2.5 overflow-hidden border border-slate-800 mt-2">
            <div
              className="bg-sky-500/70 h-2.5 rounded-full transition-all duration-500"
              style={{ width: `${Math.min(100, (latencyP99 / 100.0) * 100)}%` }}
            />
          </div>
          <div className="flex justify-between text-[10px] text-slate-500 font-mono mt-1.5">
            <span>0ms</span>
            <span>50ms</span>
            <span>100ms</span>
          </div>
        </div>
      </div>
    </div>
  );
};
