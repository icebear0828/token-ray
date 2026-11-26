import React, { useState } from "react";
import { useTranslation } from "react-i18next";
import { PeriodStats } from "../types/telemetry";
import {
  calcBaseInput,
  formatCompactNumber,
  formatExactNumber,
  formatTokenRate,
  tokenRatePerSecond,
} from "../utils/formatters";

interface TokenTaxonomyStripProps {
  period: PeriodStats;
}

export const TokenTaxonomyStrip = React.memo(function TokenTaxonomyStrip({
  period,
}: TokenTaxonomyStripProps) {
  const { t } = useTranslation();
  const [hoveredKey, setHoveredKey] = useState<string | null>(null);

  const baseInput = calcBaseInput(
    period.input_tokens,
    period.cached_tokens,
    period.cache_creation_tokens
  );
  const cacheRead = period.cached_tokens || 0;
  const cacheWrite = period.cache_creation_tokens || 0;
  const output = period.output_tokens || 0;
  const outputRateValue = tokenRatePerSecond(period.timed_output_tokens, period.duration_ms);
  const outputRate = formatTokenRate(period.timed_output_tokens, period.duration_ms);

  const totalVolume = Math.max(1, baseInput + cacheRead + cacheWrite + output);

  const segments = [
    {
      key: "base_input",
      code: t("taxonomy.base_input"),
      label: t("taxonomy.base_input_cn"),
      value: baseInput,
      pct: (baseInput / totalVolume) * 100,
      barColor: "bg-slate-800 dark:bg-slate-300",
      dotColor: "bg-slate-800 dark:bg-slate-300",
      badgeBg: "bg-slate-100 text-slate-700 dark:bg-slate-800 dark:text-slate-200",
    },
    {
      key: "cache_read",
      code: t("taxonomy.cache_read"),
      label: t("taxonomy.cache_read_cn"),
      value: cacheRead,
      pct: (cacheRead / totalVolume) * 100,
      barColor: "bg-emerald-500 dark:bg-emerald-400",
      dotColor: "bg-emerald-500 dark:bg-emerald-400",
      badgeBg: "bg-emerald-50 text-emerald-700 dark:bg-emerald-500/15 dark:text-emerald-300",
    },
    {
      key: "cache_write",
      code: t("taxonomy.cache_write"),
      label: t("taxonomy.cache_write_cn"),
      value: cacheWrite,
      pct: (cacheWrite / totalVolume) * 100,
      barColor: "bg-amber-400 dark:bg-amber-400",
      dotColor: "bg-amber-400 dark:bg-amber-400",
      badgeBg: "bg-amber-50 text-amber-700 dark:bg-amber-500/15 dark:text-amber-300",
    },
    {
      key: "output",
      code: t("taxonomy.output"),
      label: t("taxonomy.output_cn"),
      value: output,
      pct: (output / totalVolume) * 100,
      barColor: "bg-sky-500 dark:bg-sky-400",
      dotColor: "bg-sky-500 dark:bg-sky-400",
      badgeBg: "bg-sky-50 text-sky-700 dark:bg-sky-500/15 dark:text-sky-300",
    },
  ];

  return (
    <section className="rounded-xl border border-slate-200/90 bg-white p-5 shadow-[0_1px_2px_rgba(15,23,42,0.03)] dark:border-slate-800 dark:bg-slate-900 dark:shadow-none">
      <div className="mb-3.5 flex flex-wrap items-center justify-between gap-2">
        <div className="flex items-center gap-2.5">
          <span className="text-[11px] font-semibold tracking-wider text-slate-400 uppercase dark:text-slate-500">
            {t("taxonomy.subtitle")}
          </span>
          <span className="text-slate-300 dark:text-slate-700">·</span>
          <h2 className="text-sm font-semibold text-slate-800 dark:text-slate-100">
            {t("taxonomy.title")}
          </h2>
        </div>
        <div className="flex items-center gap-2 text-xs text-slate-500 dark:text-slate-400">
          <span className="text-[11px] font-medium tracking-wide text-slate-400 uppercase dark:text-slate-500">
            {t("taxonomy.total_input")}:
          </span>
          <span
            className="font-mono-num font-semibold text-slate-700 dark:text-slate-200"
            title={formatExactNumber(period.input_tokens)}
          >
            {formatCompactNumber(period.input_tokens)}
          </span>
        </div>
      </div>

      {/* Proportional Stacked Bar */}
      <div
        className="mb-4 flex h-2.5 w-full overflow-hidden rounded-full bg-slate-100 p-0.5 dark:bg-slate-800"
        onMouseLeave={() => setHoveredKey(null)}
      >
        {segments.map((seg) => {
          if (seg.pct <= 0) return null;
          const isActive = hoveredKey === seg.key;
          const isDimmed = hoveredKey !== null && !isActive;
          return (
            <div
              key={seg.key}
              data-testid={`taxonomy-segment-${seg.key}`}
              data-active={isActive ? "true" : "false"}
              style={{ width: `${Math.max(seg.pct, 0.8)}%` }}
              onMouseEnter={() => setHoveredKey(seg.key)}
              onMouseLeave={() => setHoveredKey(null)}
              className={`${seg.barColor} mx-[0.5px] cursor-pointer transition-all duration-200 first:rounded-l-full last:rounded-r-full ${
                isDimmed ? "opacity-30" : "opacity-100"
              } ${isActive ? "ring-1 ring-slate-900/40 dark:ring-white/60" : ""}`}
              title={`${seg.code}: ${formatExactNumber(seg.value)} (${seg.pct.toFixed(1)}%)`}
            />
          );
        })}
      </div>

      {/* 4-Column Crisp Telemetry Readout */}
      <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
        {segments.map((seg) => {
          const isActive = hoveredKey === seg.key;
          const isDimmed = hoveredKey !== null && !isActive;
          return (
            <div
              key={seg.key}
              data-testid={`taxonomy-card-${seg.key}`}
              data-active={isActive ? "true" : "false"}
              onMouseEnter={() => setHoveredKey(seg.key)}
              onMouseLeave={() => setHoveredKey(null)}
              className={`group rounded-lg border px-3.5 py-2.5 transition-all duration-150 ${
                isActive
                  ? "border-slate-400 bg-white ring-1 ring-slate-300 dark:border-slate-500 dark:bg-slate-800/90 dark:ring-slate-600"
                  : isDimmed
                    ? "border-slate-100 bg-slate-50/40 opacity-55 dark:border-slate-800/70 dark:bg-slate-900/40"
                    : "border-slate-100 bg-slate-50/60 hover:border-slate-200 hover:bg-white dark:border-slate-800 dark:bg-slate-800/40 dark:hover:border-slate-700 dark:hover:bg-slate-800/80"
              }`}
            >
              <div className="flex items-center justify-between gap-1.5">
                <div className="flex items-center gap-1.5">
                  <span className={`h-2 w-2 rounded-full ${seg.dotColor}`} />
                  <span className="text-[11px] font-semibold tracking-wider text-slate-500 uppercase dark:text-slate-400">
                    {seg.code}
                  </span>
                </div>
                <span
                  className={`font-mono-num rounded px-1.5 py-0.5 text-[10px] font-semibold ${seg.badgeBg}`}
                >
                  {seg.pct.toFixed(1)}%
                </span>
              </div>
              <div className="mt-1.5 flex items-baseline justify-between gap-2">
                <span
                  className="font-mono-num text-lg font-semibold tracking-tight text-slate-900 dark:text-slate-100"
                  title={formatExactNumber(seg.value)}
                >
                  {formatCompactNumber(seg.value)}
                </span>
                <span className="truncate text-[11px] text-slate-400 dark:text-slate-500">
                  {seg.key === "output" && outputRateValue !== null
                    ? `≈ ${outputRate} tok/s`
                    : seg.label}
                </span>
              </div>
            </div>
          );
        })}
      </div>
    </section>
  );
});
