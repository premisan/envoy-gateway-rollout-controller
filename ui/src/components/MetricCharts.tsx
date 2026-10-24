import React from 'react';
import { Activity, AlertTriangle, ShieldCheck } from 'lucide-react';
import { useI18n } from '../i18n';
import { MetricRule } from '../types';

interface Props {
  metricsSnapshot?: Record<string, number>;
  failureLimit?: number;
  consecutiveFailures?: number;
  definedMetrics?: MetricRule[];
}

interface DisplayMetric {
  name: string;
  label: string;
  value: number;
  threshold: number;
  unit: string;
  decimals: number;
  hasThreshold: boolean;
  isBreached: boolean;
}

export const MetricCharts: React.FC<Props> = ({
  metricsSnapshot = {},
  definedMetrics,
}) => {
  const { t } = useI18n();

  // Default metric rules if none defined in rollout spec
  const defaultRules: MetricRule[] = [
    { name: 'http-5xx-rate', threshold: 1.0, query: '' },
    { name: 'http-4xx-rate', threshold: 5.0, query: '' },
    { name: 'connection-errors', threshold: 5.0, query: '' },
    { name: 'p99-latency', threshold: 100.0, query: '' },
  ];

  const activeRules = definedMetrics && definedMetrics.length > 0 ? definedMetrics : defaultRules;

  // Build merged metric display list: defined rules + any extra keys from metricsSnapshot
  const seenKeys = new Set<string>();
  const displayList: DisplayMetric[] = [];

  for (const rule of activeRules) {
    seenKeys.add(rule.name);
    const rawVal = metricsSnapshot[rule.name];
    const val = typeof rawVal === 'number' && !isNaN(rawVal) ? rawVal : 0.0;
    const threshold = rule.threshold ?? 0;
    const hasThreshold = threshold > 0;
    const isBreached = hasThreshold && val > threshold;

    let label = rule.name;
    let unit = '';
    let decimals = 2;

    if (rule.name === 'http-5xx-rate') {
      label = t('metric5xx');
      unit = '%';
      decimals = 2;
    } else if (rule.name === 'http-4xx-rate') {
      label = t('metric4xx');
      unit = '%';
      decimals = 2;
    } else if (rule.name === 'connection-errors') {
      label = t('metricConnError');
      unit = 'errors/min';
      decimals = 0;
    } else if (rule.name === 'p99-latency') {
      label = t('metricLatency');
      unit = 'ms';
      decimals = 1;
    } else {
      // Dynamic heuristics for user-defined custom metrics
      if (rule.name.includes('rate') || rule.name.includes('ratio') || rule.name.includes('percent')) {
        unit = '%';
        decimals = 2;
      } else if (rule.name.includes('latency') || rule.name.includes('time') || rule.name.includes('duration')) {
        unit = 'ms';
        decimals = 1;
      } else if (rule.name.includes('error') || rule.name.includes('count')) {
        unit = 'errors';
        decimals = 0;
      } else if (rule.name.includes('qps') || rule.name.includes('rps')) {
        unit = 'req/s';
        decimals = 1;
      } else {
        decimals = Number.isInteger(val) ? 0 : 2;
      }
    }

    displayList.push({
      name: rule.name,
      label,
      value: val,
      threshold,
      unit,
      decimals,
      hasThreshold,
      isBreached,
    });
  }

  // Include any extra metrics received from metricsSnapshot not in defined rules
  for (const [key, rawVal] of Object.entries(metricsSnapshot)) {
    if (seenKeys.has(key)) continue;
    seenKeys.add(key);
    const val = typeof rawVal === 'number' && !isNaN(rawVal) ? rawVal : 0.0;

    let unit = '';
    let decimals = 2;
    if (key.includes('rate') || key.includes('percent')) {
      unit = '%';
    } else if (key.includes('latency') || key.includes('time')) {
      unit = 'ms';
      decimals = 1;
    } else {
      decimals = Number.isInteger(val) ? 0 : 2;
    }

    displayList.push({
      name: key,
      label: key,
      value: val,
      threshold: 0,
      unit,
      decimals,
      hasThreshold: false,
      isBreached: false,
    });
  }

  return (
    <div className="bg-white border border-slate-200 rounded-xl p-5 shadow-sm mb-5">
      <div className="flex items-center justify-between pb-3.5 mb-4 border-b border-slate-100">
        <h3 className="font-bold text-slate-900 text-sm flex items-center gap-2">
          <Activity className="w-4 h-4 text-emerald-600" />
          {t('metricAnalysisTitle')}
        </h3>
        <div className="flex items-center gap-2 text-xs text-slate-500 font-mono">
          <span>Active Metrics: <strong className="text-slate-800">{displayList.length}</strong></span>
          <span>•</span>
          <span>Interval: <strong className="text-slate-800 font-semibold">15s</strong></span>
        </div>
      </div>

      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
        {displayList.map((metric) => {
          const maxScale = metric.hasThreshold ? metric.threshold * 2 : 100;
          const progressPercent = Math.min(100, Math.max(0, (metric.value / maxScale) * 100));

          return (
            <div
              key={metric.name}
              className={`p-4 rounded-lg border transition-all ${
                metric.isBreached
                  ? 'bg-rose-50/40 border-rose-300 ring-1 ring-rose-200'
                  : 'bg-slate-50 border-slate-200'
              }`}
            >
              <div className="flex items-center justify-between mb-1.5">
                <span className="text-xs font-semibold text-slate-700 truncate max-w-[150px]" title={metric.label}>
                  {metric.label}
                </span>
                {metric.hasThreshold ? (
                  <span
                    className={`text-[11px] font-mono font-bold flex items-center gap-1 ${
                      metric.isBreached ? 'text-rose-600' : 'text-slate-500'
                    }`}
                  >
                    {metric.isBreached ? (
                      <AlertTriangle className="w-3 h-3 text-rose-500 inline" />
                    ) : (
                      <ShieldCheck className="w-3 h-3 text-emerald-500 inline" />
                    )}
                    Limit: {metric.threshold.toFixed(metric.threshold < 1 ? 2 : 0)}{metric.unit === '%' ? '%' : ''}
                  </span>
                ) : (
                  <span className="text-[11px] font-mono text-emerald-600 font-medium">
                    {t('safeZone')}
                  </span>
                )}
              </div>

              <div className="flex items-baseline gap-2 mb-2.5">
                <span
                  className={`text-2xl font-extrabold font-mono ${
                    metric.isBreached
                      ? 'text-rose-600'
                      : metric.value > 0
                      ? 'text-slate-900'
                      : 'text-slate-700'
                  }`}
                >
                  {metric.value.toFixed(metric.decimals)}
                  {metric.unit === '%' ? '%' : ''}
                </span>
                {metric.unit && metric.unit !== '%' && (
                  <span className="text-xs text-slate-400 font-medium">{metric.unit}</span>
                )}
              </div>

              {/* Threshold Progress Bar with 50% Threshold Mark */}
              <div className="relative pt-1">
                <div className="w-full bg-slate-200 rounded-full h-2 overflow-hidden">
                  <div
                    className={`h-2 rounded-full transition-all duration-500 ${
                      metric.isBreached
                        ? 'bg-rose-500'
                        : metric.name.includes('5xx') || metric.name.includes('4xx')
                        ? 'bg-emerald-500'
                        : 'bg-sky-600'
                    }`}
                    style={{ width: `${progressPercent}%` }}
                  />
                </div>

                {metric.hasThreshold && (
                  <div
                    className="absolute top-0 bottom-0 w-0.5 bg-rose-500 shadow-sm"
                    style={{ left: '50%' }}
                    title={`Limit: ${metric.threshold}`}
                  />
                )}
              </div>

              {metric.hasThreshold ? (
                <div className="flex justify-between text-[10px] text-slate-400 font-mono mt-1">
                  <span>0{metric.unit === '%' ? '%' : ''}</span>
                  <span className={metric.isBreached ? 'text-rose-600 font-bold' : 'text-slate-600 font-medium'}>
                    {metric.threshold.toFixed(metric.threshold < 1 ? 2 : 0)}{metric.unit === '%' ? '%' : ''}
                  </span>
                  <span>{(metric.threshold * 2).toFixed(metric.threshold < 1 ? 2 : 0)}{metric.unit === '%' ? '%' : ''}</span>
                </div>
              ) : (
                <div className="flex justify-between text-[10px] text-slate-400 font-mono mt-1">
                  <span>0</span>
                  <span>50</span>
                  <span>100</span>
                </div>
              )}
            </div>
          );
        })}
      </div>
    </div>
  );
};
