import React, { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { FolderGit2, Search } from "lucide-react";
import { CurrencyCode, ModelStats, ProjectStat, SourceStats } from "../types/telemetry";
import {
  calcBaseInput,
  formatCompactNumber,
  formatCost,
  formatExactNumber,
  formatPercent,
  formatUnitPrice,
} from "../utils/formatters";
import { SourceBadge } from "./SourceBadge";

interface AttributionTablesProps {
  sources: SourceStats[];
  projects?: ProjectStat[];
  isHydratingDetails: boolean;
  currency: CurrencyCode;
}

interface FlattenedModelRow extends ModelStats {
  sourceName: string;
}

export const AttributionTables: React.FC<AttributionTablesProps> = ({
  sources,
  projects,
  isHydratingDetails,
  currency,
}) => {
  const { t } = useTranslation();
  const [query, setQuery] = useState("");
  const [sortBy, setSortBy] = useState<"cost" | "tokens" | "cache">("cost");

  const flattenedModels = useMemo(() => {
    const rows: FlattenedModelRow[] = [];
    for (const src of sources ?? []) {
      for (const m of src.models ?? []) {
        rows.push({
          ...m,
          sourceName: src.name,
        });
      }
    }
    const q = query.trim().toLowerCase();
    const filtered = q
      ? rows.filter(
          (r) =>
            r.model.toLowerCase().includes(q) || r.sourceName.toLowerCase().includes(q)
        )
      : rows;

    return filtered.sort((a, b) => {
      if (sortBy === "tokens") {
        return b.input_tokens + b.output_tokens - (a.input_tokens + a.output_tokens);
      }
      if (sortBy === "cache") {
        return b.cache_hit_rate - a.cache_hit_rate;
      }
      return b.total_cost - a.total_cost;
    });
  }, [sources, query, sortBy]);

  const filteredProjects = useMemo(() => {
    const list = projects ?? [];
    const q = query.trim().toLowerCase();
    const filtered = q ? list.filter((p) => p.project.toLowerCase().includes(q)) : list;
    return [...filtered].sort((a, b) => b.total_cost - a.total_cost);
  }, [projects, query]);

  const totalProjectCost = useMemo(
    () => (projects ?? []).reduce((acc, p) => acc + (p.total_cost || 0), 0),
    [projects]
  );

  return (
    <div className="space-y-6">
      {/* Shared Search & Sort Toolbar */}
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="relative w-full sm:w-72">
          <Search className="pointer-events-none absolute top-1/2 left-3 h-3.5 w-3.5 -translate-y-1/2 text-slate-400" />
          <input
            type="text"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder={t("tables.search_placeholder")}
            className="w-full rounded-lg border border-slate-200 bg-white py-1.5 pr-3 pl-8 text-xs text-slate-800 placeholder:text-slate-400 focus:border-slate-900 focus:outline-none"
          />
        </div>

        <div className="flex items-center gap-1.5 text-[11px]">
          <span className="font-semibold tracking-wider text-slate-400 uppercase">SORT:</span>
          {(["cost", "tokens", "cache"] as const).map((key) => (
            <button
              key={key}
              type="button"
              onClick={() => setSortBy(key)}
              className={`rounded-md border px-2.5 py-1 font-medium transition-colors ${
                sortBy === key
                  ? "border-slate-900 bg-slate-900 text-white"
                  : "border-slate-200 bg-white text-slate-600 hover:bg-slate-50"
              }`}
            >
              {key === "cost" ? "COST" : key === "tokens" ? "TOKENS" : "CACHE HIT"}
            </button>
          ))}
        </div>
      </div>

      {/* 1. Model Attribution Table */}
      <section className="overflow-hidden rounded-xl border border-slate-200/90 bg-white shadow-[0_1px_2px_rgba(15,23,42,0.03)]">
        <div className="flex flex-wrap items-center justify-between gap-2 border-b border-slate-100 px-5 py-4">
          <div>
            <div className="text-[11px] font-semibold tracking-wider text-slate-400 uppercase">
              {t("tables.models_sub")}
            </div>
            <h3 className="mt-0.5 text-sm font-semibold text-slate-900">
              {t("tables.models_title")}
            </h3>
          </div>
          <span className="font-mono-num rounded-md bg-slate-100 px-2 py-0.5 text-[11px] font-medium text-slate-600">
            {flattenedModels.length} MODELS
          </span>
        </div>

        <div className="overflow-x-auto">
          <table className="w-full border-collapse text-left text-xs">
            <thead>
              <tr className="border-b border-slate-100 bg-slate-50/70 text-[10px] font-semibold tracking-wider text-slate-400 uppercase">
                <th className="py-2.5 pr-3 pl-5">{t("tables.col_model")}</th>
                <th className="px-3 py-2.5 text-right">{t("tables.col_events")}</th>
                <th className="px-3 py-2.5 text-right">{t("tables.col_base_input")}</th>
                <th className="px-3 py-2.5 text-right">{t("tables.col_cached")}</th>
                <th className="px-3 py-2.5 text-right">{t("tables.col_cache_write")}</th>
                <th className="px-3 py-2.5 text-right">{t("tables.col_output")}</th>
                <th className="px-3 py-2.5 text-right">{t("tables.col_hit_rate")}</th>
                <th className="px-3 py-2.5 text-right">{t("tables.col_unit_price")}</th>
                <th className="py-2.5 pr-5 pl-3 text-right">{t("tables.col_cost")}</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-100">
              {isHydratingDetails && flattenedModels.length === 0 ? (
                Array.from({ length: 4 }).map((_, idx) => (
                  <tr key={idx} className="animate-pulse">
                    <td className="py-3.5 pr-3 pl-5">
                      <div className="h-4 w-44 rounded bg-slate-100" />
                    </td>
                    <td className="px-3 py-3.5">
                      <div className="ml-auto h-3.5 w-10 rounded bg-slate-100" />
                    </td>
                    <td className="px-3 py-3.5">
                      <div className="ml-auto h-3.5 w-14 rounded bg-slate-100" />
                    </td>
                    <td className="px-3 py-3.5">
                      <div className="ml-auto h-3.5 w-14 rounded bg-slate-100" />
                    </td>
                    <td className="px-3 py-3.5">
                      <div className="ml-auto h-3.5 w-12 rounded bg-slate-100" />
                    </td>
                    <td className="px-3 py-3.5">
                      <div className="ml-auto h-3.5 w-14 rounded bg-slate-100" />
                    </td>
                    <td className="px-3 py-3.5">
                      <div className="ml-auto h-3.5 w-16 rounded bg-slate-100" />
                    </td>
                    <td className="px-3 py-3.5">
                      <div className="ml-auto h-3.5 w-20 rounded bg-slate-100" />
                    </td>
                    <td className="py-3.5 pr-5 pl-3">
                      <div className="ml-auto h-4 w-14 rounded bg-slate-100" />
                    </td>
                  </tr>
                ))
              ) : flattenedModels.length === 0 ? (
                <tr>
                  <td colSpan={9} className="py-10 text-center text-xs text-slate-400">
                    {t("tables.empty_models")}
                  </td>
                </tr>
              ) : (
                flattenedModels.map((row) => {
                  const baseInput = calcBaseInput(
                    row.input_tokens,
                    row.cached_tokens,
                    row.cache_creation_tokens
                  );
                  return (
                    <tr
                      key={`${row.sourceName}-${row.model}`}
                      className="transition-colors hover:bg-slate-50/80"
                    >
                      <td className="py-3 pr-3 pl-5">
                        <div className="flex flex-wrap items-center gap-2">
                          <span className="font-mono-num font-semibold text-slate-900">
                            {row.model}
                          </span>
                          <SourceBadge source={row.sourceName} />
                        </div>
                      </td>
                      <td className="font-mono-num px-3 py-3 text-right text-slate-500">
                        {formatCompactNumber(row.events)}
                      </td>
                      <td
                        className="font-mono-num px-3 py-3 text-right font-medium text-slate-800"
                        title={formatExactNumber(baseInput)}
                      >
                        {formatCompactNumber(baseInput)}
                      </td>
                      <td
                        className="font-mono-num px-3 py-3 text-right font-medium text-emerald-700"
                        title={formatExactNumber(row.cached_tokens)}
                      >
                        {formatCompactNumber(row.cached_tokens)}
                      </td>
                      <td
                        className="font-mono-num px-3 py-3 text-right text-slate-500"
                        title={formatExactNumber(row.cache_creation_tokens)}
                      >
                        {formatCompactNumber(row.cache_creation_tokens)}
                      </td>
                      <td
                        className="font-mono-num px-3 py-3 text-right text-slate-700"
                        title={formatExactNumber(row.output_tokens)}
                      >
                        {formatCompactNumber(row.output_tokens)}
                      </td>
                      <td className="px-3 py-3 text-right">
                        <div className="inline-flex items-center justify-end gap-2">
                          <div className="h-1.5 w-12 overflow-hidden rounded-full bg-slate-100">
                            <div
                              style={{ width: `${Math.min(100, row.cache_hit_rate)}%` }}
                              className="h-full rounded-full bg-emerald-500"
                            />
                          </div>
                          <span className="font-mono-num w-11 text-right font-medium text-slate-700">
                            {formatPercent(row.cache_hit_rate)}
                          </span>
                        </div>
                      </td>
                      <td className="font-mono-num px-3 py-3 text-right text-[11px] text-slate-400">
                        {formatUnitPrice(row.input_price_per_m, currency)} /{" "}
                        {formatUnitPrice(row.cached_price_per_m, currency)} /{" "}
                        {formatUnitPrice(row.cache_creation_price_per_m, currency)} /{" "}
                        {formatUnitPrice(row.output_price_per_m, currency)}
                      </td>
                      <td className="font-mono-num py-3 pr-5 pl-3 text-right font-semibold text-slate-900">
                        {formatCost(row.total_cost, currency)}
                      </td>
                    </tr>
                  );
                })
              )}
            </tbody>
          </table>
        </div>
      </section>

      {/* 2. Project Workspace Attribution Table */}
      <section className="overflow-hidden rounded-xl border border-slate-200/90 bg-white shadow-[0_1px_2px_rgba(15,23,42,0.03)]">
        <div className="flex flex-wrap items-center justify-between gap-2 border-b border-slate-100 px-5 py-4">
          <div>
            <div className="text-[11px] font-semibold tracking-wider text-slate-400 uppercase">
              {t("tables.projects_sub")}
            </div>
            <h3 className="mt-0.5 text-sm font-semibold text-slate-900">
              {t("tables.projects_title")}
            </h3>
          </div>
          <span className="font-mono-num rounded-md bg-slate-100 px-2 py-0.5 text-[11px] font-medium text-slate-600">
            {filteredProjects.length} WORKSPACES
          </span>
        </div>

        <div className="overflow-x-auto">
          <table className="w-full border-collapse text-left text-xs">
            <thead>
              <tr className="border-b border-slate-100 bg-slate-50/70 text-[10px] font-semibold tracking-wider text-slate-400 uppercase">
                <th className="py-2.5 pr-3 pl-5">{t("tables.col_project")}</th>
                <th className="px-3 py-2.5 text-right">{t("tables.col_events")}</th>
                <th className="px-3 py-2.5 text-right">{t("tables.col_base_input")}</th>
                <th className="px-3 py-2.5 text-right">{t("tables.col_cached")}</th>
                <th className="px-3 py-2.5 text-right">{t("tables.col_output")}</th>
                <th className="px-3 py-2.5 text-right">{t("tables.col_hit_rate")}</th>
                <th className="px-3 py-2.5 text-right">{t("tables.col_share")}</th>
                <th className="py-2.5 pr-5 pl-3 text-right">{t("tables.col_cost")}</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-100">
              {isHydratingDetails && !projects ? (
                Array.from({ length: 3 }).map((_, idx) => (
                  <tr key={idx} className="animate-pulse">
                    <td className="py-3.5 pr-3 pl-5">
                      <div className="h-4 w-40 rounded bg-slate-100" />
                    </td>
                    <td className="px-3 py-3.5">
                      <div className="ml-auto h-3.5 w-10 rounded bg-slate-100" />
                    </td>
                    <td className="px-3 py-3.5">
                      <div className="ml-auto h-3.5 w-14 rounded bg-slate-100" />
                    </td>
                    <td className="px-3 py-3.5">
                      <div className="ml-auto h-3.5 w-14 rounded bg-slate-100" />
                    </td>
                    <td className="px-3 py-3.5">
                      <div className="ml-auto h-3.5 w-14 rounded bg-slate-100" />
                    </td>
                    <td className="px-3 py-3.5">
                      <div className="ml-auto h-3.5 w-12 rounded bg-slate-100" />
                    </td>
                    <td className="px-3 py-3.5">
                      <div className="ml-auto h-3.5 w-16 rounded bg-slate-100" />
                    </td>
                    <td className="py-3.5 pr-5 pl-3">
                      <div className="ml-auto h-4 w-14 rounded bg-slate-100" />
                    </td>
                  </tr>
                ))
              ) : filteredProjects.length === 0 ? (
                <tr>
                  <td colSpan={8} className="py-10 text-center text-xs text-slate-400">
                    {t("tables.empty_projects")}
                  </td>
                </tr>
              ) : (
                filteredProjects.map((proj) => {
                  const baseInput = calcBaseInput(
                    proj.input_tokens,
                    proj.cached_tokens,
                    proj.cache_creation_tokens
                  );
                  const costSharePct =
                    totalProjectCost > 0 ? (proj.total_cost / totalProjectCost) * 100 : 0;

                  return (
                    <tr
                      key={proj.project}
                      className="transition-colors hover:bg-slate-50/80"
                    >
                      <td className="py-3 pr-3 pl-5">
                        <div className="flex items-center gap-2.5">
                          <div className="flex h-7 w-7 shrink-0 items-center justify-center rounded-md border border-slate-200 bg-slate-50 text-slate-600">
                            <FolderGit2 className="h-3.5 w-3.5" />
                          </div>
                          <span className="font-mono-num font-semibold text-slate-900">
                            {proj.project}
                          </span>
                        </div>
                      </td>
                      <td className="font-mono-num px-3 py-3 text-right text-slate-500">
                        {formatCompactNumber(proj.events)}
                      </td>
                      <td
                        className="font-mono-num px-3 py-3 text-right font-medium text-slate-800"
                        title={formatExactNumber(baseInput)}
                      >
                        {formatCompactNumber(baseInput)}
                      </td>
                      <td
                        className="font-mono-num px-3 py-3 text-right font-medium text-emerald-700"
                        title={formatExactNumber(proj.cached_tokens)}
                      >
                        {formatCompactNumber(proj.cached_tokens)}
                      </td>
                      <td
                        className="font-mono-num px-3 py-3 text-right text-slate-700"
                        title={formatExactNumber(proj.output_tokens)}
                      >
                        {formatCompactNumber(proj.output_tokens)}
                      </td>
                      <td className="font-mono-num px-3 py-3 text-right font-medium text-slate-700">
                        {formatPercent(proj.cache_hit_rate)}
                      </td>
                      <td className="px-3 py-3 text-right">
                        <div className="inline-flex items-center justify-end gap-2">
                          <div className="h-1.5 w-14 overflow-hidden rounded-full bg-slate-100">
                            <div
                              style={{ width: `${Math.min(100, costSharePct)}%` }}
                              className="h-full rounded-full bg-slate-800"
                            />
                          </div>
                          <span className="font-mono-num w-10 text-right text-slate-500">
                            {costSharePct.toFixed(0)}%
                          </span>
                        </div>
                      </td>
                      <td className="font-mono-num py-3 pr-5 pl-3 text-right font-semibold text-slate-900">
                        {formatCost(proj.total_cost, currency)}
                      </td>
                    </tr>
                  );
                })
              )}
            </tbody>
          </table>
        </div>
      </section>
    </div>
  );
};
