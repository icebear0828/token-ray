import React, { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { CurrencyCode, DailyCostPoint } from "../types/telemetry";
import { formatCost } from "../utils/formatters";

interface DailyCostChartProps {
  data: DailyCostPoint[];
  currency: CurrencyCode;
}

export const DailyCostChart: React.FC<DailyCostChartProps> = ({ data, currency }) => {
  const { t } = useTranslation();
  const [hoveredIdx, setHoveredIdx] = useState<number | null>(null);

  const stats = useMemo(() => {
    const safeData = data ?? [];
    const maxVal = safeData.reduce((m, d) => Math.max(m, d.cost || 0), 0);
    const total = safeData.reduce((sum, d) => sum + (d.cost || 0), 0);
    const avg = safeData.length > 0 ? total / safeData.length : 0;
    const ceiling = maxVal > 0 ? maxVal * 1.15 : 1;
    return { safeData, maxVal, total, avg, ceiling };
  }, [data]);

  const hoveredItem = hoveredIdx !== null ? stats.safeData[hoveredIdx] : null;

  return (
    <div className="flex h-full flex-col justify-between rounded-xl border border-slate-200/90 bg-white p-5 shadow-[0_1px_2px_rgba(15,23,42,0.03)]">
      {/* Header */}
      <div className="mb-4 flex flex-wrap items-start justify-between gap-3">
        <div>
          <div className="text-[11px] font-semibold tracking-wider text-slate-400 uppercase">
            {t("charts.daily_cost_sub")}
          </div>
          <h3 className="mt-0.5 text-sm font-semibold text-slate-900">
            {t("charts.daily_cost_title")}
          </h3>
        </div>

        <div className="flex items-center gap-3">
          <div className="rounded-lg border border-slate-100 bg-slate-50 px-2.5 py-1 text-right">
            <div className="text-[10px] font-medium text-slate-400">{t("charts.daily_avg")}</div>
            <div className="font-mono-num text-xs font-semibold text-slate-700">
              {formatCost(stats.avg, currency)}
            </div>
          </div>
          <div className="rounded-lg border border-slate-100 bg-slate-50 px-2.5 py-1 text-right">
            <div className="text-[10px] font-medium text-slate-400">{t("charts.daily_peak")}</div>
            <div className="font-mono-num text-xs font-semibold text-slate-900">
              {formatCost(stats.maxVal, currency)}
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
                {formatCost(hoveredItem.cost, currency)}
              </span>
              {stats.avg > 0 && (
                <span
                  className={`font-mono-num rounded px-1.5 py-0.5 text-[10px] font-semibold ${
                    hoveredItem.cost >= stats.avg
                      ? "bg-amber-50 text-amber-700"
                      : "bg-emerald-50 text-emerald-700"
                  }`}
                >
                  {hoveredItem.cost >= stats.avg ? "+" : ""}
                  {(((hoveredItem.cost - stats.avg) / stats.avg) * 100).toFixed(0)}%{" "}
                  {t("charts.vs_avg")}
                </span>
              )}
            </div>
          </>
        ) : (
          <span className="text-[11px] text-slate-400">
            {t("charts.daily_total_30d")}:{" "}
            <strong className="font-mono-num font-semibold text-slate-700">
              {formatCost(stats.total, currency)}
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
                {formatCost(stats.ceiling * ratio, currency)}
              </span>
              <div className="h-px flex-1 border-b border-dashed border-slate-200/80" />
            </div>
          ))}
        </div>

        {/* Bars Container */}
        <div className="relative z-10 flex h-full items-end gap-[3px] pl-14 pb-5">
          {stats.safeData.map((point, index) => {
            const heightPct =
              point.cost > 0 && stats.ceiling > 0
                ? Math.min(100, Math.max(2, (point.cost / stats.ceiling) * 100))
                : 0;
            const isHovered = hoveredIdx === index;
            const isToday = index === stats.safeData.length - 1;

            return (
              <div
                key={point.date || index}
                onMouseEnter={() => setHoveredIdx(index)}
                onMouseLeave={() => setHoveredIdx(null)}
                className="group relative flex h-full flex-1 cursor-pointer flex-col items-center justify-end"
              >
                {/* Hover column highlight */}
                {isHovered && (
                  <div className="pointer-events-none absolute inset-y-0 -inset-x-[1px] rounded bg-slate-100/80" />
                )}

                {/* Actual Daily Cost Bar (Playwright selector preserved) */}
                <div
                  data-testid="daily-cost-bar"
                  data-date={point.date}
                  data-cost={point.cost}
                  style={{ height: `${heightPct}%` }}
                  className={`relative w-full rounded-t-[3px] transition-all duration-150 ${
                    isHovered
                      ? "bg-slate-900"
                      : isToday
                        ? "bg-blue-600"
                        : point.cost === 0
                          ? ""
                          : "bg-slate-700/85 hover:bg-slate-900"
                  }`}
                />
              </div>
            );
          })}
        </div>

        {/* Sparse X-Axis Date Labels */}
        <div className="flex justify-between pl-14 text-[10px] text-slate-400">
          {stats.safeData.map((point, idx) => {
            const showLabel =
              idx === 0 ||
              idx === stats.safeData.length - 1 ||
              idx % 6 === 0;
            return (
              <span
                key={point.date || idx}
                className="font-mono-num flex-1 text-center whitespace-nowrap"
              >
                {showLabel ? point.date.slice(5) : ""}
              </span>
            );
          })}
        </div>
      </div>
    </div>
  );
};
