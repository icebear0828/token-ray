import React, { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { CurrencyCode, HeatmapPoint } from "../types/telemetry";
import { formatCompactNumber, formatCost } from "../utils/formatters";

interface ActivityHeatmapProps {
  data: HeatmapPoint[];
  currency: CurrencyCode;
}

export const ActivityHeatmap = React.memo(function ActivityHeatmap({
  data,
  currency,
}: ActivityHeatmapProps) {
  const { t } = useTranslation();
  const [hoveredCell, setHoveredCell] = useState<HeatmapPoint | null>(null);

  const { weeks, monthLabels, maxTokens, activeDays, totalEvents } = useMemo(() => {
    const safe = data ?? [];
    let maxTok = 0;
    let active = 0;
    let eventsSum = 0;

    for (const pt of safe) {
      if (pt.tokens > maxTok) maxTok = pt.tokens;
      if (pt.tokens > 0 || pt.events > 0) active += 1;
      eventsSum += pt.events || 0;
    }

    // Group into columns of 7 days (Sunday=0 .. Saturday=6)
    const cols: (HeatmapPoint | null)[][] = [];
    if (safe.length > 0) {
      const firstDate = new Date(`${safe[0].date}T00:00:00Z`);
      const startDay = Number.isNaN(firstDate.getTime()) ? 0 : firstDate.getUTCDay();
      let currentWeek: (HeatmapPoint | null)[] = Array(startDay).fill(null);

      for (const item of safe) {
        currentWeek.push(item);
        if (currentWeek.length === 7) {
          cols.push(currentWeek);
          currentWeek = [];
        }
      }
      if (currentWeek.length > 0) {
        while (currentWeek.length < 7) {
          currentWeek.push(null);
        }
        cols.push(currentWeek);
      }
    }

    // Extract month markers per week column
    const labels: { colIdx: number; monthIndex: number }[] = [];
    let lastMonth = -1;
    cols.forEach((week, colIdx) => {
      const firstValid = week.find((c) => c !== null);
      if (firstValid) {
        const dt = new Date(`${firstValid.date}T00:00:00Z`);
        if (!Number.isNaN(dt.getTime())) {
          const m = dt.getUTCMonth();
          if (m !== lastMonth) {
            labels.push({ colIdx, monthIndex: m });
            lastMonth = m;
          }
        }
      }
    });

    return {
      weeks: cols,
      monthLabels: labels,
      maxTokens: maxTok,
      activeDays: active,
      totalEvents: eventsSum,
    };
  }, [data]);

  const getCellLevelClass = (tokens: number): string => {
    if (!tokens || tokens <= 0 || maxTokens <= 0) {
      return "bg-slate-100 hover:ring-1 hover:ring-slate-400 dark:bg-slate-800/80 dark:hover:ring-slate-500";
    }
    const ratio = tokens / maxTokens;
    if (ratio < 0.22) {
      return "bg-emerald-200 hover:ring-1 hover:ring-emerald-600 dark:bg-emerald-950 dark:ring-1 dark:ring-inset dark:ring-emerald-800/70 dark:hover:ring-emerald-500";
    }
    if (ratio < 0.48) {
      return "bg-emerald-300 hover:ring-1 hover:ring-emerald-700 dark:bg-emerald-800 dark:hover:ring-emerald-400";
    }
    if (ratio < 0.75) {
      return "bg-emerald-500 hover:ring-1 hover:ring-emerald-800 dark:bg-emerald-600 dark:hover:ring-emerald-300";
    }
    return "bg-emerald-700 hover:ring-1 hover:ring-slate-900 dark:bg-emerald-400 dark:hover:ring-emerald-200";
  };

  return (
    <section className="rounded-xl border border-slate-200/90 bg-white p-5 shadow-[0_1px_2px_rgba(15,23,42,0.03)] dark:border-slate-800 dark:bg-slate-900 dark:shadow-none">
      {/* Top Row: Title & Summary Pills */}
      <div className="mb-4 flex flex-wrap items-center justify-between gap-3">
        <div>
          <div className="text-[11px] font-semibold tracking-wider text-slate-400 uppercase dark:text-slate-500">
            {t("charts.heatmap_sub")}
          </div>
          <h3 className="mt-0.5 text-sm font-semibold text-slate-900 dark:text-slate-100">
            {t("charts.heatmap_title")}
          </h3>
        </div>

        <div className="flex flex-wrap items-center gap-3 text-xs sm:gap-4">
          <div className="flex items-center gap-1.5">
            <span className="text-slate-400 dark:text-slate-500">{t("charts.active_days")}:</span>
            <span className="font-mono-num font-semibold text-slate-800 dark:text-slate-200">
              {t("charts.days_unit", { count: activeDays })}
            </span>
          </div>
          <div className="h-3 w-px bg-slate-200 dark:bg-slate-800" />
          <div className="flex items-center gap-1.5">
            <span className="text-slate-400 dark:text-slate-500">{t("charts.peak_tokens")}:</span>
            <span className="font-mono-num font-semibold text-slate-800 dark:text-slate-200">
              {formatCompactNumber(maxTokens)}
            </span>
          </div>
          <div className="h-3 w-px bg-slate-200 dark:bg-slate-800" />
          <div className="flex items-center gap-1.5">
            <span className="text-slate-400 dark:text-slate-500">{t("charts.total_events")}:</span>
            <span className="font-mono-num font-semibold text-slate-800 dark:text-slate-200">
              {formatCompactNumber(totalEvents)}
            </span>
          </div>
        </div>
      </div>

      {/* Interactive Matrix Scroll Wrapper */}
      <div className="overflow-x-auto pb-1">
        <div className="inline-block min-w-full">
          {/* Month Header Row */}
          <div className="mb-1.5 flex pl-9 text-[10px] font-medium text-slate-400 dark:text-slate-500">
            {weeks.map((_, colIdx) => {
              const found = monthLabels.find((m) => m.colIdx === colIdx);
              return (
                <div key={colIdx} className="w-[15px] shrink-0 whitespace-nowrap">
                  {found ? t(`charts.month_${found.monthIndex}`) : ""}
                </div>
              );
            })}
          </div>

          {/* Grid Body: Weekday labels + Week columns */}
          <div className="flex items-start gap-2">
            {/* Weekday Anchors */}
            <div className="flex w-7 shrink-0 flex-col gap-[3px] pt-[1px] text-[10px] leading-[12px] text-slate-400 dark:text-slate-500">
              <span className="h-[12px]" />
              <span className="h-[12px]">{t("charts.weekday_mon")}</span>
              <span className="h-[12px]" />
              <span className="h-[12px]">{t("charts.weekday_wed")}</span>
              <span className="h-[12px]" />
              <span className="h-[12px]">{t("charts.weekday_fri")}</span>
              <span className="h-[12px]" />
            </div>

            {/* Week Columns */}
            <div
              className="flex gap-[3px]"
              onMouseLeave={() => setHoveredCell(null)}
            >
              {weeks.map((week, colIdx) => (
                <div key={colIdx} className="flex flex-col gap-[3px]">
                  {week.map((cell, rowIdx) => {
                    if (!cell) {
                      return (
                        <div
                          key={`empty-${colIdx}-${rowIdx}`}
                          className="h-[12px] w-[12px] rounded-[2.5px] bg-transparent"
                        />
                      );
                    }
                    return (
                      <div
                        key={cell.date}
                        data-testid="heatmap-cell"
                        data-date={cell.date}
                        data-tokens={cell.tokens}
                        data-cost={cell.cost}
                        onMouseEnter={() => setHoveredCell(cell)}
                        onMouseLeave={() => setHoveredCell(null)}
                        className={`h-[12px] w-[12px] cursor-pointer rounded-[2.5px] transition-transform duration-100 hover:scale-125 ${getCellLevelClass(
                          cell.tokens
                        )}`}
                      />
                    );
                  })}
                </div>
              ))}
            </div>
          </div>
        </div>
      </div>

      {/* Bottom Readout & Legend Bar */}
      <div className="mt-3 flex flex-wrap items-center justify-between gap-2 border-t border-slate-100 pt-2.5 text-xs dark:border-slate-800">
        <div className="flex min-h-[20px] flex-wrap items-center gap-2.5">
          {hoveredCell ? (
            <>
              <span className="font-mono-num font-semibold text-slate-800 dark:text-slate-100">
                {hoveredCell.date}
              </span>
              <span className="text-slate-300 dark:text-slate-700">·</span>
              <span className="font-mono-num text-slate-600 dark:text-slate-400">
                <strong className="text-slate-900 dark:text-slate-100">
                  {formatCompactNumber(hoveredCell.tokens)}
                </strong>{" "}
                {t("charts.unit_tokens")}
              </span>
              <span className="text-slate-300 dark:text-slate-700">·</span>
              <span className="font-mono-num text-slate-600 dark:text-slate-400">
                <strong className="text-slate-900 dark:text-slate-100">
                  {formatCost(hoveredCell.cost, currency)}
                </strong>
              </span>
              <span className="text-slate-300 dark:text-slate-700">·</span>
              <span className="font-mono-num text-slate-500 dark:text-slate-400">
                {t("charts.unit_events", { count: hoveredCell.events })}
              </span>
            </>
          ) : (
            <span className="text-[11px] text-slate-400 dark:text-slate-500">
              {t("charts.hover_hint")}
            </span>
          )}
        </div>

        {/* Intensity Scale Legend */}
        <div className="flex items-center gap-1.5 text-[10px] text-slate-400 dark:text-slate-500">
          <span>{t("charts.less")}</span>
          <span className="h-2.5 w-2.5 rounded-[2px] bg-slate-100 dark:bg-slate-800/80" />
          <span className="h-2.5 w-2.5 rounded-[2px] bg-emerald-200 dark:bg-emerald-950 dark:ring-1 dark:ring-inset dark:ring-emerald-800/70" />
          <span className="h-2.5 w-2.5 rounded-[2px] bg-emerald-300 dark:bg-emerald-800" />
          <span className="h-2.5 w-2.5 rounded-[2px] bg-emerald-500 dark:bg-emerald-600" />
          <span className="h-2.5 w-2.5 rounded-[2px] bg-emerald-700 dark:bg-emerald-400" />
          <span>{t("charts.more")}</span>
        </div>
      </div>
    </section>
  );
});
