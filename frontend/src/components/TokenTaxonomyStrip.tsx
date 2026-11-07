import React from "react";
import { useTranslation } from "react-i18next";
import { PeriodStats } from "../types/telemetry";
import { calcBaseInput, formatCompactNumber, formatExactNumber } from "../utils/formatters";

interface TokenTaxonomyStripProps {
  period: PeriodStats;
}

export const TokenTaxonomyStrip: React.FC<TokenTaxonomyStripProps> = ({ period }) => {
  const { t } = useTranslation();

  const baseInput = calcBaseInput(
    period.input_tokens,
    period.cached_tokens,
    period.cache_creation_tokens
  );
  const cacheRead = period.cached_tokens || 0;
  const cacheWrite = period.cache_creation_tokens || 0;
  const output = period.output_tokens || 0;

  const totalVolume = Math.max(1, baseInput + cacheRead + cacheWrite + output);

  const segments = [
    {
      key: "base_input",
      code: t("taxonomy.base_input"),
      label: t("taxonomy.base_input_cn"),
      value: baseInput,
      pct: (baseInput / totalVolume) * 100,
      barColor: "bg-slate-800",
      dotColor: "bg-slate-800",
      badgeBg: "bg-slate-100 text-slate-700",
    },
    {
      key: "cache_read",
      code: t("taxonomy.cache_read"),
      label: t("taxonomy.cache_read_cn"),
      value: cacheRead,
      pct: (cacheRead / totalVolume) * 100,
      barColor: "bg-emerald-500",
      dotColor: "bg-emerald-500",
      badgeBg: "bg-emerald-50 text-emerald-700",
    },
    {
      key: "cache_write",
      code: t("taxonomy.cache_write"),
      label: t("taxonomy.cache_write_cn"),
      value: cacheWrite,
      pct: (cacheWrite / totalVolume) * 100,
      barColor: "bg-amber-400",
      dotColor: "bg-amber-400",
      badgeBg: "bg-amber-50 text-amber-700",
    },
    {
      key: "output",
      code: t("taxonomy.output"),
      label: t("taxonomy.output_cn"),
      value: output,
      pct: (output / totalVolume) * 100,
      barColor: "bg-blue-500",
      dotColor: "bg-blue-500",
      badgeBg: "bg-blue-50 text-blue-700",
    },
  ];

  return (
    <section className="rounded-xl border border-slate-200/90 bg-white p-5 shadow-[0_1px_2px_rgba(15,23,42,0.03)]">
      <div className="mb-3.5 flex flex-wrap items-center justify-between gap-2">
        <div className="flex items-center gap-2.5">
          <span className="text-[11px] font-semibold tracking-wider text-slate-400 uppercase">
            {t("taxonomy.subtitle")}
          </span>
          <span className="text-slate-300">·</span>
          <h2 className="text-sm font-semibold text-slate-800">{t("taxonomy.title")}</h2>
        </div>
        <div className="flex items-center gap-2 text-xs text-slate-500">
          <span className="text-[11px] font-medium tracking-wide text-slate-400 uppercase">
            {t("taxonomy.total_input")}:
          </span>
          <span
            className="font-mono-num font-semibold text-slate-700"
            title={formatExactNumber(period.input_tokens)}
          >
            {formatCompactNumber(period.input_tokens)}
          </span>
        </div>
      </div>

      {/* Proportional Stacked Bar */}
      <div className="mb-4 flex h-2.5 w-full overflow-hidden rounded-full bg-slate-100 p-0.5">
        {segments.map((seg) =>
          seg.pct > 0 ? (
            <div
              key={seg.key}
              style={{ width: `${Math.max(seg.pct, 0.8)}%` }}
              className={`${seg.barColor} first:rounded-l-full last:rounded-r-full transition-all duration-300 mx-[0.5px]`}
              title={`${seg.code}: ${formatExactNumber(seg.value)} (${seg.pct.toFixed(1)}%)`}
            />
          ) : null
        )}
      </div>

      {/* 4-Column Crisp Telemetry Readout */}
      <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
        {segments.map((seg) => (
          <div
            key={seg.key}
            className="group rounded-lg border border-slate-100 bg-slate-50/60 px-3.5 py-2.5 transition-colors hover:border-slate-200 hover:bg-white"
          >
            <div className="flex items-center justify-between gap-1.5">
              <div className="flex items-center gap-1.5">
                <span className={`h-2 w-2 rounded-full ${seg.dotColor}`} />
                <span className="text-[11px] font-semibold tracking-wider text-slate-500 uppercase">
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
                className="font-mono-num text-lg font-semibold tracking-tight text-slate-900"
                title={formatExactNumber(seg.value)}
              >
                {formatCompactNumber(seg.value)}
              </span>
              <span className="truncate text-[11px] text-slate-400">{seg.label}</span>
            </div>
          </div>
        ))}
      </div>
    </section>
  );
};
