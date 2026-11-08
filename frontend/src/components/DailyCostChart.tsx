import React, { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { BucketGranularity, CurrencyCode, DailyCostPoint } from "../types/telemetry";
import { formatCompactNumber, formatCost } from "../utils/formatters";

type TrendMetric = "cost" | "tokens";

interface DailyCostChartProps {
  data: DailyCostPoint[];
  granularity: BucketGranularity;
  period: string;
  currency: CurrencyCode;
}

const axisLabel = (date: string, granularity: BucketGranularity): string => {
  if (granularity === "hourly") return date.slice(11, 16);
  if (granularity === "monthly") return date;
  return date.slice(5);
};

export const DailyCostChart: React.FC<DailyCostChartProps> = ({
  data,
  granularity,
  period,
  currency,
}) => {
  const { t } = useTranslation();
  const [hoveredIdx, setHoveredIdx] = useState<number | null>(null);
  const [metric, setMetric] = useState<TrendMetric>("cost");

  const stats = useMemo(() => {
    const safeData = data ?? [];
    const valueOf = (point: DailyCostPoint) =>
      metric === "cost" ? point.cost || 0 : point.tokens || 0;
    const maxVal = safeData.reduce((m, point) => Math.max(m, valueOf(point)), 0);
    const total = safeData.reduce((sum, point) => sum + valueOf(point), 0);
    const avg = safeData.length > 0 ? total / safeData.length : 0;
    const ceiling = maxVal > 0 ? maxVal * 1.15 : 1;
    return { safeData, maxVal, total, avg, ceiling, valueOf };
  }, [data, metric]);

  const formatValue = (value: number) =>
    metric === "cost" ? formatCost(value, currency) : formatCompactNumber(value);

  const hoveredItem = hoveredIdx !== null ? stats.safeData[hoveredIdx] : null;
  const hoveredValue = hoveredItem ? stats.valueOf(hoveredItem) : 0;
  const labelStep = Math.max(1, Math.ceil(stats.safeData.length / 6));
  const granularityLabel = t(`charts.granularity_${granularity}`);

  return (
    <div className="flex h-full flex-col justify-between rounded-xl border border-slate-200/90 bg-white p-5 shadow-[0_1px_2px_rgba(15,23,42,0.03)]">
      {/* Header */}
      <div className="mb-4 flex flex-wrap items-start justify-between gap-3">
        <div>
          <div className="text-[11px] font-semibold tracking-wider text-slate-400 uppercase">
            {t("charts.trend_sub", { period, granularity: granularityLabel })}
          </div>
          <h3 className="mt-0.5 text-sm font-semibold text-slate-900">
            {t("charts.trend_title")}
          </h3>
        </div>

        <div className="flex flex-wrap items-center gap-2">
          {/* Metric Toggle */}
          <div className="inline-flex rounded-lg border border-slate-200 bg-slate-50 p-0.5 text-[11px] font-medium">
            <button
              type="button"
              onClick={() => setMetric("cost")}
              className={`rounded-md px-2.5 py-1 transition-all ${
                metric === "cost"
                  ? "bg-white font-semibold text-slate-900 shadow-xs"
                  : "text-slate-500 hover:text-slate-800"
              }`}
            >
              {t("charts.trend_metric_cost")}
            </button>
            <button
              type="button"
              onClick={() => setMetric("tokens")}
              className={`rounded-md px-2.5 py-1 transition-all ${
                metric === "tokens"
                  ? "bg-white font-semibold text-slate-900 shadow-xs"
                  : "text-slate-500 hover:text-slate-800"
              }`}
            >
              {t("charts.trend_metric_tokens")}
            </button>
          </div>

          <div className="rounded-lg border border-slate-100 bg-slate-50 px-2.5 py-1 text-right">
            <div className="text-[10px] font-medium text-slate-400">{t("charts.trend_avg")}</div>
            <div className="font-mono-num text-xs font-semibold text-slate-700">
              {formatValue(stats.avg)}
            </div>
          </div>
          <div className="rounded-lg border border-slate-100 bg-slate-50 px-2.5 py-1 text-right">
            <div className="text-[10px] font-medium text-slate-400">{t("charts.trend_peak")}</div>
            <div className="font-mono-num text-xs font-semibold text-slate-900">
              {formatValue(stats.maxVal)}
            </div>
          </div>
        </div>
      </div>

      {/* Hover Status Readout Strip */}
      <div className="mb-2 flex h-6 items-center justify-between rounded-md bg-slate-50/80 px-2.5 text-xs">
        {hoveredItem ? (
          <>
            <span className="font-mono-num font-medium text-slate-600">{hoveredItem.date}</span>
            <div className="flex items-center gap-2">
              <span className="font-mono-num font-semibold text-slate-900">
                {formatValue(hoveredValue)}
              </span>
              {stats.avg > 0 && (
                <span
                  className={`font-mono-num rounded px-1.5 py-0.5 text-[10px] font-semibold ${
                    hoveredValue >= stats.avg
                      ? "bg-amber-50 text-amber-700"
                      : "bg-emerald-50 text-emerald-700"
                  }`}
                >
                  {hoveredValue >= stats.avg ? "+" : ""}
                  {(((hoveredValue - stats.avg) / stats.avg) * 100).toFixed(0)}%{" "}
                  {t("charts.vs_avg")}
                </span>
              )}
            </div>
          </>
        ) : (
          <span className="text-[11px] text-slate-400">
            {t("charts.trend_total")}:{" "}
            <strong className="font-mono-num font-semibold text-slate-700">
              {formatValue(stats.total)}
            </strong>
          </span>
        )}
      </div>

      {/* Handcrafted Bar Chart Canvas */}
      <div className="relative mt-1 flex h-44 select-none flex-col justify-end">
        {/* Horizontal Dashed Gridlines */}
        <div className="pointer-events-none absolute inset-0 flex flex-col justify-between pb-5">
          {[1, 0.5, 0].map((ratio, idx) => (
            <div key={idx} className="relative flex w-full items-center">
              <span className="font-mono-num w-14 pr-2 text-right text-[10px] text-slate-400">
                {formatValue(stats.ceiling * ratio)}
              </span>
              <div className="h-px flex-1 border-b border-dashed border-slate-200/80" />
            </div>
          ))}
        </div>

        {/* Bars Container */}
        <div
          className="relative z-10 flex h-full items-end justify-center gap-[3px] pl-14 pb-5"
          onMouseLeave={() => setHoveredIdx(null)}
        >
          {stats.safeData.map((point, index) => {
            const value = stats.valueOf(point);
            const heightPct =
              value > 0 && stats.ceiling > 0
                ? Math.min(100, Math.max(2, (value / stats.ceiling) * 100))
                : 0;
            const isHovered = hoveredIdx === index;
            const isCurrent = index === stats.safeData.length - 1;

            return (
              <div
                key={point.date || index}
                onMouseEnter={() => setHoveredIdx(index)}
                onMouseLeave={() => setHoveredIdx(null)}
                className="group relative flex h-full max-w-[48px] flex-1 cursor-pointer flex-col items-center justify-end"
              >
                {/* Hover column highlight */}
                {isHovered && (
                  <div className="pointer-events-none absolute inset-y-0 -inset-x-[1px] rounded bg-slate-100/80" />
                )}

                {/* Actual bucket bar (Playwright selector preserved) */}
                <div
                  data-testid="daily-cost-bar"
                  data-date={point.date}
                  data-cost={point.cost}
                  data-tokens={point.tokens}
                  style={{ height: `${heightPct}%` }}
                  className={`relative w-full rounded-t-[3px] transition-all duration-150 ${
                    isHovered
                      ? "bg-slate-900"
                      : isCurrent
                        ? "bg-blue-600"
                        : value === 0
                          ? ""
                          : "bg-slate-700/85 hover:bg-slate-900"
                  }`}
                />
              </div>
            );
          })}
        </div>

        {/* Sparse X-Axis Labels */}
        <div className="flex justify-center gap-[3px] pl-14 text-[10px] text-slate-400">
          {stats.safeData.map((point, idx) => {
            const showLabel =
              idx === 0 ||
              idx === stats.safeData.length - 1 ||
              idx % labelStep === 0;
            return (
              <span
                key={point.date || idx}
                className="font-mono-num max-w-[48px] flex-1 text-center whitespace-nowrap"
              >
                {showLabel ? axisLabel(point.date, granularity) : ""}
              </span>
            );
          })}
        </div>
      </div>
    </div>
  );
};
