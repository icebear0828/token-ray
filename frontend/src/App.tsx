import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import {
  Activity,
  AlertTriangle,
  DollarSign,
  Globe,
  Laptop,
  Loader2,
  Moon,
  Package,
  Pause,
  Play,
  Radio,
  RefreshCw,
  ShieldCheck,
  Sliders,
  Sun,
} from "lucide-react";
import { fetchSources, fetchStats, isDesktopRuntime, togglePause } from "./api/client";
import { ActivityHeatmap } from "./components/ActivityHeatmap";
import { AttributionTables } from "./components/AttributionTables";
import { DailyCostChart } from "./components/DailyCostChart";
import { DrawerType, SlideOverDrawers } from "./components/SlideOverDrawers";
import { SourceIcon } from "./components/SourceBadge";
import { SourceShareChart } from "./components/SourceShareChart";
import { TokenTaxonomyStrip } from "./components/TokenTaxonomyStrip";
import { ErrorBoundary } from "./ErrorBoundary";
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
  formatTokenRate,
  tokenRatePerSecond,
} from "./utils/formatters";

type ThemeMode = "light" | "dark";

const THEME_STORAGE_KEY = "ai-flight-dashboard_theme";

const readInitialTheme = (): ThemeMode => {
  if (typeof window === "undefined") return "light";
  try {
    return localStorage.getItem(THEME_STORAGE_KEY) === "dark" ? "dark" : "light";
  } catch {
    return "light";
  }
};

const syncDocumentTheme = (theme: ThemeMode) => {
  if (typeof document === "undefined") return;
  document.documentElement.classList.toggle("dark", theme === "dark");
  document.documentElement.style.colorScheme = theme;
};

const DEFAULT_SOURCES = [
  "Claude Code",
  "Gemini CLI",
  "Codex",
  "Antigravity",
  "DeepSeek Harness",
  "Pi Agent",
];

// Client-side cache of fully hydrated dashboard snapshots. Switching back to a
// recently viewed device/source/window paints instantly while the background
// request revalidates the data.
const STATS_CACHE_LIMIT = 32;

const statsCacheKey = (device: string, source: string, period: string) =>
  `${device}|${source}|${period}`;

const rememberStats = (
  cache: Map<string, DashboardData>,
  key: string,
  value: DashboardData
) => {
  cache.delete(key);
  cache.set(key, value);
  while (cache.size > STATS_CACHE_LIMIT) {
    const oldest = cache.keys().next().value;
    if (oldest === undefined) break;
    cache.delete(oldest);
  }
};

const mergeHydrated = (summary: DashboardData, details: DashboardData): DashboardData => ({
  ...summary,
  sources: details.sources,
  projects: details.projects ?? [],
  is_paused: details.is_paused ?? summary.is_paused,
});

// The backend cache makes consecutive polls return identical payloads when no
// new logs arrived; this signature lets the poll skip the resulting re-render.
const telemetrySignature = (summary: DashboardData, details: DashboardData) =>
  `${JSON.stringify(summary)}\u0000${JSON.stringify(details)}`;

export function App() {
  const { t, i18n } = useTranslation();

  // Theme State (persisted under ai-flight-dashboard_theme, default "light")
  const [theme, setTheme] = useState<ThemeMode>(() => {
    const initial = readInitialTheme();
    syncDocumentTheme(initial);
    return initial;
  });

  useEffect(() => {
    syncDocumentTheme(theme);
    try {
      localStorage.setItem(THEME_STORAGE_KEY, theme);
    } catch {
      // Ignore storage quota / privacy mode write errors
    }
  }, [theme]);

  // Global Scope & View State
  const [selectedDevice, setSelectedDevice] = useState<string>("all");
  const [selectedSource, setSelectedSource] = useState<string>("");
  const [selectedWindow, setSelectedWindow] = useState<TimeWindowLabel>("30d");
  const [currency, setCurrency] = useState<CurrencyCode>("USD");
  const [activeDrawer, setActiveDrawer] = useState<DrawerType>(null);

  // Two-stage Telemetry State
  const [data, setData] = useState<DashboardData | null>(null);
  const [sourceDescriptors, setSourceDescriptors] = useState<SourceDescriptor[]>([]);
  const [isSummaryLoading, setIsSummaryLoading] = useState<boolean>(false);
  const [isHydratingDetails, setIsHydratingDetails] = useState<boolean>(true);
  const [loadError, setLoadError] = useState<string>("");
  const [pauseBusy, setPauseBusy] = useState<boolean>(false);
  const [updateBanner, setUpdateBanner] = useState<{ version: string } | null>(null);
  const [dismissedUpdate, setDismissedUpdate] = useState<boolean>(false);

  useEffect(() => {
    fetch("/api/system/update/check")
      .then((res) => (res.ok ? res.json() : null))
      .then((payload: { has_update?: boolean; latest_version?: string } | null) => {
        if (payload && payload.has_update && payload.latest_version) {
          setUpdateBanner({ version: payload.latest_version });
        }
      })
      .catch(() => {});
  }, []);

  const requestSeqRef = useRef<number>(0);
  const abortRef = useRef<AbortController | null>(null);
  const statsCacheRef = useRef<Map<string, DashboardData>>(new Map());
  const pollSignatureRef = useRef<string>("");

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
      abortRef.current?.abort();
      const controller = new AbortController();
      abortRef.current = controller;

      const cacheKey = statsCacheKey(selectedDevice, selectedSource, selectedWindow);
      const cached = isBackgroundPoll ? undefined : statsCacheRef.current.get(cacheKey);

      try {
        if (!isBackgroundPoll) {
          // A cached snapshot for this exact scope can paint immediately, so
          // only surface the loading skeleton when we have nothing to show.
          setIsSummaryLoading(!cached);
          setIsHydratingDetails(true);
          if (cached) setData(cached);
        }
        const summaryRes = await fetchStats(
          {
            device: selectedDevice,
            source: selectedSource,
            period: selectedWindow,
            detail: "summary",
          },
          { signal: controller.signal }
        );
        if (seq !== requestSeqRef.current) return;

        if (isBackgroundPoll) {
          // Background polls skip both renders entirely when nothing changed.
          const detailsRes = await fetchStats(
            {
              device: selectedDevice,
              source: selectedSource,
              period: selectedWindow,
              detail: "details",
            },
            { signal: controller.signal }
          );
          if (seq !== requestSeqRef.current) return;
          setIsSummaryLoading(false);
          setLoadError("");
          const signature = telemetrySignature(summaryRes, detailsRes);
          if (signature === pollSignatureRef.current) {
            setIsHydratingDetails(false);
            return;
          }
          pollSignatureRef.current = signature;
          const hydrated = mergeHydrated(summaryRes, detailsRes);
          rememberStats(statsCacheRef.current, cacheKey, hydrated);
          setData(hydrated);
          setIsHydratingDetails(false);
          return;
        }

        const preservePrevious = Boolean(cached);
        setData((prev) => {
          const previousModels = new Map(
            (prev?.sources ?? []).map((source) => [source.name, source.models])
          );
          return {
            ...summaryRes,
            sources: summaryRes.sources.map((source) => {
              const models = source.models.length
                ? source.models
                : preservePrevious
                  ? previousModels.get(source.name) ?? []
                  : [];
              return { ...source, models };
            }),
            // A user-initiated scope change must not keep the previous
            // window's projects visible while details are in flight.
            projects: preservePrevious ? prev?.projects : undefined,
          };
        });
        setIsSummaryLoading(false);
        setLoadError("");

        const detailsRes = await fetchStats(
          {
            device: selectedDevice,
            source: selectedSource,
            period: selectedWindow,
            detail: "details",
          },
          { signal: controller.signal }
        );
        if (seq !== requestSeqRef.current) return;
        // A successful details response is authoritative even when it is
        // empty: the selected window/source may have no usage.
        const hydrated = mergeHydrated(summaryRes, detailsRes);
        pollSignatureRef.current = telemetrySignature(summaryRes, detailsRes);
        rememberStats(statsCacheRef.current, cacheKey, hydrated);
        setData(hydrated);
        setIsHydratingDetails(false);
        setLoadError("");
      } catch (error: unknown) {
        if (seq !== requestSeqRef.current) return;
        if (controller.signal.aborted) return;
        setLoadError(error instanceof Error ? error.message : String(error));
        setIsSummaryLoading(false);
        setIsHydratingDetails(false);
      }
    },
    [selectedDevice, selectedSource, selectedWindow]
  );

  // Initial & filter-change two-stage load
  useEffect(() => {
    loadTelemetry(false);
  }, [loadTelemetry]);

  // Visibility-aware 5-second background telemetry polling:
  // pauses completely while the tab is hidden and refreshes immediately on return.
  useEffect(() => {
    let interval: ReturnType<typeof setInterval> | null = null;

    const startPolling = () => {
      if (interval !== null) return;
      interval = setInterval(() => {
        if (document.visibilityState !== "hidden") {
          loadTelemetry(true);
        }
      }, 5000);
    };

    const stopPolling = () => {
      if (interval !== null) {
        clearInterval(interval);
        interval = null;
      }
    };

    const handleVisibilityChange = () => {
      if (document.visibilityState === "visible") {
        loadTelemetry(true);
        startPolling();
      } else {
        stopPolling();
      }
    };

    if (document.visibilityState !== "hidden") {
      startPolling();
    }
    document.addEventListener("visibilitychange", handleVisibilityChange);
    return () => {
      stopPolling();
      document.removeEventListener("visibilitychange", handleVisibilityChange);
    };
  }, [loadTelemetry]);

  // Drop any in-flight telemetry request when the app unmounts.
  useEffect(() => () => abortRef.current?.abort(), []);

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

  const handleThemeToggle = () => {
    setTheme((prev) => (prev === "dark" ? "light" : "dark"));
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
        timed_output_tokens: 0,
        duration_ms: 0,
        timed_events: 0,
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
  const outputRate = tokenRatePerSecond(
    currentPeriod.timed_output_tokens,
    currentPeriod.duration_ms
  );
  const outputRateLabel = formatTokenRate(
    currentPeriod.timed_output_tokens,
    currentPeriod.duration_ms
  );
  const baseInputPct =
    currentPeriod.input_tokens > 0
      ? ((baseInputTokens / currentPeriod.input_tokens) * 100).toFixed(1)
      : null;

  // Estimated USD savings from cache hits (cached tokens billed at ~10% vs full input rate)
  const estCacheSavingsUsd = useMemo(() => {
    const cached = currentPeriod.cached_tokens || 0;
    // Conservative blended $2.70/1M saved differential
    return (cached / 1_000_000) * 2.7;
  }, [currentPeriod.cached_tokens]);

  const isPaused = Boolean(data?.is_paused);
  const isDesktop = isDesktopRuntime();

  // Local fallback so one broken section cannot blank the whole dashboard.
  const renderSectionError = (error: Error, reset: () => void) => (
    <div
      role="alert"
      className="rounded-xl border border-rose-200 bg-white p-5 text-xs text-rose-700 shadow-sm dark:border-rose-500/30 dark:bg-slate-900 dark:text-rose-300"
    >
      <div className="flex items-center gap-2 font-semibold">
        <AlertTriangle className="h-4 w-4" />
        {t("app.render_error_title")}
      </div>
      <p className="mt-2 break-all rounded-lg bg-rose-50 px-3 py-2 font-mono-num text-[11px] dark:bg-rose-950/60 dark:text-rose-200">
        {error.message}
      </p>
      <button
        type="button"
        onClick={reset}
        className="mt-3 inline-flex items-center gap-1.5 rounded-lg bg-slate-900 px-3 py-1.5 text-xs font-semibold text-white hover:bg-slate-800 dark:bg-slate-100 dark:text-slate-950 dark:hover:bg-slate-200"
      >
        <RefreshCw className="h-3.5 w-3.5" />
        {t("app.retry")}
      </button>
    </div>
  );

  // Radial gauge geometry for Cache Hit Rate KPI
  const gaugeRadius = 18;
  const gaugeCircumference = 2 * Math.PI * gaugeRadius;
  const clampedHitRate = Math.min(100, Math.max(0, currentPeriod.cache_hit_rate || 0));
  const gaugeDashOffset =
    gaugeCircumference - (clampedHitRate / 100) * gaugeCircumference;

  if (!data) {
    return (
      <div
        className={`telemetry-grid-bg min-h-screen bg-[#F8FAFC] text-[#0F172A] dark:bg-[#090D16] dark:text-slate-100 ${
          isDesktop ? "pt-8" : ""
        }`}
      >
        {isDesktop && (
          <div
            className="wails-drag fixed top-0 left-0 z-40 h-8 w-full bg-[#F8FAFC] dark:bg-[#090D16]"
            aria-hidden="true"
          />
        )}
        <div className="flex min-h-[80vh] items-center justify-center px-4">
          {loadError ? (
            <div
              role="alert"
              className="w-full max-w-lg rounded-xl border border-rose-200 bg-white p-6 shadow-sm dark:border-rose-500/30 dark:bg-slate-900"
            >
              <div className="flex items-center gap-2 text-rose-700 dark:text-rose-400">
                <AlertTriangle className="h-5 w-5" />
                <h1 className="text-sm font-semibold">{t("app.load_error_title")}</h1>
              </div>
              <p className="mt-3 break-all rounded-lg bg-rose-50 px-3 py-2 font-mono-num text-xs text-rose-700 dark:bg-rose-950/60 dark:text-rose-200">
                {loadError}
              </p>
              <p className="mt-2 text-xs text-slate-500 dark:text-slate-400">
                {t("app.load_error_hint")}
              </p>
              <button
                type="button"
                onClick={() => {
                  setLoadError("");
                  loadTelemetry(false);
                }}
                className="mt-4 inline-flex items-center gap-1.5 rounded-lg bg-slate-900 px-4 py-2 text-xs font-semibold text-white hover:bg-slate-800 dark:bg-slate-100 dark:text-slate-950 dark:hover:bg-slate-200"
              >
                <RefreshCw className="h-3.5 w-3.5" />
                {t("app.retry")}
              </button>
            </div>
          ) : (
            <div
              role="status"
              aria-live="polite"
              className="inline-flex items-center gap-2 rounded-full border border-slate-200 bg-white px-4 py-2 text-xs font-medium text-slate-600 shadow-sm dark:border-slate-800 dark:bg-slate-900 dark:text-slate-300"
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
    <div
      className={`telemetry-grid-bg min-h-screen bg-[#F8FAFC] text-[#0F172A] dark:bg-[#090D16] dark:text-slate-100 ${
        isDesktop ? "pt-8" : ""
      }`}
    >
      {/* macOS hidden-title-bar drag strip. Interactive controls opt out below. */}
      {isDesktop && (
        <div
          className="wails-drag fixed top-0 left-0 z-40 h-8 w-full bg-[#F8FAFC] dark:bg-[#090D16]"
          aria-hidden="true"
        />
      )}

      {/* =====================================================================
          1. TOP PRECISION COMMAND BAR (Sticky 56px)
         ===================================================================== */}
      <header
        className={`wails-drag sticky z-30 border-b border-slate-200/90 bg-white/90 backdrop-blur-md dark:border-slate-800/90 dark:bg-slate-950/90 ${
          isDesktop ? "top-8" : "top-0"
        }`}
      >
        <div className="mx-auto flex max-w-[1440px] flex-wrap items-center justify-between gap-2.5 px-4 py-2.5 sm:px-6 lg:px-8">
          {/* Left: Brand + Local Privacy + Live Scanner Status */}
          <div className={`flex flex-wrap items-center gap-2.5 sm:gap-3 ${isDesktop ? "pl-14" : ""}`}>
            <div className="flex items-center gap-2.5">
              {/* Custom Geometric Pulse Logo */}
              <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg bg-slate-900 text-white ring-1 ring-slate-900/10 dark:bg-slate-100 dark:text-slate-950 dark:ring-white/10">
                <svg viewBox="0 0 24 24" fill="none" className="h-4.5 w-4.5" aria-hidden="true">
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
                  <span className="text-sm font-bold tracking-tight text-slate-900 dark:text-slate-100">
                    {t("app.title")}
                  </span>
                  <span className="inline-flex items-center gap-1 rounded border border-slate-200 bg-slate-50 px-1.5 py-0.5 text-[9px] font-semibold tracking-wider text-slate-500 uppercase dark:border-slate-800 dark:bg-slate-900 dark:text-slate-400">
                    <ShieldCheck className="h-2.5 w-2.5 text-emerald-600 dark:text-emerald-400" />
                    {t("app.badge_local")}
                  </span>
                </div>
                <p className="hidden text-[11px] text-slate-400 xl:block dark:text-slate-500">
                  {t("app.subtitle")}
                </p>
              </div>
            </div>

            <div className="hidden h-5 w-px bg-slate-200 sm:block dark:bg-slate-800" />

            {/* Live Scanner Status & Pause/Resume Control */}
            <button
              type="button"
              disabled={pauseBusy}
              onClick={handleTogglePause}
              title={isPaused ? t("app.resume_btn") : t("app.pause_btn")}
              className={`inline-flex items-center gap-1.5 rounded-full border px-2.5 py-1 text-[10px] font-semibold tracking-wider uppercase transition-all ${
                isPaused
                  ? "border-amber-200 bg-amber-50 text-amber-700 hover:bg-amber-100 dark:border-amber-500/30 dark:bg-amber-500/10 dark:text-amber-300 dark:hover:bg-amber-500/20"
                  : "border-emerald-200 bg-emerald-50/80 text-emerald-700 hover:bg-emerald-100/80 dark:border-emerald-500/30 dark:bg-emerald-500/10 dark:text-emerald-300 dark:hover:bg-emerald-500/20"
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

          {/* Right: Drawers, Currency, Theme & Language Switchers */}
          <div className="flex flex-wrap items-center gap-1.5 sm:gap-2">
            {/* LAN Radar Trigger */}
            <button
              type="button"
              onClick={() => setActiveDrawer("lan")}
              className="inline-flex items-center gap-1.5 rounded-lg border border-slate-200 bg-white px-2.5 py-1.5 text-xs font-medium text-slate-700 transition-colors hover:border-slate-300 hover:bg-slate-50 dark:border-slate-800 dark:bg-slate-900 dark:text-slate-200 dark:hover:border-slate-700 dark:hover:bg-slate-800"
            >
              <Radio className="h-3.5 w-3.5 text-emerald-600 dark:text-emerald-400" />
              <span>{t("drawers.lan_btn")}</span>
              {(data?.devices?.length ?? 0) > 1 && (
                <span className="font-mono-num rounded bg-slate-100 px-1.5 py-0.2 text-[10px] font-semibold text-slate-700 dark:bg-slate-800 dark:text-slate-300">
                  {data?.devices?.length}
                </span>
              )}
            </button>

            {/* Devices Manager Trigger */}
            <button
              type="button"
              onClick={() => setActiveDrawer("devices")}
              aria-label={t("drawers.devices_btn")}
              className="inline-flex items-center gap-1.5 rounded-lg border border-slate-200 bg-white px-2.5 py-1.5 text-xs font-medium text-slate-700 transition-colors hover:border-slate-300 hover:bg-slate-50 dark:border-slate-800 dark:bg-slate-900 dark:text-slate-200 dark:hover:border-slate-700 dark:hover:bg-slate-800"
            >
              <Laptop className="h-3.5 w-3.5 text-slate-500 dark:text-slate-400" />
              <span className="hidden md:inline">{t("drawers.devices_btn")}</span>
            </button>

            {/* Model Pricing Trigger */}
            <button
              type="button"
              onClick={() => setActiveDrawer("pricing")}
              aria-label={t("drawers.pricing_btn")}
              className="inline-flex items-center gap-1.5 rounded-lg border border-slate-200 bg-white px-2.5 py-1.5 text-xs font-medium text-slate-700 transition-colors hover:border-slate-300 hover:bg-slate-50 dark:border-slate-800 dark:bg-slate-900 dark:text-slate-200 dark:hover:border-slate-700 dark:hover:bg-slate-800"
            >
              <DollarSign className="h-3.5 w-3.5 text-slate-500 dark:text-slate-400" />
              <span className="hidden md:inline">{t("drawers.pricing_btn")}</span>
            </button>

            {/* Scanner Config Trigger */}
            <button
              type="button"
              onClick={() => setActiveDrawer("config")}
              aria-label={t("drawers.config_btn")}
              className="inline-flex items-center gap-1.5 rounded-lg border border-slate-200 bg-white px-2.5 py-1.5 text-xs font-medium text-slate-700 transition-colors hover:border-slate-300 hover:bg-slate-50 dark:border-slate-800 dark:bg-slate-900 dark:text-slate-200 dark:hover:border-slate-700 dark:hover:bg-slate-800"
            >
              <Sliders className="h-3.5 w-3.5 text-slate-500 dark:text-slate-400" />
              <span className="hidden md:inline">{t("drawers.config_btn")}</span>
            </button>

            {/* Source Plugins Trigger */}
            <button
              type="button"
              onClick={() => setActiveDrawer("plugins")}
              aria-label={t("plugins.open_btn")}
              className="inline-flex items-center gap-1.5 rounded-lg border border-slate-200 bg-white px-2.5 py-1.5 text-xs font-medium text-slate-700 transition-colors hover:border-slate-300 hover:bg-slate-50 dark:border-slate-800 dark:bg-slate-900 dark:text-slate-200 dark:hover:border-slate-700 dark:hover:bg-slate-800"
            >
              <Package className="h-3.5 w-3.5 text-slate-500 dark:text-slate-400" />
              <span className="hidden md:inline">{t("plugins.open_btn")}</span>
            </button>

            <div className="hidden h-4 w-px bg-slate-200 sm:block dark:bg-slate-800" />

            {/* Currency Switcher */}
            <select
              aria-label="Currency"
              value={currency}
              onChange={(e) => setCurrency(e.target.value as CurrencyCode)}
              className="font-mono-num cursor-pointer rounded-lg border border-slate-200 bg-slate-50 px-2 py-1.5 text-xs font-semibold text-slate-700 transition-colors hover:border-slate-300 focus:border-slate-900 focus:outline-none dark:border-slate-800 dark:bg-slate-900 dark:text-slate-200 dark:hover:border-slate-700 dark:focus:border-sky-400"
            >
              {(Object.keys(EXCHANGE_RATES) as CurrencyCode[]).map((code) => (
                <option key={code} value={code}>
                  {code}
                </option>
              ))}
            </select>

            {/* Light / Dark Theme Toggle */}
            <button
              type="button"
              data-testid="theme-toggle"
              onClick={handleThemeToggle}
              aria-label={theme === "dark" ? t("theme.switch_to_light") : t("theme.switch_to_dark")}
              aria-pressed={theme === "dark"}
              title={theme === "dark" ? t("theme.switch_to_light") : t("theme.switch_to_dark")}
              className="inline-flex items-center justify-center rounded-lg border border-slate-200 bg-slate-50 p-1.5 text-slate-600 transition-colors hover:border-slate-300 hover:bg-slate-100 hover:text-slate-900 dark:border-slate-800 dark:bg-slate-900 dark:text-slate-300 dark:hover:border-slate-700 dark:hover:bg-slate-800 dark:hover:text-slate-100"
            >
              {theme === "dark" ? (
                <Sun className="h-3.5 w-3.5 text-amber-400" aria-hidden="true" />
              ) : (
                <Moon className="h-3.5 w-3.5 text-slate-600" aria-hidden="true" />
              )}
            </button>

            {/* Language Switcher (zh / en) */}
            <button
              type="button"
              onClick={handleLanguageSwitch}
              className="inline-flex items-center gap-1 rounded-lg border border-slate-200 bg-slate-50 px-2.5 py-1.5 text-xs font-semibold text-slate-700 transition-colors hover:border-slate-300 hover:bg-slate-100 dark:border-slate-800 dark:bg-slate-900 dark:text-slate-200 dark:hover:border-slate-700 dark:hover:bg-slate-800"
            >
              <Globe className="h-3.5 w-3.5 text-slate-500 dark:text-slate-400" />
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
          <div className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-amber-300 bg-amber-50 px-4 py-2.5 text-xs text-amber-900 shadow-xs dark:border-amber-500/35 dark:bg-amber-950/50 dark:text-amber-100">
            <div className="flex items-center gap-2 font-medium">
              <span aria-hidden="true">🚀</span>
              <span>{t("newVersionAvailable")}:</span>
              <span className="font-mono-num rounded bg-amber-200/80 px-2 py-0.5 font-bold text-amber-950 dark:bg-amber-500/20 dark:text-amber-200">
                {updateBanner.version}
              </span>
            </div>
            <div className="flex items-center gap-3">
              <button
                type="button"
                onClick={() => setActiveDrawer("config")}
                className="font-semibold underline hover:text-amber-950 dark:hover:text-amber-50"
              >
                {t("viewDetails")}
              </button>
              <button
                type="button"
                onClick={() => setDismissedUpdate(true)}
                className="font-bold text-slate-500 hover:text-slate-800 dark:text-amber-300/70 dark:hover:text-amber-100"
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
            className="flex flex-wrap items-center justify-between gap-2 rounded-xl border border-amber-200 bg-amber-50 px-4 py-2.5 text-xs text-amber-800 dark:border-amber-500/30 dark:bg-amber-950/40 dark:text-amber-200"
          >
            <span className="flex items-center gap-1.5">
              <AlertTriangle className="h-3.5 w-3.5" />
              {t("app.refresh_error")}: {loadError}
            </span>
            <button
              type="button"
              onClick={() => loadTelemetry(false)}
              className="rounded-md border border-amber-300 bg-white px-2.5 py-1 font-medium text-amber-800 hover:bg-amber-100 dark:border-amber-500/40 dark:bg-slate-900 dark:text-amber-200 dark:hover:bg-slate-800"
            >
              {t("app.retry")}
            </button>
          </div>
        )}

        {/* Global Filter & Time Window Control Strip */}
        <div className="flex flex-col justify-between gap-3 rounded-xl border border-slate-200/90 bg-white p-3.5 shadow-[0_1px_2px_rgba(15,23,42,0.03)] lg:flex-row lg:items-center dark:border-slate-800 dark:bg-slate-900 dark:shadow-none">
          {/* Left: Device Selector + CLI Source Filter Pills */}
          <div className="flex flex-wrap items-center gap-2.5">
            {/* Device Dropdown */}
            <div className="flex items-center gap-1.5">
              <Laptop className="h-3.5 w-3.5 text-slate-400 dark:text-slate-500" />
              <select
                aria-label={t("filters.device_label")}
                value={selectedDevice}
                onChange={(e) => setSelectedDevice(e.target.value)}
                className="cursor-pointer rounded-lg border border-slate-200 bg-slate-50/80 px-2.5 py-1.5 text-xs font-semibold text-slate-800 transition-colors hover:border-slate-300 focus:border-slate-900 focus:outline-none dark:border-slate-700 dark:bg-slate-800/80 dark:text-slate-100 dark:hover:border-slate-600 dark:focus:border-sky-400"
              >
                <option value="all">{t("filters.all_devices")}</option>
                {(data?.devices ?? []).map((dev) => (
                  <option key={dev.id} value={dev.id}>
                    {dev.display_name || dev.id}
                  </option>
                ))}
              </select>
            </div>

            <div className="hidden h-4 w-px bg-slate-200 sm:block dark:bg-slate-800" />

            {/* Source Segmented Pills */}
            <div className="flex flex-wrap items-center gap-1">
              <button
                type="button"
                onClick={() => setSelectedSource("")}
                className={`rounded-lg px-2.5 py-1.5 text-xs font-medium transition-all ${
                  selectedSource === ""
                    ? "bg-slate-900 font-semibold text-white shadow-2xs dark:bg-slate-100 dark:text-slate-950"
                    : "text-slate-600 hover:bg-slate-100 hover:text-slate-900 dark:text-slate-300 dark:hover:bg-slate-800 dark:hover:text-slate-100"
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
                        ? "border-slate-900 bg-slate-900 font-semibold text-white shadow-2xs dark:border-slate-200 dark:bg-slate-100 dark:text-slate-950"
                        : "border-transparent bg-slate-50/80 text-slate-700 hover:border-slate-200 hover:bg-white dark:bg-slate-800/60 dark:text-slate-300 dark:hover:border-slate-700 dark:hover:bg-slate-800"
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
                  ? "bg-sky-50 text-sky-600 dark:bg-sky-500/15 dark:text-sky-300"
                  : "bg-slate-100 text-slate-400 dark:bg-slate-800 dark:text-slate-500"
              }`}
            >
              <Activity className="h-2.5 w-2.5" />
              {isHydratingDetails ? t("app.stage_hydrating") : t("app.stage_synced")}
            </span>

            {/* Fixed 8 Time Windows: 1h, 24h, 7d, 30d, 3mo, 6mo, 1y, ALL */}
            <div className="max-w-full overflow-x-auto">
              <div
                role="tablist"
                aria-label={t("filters.window_label")}
                className="inline-flex rounded-lg border border-slate-200 bg-slate-100/80 p-0.5 dark:border-slate-700/80 dark:bg-slate-800/80"
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
                      className={`font-mono-num rounded-md px-2.5 py-1 text-xs font-semibold whitespace-nowrap transition-all ${
                        isActive
                          ? "bg-white text-slate-900 shadow-xs dark:bg-slate-900 dark:text-slate-100"
                          : "text-slate-500 hover:text-slate-900 dark:text-slate-400 dark:hover:text-slate-200"
                      }`}
                    >
                      {win}
                    </button>
                  );
                })}
              </div>
            </div>
          </div>
        </div>

        {/* ===================================================================
            3. FOUR HERO KPI CARDS
           =================================================================== */}
        <section className="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-4">
          {/* KPI 1: Estimated Cost */}
          <div className="relative overflow-hidden rounded-xl border border-slate-200/90 bg-white p-5 shadow-[0_1px_2px_rgba(15,23,42,0.03)] dark:border-slate-800 dark:bg-slate-900 dark:shadow-none">
            <div
              className="absolute inset-x-0 top-0 h-[2px] bg-slate-900 dark:bg-slate-200"
              aria-hidden="true"
            />
            <div className="flex items-center justify-between gap-2">
              <span className="text-[11px] font-semibold tracking-wider text-slate-400 uppercase dark:text-slate-500">
                {t("kpi.eyebrow_cost", { period: selectedWindow })}
              </span>
              <span className="font-mono-num rounded bg-slate-100 px-1.5 py-0.5 text-[10px] font-semibold text-slate-600 dark:bg-slate-800 dark:text-slate-300">
                {currency}
              </span>
            </div>
            <div className="mt-2 flex items-baseline justify-between gap-2">
              <div className="font-mono-num text-3xl font-bold tracking-tight text-slate-900 dark:text-slate-50">
                {formatCost(currentPeriod.cost, currency)}
              </div>
              <span className="font-mono-num text-xs font-medium text-slate-400 dark:text-slate-500">
                ~{formatCost(blendedCostPerM, currency)} / 1M
              </span>
            </div>
            <div className="mt-2.5 flex items-center justify-between gap-2 border-t border-slate-100 pt-2 text-xs text-slate-500 dark:border-slate-800 dark:text-slate-400">
              <span>{t("kpi.total_cost")}</span>
              <span className="truncate text-[11px] text-slate-400 dark:text-slate-500">
                {t("kpi.total_cost_sub")}
              </span>
            </div>
          </div>

          {/* KPI 2: Total Token Volume */}
          <div className="relative overflow-hidden rounded-xl border border-slate-200/90 bg-white p-5 shadow-[0_1px_2px_rgba(15,23,42,0.03)] dark:border-slate-800 dark:bg-slate-900 dark:shadow-none">
            <div
              className="absolute inset-x-0 top-0 h-[2px] bg-sky-500 dark:bg-sky-400"
              aria-hidden="true"
            />
            <div className="flex items-center justify-between gap-2">
              <span className="text-[11px] font-semibold tracking-wider text-slate-400 uppercase dark:text-slate-500">
                {t("kpi.eyebrow_tokens", { period: selectedWindow })}
              </span>
              <span className="font-mono-num rounded bg-sky-50 px-1.5 py-0.5 text-[10px] font-semibold text-sky-700 dark:bg-sky-500/15 dark:text-sky-300">
                {t("kpi.badge_in_out")}
              </span>
            </div>
            <div className="mt-2 flex items-baseline justify-between gap-2">
              <div
                className="font-mono-num text-3xl font-bold tracking-tight text-slate-900 dark:text-slate-50"
                title={formatExactNumber(totalVolumeTokens)}
              >
                {formatCompactNumber(totalVolumeTokens)}
              </div>
              <div className="font-mono-num text-[11px] text-slate-500 dark:text-slate-400">
                IN {formatCompactNumber(currentPeriod.input_tokens)} · OUT{" "}
                {formatCompactNumber(currentPeriod.output_tokens)}
              </div>
            </div>
            <div className="mt-2.5 flex items-center justify-between gap-2 border-t border-slate-100 pt-2 text-xs text-slate-500 dark:border-slate-800 dark:text-slate-400">
              <span>{t("kpi.total_tokens")}</span>
              <span
                className="truncate text-[11px] text-slate-400 dark:text-slate-500"
                title={
                  outputRate !== null
                    ? t("kpi.output_rate_tooltip", {
                        events: currentPeriod.timed_events || 0,
                      })
                    : undefined
                }
              >
                {outputRate !== null
                  ? `≈ ${outputRateLabel} tok/s`
                  : t("kpi.total_tokens_sub")}
              </span>
            </div>
          </div>

          {/* KPI 3: Cache Hit Rate + Radial Gauge */}
          <div className="relative overflow-hidden rounded-xl border border-slate-200/90 bg-white p-5 shadow-[0_1px_2px_rgba(15,23,42,0.03)] dark:border-slate-800 dark:bg-slate-900 dark:shadow-none">
            <div
              className="absolute inset-x-0 top-0 h-[2px] bg-emerald-500 dark:bg-emerald-400"
              aria-hidden="true"
            />
            <div className="flex items-start justify-between gap-3">
              <div>
                <span className="text-[11px] font-semibold tracking-wider text-slate-400 uppercase dark:text-slate-500">
                  {t("kpi.eyebrow_cache")}
                </span>
                <div className="font-mono-num mt-2 text-3xl font-bold tracking-tight text-emerald-600 dark:text-emerald-400">
                  {formatPercent(currentPeriod.cache_hit_rate)}
                </div>
              </div>

              {/* Handcrafted Radial SVG Ring */}
              <div className="relative flex h-12 w-12 shrink-0 items-center justify-center">
                <svg viewBox="0 0 44 44" className="h-full w-full -rotate-90" aria-hidden="true">
                  <circle
                    cx="22"
                    cy="22"
                    r={gaugeRadius}
                    fill="none"
                    className="stroke-slate-200 dark:stroke-slate-800"
                    strokeWidth="4"
                  />
                  <circle
                    cx="22"
                    cy="22"
                    r={gaugeRadius}
                    fill="none"
                    className="stroke-emerald-500 transition-all duration-500 dark:stroke-emerald-400"
                    strokeWidth="4"
                    strokeDasharray={gaugeCircumference}
                    strokeDashoffset={gaugeDashOffset}
                    strokeLinecap="round"
                  />
                </svg>
                <span className="font-mono-num absolute text-[10px] font-bold text-slate-700 dark:text-slate-200">
                  {Math.round(clampedHitRate)}%
                </span>
              </div>
            </div>

            <div className="mt-2.5 flex items-center justify-between gap-2 border-t border-slate-100 pt-2 text-xs dark:border-slate-800">
              <span className="text-slate-500 dark:text-slate-400">{t("kpi.cache_hit_rate")}</span>
              <span className="font-mono-num font-semibold text-emerald-700 dark:text-emerald-400">
                {t("kpi.cache_savings")} {formatCost(estCacheSavingsUsd, currency)}
              </span>
            </div>
          </div>

          {/* KPI 4: Base Input (Uncached New Input) */}
          <div className="relative overflow-hidden rounded-xl border border-slate-200/90 bg-white p-5 shadow-[0_1px_2px_rgba(15,23,42,0.03)] dark:border-slate-800 dark:bg-slate-900 dark:shadow-none">
            <div
              className="absolute inset-x-0 top-0 h-[2px] bg-amber-500 dark:bg-amber-400"
              aria-hidden="true"
            />
            <div className="flex items-center justify-between gap-2">
              <span className="text-[11px] font-semibold tracking-wider text-slate-400 uppercase dark:text-slate-500">
                {t("kpi.eyebrow_base_input")}
              </span>
              <span className="rounded bg-slate-900 px-1.5 py-0.5 text-[10px] font-semibold text-white dark:bg-amber-500/15 dark:text-amber-300">
                {t("kpi.badge_billable")}
              </span>
            </div>
            <div className="mt-2 flex items-baseline justify-between gap-2">
              <div
                className="font-mono-num text-3xl font-bold tracking-tight text-slate-900 dark:text-slate-50"
                title={formatExactNumber(baseInputTokens)}
              >
                {formatCompactNumber(baseInputTokens)}
              </div>
              <span className="font-mono-num text-xs text-slate-400 dark:text-slate-500">
                {baseInputPct !== null ? t("kpi.of_input", { pct: baseInputPct }) : "0%"}
              </span>
            </div>
            <div className="mt-2.5 flex items-center justify-between gap-2 border-t border-slate-100 pt-2 text-xs text-slate-500 dark:border-slate-800 dark:text-slate-400">
              <span>{t("kpi.base_input")}</span>
              <span className="truncate text-[11px] text-slate-400 dark:text-slate-500">
                {t("kpi.base_input_sub")}
              </span>
            </div>
          </div>
        </section>

        {/* ===================================================================
            4. FOUR-CATEGORY TOKEN TAXONOMY STRIP
           =================================================================== */}
        <TokenTaxonomyStrip period={currentPeriod} />

        {/* ===================================================================
            5. VISUAL ANALYTICS CHARTS (WINDOW-SCOPED TREND + CLI SOURCE SHARE)
           =================================================================== */}
        <ErrorBoundary fallback={renderSectionError}>
          <section className="grid grid-cols-1 gap-6 lg:grid-cols-12">
            <div className="lg:col-span-7">
              <DailyCostChart
                data={data?.charts?.daily_costs ?? []}
                granularity={data?.charts?.granularity ?? "daily"}
                period={selectedWindow}
                currency={currency}
                loading={isSummaryLoading}
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
        </ErrorBoundary>

        {/* ===================================================================
            6. GITHUB-STYLE ACTIVITY HEATMAP MATRIX
           =================================================================== */}
        <ErrorBoundary fallback={renderSectionError}>
          <ActivityHeatmap
            data={data?.charts?.calendar_heatmap ?? []}
            currency={currency}
          />
        </ErrorBoundary>

        {/* ===================================================================
            7. TWO-STAGE PROGRESSIVE ATTRIBUTION TABLES (MODELS & PROJECTS)
           =================================================================== */}
        <ErrorBoundary fallback={renderSectionError}>
          <AttributionTables
            sources={data?.sources ?? []}
            projects={data?.projects}
            windowLabel={selectedWindow}
            isHydratingDetails={isHydratingDetails}
            currency={currency}
          />
        </ErrorBoundary>
      </main>

      {/* =====================================================================
          8. SLIDE-OVER DRAWERS (LAN RADAR, DEVICES, PRICING, SYSTEM CONFIG)
         ===================================================================== */}
      <ErrorBoundary fallback={renderSectionError}>
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
      </ErrorBoundary>
    </div>
  );
}

export default App;
