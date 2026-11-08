import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import {
  Activity,
  AlertTriangle,
  DollarSign,
  Globe,
  Laptop,
  Loader2,
  Package,
  Pause,
  Play,
  Radio,
  RefreshCw,
  ShieldCheck,
  Sliders,
} from "lucide-react";
import { fetchSources, fetchStats, isDesktopRuntime, togglePause } from "./api/client";
import { ActivityHeatmap } from "./components/ActivityHeatmap";
import { AttributionTables } from "./components/AttributionTables";
import { DailyCostChart } from "./components/DailyCostChart";
import { DrawerType, SlideOverDrawers } from "./components/SlideOverDrawers";
import { SourceIcon } from "./components/SourceBadge";
import { SourceShareChart } from "./components/SourceShareChart";
import { TokenTaxonomyStrip } from "./components/TokenTaxonomyStrip";
import {
  CurrencyCode,
  DashboardData,
  PeriodStats,
  SourceDescriptor,
  TIME_WINDOWS,
  TimeWindowLabel,
} from "./types/telemetry";
import {
  calcBaseInput,
  EXCHANGE_RATES,
  formatCompactNumber,
  formatCost,
  formatExactNumber,
  formatPercent,
} from "./utils/formatters";

const DEFAULT_SOURCES = ["Claude Code", "Gemini CLI", "Codex", "Antigravity"];

export function App() {
  const { t, i18n } = useTranslation();

  // Global Scope & View State
  const [selectedDevice, setSelectedDevice] = useState<string>("all");
  const [selectedSource, setSelectedSource] = useState<string>("");
  const [selectedWindow, setSelectedWindow] = useState<TimeWindowLabel>("30d");
  const [currency, setCurrency] = useState<CurrencyCode>("USD");
  const [activeDrawer, setActiveDrawer] = useState<DrawerType>(null);

  // Two-stage Telemetry State
  const [data, setData] = useState<DashboardData | null>(null);
  const [sourceDescriptors, setSourceDescriptors] = useState<SourceDescriptor[]>([]);
  const [isHydratingDetails, setIsHydratingDetails] = useState<boolean>(true);
  const [loadError, setLoadError] = useState<string>("");
  const [pauseBusy, setPauseBusy] = useState<boolean>(false);
  const [updateBanner, setUpdateBanner] = useState<{ version: string } | null>(null);
  const [dismissedUpdate, setDismissedUpdate] = useState<boolean>(false);

  useEffect(() => {
    fetch("/api/system/update/check")
      .then((res) => (res.ok ? res.json() : null))
      .then((data: { has_update?: boolean; latest_version?: string } | null) => {
        if (data && data.has_update && data.latest_version) {
          setUpdateBanner({ version: data.latest_version });
        }
      })
      .catch(() => {});
  }, []);

  const requestSeqRef = useRef<number>(0);

  const refreshSources = useCallback(() => {
    fetchSources()
      .then((rows) => setSourceDescriptors(rows))
      .catch(() => undefined);
  }, []);

  useEffect(() => {
    refreshSources();
  }, [refreshSources]);

  /**
   * Two-stage progressive fetch:
   * Stage 1: detail=summary -> instant first paint for KPIs & charts
   * Stage 2: detail=details -> hydrates models & projects asynchronously
   *
   * The details response intentionally omits periods, charts and devices, so it
   * must be merged into the last summary rather than replacing it.
   */
  const loadTelemetry = useCallback(
    async (isBackgroundPoll = false) => {
      const seq = ++requestSeqRef.current;

      try {
        if (!isBackgroundPoll) {
          setIsHydratingDetails(true);
        }
        const summaryRes = await fetchStats({
          device: selectedDevice,
          source: selectedSource,
          detail: "summary",
        });
        if (seq !== requestSeqRef.current) return;
        setData((prev) => {
          const previousModels = new Map(
            (prev?.sources ?? []).map((source) => [source.name, source.models])
          );
          return {
            ...summaryRes,
            sources: summaryRes.sources.map((source) => {
              const models = source.models.length
                ? source.models
                : previousModels.get(source.name) ?? [];
              return { ...source, models };
            }),
            projects: prev?.projects,
          };
        });
        setLoadError("");

        const detailsRes = await fetchStats({
          device: selectedDevice,
          source: selectedSource,
          detail: "details",
        });
        if (seq !== requestSeqRef.current) return;
        setData((prev) =>
          prev
            ? {
                ...prev,
                sources:
                  detailsRes.sources.length > 0 ? detailsRes.sources : prev.sources,
                projects: detailsRes.projects ?? [],
                is_paused: detailsRes.is_paused ?? prev.is_paused,
              }
            : detailsRes
        );
        setIsHydratingDetails(false);
        setLoadError("");
      } catch (error: unknown) {
        if (seq !== requestSeqRef.current) return;
        setLoadError(error instanceof Error ? error.message : String(error));
        setIsHydratingDetails(false);
      }
    },
    [selectedDevice, selectedSource]
  );

  // Initial & filter-change two-stage load
  useEffect(() => {
    loadTelemetry(false);
  }, [loadTelemetry]);

  // 5-second main telemetry polling
  useEffect(() => {
    const interval = setInterval(() => {
      loadTelemetry(true);
    }, 5000);
    return () => clearInterval(interval);
  }, [loadTelemetry]);

  const handleTogglePause = async () => {
    setPauseBusy(true);
    try {
      const res = await togglePause();
      setData((prev) => (prev ? { ...prev, is_paused: res.is_paused } : prev));
    } finally {
      setPauseBusy(false);
    }
  };

  const handleLanguageSwitch = () => {
    const next = i18n.language === "zh" ? "en" : "zh";
    i18n.changeLanguage(next);
    localStorage.setItem("ai-flight-dashboard_lang", next);
  };

  // Active time window stats directly from backend periods array
  const currentPeriod: PeriodStats = useMemo(() => {
    const periods = data?.periods ?? [];
    return (
      periods.find((p) => p.label === selectedWindow) ??
      periods.find((p) => p.label === "30d") ??
      periods[0] ?? {
        label: selectedWindow,
        input_tokens: 0,
        cached_tokens: 0,
        cache_creation_tokens: 0,
        output_tokens: 0,
        cost: 0,
        cache_hit_rate: 0,
      }
    );
  }, [data, selectedWindow]);

  // Dynamic list of CLI sources (built-ins + plugin sources + any source that reported data)
  const availableSources = useMemo(() => {
    const set = new Set<string>(DEFAULT_SOURCES);
    for (const descriptor of sourceDescriptors) {
      if (descriptor.source) set.add(descriptor.source);
    }
    for (const s of data?.sources ?? []) {
      if (s.name) set.add(s.name);
    }
    for (const ts of data?.charts?.tool_share ?? []) {
      if (ts.source) set.add(ts.source);
    }
    return Array.from(set);
  }, [data, sourceDescriptors]);

  // Derived KPI metrics
  const baseInputTokens = calcBaseInput(
    currentPeriod.input_tokens,
    currentPeriod.cached_tokens,
    currentPeriod.cache_creation_tokens
  );
  const totalVolumeTokens =
    (currentPeriod.input_tokens || 0) + (currentPeriod.output_tokens || 0);
  const blendedCostPerM =
    totalVolumeTokens > 0 ? (currentPeriod.cost / totalVolumeTokens) * 1_000_000 : 0;

  // Estimated USD savings from cache hits (cached tokens billed at ~10% vs full input rate)
  const estCacheSavingsUsd = useMemo(() => {
    const cached = currentPeriod.cached_tokens || 0;
    // Conservative blended $2.70/1M saved differential
    return (cached / 1_000_000) * 2.7;
  }, [currentPeriod.cached_tokens]);

  const isPaused = Boolean(data?.is_paused);
  const isDesktop = isDesktopRuntime();

  // Radial gauge geometry for Cache Hit Rate KPI
  const gaugeRadius = 18;
  const gaugeCircumference = 2 * Math.PI * gaugeRadius;
  const clampedHitRate = Math.min(100, Math.max(0, currentPeriod.cache_hit_rate || 0));
  const gaugeDashOffset =
    gaugeCircumference - (clampedHitRate / 100) * gaugeCircumference;

  if (!data) {
    return (
      <div className={`min-h-screen bg-[#F8FAFC] text-[#0F172A] ${isDesktop ? "pt-8" : ""}`}>
        {isDesktop && (
          <div
            className="wails-drag fixed top-0 left-0 z-40 h-8 w-full bg-[#F8FAFC]"
            aria-hidden="true"
          />
        )}
        <div className="flex min-h-[80vh] items-center justify-center px-4">
          {loadError ? (
            <div
              role="alert"
              className="w-full max-w-lg rounded-xl border border-rose-200 bg-white p-6 shadow-sm"
            >
              <div className="flex items-center gap-2 text-rose-700">
                <AlertTriangle className="h-5 w-5" />
                <h1 className="text-sm font-semibold">{t("app.load_error_title")}</h1>
              </div>
              <p className="mt-3 break-all rounded-lg bg-rose-50 px-3 py-2 font-mono-num text-xs text-rose-700">
                {loadError}
              </p>
              <p className="mt-2 text-xs text-slate-500">{t("app.load_error_hint")}</p>
              <button
                type="button"
                onClick={() => {
                  setLoadError("");
                  loadTelemetry(false);
                }}
                className="mt-4 inline-flex items-center gap-1.5 rounded-lg bg-slate-900 px-4 py-2 text-xs font-semibold text-white hover:bg-slate-800"
              >
                <RefreshCw className="h-3.5 w-3.5" />
                {t("app.retry")}
              </button>
            </div>
          ) : (
            <div
              role="status"
              aria-live="polite"
              className="inline-flex items-center gap-2 rounded-full border border-slate-200 bg-white px-4 py-2 text-xs font-medium text-slate-600 shadow-sm"
            >
              <Loader2 className="h-4 w-4 animate-spin" />
              {t("app.loading")}
            </div>
          )}
        </div>
      </div>
    );
  }

  return (
    <div className={`min-h-screen bg-[#F8FAFC] text-[#0F172A] ${isDesktop ? "pt-8" : ""}`}>
      {/* macOS hidden-title-bar drag strip. Interactive controls opt out below. */}
      {isDesktop && (
        <div
          className="wails-drag fixed top-0 left-0 z-40 h-8 w-full bg-[#F8FAFC]"
          aria-hidden="true"
        />
      )}

      {/* =====================================================================
          1. TOP PRECISION COMMAND BAR (Sticky 56px)
         ===================================================================== */}
      <header className={`wails-drag sticky z-30 border-b border-slate-200/90 bg-white/90 backdrop-blur-md ${isDesktop ? "top-8" : "top-0"}`}>
        <div className="mx-auto flex max-w-[1440px] flex-wrap items-center justify-between gap-3 px-4 py-2.5 sm:px-6 lg:px-8">
          {/* Left: Brand + Local Privacy + Live Scanner Status */}
          <div className={`flex items-center gap-3 ${isDesktop ? "pl-14" : ""}`}>
            <div className="flex items-center gap-2.5">
              {/* Custom Geometric Pulse Logo */}
              <div className="flex h-8 w-8 items-center justify-center rounded-lg bg-slate-900 text-white shadow-xs">
                <svg viewBox="0 0 24 24" fill="none" className="h-4.5 w-4.5">
                  <path
                    d="M3 12H7L9.5 5.5L14.5 18.5L17 12H21"
                    stroke="currentColor"
                    strokeWidth="2.2"
                    strokeLinecap="round"
                    strokeLinejoin="round"
                  />
                </svg>
              </div>
              <div>
                <div className="flex items-center gap-2">
                  <span className="text-sm font-bold tracking-tight text-slate-900">
                    {t("app.title")}
                  </span>
                  <span className="inline-flex items-center gap-1 rounded border border-slate-200 bg-slate-50 px-1.5 py-0.5 text-[9px] font-semibold tracking-wider text-slate-500 uppercase">
                    <ShieldCheck className="h-2.5 w-2.5 text-emerald-600" />
                    {t("app.badge_local")}
                  </span>
                </div>
                <p className="hidden text-[11px] text-slate-400 xl:block">
                  {t("app.subtitle")}
                </p>
              </div>
            </div>

            <div className="hidden h-5 w-px bg-slate-200 sm:block" />

            {/* Live Scanner Status & Pause/Resume Control */}
            <button
              type="button"
              disabled={pauseBusy}
              onClick={handleTogglePause}
              title={isPaused ? t("app.resume_btn") : t("app.pause_btn")}
              className={`inline-flex items-center gap-1.5 rounded-full border px-2.5 py-1 text-[10px] font-semibold tracking-wider uppercase transition-all ${
                isPaused
                  ? "border-amber-200 bg-amber-50 text-amber-700 hover:bg-amber-100"
                  : "border-emerald-200 bg-emerald-50/80 text-emerald-700 hover:bg-emerald-100/80"
              }`}
            >
              <span className="relative flex h-2 w-2">
                {!isPaused && (
                  <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-emerald-400 opacity-75" />
                )}
                <span
                  className={`relative inline-flex h-2 w-2 rounded-full ${
                    isPaused ? "bg-amber-500" : "bg-emerald-500"
                  }`}
                />
              </span>
              <span>{isPaused ? t("app.paused") : t("app.scanning")}</span>
              {isPaused ? (
                <Play className="ml-0.5 h-2.5 w-2.5 fill-current" />
              ) : (
                <Pause className="ml-0.5 h-2.5 w-2.5" />
              )}
            </button>
          </div>

          {/* Right: Drawers, Currency & Language Switchers */}
          <div className="flex flex-wrap items-center gap-1.5 sm:gap-2">
            {/* LAN Radar Trigger */}
            <button
              type="button"
              onClick={() => setActiveDrawer("lan")}
              className="inline-flex items-center gap-1.5 rounded-lg border border-slate-200 bg-white px-2.5 py-1.5 text-xs font-medium text-slate-700 transition-colors hover:border-slate-300 hover:bg-slate-50"
            >
              <Radio className="h-3.5 w-3.5 text-emerald-600" />
              <span>{t("drawers.lan_btn")}</span>
              {(data?.devices?.length ?? 0) > 1 && (
                <span className="font-mono-num rounded bg-slate-100 px-1.5 py-0.2 text-[10px] font-semibold text-slate-700">
                  {data?.devices?.length}
                </span>
              )}
            </button>

            {/* Devices Manager Trigger */}
            <button
              type="button"
              onClick={() => setActiveDrawer("devices")}
              aria-label={t("drawers.devices_btn")}
              className="inline-flex items-center gap-1.5 rounded-lg border border-slate-200 bg-white px-2.5 py-1.5 text-xs font-medium text-slate-700 transition-colors hover:border-slate-300 hover:bg-slate-50"
            >
              <Laptop className="h-3.5 w-3.5 text-slate-500" />
              <span className="hidden md:inline">{t("drawers.devices_btn")}</span>
            </button>

            {/* Model Pricing Trigger */}
            <button
              type="button"
              onClick={() => setActiveDrawer("pricing")}
              aria-label={t("drawers.pricing_btn")}
              className="inline-flex items-center gap-1.5 rounded-lg border border-slate-200 bg-white px-2.5 py-1.5 text-xs font-medium text-slate-700 transition-colors hover:border-slate-300 hover:bg-slate-50"
            >
              <DollarSign className="h-3.5 w-3.5 text-slate-500" />
              <span className="hidden md:inline">{t("drawers.pricing_btn")}</span>
            </button>

            {/* Scanner Config Trigger */}
            <button
              type="button"
              onClick={() => setActiveDrawer("config")}
              aria-label={t("drawers.config_btn")}
              className="inline-flex items-center gap-1.5 rounded-lg border border-slate-200 bg-white px-2.5 py-1.5 text-xs font-medium text-slate-700 transition-colors hover:border-slate-300 hover:bg-slate-50"
            >
              <Sliders className="h-3.5 w-3.5 text-slate-500" />
              <span className="hidden md:inline">{t("drawers.config_btn")}</span>
            </button>

            {/* Source Plugins Trigger */}
            <button
              type="button"
              onClick={() => setActiveDrawer("plugins")}
              aria-label={t("plugins.open_btn")}
              className="inline-flex items-center gap-1.5 rounded-lg border border-slate-200 bg-white px-2.5 py-1.5 text-xs font-medium text-slate-700 transition-colors hover:border-slate-300 hover:bg-slate-50"
            >
              <Package className="h-3.5 w-3.5 text-slate-500" />
              <span className="hidden md:inline">{t("plugins.open_btn")}</span>
            </button>

            <div className="h-4 w-px bg-slate-200" />

            {/* Currency Switcher */}
            <select
              aria-label="Currency"
              value={currency}
              onChange={(e) => setCurrency(e.target.value as CurrencyCode)}
              className="font-mono-num cursor-pointer rounded-lg border border-slate-200 bg-slate-50 px-2 py-1.5 text-xs font-semibold text-slate-700 transition-colors hover:border-slate-300 focus:border-slate-900 focus:outline-none"
            >
              {(Object.keys(EXCHANGE_RATES) as CurrencyCode[]).map((code) => (
                <option key={code} value={code}>
                  {code}
                </option>
              ))}
            </select>

            {/* Language Switcher (zh / en) */}
            <button
              type="button"
              onClick={handleLanguageSwitch}
              className="inline-flex items-center gap-1 rounded-lg border border-slate-200 bg-slate-50 px-2.5 py-1.5 text-xs font-semibold text-slate-700 transition-colors hover:border-slate-300 hover:bg-slate-100"
            >
              <Globe className="h-3.5 w-3.5 text-slate-500" />
              <span>{i18n.language === "zh" ? "EN" : "中文"}</span>
            </button>
          </div>
        </div>
      </header>

      {updateBanner && !dismissedUpdate && (
        <aside
          aria-label="update-notification"
          className="mx-auto mt-3 max-w-[1440px] px-4 sm:px-6 lg:px-8"
        >
          <div className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-amber-300 bg-amber-50 px-4 py-2.5 text-xs text-amber-900 shadow-sm">
            <div className="flex items-center gap-2 font-medium">
              <span>🚀</span>
              <span>{t("newVersionAvailable")}:</span>
              <span className="font-mono-num rounded bg-amber-200/80 px-2 py-0.5 font-bold text-amber-950">
                {updateBanner.version}
              </span>
            </div>
            <div className="flex items-center gap-3">
              <button
                type="button"
                onClick={() => setActiveDrawer("config")}
                className="font-semibold underline hover:text-amber-950"
              >
                {t("viewDetails")}
              </button>
              <button
                type="button"
                onClick={() => setDismissedUpdate(true)}
                className="font-bold text-slate-500 hover:text-slate-800"
                aria-label="Dismiss"
              >
                ✕
              </button>
            </div>
          </div>
        </aside>
      )}

      {/* =====================================================================
          2. MAIN TELEMETRY WORKSPACE CONTAINER
         ===================================================================== */}
      <main className="mx-auto max-w-[1440px] space-y-6 px-4 py-6 sm:px-6 lg:px-8">
        {loadError && (
          <div
            role="alert"
            className="flex flex-wrap items-center justify-between gap-2 rounded-xl border border-amber-200 bg-amber-50 px-4 py-2.5 text-xs text-amber-800"
          >
            <span className="flex items-center gap-1.5">
              <AlertTriangle className="h-3.5 w-3.5" />
              {t("app.refresh_error")}: {loadError}
            </span>
            <button
              type="button"
              onClick={() => loadTelemetry(false)}
              className="rounded-md border border-amber-300 bg-white px-2.5 py-1 font-medium text-amber-800 hover:bg-amber-100"
            >
              {t("app.retry")}
            </button>
          </div>
        )}

        {/* Global Filter & Time Window Control Strip */}
        <div className="flex flex-col justify-between gap-3 rounded-xl border border-slate-200/90 bg-white p-3.5 shadow-[0_1px_2px_rgba(15,23,42,0.03)] lg:flex-row lg:items-center">
          {/* Left: Device Selector + CLI Source Filter Pills */}
          <div className="flex flex-wrap items-center gap-2.5">
            {/* Device Dropdown */}
            <div className="flex items-center gap-1.5">
              <Laptop className="h-3.5 w-3.5 text-slate-400" />
              <select
                aria-label={t("filters.device_label")}
                value={selectedDevice}
                onChange={(e) => setSelectedDevice(e.target.value)}
                className="cursor-pointer rounded-lg border border-slate-200 bg-slate-50/80 px-2.5 py-1.5 text-xs font-semibold text-slate-800 transition-colors hover:border-slate-300 focus:border-slate-900 focus:outline-none"
              >
                <option value="all">{t("filters.all_devices")}</option>
                {(data?.devices ?? []).map((dev) => (
                  <option key={dev.id} value={dev.id}>
                    {dev.display_name || dev.id}
                  </option>
                ))}
              </select>
            </div>

            <div className="hidden h-4 w-px bg-slate-200 sm:block" />

            {/* Source Segmented Pills */}
            <div className="flex flex-wrap items-center gap-1">
              <button
                type="button"
                onClick={() => setSelectedSource("")}
                className={`rounded-lg px-2.5 py-1.5 text-xs font-medium transition-all ${
                  selectedSource === ""
                    ? "bg-slate-900 font-semibold text-white shadow-2xs"
                    : "text-slate-600 hover:bg-slate-100 hover:text-slate-900"
                }`}
              >
                {t("filters.all_sources")}
              </button>

              {availableSources.map((src) => {
                const active = selectedSource === src;
                return (
                  <button
                    key={src}
                    type="button"
                    onClick={() => setSelectedSource(active ? "" : src)}
                    className={`inline-flex items-center gap-1.5 rounded-lg border px-2.5 py-1 text-xs font-medium transition-all ${
                      active
                        ? "border-slate-900 bg-slate-900 font-semibold text-white shadow-2xs"
                        : "border-transparent bg-slate-50/80 text-slate-700 hover:border-slate-200 hover:bg-white"
                    }`}
                  >
                    <SourceIcon source={src} className="h-3.5 w-3.5" />
                    <span>{src}</span>
                  </button>
                );
              })}
            </div>
          </div>

          {/* Right: 8 Fixed Time Window Tabs + Two-Stage Hydration Indicator */}
          <div className="flex flex-wrap items-center justify-between gap-2 lg:justify-end">
            {/* Two-Stage Load Status Pill */}
            <span
              className={`hidden items-center gap-1 rounded px-2 py-0.5 text-[10px] font-semibold tracking-wider uppercase xl:inline-flex ${
                isHydratingDetails
                  ? "bg-blue-50 text-blue-600"
                  : "bg-slate-100 text-slate-400"
              }`}
            >
              <Activity className="h-2.5 w-2.5" />
              {isHydratingDetails ? t("app.stage_hydrating") : t("app.stage_synced")}
            </span>

            {/* Fixed 8 Time Windows: 1h, 24h, 7d, 30d, 3mo, 6mo, 1y, ALL */}
            <div
              role="tablist"
              aria-label={t("filters.window_label")}
              className="inline-flex rounded-lg border border-slate-200 bg-slate-100/80 p-0.5"
            >
              {TIME_WINDOWS.map((win) => {
                const isActive = selectedWindow === win;
                return (
                  <button
                    key={win}
                    type="button"
                    role="tab"
                    aria-selected={isActive}
                    onClick={() => setSelectedWindow(win)}
                    className={`font-mono-num rounded-md px-2.5 py-1 text-xs font-semibold transition-all ${
                      isActive
                        ? "bg-white text-slate-900 shadow-xs"
                        : "text-slate-500 hover:text-slate-900"
                    }`}
                  >
                    {win}
                  </button>
                );
              })}
            </div>
          </div>
        </div>

        {/* ===================================================================
            3. FOUR HERO KPI CARDS
           =================================================================== */}
        <section className="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-4">
          {/* KPI 1: Estimated Cost */}
          <div className="rounded-xl border border-slate-200/90 bg-white p-5 shadow-[0_1px_2px_rgba(15,23,42,0.03)]">
            <div className="flex items-center justify-between">
              <span className="text-[11px] font-semibold tracking-wider text-slate-400 uppercase">
                ESTIMATED COST · {selectedWindow}
              </span>
              <span className="rounded bg-slate-100 px-1.5 py-0.5 text-[10px] font-semibold text-slate-600">
                {currency}
              </span>
            </div>
            <div className="mt-2 flex items-baseline justify-between gap-2">
              <div className="font-mono-num text-3xl font-bold tracking-tight text-slate-900">
                {formatCost(currentPeriod.cost, currency)}
              </div>
              <span className="font-mono-num text-xs font-medium text-slate-400">
                ~{formatCost(blendedCostPerM, currency)} / 1M
              </span>
            </div>
            <div className="mt-2.5 flex items-center justify-between border-t border-slate-100 pt-2 text-xs text-slate-500">
              <span>{t("kpi.total_cost")}</span>
              <span className="text-[11px] text-slate-400">{t("kpi.total_cost_sub")}</span>
            </div>
          </div>

          {/* KPI 2: Total Token Volume */}
          <div className="rounded-xl border border-slate-200/90 bg-white p-5 shadow-[0_1px_2px_rgba(15,23,42,0.03)]">
            <div className="flex items-center justify-between">
              <span className="text-[11px] font-semibold tracking-wider text-slate-400 uppercase">
                TOTAL TOKENS · {selectedWindow}
              </span>
              <span className="font-mono-num rounded bg-blue-50 px-1.5 py-0.5 text-[10px] font-semibold text-blue-700">
                IN + OUT
              </span>
            </div>
            <div className="mt-2 flex items-baseline justify-between gap-2">
              <div
                className="font-mono-num text-3xl font-bold tracking-tight text-slate-900"
                title={formatExactNumber(totalVolumeTokens)}
              >
                {formatCompactNumber(totalVolumeTokens)}
              </div>
              <div className="font-mono-num text-[11px] text-slate-500">
                IN {formatCompactNumber(currentPeriod.input_tokens)} · OUT{" "}
                {formatCompactNumber(currentPeriod.output_tokens)}
              </div>
            </div>
            <div className="mt-2.5 flex items-center justify-between border-t border-slate-100 pt-2 text-xs text-slate-500">
              <span>{t("kpi.total_tokens")}</span>
              <span className="text-[11px] text-slate-400">{t("kpi.total_tokens_sub")}</span>
            </div>
          </div>

          {/* KPI 3: Cache Hit Rate + Radial Gauge */}
          <div className="rounded-xl border border-slate-200/90 bg-white p-5 shadow-[0_1px_2px_rgba(15,23,42,0.03)]">
            <div className="flex items-start justify-between gap-3">
              <div>
                <span className="text-[11px] font-semibold tracking-wider text-slate-400 uppercase">
                  CACHE HIT RATE
                </span>
                <div className="font-mono-num mt-2 text-3xl font-bold tracking-tight text-emerald-600">
                  {formatPercent(currentPeriod.cache_hit_rate)}
                </div>
              </div>

              {/* Handcrafted Radial SVG Ring */}
              <div className="relative flex h-12 w-12 shrink-0 items-center justify-center">
                <svg viewBox="0 0 44 44" className="h-full w-full -rotate-90">
                  <circle
                    cx="22"
                    cy="22"
                    r={gaugeRadius}
                    fill="none"
                    stroke="#E2E8F0"
                    strokeWidth="4"
                  />
                  <circle
                    cx="22"
                    cy="22"
                    r={gaugeRadius}
                    fill="none"
                    stroke="#10B981"
                    strokeWidth="4"
                    strokeDasharray={gaugeCircumference}
                    strokeDashoffset={gaugeDashOffset}
                    strokeLinecap="round"
                    className="transition-all duration-500"
                  />
                </svg>
                <span className="font-mono-num absolute text-[10px] font-bold text-slate-700">
                  {Math.round(clampedHitRate)}%
                </span>
              </div>
            </div>

            <div className="mt-2.5 flex items-center justify-between border-t border-slate-100 pt-2 text-xs">
              <span className="text-slate-500">{t("kpi.cache_hit_rate")}</span>
              <span className="font-mono-num font-semibold text-emerald-700">
                {t("kpi.cache_savings")} {formatCost(estCacheSavingsUsd, currency)}
              </span>
            </div>
          </div>

          {/* KPI 4: Base Input (Uncached New Input) */}
          <div className="rounded-xl border border-slate-200/90 bg-white p-5 shadow-[0_1px_2px_rgba(15,23,42,0.03)]">
            <div className="flex items-center justify-between">
              <span className="text-[11px] font-semibold tracking-wider text-slate-400 uppercase">
                BASE INPUT
              </span>
              <span className="rounded bg-slate-900 px-1.5 py-0.5 text-[10px] font-semibold text-white">
                BILLABLE NEW
              </span>
            </div>
            <div className="mt-2 flex items-baseline justify-between gap-2">
              <div
                className="font-mono-num text-3xl font-bold tracking-tight text-slate-900"
                title={formatExactNumber(baseInputTokens)}
              >
                {formatCompactNumber(baseInputTokens)}
              </div>
              <span className="font-mono-num text-xs text-slate-400">
                {currentPeriod.input_tokens > 0
                  ? `${((baseInputTokens / currentPeriod.input_tokens) * 100).toFixed(1)}% of IN`
                  : "0%"}
              </span>
            </div>
            <div className="mt-2.5 flex items-center justify-between border-t border-slate-100 pt-2 text-xs text-slate-500">
              <span>{t("kpi.base_input")}</span>
              <span className="text-[11px] text-slate-400">{t("kpi.base_input_sub")}</span>
            </div>
          </div>
        </section>

        {/* ===================================================================
            4. FOUR-CATEGORY TOKEN TAXONOMY STRIP
           =================================================================== */}
        <TokenTaxonomyStrip period={currentPeriod} />

        {/* ===================================================================
            5. VISUAL ANALYTICS CHARTS (30D DAILY COST + CLI SOURCE SHARE)
           =================================================================== */}
        <section className="grid grid-cols-1 gap-6 lg:grid-cols-12">
          <div className="lg:col-span-7">
            <DailyCostChart
              data={data?.charts?.daily_costs ?? []}
              currency={currency}
            />
          </div>
          <div className="lg:col-span-5">
            <SourceShareChart
              toolShare={data?.charts?.tool_share ?? []}
              sources={data?.sources ?? []}
              activeSource={selectedSource}
              onSelectSource={setSelectedSource}
              currency={currency}
            />
          </div>
        </section>

        {/* ===================================================================
            6. GITHUB-STYLE ACTIVITY HEATMAP MATRIX
           =================================================================== */}
        <ActivityHeatmap
          data={data?.charts?.calendar_heatmap ?? []}
          currency={currency}
        />

        {/* ===================================================================
            7. TWO-STAGE PROGRESSIVE ATTRIBUTION TABLES (MODELS & PROJECTS)
           =================================================================== */}
        <AttributionTables
          sources={data?.sources ?? []}
          projects={data?.projects}
          isHydratingDetails={isHydratingDetails}
          currency={currency}
        />
      </main>

      {/* =====================================================================
          8. SLIDE-OVER DRAWERS (LAN RADAR, DEVICES, PRICING, SYSTEM CONFIG)
         ===================================================================== */}
      <SlideOverDrawers
        activeDrawer={activeDrawer}
        onClose={() => {
          setActiveDrawer(null);
          refreshSources();
        }}
        currency={currency}
        onDataMutated={() => {
          loadTelemetry(true);
          refreshSources();
        }}
      />
    </div>
  );
}

export default App;
