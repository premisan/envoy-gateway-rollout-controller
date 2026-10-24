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
}) => {
  const { t } = useI18n();
  const error5xxRate = metricsSnapshot['http-5xx-rate'] ?? 0.0;
  const error4xxRate = metricsSnapshot['http-4xx-rate'] ?? 0.0;
  const connErrors = metricsSnapshot['connection-errors'] ?? 0.0;
  const latencyP99 = metricsSnapshot['p99-latency'] ?? 14.2;

  const threshold5xx = 1.0; // 1%
  const threshold4xx = 5.0; // 5%
  const thresholdConn = 5.0;

  return (
    <div className="bg-white border border-slate-200 rounded-xl p-5 shadow-sm mb-5">
      <div className="flex items-center justify-between pb-3.5 mb-4 border-b border-slate-100">
        <h3 className="font-bold text-slate-900 text-sm flex items-center gap-2">
          <Activity className="w-4 h-4 text-emerald-600" />
          {t('metricAnalysisTitle')}
        </h3>
        <div className="text-xs text-slate-500 font-mono">
          Interval: <span className="text-slate-800 font-semibold">15s</span>
        </div>
      </div>

      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
        {/* Metric 1: HTTP 5xx Error Rate */}
        <div className="bg-slate-50 p-4 rounded-lg border border-slate-200">
          <div className="flex items-center justify-between mb-1.5">
            <span className="text-xs font-semibold text-slate-700">{t('metric5xx')}</span>
            <span className="text-[11px] font-mono text-rose-600 font-bold">Limit: 1.00%</span>
          </div>

          <div className="flex items-baseline gap-2 mb-2.5">
            <span className={`text-2xl font-extrabold font-mono ${error5xxRate > threshold5xx ? 'text-rose-600' : 'text-emerald-600'}`}>
              {error5xxRate.toFixed(2)}%
            </span>
            <span className="text-xs text-slate-400">Canary</span>
          </div>

          {/* Threshold Progress Bar with Red Line Indicator */}
          <div className="relative pt-1">
            <div className="w-full bg-slate-200 rounded-full h-2 overflow-hidden">
              <div
                className={`h-2 rounded-full transition-all duration-500 ${
                  error5xxRate > threshold5xx ? 'bg-rose-500' : 'bg-emerald-500'
                }`}
                style={{ width: `${Math.min(100, (error5xxRate / 2.0) * 100)}%` }}
              />
            </div>
            {/* 1.0% Threshold Mark (middle at 50%) */}
            <div
              className="absolute top-0 bottom-0 w-0.5 bg-rose-500"
              style={{ left: '50%' }}
              title="1.0% Limit"
            />
          </div>
          <div className="flex justify-between text-[10px] text-slate-400 font-mono mt-1">
            <span>0.0%</span>
            <span className="text-rose-600 font-medium">1.0%</span>
            <span>2.0%</span>
          </div>
        </div>

        {/* Metric 2: HTTP 4xx Error Rate (Host / Route Guardrail) */}
        <div className="bg-slate-50 p-4 rounded-lg border border-slate-200">
          <div className="flex items-center justify-between mb-1.5">
            <span className="text-xs font-semibold text-slate-700">{t('metric4xx')}</span>
            <span className="text-[11px] font-mono text-amber-600 font-bold">Limit: 5.00%</span>
          </div>

          <div className="flex items-baseline gap-2 mb-2.5">
            <span className={`text-2xl font-extrabold font-mono ${error4xxRate > threshold4xx ? 'text-rose-600' : 'text-emerald-600'}`}>
              {error4xxRate.toFixed(2)}%
            </span>
            <span className="text-xs text-slate-400">Canary</span>
          </div>

          {/* Threshold Progress Bar with Amber/Red Line Indicator */}
          <div className="relative pt-1">
            <div className="w-full bg-slate-200 rounded-full h-2 overflow-hidden">
              <div
                className={`h-2 rounded-full transition-all duration-500 ${
                  error4xxRate > threshold4xx ? 'bg-rose-500' : 'bg-emerald-500'
                }`}
                style={{ width: `${Math.min(100, (error4xxRate / 10.0) * 100)}%` }}
              />
            </div>
            {/* 5.0% Threshold Mark (middle at 50%) */}
            <div
              className="absolute top-0 bottom-0 w-0.5 bg-amber-500"
              style={{ left: '50%' }}
              title="5.0% Limit"
            />
          </div>
          <div className="flex justify-between text-[10px] text-slate-400 font-mono mt-1">
            <span>0.0%</span>
            <span className="text-amber-600 font-medium">5.0%</span>
            <span>10.0%</span>
          </div>
        </div>

        {/* Metric 3: Connection Errors */}
        <div className="bg-slate-50 p-4 rounded-lg border border-slate-200">
          <div className="flex items-center justify-between mb-1.5">
            <span className="text-xs font-semibold text-slate-700">{t('metricConnError')}</span>
            <span className="text-[11px] font-mono text-amber-600 font-bold">Limit: 5/min</span>
          </div>

          <div className="flex items-baseline gap-2 mb-2.5">
            <span className={`text-2xl font-extrabold font-mono ${connErrors > thresholdConn ? 'text-rose-600' : 'text-slate-800'}`}>
              {connErrors.toFixed(0)}
            </span>
            <span className="text-xs text-slate-400">errors / min</span>
          </div>

          <div className="w-full bg-slate-200 rounded-full h-2 overflow-hidden mt-1">
            <div
              className="bg-sky-600 h-2 rounded-full transition-all duration-500"
              style={{ width: `${Math.min(100, (connErrors / 10.0) * 100)}%` }}
            />
          </div>
          <div className="flex justify-between text-[10px] text-slate-400 font-mono mt-1">
            <span>0</span>
            <span className="text-amber-600">5</span>
            <span>10+</span>
          </div>
        </div>

        {/* Metric 3: P99 Latency */}
        <div className="bg-slate-50 p-4 rounded-lg border border-slate-200">
          <div className="flex items-center justify-between mb-1.5">
            <span className="text-xs font-semibold text-slate-700">{t('metricLatency')}</span>
            <span className="text-[11px] font-mono text-emerald-600">Normal</span>
          </div>

          <div className="flex items-baseline gap-2 mb-2.5">
            <span className="text-2xl font-extrabold font-mono text-slate-800">
              {latencyP99.toFixed(1)}
            </span>
            <span className="text-xs text-slate-400">ms</span>
          </div>

          <div className="w-full bg-slate-200 rounded-full h-2 overflow-hidden mt-1">
            <div
              className="bg-sky-600 h-2 rounded-full transition-all duration-500"
              style={{ width: `${Math.min(100, (latencyP99 / 100.0) * 100)}%` }}
            />
          </div>
          <div className="flex justify-between text-[10px] text-slate-400 font-mono mt-1">
            <span>0ms</span>
            <span>50ms</span>
            <span>100ms</span>
          </div>
        </div>
      </div>
    </div>
  );
};
