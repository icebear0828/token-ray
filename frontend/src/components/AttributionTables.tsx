import React, { useDeferredValue, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { ChevronDown, ChevronsUpDown, ChevronUp, FolderGit2, Search } from "lucide-react";
import { CurrencyCode, ModelStats, ProjectStat, SourceStats } from "../types/telemetry";
import {
  calcBaseInput,
  formatCompactNumber,
  formatCost,
  formatExactNumber,
  formatPercent,
  formatTokenRate,
  formatUnitPrice,
  tokenRatePerSecond,
} from "../utils/formatters";
import { SourceBadge } from "./SourceBadge";

const PAGE_SIZE = 25;

type ToolbarSortKey = "cost" | "tokens" | "cache" | "speed";
type ModelSortKey =
  | "model"
  | "events"
  | "base_input"
  | "cached"
  | "cache_write"
  | "output"
  | "speed"
  | "cache"
  | "cost"
  | "tokens";
type ProjectSortKey =
  | "project"
  | "events"
  | "base_input"
  | "cached"
  | "output"
  | "cache"
  | "share"
  | "cost"
  | "tokens";
type SortDir = "asc" | "desc";

interface AttributionTablesProps {
  sources: SourceStats[];
  projects?: ProjectStat[];
  windowLabel: string;
  isHydratingDetails: boolean;
  currency: CurrencyCode;
}

interface FlattenedModelRow extends ModelStats {
  sourceName: string;
}

interface SortHeaderButtonProps {
  label: string;
  active: boolean;
  dir: SortDir;
  align?: "left" | "right";
  onClick: () => void;
}

function SortHeaderButton({
  label,
  active,
  dir,
  align = "right",
  onClick,
}: SortHeaderButtonProps) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={`inline-flex items-center gap-1 font-semibold tracking-wider uppercase transition-colors hover:text-slate-700 dark:hover:text-slate-200 ${
        align === "right" ? "ml-auto justify-end" : "justify-start"
      } ${active ? "text-slate-800 dark:text-slate-100" : ""}`}
    >
      <span>{label}</span>
      {active ? (
        dir === "asc" ? (
          <ChevronUp className="h-3 w-3 shrink-0" aria-hidden="true" />
        ) : (
          <ChevronDown className="h-3 w-3 shrink-0" aria-hidden="true" />
        )
      ) : (
        <ChevronsUpDown className="h-3 w-3 shrink-0 opacity-45" aria-hidden="true" />
      )}
    </button>
  );
}

export const AttributionTables = React.memo(function AttributionTables({
  sources,
  projects,
  windowLabel,
  isHydratingDetails,
  currency,
}: AttributionTablesProps) {
  const { t } = useTranslation();
  const [query, setQuery] = useState("");
  const deferredQuery = useDeferredValue(query);

  const [toolbarSort, setToolbarSort] = useState<ToolbarSortKey>("cost");
  const [modelSortKey, setModelSortKey] = useState<ModelSortKey>("cost");
  const [modelSortDir, setModelSortDir] = useState<SortDir>("desc");
  const [projectSortKey, setProjectSortKey] = useState<ProjectSortKey>("cost");
  const [projectSortDir, setProjectSortDir] = useState<SortDir>("desc");
  const [showAllModels, setShowAllModels] = useState(false);
  const [showAllProjects, setShowAllProjects] = useState(false);

  const handleToolbarSort = (key: ToolbarSortKey) => {
    setToolbarSort(key);
    setModelSortKey(key);
    setModelSortDir("desc");
    if (key !== "speed") {
      setProjectSortKey(key);
      setProjectSortDir("desc");
    }
  };

  const toggleModelSort = (key: ModelSortKey) => {
    if (modelSortKey === key) {
      setModelSortDir((prev) => (prev === "desc" ? "asc" : "desc"));
    } else {
      setModelSortKey(key);
      setModelSortDir(key === "model" ? "asc" : "desc");
    }
    if (key === "cost" || key === "tokens" || key === "cache" || key === "speed") {
      setToolbarSort(key);
    }
  };

  const toggleProjectSort = (key: ProjectSortKey) => {
    if (projectSortKey === key) {
      setProjectSortDir((prev) => (prev === "desc" ? "asc" : "desc"));
    } else {
      setProjectSortKey(key);
      setProjectSortDir(key === "project" ? "asc" : "desc");
    }
  };

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
    const q = deferredQuery.trim().toLowerCase();
    const filtered = q
      ? rows.filter(
          (r) =>
            r.model.toLowerCase().includes(q) || r.sourceName.toLowerCase().includes(q)
        )
      : rows;

    const dirMultiplier = modelSortDir === "asc" ? 1 : -1;

    return [...filtered].sort((a, b) => {
      if (modelSortKey === "model") {
        const cmp = `${a.model}:${a.sourceName}`.localeCompare(`${b.model}:${b.sourceName}`);
        return cmp * dirMultiplier;
      }
      const metric = (row: FlattenedModelRow): number => {
        switch (modelSortKey) {
          case "events":
            return row.events;
          case "base_input":
            return calcBaseInput(row.input_tokens, row.cached_tokens, row.cache_creation_tokens);
          case "cached":
            return row.cached_tokens;
          case "cache_write":
            return row.cache_creation_tokens;
          case "output":
            return row.output_tokens;
          case "tokens":
            return row.input_tokens + row.output_tokens;
          case "cache":
            return row.cache_hit_rate;
          case "speed":
            return tokenRatePerSecond(row.timed_output_tokens, row.duration_ms) ?? -1;
          case "cost":
          default:
            return row.total_cost;
        }
      };
      if (modelSortKey === "speed") {
        const ra = tokenRatePerSecond(a.timed_output_tokens, a.duration_ms);
        const rb = tokenRatePerSecond(b.timed_output_tokens, b.duration_ms);
        if (ra === null && rb === null) return 0;
        if (ra === null) return 1;
        if (rb === null) return -1;
        return (ra - rb) * dirMultiplier;
      }
      return (metric(a) - metric(b)) * dirMultiplier;
    });
  }, [sources, deferredQuery, modelSortKey, modelSortDir]);

  const totalModelCost = useMemo(
    () =>
      (sources ?? []).reduce(
        (sum, src) =>
          sum + (src.models ?? []).reduce((mSum, m) => mSum + (m.total_cost || 0), 0),
        0
      ),
    [sources]
  );

  const filteredProjects = useMemo(() => {
    const list = projects ?? [];
    const q = deferredQuery.trim().toLowerCase();
    const filtered = q ? list.filter((p) => p.project.toLowerCase().includes(q)) : list;
    const dirMultiplier = projectSortDir === "asc" ? 1 : -1;

    return [...filtered].sort((a, b) => {
      if (projectSortKey === "project") {
        return a.project.localeCompare(b.project) * dirMultiplier;
      }
      const metric = (row: ProjectStat): number => {
        switch (projectSortKey) {
          case "events":
            return row.events;
          case "base_input":
            return calcBaseInput(row.input_tokens, row.cached_tokens, row.cache_creation_tokens);
          case "cached":
            return row.cached_tokens;
          case "output":
            return row.output_tokens;
          case "tokens":
            return row.input_tokens + row.output_tokens;
          case "cache":
            return row.cache_hit_rate;
          case "share":
          case "cost":
          default:
            return row.total_cost;
        }
      };
      return (metric(a) - metric(b)) * dirMultiplier;
    });
  }, [projects, deferredQuery, projectSortKey, projectSortDir]);

  const totalProjectCost = useMemo(
    () => (projects ?? []).reduce((acc, p) => acc + (p.total_cost || 0), 0),
    [projects]
  );

  const visibleModels = showAllModels ? flattenedModels : flattenedModels.slice(0, PAGE_SIZE);
  const visibleProjects = showAllProjects ? filteredProjects : filteredProjects.slice(0, PAGE_SIZE);

  return (
    <div className="space-y-6">
      {/* Shared Search & Sort Toolbar */}
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="relative w-full sm:w-72">
          <Search className="pointer-events-none absolute top-1/2 left-3 h-3.5 w-3.5 -translate-y-1/2 text-slate-400 dark:text-slate-500" />
          <input
            type="text"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder={t("tables.search_placeholder")}
            className="w-full rounded-lg border border-slate-200 bg-white py-1.5 pr-3 pl-8 text-xs text-slate-800 placeholder:text-slate-400 focus:border-slate-900 focus:outline-none dark:border-slate-700 dark:bg-slate-900 dark:text-slate-100 dark:placeholder:text-slate-500 dark:focus:border-sky-400"
          />
        </div>

        <div className="flex flex-wrap items-center gap-1.5 text-[11px]">
          <span className="font-semibold tracking-wider text-slate-400 uppercase dark:text-slate-500">
            {t("tables.sort_label")}
          </span>
          {(["cost", "tokens", "cache", "speed"] as const).map((key) => (
            <button
              key={key}
              type="button"
              onClick={() => handleToolbarSort(key)}
              aria-pressed={toolbarSort === key}
              className={`rounded-md border px-2.5 py-1 font-medium transition-colors ${
                toolbarSort === key
                  ? "border-slate-900 bg-slate-900 text-white dark:border-slate-200 dark:bg-slate-100 dark:text-slate-950"
                  : "border-slate-200 bg-white text-slate-600 hover:bg-slate-50 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-300 dark:hover:bg-slate-800"
              }`}
            >
              {key === "cost"
                ? "COST"
                : key === "tokens"
                  ? "TOKENS"
                  : key === "cache"
                    ? "CACHE HIT"
                    : "SPEED"}
            </button>
          ))}
        </div>
      </div>

      {/* 1. Model Attribution Table */}
      <section className="overflow-hidden rounded-xl border border-slate-200/90 bg-white shadow-[0_1px_2px_rgba(15,23,42,0.03)] dark:border-slate-800 dark:bg-slate-900 dark:shadow-none">
        <div className="flex flex-wrap items-center justify-between gap-2 border-b border-slate-100 px-5 py-4 dark:border-slate-800">
          <div>
            <div className="text-[11px] font-semibold tracking-wider text-slate-400 uppercase dark:text-slate-500">
              {t("tables.models_sub")}
            </div>
            <h3 className="mt-0.5 text-sm font-semibold text-slate-900 dark:text-slate-100">
              {t("tables.models_title")}
            </h3>
          </div>
          <span className="font-mono-num rounded-md bg-slate-100 px-2 py-0.5 text-[11px] font-medium text-slate-600 dark:bg-slate-800 dark:text-slate-300">
            {t("tables.models_count", { period: windowLabel, count: flattenedModels.length })}
          </span>
        </div>

        <div className="max-h-[580px] overflow-auto">
          <table className="w-full border-collapse text-left text-xs">
            <thead>
              <tr className="border-b border-slate-100 bg-slate-50/95 text-[10px] font-semibold tracking-wider text-slate-400 uppercase dark:border-slate-800 dark:bg-slate-900/95 dark:text-slate-500">
                <th
                  aria-sort={
                    modelSortKey === "model"
                      ? modelSortDir === "asc"
                        ? "ascending"
                        : "descending"
                      : "none"
                  }
                  className="sticky top-0 z-10 bg-slate-50/95 py-2.5 pr-3 pl-5 backdrop-blur-xs dark:bg-slate-900/95"
                >
                  <SortHeaderButton
                    label={t("tables.col_model")}
                    active={modelSortKey === "model"}
                    dir={modelSortDir}
                    align="left"
                    onClick={() => toggleModelSort("model")}
                  />
                </th>
                <th
                  aria-sort={
                    modelSortKey === "events"
                      ? modelSortDir === "asc"
                        ? "ascending"
                        : "descending"
                      : "none"
                  }
                  className="sticky top-0 z-10 bg-slate-50/95 px-3 py-2.5 text-right backdrop-blur-xs dark:bg-slate-900/95"
                >
                  <SortHeaderButton
                    label={t("tables.col_events")}
                    active={modelSortKey === "events"}
                    dir={modelSortDir}
                    onClick={() => toggleModelSort("events")}
                  />
                </th>
                <th
                  aria-sort={
                    modelSortKey === "base_input"
                      ? modelSortDir === "asc"
                        ? "ascending"
                        : "descending"
                      : "none"
                  }
                  className="sticky top-0 z-10 bg-slate-50/95 px-3 py-2.5 text-right backdrop-blur-xs dark:bg-slate-900/95"
                >
                  <SortHeaderButton
                    label={t("tables.col_base_input")}
                    active={modelSortKey === "base_input"}
                    dir={modelSortDir}
                    onClick={() => toggleModelSort("base_input")}
                  />
                </th>
                <th
                  aria-sort={
                    modelSortKey === "cached"
                      ? modelSortDir === "asc"
                        ? "ascending"
                        : "descending"
                      : "none"
                  }
                  className="sticky top-0 z-10 bg-slate-50/95 px-3 py-2.5 text-right backdrop-blur-xs dark:bg-slate-900/95"
                >
                  <SortHeaderButton
                    label={t("tables.col_cached")}
                    active={modelSortKey === "cached"}
                    dir={modelSortDir}
                    onClick={() => toggleModelSort("cached")}
                  />
                </th>
                <th
                  aria-sort={
                    modelSortKey === "cache_write"
                      ? modelSortDir === "asc"
                        ? "ascending"
                        : "descending"
                      : "none"
                  }
                  className="sticky top-0 z-10 bg-slate-50/95 px-3 py-2.5 text-right backdrop-blur-xs dark:bg-slate-900/95"
                >
                  <SortHeaderButton
                    label={t("tables.col_cache_write")}
                    active={modelSortKey === "cache_write"}
                    dir={modelSortDir}
                    onClick={() => toggleModelSort("cache_write")}
                  />
                </th>
                <th
                  aria-sort={
                    modelSortKey === "output"
                      ? modelSortDir === "asc"
                        ? "ascending"
                        : "descending"
                      : "none"
                  }
                  className="sticky top-0 z-10 bg-slate-50/95 px-3 py-2.5 text-right backdrop-blur-xs dark:bg-slate-900/95"
                >
                  <SortHeaderButton
                    label={t("tables.col_output")}
                    active={modelSortKey === "output"}
                    dir={modelSortDir}
                    onClick={() => toggleModelSort("output")}
                  />
                </th>
                <th
                  aria-sort={
                    modelSortKey === "speed"
                      ? modelSortDir === "asc"
                        ? "ascending"
                        : "descending"
                      : "none"
                  }
                  className="sticky top-0 z-10 bg-slate-50/95 px-3 py-2.5 text-right backdrop-blur-xs dark:bg-slate-900/95"
                >
                  <SortHeaderButton
                    label={t("tables.col_speed")}
                    active={modelSortKey === "speed"}
                    dir={modelSortDir}
                    onClick={() => toggleModelSort("speed")}
                  />
                </th>
                <th
                  aria-sort={
                    modelSortKey === "cache"
                      ? modelSortDir === "asc"
                        ? "ascending"
                        : "descending"
                      : "none"
                  }
                  className="sticky top-0 z-10 bg-slate-50/95 px-3 py-2.5 text-right backdrop-blur-xs dark:bg-slate-900/95"
                >
                  <SortHeaderButton
                    label={t("tables.col_hit_rate")}
                    active={modelSortKey === "cache"}
                    dir={modelSortDir}
                    onClick={() => toggleModelSort("cache")}
                  />
                </th>
                <th className="sticky top-0 z-10 bg-slate-50/95 px-3 py-2.5 text-right backdrop-blur-xs dark:bg-slate-900/95">
                  {t("tables.col_unit_price")}
                </th>
                <th
                  aria-sort={
                    modelSortKey === "cost"
                      ? modelSortDir === "asc"
                        ? "ascending"
                        : "descending"
                      : "none"
                  }
                  className="sticky top-0 z-10 bg-slate-50/95 py-2.5 pr-5 pl-3 text-right backdrop-blur-xs dark:bg-slate-900/95"
                >
                  <SortHeaderButton
                    label={t("tables.col_cost")}
                    active={modelSortKey === "cost"}
                    dir={modelSortDir}
                    onClick={() => toggleModelSort("cost")}
                  />
                </th>
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-100 dark:divide-slate-800/80">
              {isHydratingDetails && flattenedModels.length === 0 ? (
                Array.from({ length: 4 }).map((_, idx) => (
                  <tr key={idx} className="animate-pulse">
                    <td className="py-3.5 pr-3 pl-5">
                      <div className="h-4 w-44 rounded bg-slate-100 dark:bg-slate-800" />
                    </td>
                    <td className="px-3 py-3.5">
                      <div className="ml-auto h-3.5 w-10 rounded bg-slate-100 dark:bg-slate-800" />
                    </td>
                    <td className="px-3 py-3.5">
                      <div className="ml-auto h-3.5 w-14 rounded bg-slate-100 dark:bg-slate-800" />
                    </td>
                    <td className="px-3 py-3.5">
                      <div className="ml-auto h-3.5 w-14 rounded bg-slate-100 dark:bg-slate-800" />
                    </td>
                    <td className="px-3 py-3.5">
                      <div className="ml-auto h-3.5 w-12 rounded bg-slate-100 dark:bg-slate-800" />
                    </td>
                    <td className="px-3 py-3.5">
                      <div className="ml-auto h-3.5 w-14 rounded bg-slate-100 dark:bg-slate-800" />
                    </td>
                    <td className="px-3 py-3.5">
                      <div className="ml-auto h-3.5 w-14 rounded bg-slate-100 dark:bg-slate-800" />
                    </td>
                    <td className="px-3 py-3.5">
                      <div className="ml-auto h-3.5 w-16 rounded bg-slate-100 dark:bg-slate-800" />
                    </td>
                    <td className="px-3 py-3.5">
                      <div className="ml-auto h-3.5 w-20 rounded bg-slate-100 dark:bg-slate-800" />
                    </td>
                    <td className="py-3.5 pr-5 pl-3">
                      <div className="ml-auto h-4 w-14 rounded bg-slate-100 dark:bg-slate-800" />
                    </td>
                  </tr>
                ))
              ) : visibleModels.length === 0 ? (
                <tr>
                  <td
                    colSpan={10}
                    className="py-10 text-center text-xs text-slate-400 dark:text-slate-500"
                  >
                    {t("tables.empty_models")}
                  </td>
                </tr>
              ) : (
                visibleModels.map((row) => {
                  const baseInput = calcBaseInput(
                    row.input_tokens,
                    row.cached_tokens,
                    row.cache_creation_tokens
                  );
                  const costSharePct =
                    totalModelCost > 0 ? (row.total_cost / totalModelCost) * 100 : 0;
                  return (
                    <tr
                      key={`${row.sourceName}-${row.model}`}
                      data-testid="model-attribution-row"
                      className="transition-colors hover:bg-slate-50/80 dark:hover:bg-slate-800/50"
                    >
                      <td className="py-3 pr-3 pl-5">
                        <div className="flex flex-wrap items-center gap-2">
                          <span className="font-mono-num font-semibold text-slate-900 dark:text-slate-100">
                            {row.model}
                          </span>
                          <SourceBadge source={row.sourceName} />
                        </div>
                      </td>
                      <td className="font-mono-num px-3 py-3 text-right text-slate-500 dark:text-slate-400">
                        {formatCompactNumber(row.events)}
                      </td>
                      <td
                        className="font-mono-num px-3 py-3 text-right font-medium text-slate-800 dark:text-slate-200"
                        title={formatExactNumber(baseInput)}
                      >
                        {formatCompactNumber(baseInput)}
                      </td>
                      <td
                        className="font-mono-num px-3 py-3 text-right font-medium text-emerald-700 dark:text-emerald-400"
                        title={formatExactNumber(row.cached_tokens)}
                      >
                        {formatCompactNumber(row.cached_tokens)}
                      </td>
                      <td
                        className="font-mono-num px-3 py-3 text-right text-slate-500 dark:text-slate-400"
                        title={formatExactNumber(row.cache_creation_tokens)}
                      >
                        {formatCompactNumber(row.cache_creation_tokens)}
                      </td>
                      <td
                        className="font-mono-num px-3 py-3 text-right text-slate-700 dark:text-slate-300"
                        title={formatExactNumber(row.output_tokens)}
                      >
                        {formatCompactNumber(row.output_tokens)}
                      </td>
                      <td
                        className="font-mono-num px-3 py-3 text-right text-slate-700 dark:text-slate-300"
                        title={
                          row.timed_events > 0
                            ? t("tables.speed_tooltip", { events: row.timed_events })
                            : undefined
                        }
                      >
                        {formatTokenRate(row.timed_output_tokens, row.duration_ms)}
                        {tokenRatePerSecond(row.timed_output_tokens, row.duration_ms) !== null
                          ? " tok/s"
                          : ""}
                      </td>
                      <td className="px-3 py-3 text-right">
                        <div className="inline-flex items-center justify-end gap-2">
                          <div className="h-1.5 w-12 overflow-hidden rounded-full bg-slate-100 dark:bg-slate-800">
                            <div
                              style={{ width: `${Math.min(100, row.cache_hit_rate)}%` }}
                              className="h-full rounded-full bg-emerald-500 dark:bg-emerald-400"
                            />
                          </div>
                          <span className="font-mono-num w-11 text-right font-medium text-slate-700 dark:text-slate-300">
                            {formatPercent(row.cache_hit_rate)}
                          </span>
                        </div>
                      </td>
                      <td className="font-mono-num px-3 py-3 text-right text-[11px] text-slate-400 dark:text-slate-500">
                        {formatUnitPrice(row.input_price_per_m, currency)} /{" "}
                        {formatUnitPrice(row.cached_price_per_m, currency)} /{" "}
                        {formatUnitPrice(row.cache_creation_price_per_m, currency)} /{" "}
                        {formatUnitPrice(row.output_price_per_m, currency)}
                      </td>
                      <td className="py-3 pr-5 pl-3 text-right">
                        <div className="inline-flex flex-col items-end gap-1">
                          <span className="font-mono-num font-semibold text-slate-900 dark:text-slate-100">
                            {formatCost(row.total_cost, currency)}
                          </span>
                          <div
                            className="h-1 w-14 overflow-hidden rounded-full bg-slate-100 dark:bg-slate-800"
                            aria-hidden="true"
                          >
                            <div
                              style={{
                                width: `${Math.min(100, Math.max(costSharePct > 0 ? 3 : 0, costSharePct))}%`,
                              }}
                              className="h-full rounded-full bg-slate-700 dark:bg-sky-400"
                            />
                          </div>
                        </div>
                      </td>
                    </tr>
                  );
                })
              )}
            </tbody>
          </table>
        </div>

        {flattenedModels.length > PAGE_SIZE && (
          <div className="flex items-center justify-between border-t border-slate-100 bg-slate-50/60 px-5 py-2.5 text-xs dark:border-slate-800 dark:bg-slate-900/70">
            <span className="font-mono-num text-[11px] text-slate-500 dark:text-slate-400">
              {t("tables.showing_cap", {
                shown: visibleModels.length,
                total: flattenedModels.length,
              })}
            </span>
            <button
              type="button"
              onClick={() => setShowAllModels((prev) => !prev)}
              className="rounded-md border border-slate-200 bg-white px-2.5 py-1 text-[11px] font-semibold text-slate-700 transition-colors hover:border-slate-300 hover:bg-slate-100 dark:border-slate-700 dark:bg-slate-800 dark:text-slate-200 dark:hover:bg-slate-700"
            >
              {showAllModels
                ? t("tables.show_less", { count: PAGE_SIZE })
                : t("tables.show_all", { count: flattenedModels.length })}
            </button>
          </div>
        )}
      </section>

      {/* 2. Project Workspace Attribution Table */}
      <section className="overflow-hidden rounded-xl border border-slate-200/90 bg-white shadow-[0_1px_2px_rgba(15,23,42,0.03)] dark:border-slate-800 dark:bg-slate-900 dark:shadow-none">
        <div className="flex flex-wrap items-center justify-between gap-2 border-b border-slate-100 px-5 py-4 dark:border-slate-800">
          <div>
            <div className="text-[11px] font-semibold tracking-wider text-slate-400 uppercase dark:text-slate-500">
              {t("tables.projects_sub")}
            </div>
            <h3 className="mt-0.5 text-sm font-semibold text-slate-900 dark:text-slate-100">
              {t("tables.projects_title")}
            </h3>
          </div>
          <span className="font-mono-num rounded-md bg-slate-100 px-2 py-0.5 text-[11px] font-medium text-slate-600 dark:bg-slate-800 dark:text-slate-300">
            {t("tables.projects_count", { period: windowLabel, count: filteredProjects.length })}
          </span>
        </div>

        <div className="max-h-[580px] overflow-auto">
          <table className="w-full border-collapse text-left text-xs">
            <thead>
              <tr className="border-b border-slate-100 bg-slate-50/95 text-[10px] font-semibold tracking-wider text-slate-400 uppercase dark:border-slate-800 dark:bg-slate-900/95 dark:text-slate-500">
                <th
                  aria-sort={
                    projectSortKey === "project"
                      ? projectSortDir === "asc"
                        ? "ascending"
                        : "descending"
                      : "none"
                  }
                  className="sticky top-0 z-10 bg-slate-50/95 py-2.5 pr-3 pl-5 backdrop-blur-xs dark:bg-slate-900/95"
                >
                  <SortHeaderButton
                    label={t("tables.col_project")}
                    active={projectSortKey === "project"}
                    dir={projectSortDir}
                    align="left"
                    onClick={() => toggleProjectSort("project")}
                  />
                </th>
                <th
                  aria-sort={
                    projectSortKey === "events"
                      ? projectSortDir === "asc"
                        ? "ascending"
                        : "descending"
                      : "none"
                  }
                  className="sticky top-0 z-10 bg-slate-50/95 px-3 py-2.5 text-right backdrop-blur-xs dark:bg-slate-900/95"
                >
                  <SortHeaderButton
                    label={t("tables.col_events")}
                    active={projectSortKey === "events"}
                    dir={projectSortDir}
                    onClick={() => toggleProjectSort("events")}
                  />
                </th>
                <th
                  aria-sort={
                    projectSortKey === "base_input"
                      ? projectSortDir === "asc"
                        ? "ascending"
                        : "descending"
                      : "none"
                  }
                  className="sticky top-0 z-10 bg-slate-50/95 px-3 py-2.5 text-right backdrop-blur-xs dark:bg-slate-900/95"
                >
                  <SortHeaderButton
                    label={t("tables.col_base_input")}
                    active={projectSortKey === "base_input"}
                    dir={projectSortDir}
                    onClick={() => toggleProjectSort("base_input")}
                  />
                </th>
                <th
                  aria-sort={
                    projectSortKey === "cached"
                      ? projectSortDir === "asc"
                        ? "ascending"
                        : "descending"
                      : "none"
                  }
                  className="sticky top-0 z-10 bg-slate-50/95 px-3 py-2.5 text-right backdrop-blur-xs dark:bg-slate-900/95"
                >
                  <SortHeaderButton
                    label={t("tables.col_cached")}
                    active={projectSortKey === "cached"}
                    dir={projectSortDir}
                    onClick={() => toggleProjectSort("cached")}
                  />
                </th>
                <th
                  aria-sort={
                    projectSortKey === "output"
                      ? projectSortDir === "asc"
                        ? "ascending"
                        : "descending"
                      : "none"
                  }
                  className="sticky top-0 z-10 bg-slate-50/95 px-3 py-2.5 text-right backdrop-blur-xs dark:bg-slate-900/95"
                >
                  <SortHeaderButton
                    label={t("tables.col_output")}
                    active={projectSortKey === "output"}
                    dir={projectSortDir}
                    onClick={() => toggleProjectSort("output")}
                  />
                </th>
                <th
                  aria-sort={
                    projectSortKey === "cache"
                      ? projectSortDir === "asc"
                        ? "ascending"
                        : "descending"
                      : "none"
                  }
                  className="sticky top-0 z-10 bg-slate-50/95 px-3 py-2.5 text-right backdrop-blur-xs dark:bg-slate-900/95"
                >
                  <SortHeaderButton
                    label={t("tables.col_hit_rate")}
                    active={projectSortKey === "cache"}
                    dir={projectSortDir}
                    onClick={() => toggleProjectSort("cache")}
                  />
                </th>
                <th
                  aria-sort={
                    projectSortKey === "share"
                      ? projectSortDir === "asc"
                        ? "ascending"
                        : "descending"
                      : "none"
                  }
                  className="sticky top-0 z-10 bg-slate-50/95 px-3 py-2.5 text-right backdrop-blur-xs dark:bg-slate-900/95"
                >
                  <SortHeaderButton
                    label={t("tables.col_share")}
                    active={projectSortKey === "share"}
                    dir={projectSortDir}
                    onClick={() => toggleProjectSort("share")}
                  />
                </th>
                <th
                  aria-sort={
                    projectSortKey === "cost"
                      ? projectSortDir === "asc"
                        ? "ascending"
                        : "descending"
                      : "none"
                  }
                  className="sticky top-0 z-10 bg-slate-50/95 py-2.5 pr-5 pl-3 text-right backdrop-blur-xs dark:bg-slate-900/95"
                >
                  <SortHeaderButton
                    label={t("tables.col_cost")}
                    active={projectSortKey === "cost"}
                    dir={projectSortDir}
                    onClick={() => toggleProjectSort("cost")}
                  />
                </th>
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-100 dark:divide-slate-800/80">
              {isHydratingDetails && !projects ? (
                Array.from({ length: 3 }).map((_, idx) => (
                  <tr key={idx} className="animate-pulse">
                    <td className="py-3.5 pr-3 pl-5">
                      <div className="h-4 w-40 rounded bg-slate-100 dark:bg-slate-800" />
                    </td>
                    <td className="px-3 py-3.5">
                      <div className="ml-auto h-3.5 w-10 rounded bg-slate-100 dark:bg-slate-800" />
                    </td>
                    <td className="px-3 py-3.5">
                      <div className="ml-auto h-3.5 w-14 rounded bg-slate-100 dark:bg-slate-800" />
                    </td>
                    <td className="px-3 py-3.5">
                      <div className="ml-auto h-3.5 w-14 rounded bg-slate-100 dark:bg-slate-800" />
                    </td>
                    <td className="px-3 py-3.5">
                      <div className="ml-auto h-3.5 w-14 rounded bg-slate-100 dark:bg-slate-800" />
                    </td>
                    <td className="px-3 py-3.5">
                      <div className="ml-auto h-3.5 w-12 rounded bg-slate-100 dark:bg-slate-800" />
                    </td>
                    <td className="px-3 py-3.5">
                      <div className="ml-auto h-3.5 w-16 rounded bg-slate-100 dark:bg-slate-800" />
                    </td>
                    <td className="py-3.5 pr-5 pl-3">
                      <div className="ml-auto h-4 w-14 rounded bg-slate-100 dark:bg-slate-800" />
                    </td>
                  </tr>
                ))
              ) : visibleProjects.length === 0 ? (
                <tr>
                  <td
                    colSpan={8}
                    className="py-10 text-center text-xs text-slate-400 dark:text-slate-500"
                  >
                    {t("tables.empty_projects")}
                  </td>
                </tr>
              ) : (
                visibleProjects.map((proj) => {
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
                      data-testid="project-attribution-row"
                      className="transition-colors hover:bg-slate-50/80 dark:hover:bg-slate-800/50"
                    >
                      <td className="py-3 pr-3 pl-5">
                        <div className="flex items-center gap-2.5">
                          <div className="flex h-7 w-7 shrink-0 items-center justify-center rounded-md border border-slate-200 bg-slate-50 text-slate-600 dark:border-slate-700 dark:bg-slate-800 dark:text-slate-300">
                            <FolderGit2 className="h-3.5 w-3.5" />
                          </div>
                          <span className="font-mono-num font-semibold text-slate-900 dark:text-slate-100">
                            {proj.project}
                          </span>
                        </div>
                      </td>
                      <td className="font-mono-num px-3 py-3 text-right text-slate-500 dark:text-slate-400">
                        {formatCompactNumber(proj.events)}
                      </td>
                      <td
                        className="font-mono-num px-3 py-3 text-right font-medium text-slate-800 dark:text-slate-200"
                        title={formatExactNumber(baseInput)}
                      >
                        {formatCompactNumber(baseInput)}
                      </td>
                      <td
                        className="font-mono-num px-3 py-3 text-right font-medium text-emerald-700 dark:text-emerald-400"
                        title={formatExactNumber(proj.cached_tokens)}
                      >
                        {formatCompactNumber(proj.cached_tokens)}
                      </td>
                      <td
                        className="font-mono-num px-3 py-3 text-right text-slate-700 dark:text-slate-300"
                        title={formatExactNumber(proj.output_tokens)}
                      >
                        {formatCompactNumber(proj.output_tokens)}
                      </td>
                      <td className="font-mono-num px-3 py-3 text-right font-medium text-slate-700 dark:text-slate-300">
                        {formatPercent(proj.cache_hit_rate)}
                      </td>
                      <td className="px-3 py-3 text-right">
                        <div className="inline-flex items-center justify-end gap-2">
                          <div className="h-1.5 w-14 overflow-hidden rounded-full bg-slate-100 dark:bg-slate-800">
                            <div
                              style={{ width: `${Math.min(100, costSharePct)}%` }}
                              className="h-full rounded-full bg-slate-800 dark:bg-sky-400"
                            />
                          </div>
                          <span className="font-mono-num w-10 text-right text-slate-500 dark:text-slate-400">
                            {costSharePct.toFixed(0)}%
                          </span>
                        </div>
                      </td>
                      <td className="font-mono-num py-3 pr-5 pl-3 text-right font-semibold text-slate-900 dark:text-slate-100">
                        {formatCost(proj.total_cost, currency)}
                      </td>
                    </tr>
                  );
                })
              )}
            </tbody>
          </table>
        </div>

        {filteredProjects.length > PAGE_SIZE && (
          <div className="flex items-center justify-between border-t border-slate-100 bg-slate-50/60 px-5 py-2.5 text-xs dark:border-slate-800 dark:bg-slate-900/70">
            <span className="font-mono-num text-[11px] text-slate-500 dark:text-slate-400">
              {t("tables.showing_cap", {
                shown: visibleProjects.length,
                total: filteredProjects.length,
              })}
            </span>
            <button
              type="button"
              onClick={() => setShowAllProjects((prev) => !prev)}
              className="rounded-md border border-slate-200 bg-white px-2.5 py-1 text-[11px] font-semibold text-slate-700 transition-colors hover:border-slate-300 hover:bg-slate-100 dark:border-slate-700 dark:bg-slate-800 dark:text-slate-200 dark:hover:bg-slate-700"
            >
              {showAllProjects
                ? t("tables.show_less", { count: PAGE_SIZE })
                : t("tables.show_all", { count: filteredProjects.length })}
            </button>
          </div>
        )}
      </section>
    </div>
  );
});
