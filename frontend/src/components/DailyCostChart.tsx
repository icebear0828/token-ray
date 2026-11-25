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
  loading?: boolean;
}

const SKELETON_BAR_HEIGHTS = [
  38, 52, 30, 64, 45, 72, 40, 58, 33, 49, 67, 36, 55, 42, 60, 31, 50, 70, 44, 57, 35, 62, 48, 54,
];

const axisLabel = (date: string, granularity: BucketGranularity): string => {
  if (granularity === "hourly") return date.slice(11, 16);
  if (granularity === "monthly") return date;
  return date.slice(5);
};

export const DailyCostChart = React.memo(function DailyCostChart({
  data,
  granularity,
  period,
  currency,
  loading = false,
}: DailyCostChartProps) {
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
  const avgPct =
    stats.avg > 0 && stats.ceiling > 0
      ? Math.min(100, Math.max(0, (stats.avg / stats.ceiling) * 100))
      : 0;

  return (
    <div className="flex h-full flex-col justify-between rounded-xl border border-slate-200/90 bg-white p-5 shadow-[0_1px_2px_rgba(15,23,42,0.03)] dark:border-slate-800 dark:bg-slate-900 dark:shadow-none">
      {/* Header */}
      <div className="mb-4 flex flex-wrap items-start justify-between gap-3">
        <div>
          <div className="text-[11px] font-semibold tracking-wider text-slate-400 uppercase dark:text-slate-500">
            {t("charts.trend_sub", { period, granularity: granularityLabel })}
          </div>
          <h3 className="mt-0.5 text-sm font-semibold text-slate-900 dark:text-slate-100">
            {t("charts.trend_title")}
          </h3>
        </div>

        <div className="flex flex-wrap items-center gap-2">
          {/* Sliding Metric Toggle */}
          <div className="relative grid grid-cols-2 rounded-lg border border-slate-200 bg-slate-100/80 p-0.5 text-[11px] font-medium dark:border-slate-700/80 dark:bg-slate-800/80">
            <span
              data-testid="trend-metric-slider"
              aria-hidden="true"
              className={`pointer-events-none absolute top-0.5 bottom-0.5 left-0.5 w-[calc(50%-2px)] rounded-md bg-white shadow-xs transition-transform duration-200 ease-out dark:bg-slate-900 ${
                metric === "cost" ? "translate-x-0" : "translate-x-full"
              }`}
            />
            <button
              type="button"
              onClick={() => setMetric("cost")}
              aria-pressed={metric === "cost"}
              className={`relative z-10 rounded-md px-2.5 py-1 text-center whitespace-nowrap transition-colors ${
                metric === "cost"
                  ? "font-semibold text-slate-900 dark:text-slate-100"
                  : "text-slate-500 hover:text-slate-800 dark:text-slate-400 dark:hover:text-slate-200"
              }`}
            >
              {t("charts.trend_metric_cost")}
            </button>
            <button
              type="button"
              onClick={() => setMetric("tokens")}
              aria-pressed={metric === "tokens"}
              className={`relative z-10 rounded-md px-2.5 py-1 text-center whitespace-nowrap transition-colors ${
                metric === "tokens"
                  ? "font-semibold text-slate-900 dark:text-slate-100"
                  : "text-slate-500 hover:text-slate-800 dark:text-slate-400 dark:hover:text-slate-200"
              }`}
            >
              {t("charts.trend_metric_tokens")}
            </button>
          </div>

          <div className="rounded-lg border border-slate-100 bg-slate-50 px-2.5 py-1 text-right dark:border-slate-800 dark:bg-slate-800/60">
            <div className="text-[10px] font-medium text-slate-400 dark:text-slate-500">
              {t("charts.trend_avg")}
            </div>
            <div className="font-mono-num text-xs font-semibold text-slate-700 dark:text-slate-200">
              {loading ? (
                <span className="inline-block h-3 w-10 animate-pulse rounded bg-slate-200/80 align-middle dark:bg-slate-700" />
              ) : (
                formatValue(stats.avg)
              )}
            </div>
          </div>
          <div className="rounded-lg border border-slate-100 bg-slate-50 px-2.5 py-1 text-right dark:border-slate-800 dark:bg-slate-800/60">
            <div className="text-[10px] font-medium text-slate-400 dark:text-slate-500">
              {t("charts.trend_peak")}
            </div>
            <div className="font-mono-num text-xs font-semibold text-slate-900 dark:text-slate-100">
              {loading ? (
                <span className="inline-block h-3 w-10 animate-pulse rounded bg-slate-200/80 align-middle dark:bg-slate-700" />
              ) : (
                formatValue(stats.maxVal)
              )}
            </div>
          </div>
        </div>
      </div>

      {/* Hover Status Readout Strip */}
      <div
        data-testid="trend-readout"
        className="mb-2 flex h-7 items-center justify-between rounded-md border border-slate-100 bg-slate-50/80 px-2.5 text-xs dark:border-slate-800 dark:bg-slate-800/50"
      >
        {loading ? (
          <>
            <span className="inline-block h-3 w-28 animate-pulse rounded bg-slate-200/80 dark:bg-slate-700" />
            <span className="inline-block h-3 w-16 animate-pulse rounded bg-slate-200/80 dark:bg-slate-700" />
          </>
        ) : hoveredItem ? (
          <>
            <span className="font-mono-num font-medium text-slate-600 dark:text-slate-300">
              {hoveredItem.date}
            </span>
            <div className="flex items-center gap-2">
              <span className="font-mono-num font-semibold text-slate-900 dark:text-slate-100">
                {formatValue(hoveredValue)}
              </span>
              {stats.avg > 0 && (
                <span
                  className={`font-mono-num rounded border px-1.5 py-0.5 text-[10px] font-semibold ${
                    hoveredValue >= stats.avg
                      ? "border-amber-200/80 bg-amber-50 text-amber-700 dark:border-amber-500/30 dark:bg-amber-500/15 dark:text-amber-300"
                      : "border-emerald-200/80 bg-emerald-50 text-emerald-700 dark:border-emerald-500/30 dark:bg-emerald-500/15 dark:text-emerald-300"
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
          <span className="text-[11px] text-slate-400 dark:text-slate-500">
            {t("charts.trend_total")}:{" "}
            <strong className="font-mono-num font-semibold text-slate-700 dark:text-slate-200">
              {formatValue(stats.total)}
            </strong>
          </span>
        )}
      </div>

      {/* Handcrafted Bar Chart Canvas */}
      <div className="relative mt-1 flex h-44 select-none flex-col">
        {/* Plot Box (0%..100% height shared by gridlines, avg reference line, and bars) */}
        <div className="relative flex-1">
          {/* Horizontal Dashed Gridlines */}
          <div className="pointer-events-none absolute inset-0 flex flex-col justify-between">
            {[1, 0.5, 0].map((ratio, idx) => (
              <div key={idx} className="relative flex w-full items-center">
                <span className="font-mono-num w-14 pr-2 text-right text-[10px] text-slate-400 dark:text-slate-500">
                  {loading ? "" : formatValue(stats.ceiling * ratio)}
                </span>
                <div className="h-px flex-1 border-b border-dashed border-slate-200/80 dark:border-slate-800" />
              </div>
            ))}
          </div>

          {/* Average Reference Line */}
          {!loading && stats.avg > 0 && (
            <div
              data-testid="trend-avg-line"
              aria-hidden="true"
              style={{ bottom: `${avgPct}%` }}
              className="pointer-events-none absolute right-0 left-14 z-0 border-t border-dashed border-sky-500/65 dark:border-sky-400/50"
            />
          )}

          {/* Bars Container */}
          {loading ? (
            <div className="relative z-10 flex h-full items-end justify-center gap-[3px] pl-14">
              {SKELETON_BAR_HEIGHTS.map((height, index) => (
                <div
                  key={index}
                  className="flex h-full max-w-[48px] flex-1 flex-col items-center justify-end"
                >
                  <div
                    className="w-full animate-pulse rounded-t-[3px] bg-slate-200/70 dark:bg-slate-800"
                    style={{ height: `${height}%` }}
                  />
                </div>
              ))}
            </div>
          ) : (
            <div
              className="relative z-10 flex h-full items-end justify-center gap-[3px] pl-14"
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
                      <div className="pointer-events-none absolute inset-y-0 -inset-x-[1px] rounded bg-slate-100/80 dark:bg-slate-800/60" />
                    )}

                    {/* Actual bucket bar (Playwright selector & 0-cost 0px height preserved) */}
                    <div
                      data-testid="daily-cost-bar"
                      data-date={point.date}
                      data-cost={point.cost}
                      data-tokens={point.tokens}
                      style={{ height: `${heightPct}%` }}
                      className={`relative w-full rounded-t-[3px] transition-all duration-150 ${
                        value === 0
                          ? ""
                          : isHovered
                            ? "bg-slate-900 dark:bg-slate-100"
                            : isCurrent
                              ? "bg-sky-600 dark:bg-sky-400"
                              : "bg-slate-700/85 hover:bg-slate-900 dark:bg-slate-500/80 dark:hover:bg-slate-200"
                      }`}
                    />
                  </div>
                );
              })}
            </div>
          )}
        </div>

        {/* Sparse X-Axis Labels */}
        <div className="mt-1.5 flex h-4 justify-center gap-[3px] pl-14 text-[10px] text-slate-400 dark:text-slate-500">
          {loading
            ? SKELETON_BAR_HEIGHTS.map((_, idx) => (
                <span key={idx} className="max-w-[48px] flex-1 text-center">
                  <span className="inline-block h-2 w-5 animate-pulse rounded bg-slate-100 dark:bg-slate-800" />
                </span>
              ))
            : stats.safeData.map((point, idx) => {
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
});
