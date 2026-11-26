import React, { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { CurrencyCode, SourceStats, ToolSharePoint } from "../types/telemetry";
import {
  formatCompactNumber,
  formatCost,
  getSourceTheme,
} from "../utils/formatters";
import { SourceIcon } from "./SourceBadge";

interface SourceShareChartProps {
  toolShare: ToolSharePoint[];
  sources: SourceStats[];
  activeSource: string;
  onSelectSource: (source: string) => void;
  currency: CurrencyCode;
}

export const SourceShareChart = React.memo(function SourceShareChart({
  toolShare,
  sources,
  activeSource,
  onSelectSource,
  currency,
}: SourceShareChartProps) {
  const { t } = useTranslation();
  const [metric, setMetric] = useState<"cost" | "tokens">("cost");
  const [hoveredSource, setHoveredSource] = useState<string | null>(null);

  const items = useMemo(() => {
    const baseList =
      toolShare && toolShare.length > 0
        ? toolShare
        : sources.map((s) => ({
            source: s.name,
            tokens: s.total_input + s.total_output,
            total_cost: s.total_cost,
            events: s.total_events,
          }));

    const total = baseList.reduce(
      (sum, item) => sum + (metric === "cost" ? item.total_cost : item.tokens),
      0
    );

    return baseList
      .map((item) => {
        const val = metric === "cost" ? item.total_cost : item.tokens;
        const pct = total > 0 ? (val / total) * 100 : 0;
        const srcMeta = sources.find((s) => s.name === item.source);
        return {
          ...item,
          val,
          pct,
          hitRate: srcMeta?.cache_hit_rate ?? 0,
          theme: getSourceTheme(item.source),
        };
      })
      .sort((a, b) => b.val - a.val);
  }, [toolShare, sources, metric]);

  const totalValue = useMemo(
    () => items.reduce((acc, i) => acc + i.val, 0),
    [items]
  );

  // SVG Donut geometry
  const radius = 52;
  const strokeWidth = 14;
  const circumference = 2 * Math.PI * radius;

  const segments = useMemo(() => {
    let accumulatedPct = 0;
    return items.map((item) => {
      const gapPct = items.length > 1 && item.pct > 2 ? 1.2 : 0;
      const drawPct = Math.max(0, item.pct - gapPct);
      const dashLength = (drawPct / 100) * circumference;
      const dashOffset = -((accumulatedPct + gapPct / 2) / 100) * circumference;
      accumulatedPct += item.pct;
      return {
        ...item,
        dashLength,
        dashOffset,
      };
    });
  }, [items, circumference]);

  const focusedItem = useMemo(
    () => items.find((i) => i.source === (hoveredSource || activeSource)) ?? null,
    [items, hoveredSource, activeSource]
  );

  return (
    <div className="flex h-full flex-col justify-between rounded-xl border border-slate-200/90 bg-white p-5 shadow-[0_1px_2px_rgba(15,23,42,0.03)] dark:border-slate-800 dark:bg-slate-900 dark:shadow-none">
      {/* Header */}
      <div className="mb-4 flex items-center justify-between gap-2">
        <div>
          <div className="text-[11px] font-semibold tracking-wider text-slate-400 uppercase dark:text-slate-500">
            {t("charts.source_share_sub")}
          </div>
          <h3 className="mt-0.5 text-sm font-semibold text-slate-900 dark:text-slate-100">
            {t("charts.source_share_title")}
          </h3>
        </div>

        {/* Sliding Segmented Metric Toggle */}
        <div className="relative grid grid-cols-2 rounded-lg border border-slate-200 bg-slate-100/80 p-0.5 text-[11px] font-medium dark:border-slate-700/80 dark:bg-slate-800/80">
          <span
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
            {t("charts.metric_cost")}
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
            {t("charts.metric_tokens")}
          </button>
        </div>
      </div>

      {/* Empty State or Donut + Legend Layout */}
      {items.length === 0 ? (
        <div
          role="status"
          className="flex flex-1 flex-col items-center justify-center rounded-lg border border-dashed border-slate-200 bg-slate-50/60 px-4 py-8 text-center dark:border-slate-800 dark:bg-slate-950/40"
        >
          <svg viewBox="0 0 64 64" className="mb-2.5 h-12 w-12 -rotate-90" aria-hidden="true">
            <circle
              cx="32"
              cy="32"
              r="22"
              fill="none"
              strokeWidth="6"
              className="stroke-slate-200 dark:stroke-slate-800"
            />
          </svg>
          <p className="text-xs font-medium text-slate-500 dark:text-slate-400">
            {t("charts.source_share_empty")}
          </p>
        </div>
      ) : (
        <div className="flex flex-1 flex-col items-center gap-5 sm:flex-row">
          {/* Handcrafted SVG Donut Ring */}
          <div className="relative flex h-36 w-36 shrink-0 items-center justify-center">
            <svg viewBox="0 0 140 140" className="h-full w-full -rotate-90">
              <circle
                cx="70"
                cy="70"
                r={radius}
                fill="none"
                strokeWidth={strokeWidth}
                className="stroke-slate-100 dark:stroke-slate-800"
              />
              {segments.map((seg) => {
                const isDimmed =
                  (hoveredSource && hoveredSource !== seg.source) ||
                  (activeSource && !hoveredSource && activeSource !== seg.source);
                return (
                  <circle
                    key={seg.source}
                    cx="70"
                    cy="70"
                    r={radius}
                    fill="none"
                    stroke={seg.theme.color}
                    strokeWidth={
                      hoveredSource === seg.source || activeSource === seg.source
                        ? strokeWidth + 2.5
                        : strokeWidth
                    }
                    strokeDasharray={`${seg.dashLength} ${circumference}`}
                    strokeDashoffset={seg.dashOffset}
                    strokeLinecap="butt"
                    className="cursor-pointer transition-all duration-200"
                    style={{ opacity: isDimmed ? 0.3 : 1 }}
                    onMouseEnter={() => setHoveredSource(seg.source)}
                    onMouseLeave={() => setHoveredSource(null)}
                    onClick={() =>
                      onSelectSource(activeSource === seg.source ? "" : seg.source)
                    }
                  />
                );
              })}
            </svg>

            {/* Center Readout */}
            <div className="pointer-events-none absolute inset-0 flex flex-col items-center justify-center px-3 text-center">
              <span className="max-w-[82px] truncate text-[10px] font-semibold tracking-wider text-slate-400 uppercase dark:text-slate-500">
                {focusedItem ? focusedItem.theme.shortCode : t("charts.trend_total")}
              </span>
              <span className="font-mono-num mt-0.5 text-sm font-bold text-slate-900 dark:text-slate-100">
                {focusedItem
                  ? metric === "cost"
                    ? formatCost(focusedItem.total_cost, currency)
                    : formatCompactNumber(focusedItem.tokens)
                  : metric === "cost"
                    ? formatCost(totalValue, currency)
                    : formatCompactNumber(totalValue)}
              </span>
              {focusedItem && (
                <span className="font-mono-num text-[10px] font-medium text-slate-500 dark:text-slate-400">
                  {focusedItem.pct.toFixed(1)}%
                </span>
              )}
            </div>
          </div>

          {/* Interactive Source Rows with Inline Share Bars */}
          <div className="flex w-full flex-1 flex-col gap-1.5">
            {items.map((item) => {
              const isSelected = activeSource === item.source;
              const isHovered = hoveredSource === item.source;
              const barWidth = Math.min(100, Math.max(item.pct > 0 ? 2 : 0, item.pct));
              return (
                <button
                  key={item.source}
                  type="button"
                  onMouseEnter={() => setHoveredSource(item.source)}
                  onMouseLeave={() => setHoveredSource(null)}
                  onClick={() => onSelectSource(isSelected ? "" : item.source)}
                  className={`flex w-full flex-col gap-1.5 rounded-lg border px-2.5 py-2 text-left transition-all ${
                    isSelected
                      ? "border-slate-900 bg-slate-900 text-white shadow-xs dark:border-slate-200 dark:bg-slate-100 dark:text-slate-950"
                      : isHovered
                        ? "border-slate-300 bg-slate-50 dark:border-slate-700 dark:bg-slate-800/80"
                        : "border-slate-100 bg-white hover:border-slate-200 dark:border-slate-800 dark:bg-slate-900/70 dark:hover:border-slate-700"
                  }`}
                >
                  <div className="flex w-full items-center justify-between gap-2">
                    <div className="flex min-w-0 items-center gap-2">
                      <SourceIcon source={item.source} className="h-4 w-4 shrink-0" />
                      <div className="min-w-0">
                        <div
                          className={`truncate text-xs font-semibold ${
                            isSelected ? "text-white dark:text-slate-950" : "text-slate-800 dark:text-slate-200"
                          }`}
                        >
                          {item.source}
                        </div>
                        <div
                          className={`font-mono-num text-[10px] ${
                            isSelected ? "text-slate-300 dark:text-slate-600" : "text-slate-400 dark:text-slate-500"
                          }`}
                        >
                          {formatCompactNumber(item.tokens)} tok · {item.events} ev
                        </div>
                      </div>
                    </div>

                    <div className="ml-2 text-right">
                      <div
                        className={`font-mono-num text-xs font-semibold ${
                          isSelected ? "text-white dark:text-slate-950" : "text-slate-900 dark:text-slate-100"
                        }`}
                      >
                        {metric === "cost"
                          ? formatCost(item.total_cost, currency)
                          : formatCompactNumber(item.tokens)}
                      </div>
                      <div
                        className={`font-mono-num text-[10px] ${
                          isSelected ? "text-emerald-300 dark:text-emerald-700" : "text-slate-500 dark:text-slate-400"
                        }`}
                      >
                        {item.pct.toFixed(1)}%
                      </div>
                    </div>
                  </div>

                  {/* Subtle Inline Progress Bar */}
                  <div
                    className={`h-1 w-full overflow-hidden rounded-full ${
                      isSelected
                        ? "bg-white/15 dark:bg-slate-900/15"
                        : "bg-slate-100 dark:bg-slate-800"
                    }`}
                    aria-hidden="true"
                  >
                    <div
                      className="h-full rounded-full transition-all duration-300"
                      style={{
                        width: `${barWidth}%`,
                        backgroundColor: item.theme.color,
                      }}
                    />
                  </div>
                </button>
              );
            })}
          </div>
        </div>
      )}
    </div>
  );
});
