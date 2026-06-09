import React, { lazy, Suspense, useEffect, useRef, useState } from "react";
import { type TFunction } from "i18next";
import { useTranslation } from "react-i18next";

import { LazyBlockFallback, LazyModalFallback } from "./components/LoadingFallbacks";
import {
  asRecord,
  mergeDashboardDetails,
  mergeSummaryWithPreviousDetails,
  normalizeDashboardData,
  type CalendarHeatmapBucket,
  type DashboardData,
  type DashboardCharts,
  type DeviceStats,
  type DailyCostBucket,
  type PeriodStats,
  type SourceModelStats,
  type SourceStats,
  type ToolShareBucket,
} from "./dashboardData";
import { fmt, fmtCost, fmtPercent, num, text } from "./format";
import { wailsWindow } from "./wails";

const SettingsModal = lazy(() => import("./SettingsModal"));
const Radar = lazy(() => import("./components/Radar"));

const dateShortLabel = (date: string) => {
  if (date.length >= 10) {
    return date.slice(5, 10).replace('-', '/');
  }
  return date;
};

const weekdayIndexUTC = (date: string) => {
  const parsed = new Date(`${date}T00:00:00Z`);
  return Number.isNaN(parsed.getTime()) ? 0 : parsed.getUTCDay();
};

const heatmapLevel = (tokens: number, maxTokens: number) => {
  if (tokens <= 0 || maxTokens <= 0) return 0;
  const ratio = tokens / maxTokens;
  if (ratio < 0.34) return 1;
  if (ratio < 0.67) return 2;
  return 3;
};

const heatmapClassName = (level: number) => {
  if (level === 3) return 'bg-[#000000]';
  if (level === 2) return 'bg-[#888888]';
  if (level === 1) return 'bg-[#CCCCCC]';
  return 'bg-[#FFFFFF]';
};

function DailyCostChart({ buckets, t }: { buckets: DailyCostBucket[]; t: TFunction }) {
  const maxCost = Math.max(0, ...buckets.map((bucket) => num(bucket.cost)));
  const totalCost = buckets.reduce((sum, bucket) => sum + num(bucket.cost), 0);

  return (
    <article className="border-[5px] border-[#000000] bg-[#FFFFFF] p-4 sm:p-5">
      <div className="mb-4 flex items-start justify-between gap-4 border-b-[3px] border-[#000000] pb-3">
        <h3 className="font-display text-xl sm:text-2xl uppercase leading-none">{t('dailyCost30d')}</h3>
        <div className="text-right font-mono text-xs uppercase">
          <div>{t('total30dCost')}</div>
          <div className="text-lg leading-none">{fmtCost(totalCost)}</div>
        </div>
      </div>
      <div className="flex h-44 items-end gap-[3px] border-b-[3px] border-l-[3px] border-[#000000] px-2 pt-2">
        {buckets.map((bucket) => {
          const cost = num(bucket.cost);
          const height = cost > 0 && maxCost > 0 ? Math.max(4, (cost / maxCost) * 100) : 0;
          return (
            <div key={bucket.date} className="flex h-full min-w-0 flex-1 items-end">
              <div
                data-testid="daily-cost-bar"
                aria-label={`${bucket.date} ${fmtCost(cost)}`}
                title={`${bucket.date} ${fmtCost(cost)}`}
                className={`w-full ${height > 0 ? 'border-x-[1px] border-t-[3px] border-[#000000] bg-[#000000]' : ''}`}
                style={{ height: `${height}%` }}
              ></div>
            </div>
          );
        })}
      </div>
      <div className="mt-3 flex items-center justify-between font-mono text-xs">
        <span>{dateShortLabel(buckets[0]?.date ?? '')}</span>
        <span>{t('peakDailyCost')}: {fmtCost(maxCost)}</span>
        <span>{dateShortLabel(buckets[buckets.length - 1]?.date ?? '')}</span>
      </div>
    </article>
  );
}

function CalendarHeatmap({ buckets, t }: { buckets: CalendarHeatmapBucket[]; t: TFunction }) {
  const maxTokens = Math.max(0, ...buckets.map((bucket) => num(bucket.tokens)));
  const firstWeekday = weekdayIndexUTC(buckets[0]?.date ?? '');
  const columns = Math.max(1, Math.ceil((firstWeekday + buckets.length) / 7));

  return (
    <article className="border-[5px] border-[#000000] bg-[#FFFFFF] p-4 sm:p-5">
      <div className="mb-4 flex items-start justify-between gap-4 border-b-[3px] border-[#000000] pb-3">
        <h3 className="font-display text-xl sm:text-2xl uppercase leading-none">{t('calendarHeatmap')}</h3>
        <div className="text-right font-mono text-xs uppercase">
          <div>{t('maxDayTokens')}</div>
          <div className="text-lg leading-none">{fmt(maxTokens)}</div>
        </div>
      </div>
      <div className="overflow-x-auto">
        <div
          className="grid w-fit gap-[4px]"
          style={{
            gridTemplateColumns: `repeat(${columns}, 16px)`,
            gridTemplateRows: 'repeat(7, 16px)',
          }}
        >
          {buckets.map((bucket, index) => {
            const position = firstWeekday + index;
            const level = heatmapLevel(num(bucket.tokens), maxTokens);
            return (
              <div
                key={bucket.date}
                data-testid="heatmap-cell"
                aria-label={`${bucket.date} ${fmt(bucket.tokens)} tokens`}
                title={`${bucket.date} / ${fmt(bucket.tokens)} tokens / ${fmtCost(bucket.cost)}`}
                className={`h-4 w-4 border-[2px] border-[#000000] ${heatmapClassName(level)}`}
                style={{
                  gridColumn: Math.floor(position / 7) + 1,
                  gridRow: (position % 7) + 1,
                }}
              ></div>
            );
          })}
        </div>
      </div>
      <div className="mt-3 flex items-center gap-2 font-mono text-xs uppercase">
        <span>{t('less')}</span>
        <span className="h-3 w-3 border-[2px] border-[#000000] bg-[#FFFFFF]"></span>
        <span className="h-3 w-3 border-[2px] border-[#000000] bg-[#CCCCCC]"></span>
        <span className="h-3 w-3 border-[2px] border-[#000000] bg-[#888888]"></span>
        <span className="h-3 w-3 border-[2px] border-[#000000] bg-[#000000]"></span>
        <span>{t('more')}</span>
      </div>
    </article>
  );
}

function ToolShareChart({ buckets, t }: { buckets: ToolShareBucket[]; t: TFunction }) {
  const totalTokens = buckets.reduce((sum, bucket) => sum + num(bucket.tokens), 0);

  return (
    <article className="border-[5px] border-[#000000] bg-[#FFFFFF] p-4 sm:p-5">
      <div className="mb-4 flex items-start justify-between gap-4 border-b-[3px] border-[#000000] pb-3">
        <h3 className="font-display text-xl sm:text-2xl uppercase leading-none">{t('toolTokenShare')}</h3>
        <div className="text-right font-mono text-xs uppercase">
          <div>{t('totalTokens')}</div>
          <div className="text-lg leading-none">{fmt(totalTokens)}</div>
        </div>
      </div>
      <div className="flex flex-col gap-4">
        {buckets.length === 0 && (
          <div className="border-[3px] border-[#000000] bg-[#F0F0F0] p-3 font-mono text-xs uppercase">{t('noChartData')}</div>
        )}
        {buckets.map((bucket) => {
          const share = totalTokens > 0 ? (num(bucket.tokens) / totalTokens) * 100 : 0;
          return (
            <div key={bucket.source} className="font-mono text-xs sm:text-sm">
              <div className="mb-1 flex items-center justify-between gap-3">
                <span className="font-bold">{bucket.source}</span>
                <span>{fmt(bucket.tokens)} / {fmtPercent(share)}</span>
              </div>
              <div className="h-7 border-[3px] border-[#000000] bg-[#FFFFFF]">
                <div className="h-full bg-[#000000]" style={{ width: `${share}%` }}></div>
              </div>
              <div className="mt-1 text-xs uppercase">{t('events')}: {bucket.events} / {t('subtotal')}: {fmtCost(bucket.total_cost)}</div>
            </div>
          );
        })}
      </div>
    </article>
  );
}

function CostIntelligenceSection({ charts, t }: { charts: DashboardCharts; t: TFunction }) {
  return (
    <section className="mb-12 md:mb-20">
      <div className="mb-5 border-b-[5px] border-[#000000] pb-3">
        <h2 className="font-display text-3xl sm:text-4xl md:text-5xl uppercase leading-none">{t('costIntelligence')}</h2>
      </div>
      <div className="grid grid-cols-1 gap-5 xl:grid-cols-[minmax(0,1.3fr)_minmax(260px,0.8fr)_minmax(280px,0.9fr)]">
        <DailyCostChart buckets={charts.daily_costs} t={t} />
        <CalendarHeatmap buckets={charts.calendar_heatmap} t={t} />
        <ToolShareChart buckets={charts.tool_share} t={t} />
      </div>
    </section>
  );
}

export default function App() {
  const { t, i18n } = useTranslation();
  const [data, setData] = useState<DashboardData | null>(null);
  const [selectedDevice, setSelectedDevice] = useState<string>("all");
  const [selectedSource, setSelectedSource] = useState<string>("");
  const [isSettingsOpen, setIsSettingsOpen] = useState(false);
  const [errorMsg, setErrorMsg] = useState<string>("");
  const [isStatsLoading, setIsStatsLoading] = useState(false);
  const [systemLogsNotice, setSystemLogsNotice] = useState<string>("");
  const [projectsCollapsed, setProjectsCollapsed] = useState(false);
  const [collapsedModelSources, setCollapsedModelSources] = useState<Record<string, boolean>>({});
  const statsRequestSeqRef = useRef(0);
  
  useEffect(() => {
    let cancelled = false;
    let inFlight = false;

    const statsURL = (detail: 'summary' | 'details') => {
      const params = new URLSearchParams({ device: selectedDevice, detail });
      if (selectedSource) params.set('source', selectedSource);
      return "/api/stats?" + params.toString();
    };

    const fetchData = async (showLoading: boolean) => {
      if (inFlight) return;
      inFlight = true;
      const requestSeq = statsRequestSeqRef.current + 1;
      statsRequestSeqRef.current = requestSeq;
      if (showLoading) {
        setIsStatsLoading(true);
      }
      try {
        const summaryRes = await fetch(statsURL('summary'));
        if (!summaryRes.ok) {
           throw new Error(`HTTP ${summaryRes.status}: ${await summaryRes.text()}`);
        }
        const summary = normalizeDashboardData(await summaryRes.json());
        if (cancelled || requestSeq !== statsRequestSeqRef.current) return;
        setData((prev) => showLoading ? summary : mergeSummaryWithPreviousDetails(summary, prev));
        setErrorMsg("");
        setIsStatsLoading(false);

        try {
          const detailsRes = await fetch(statsURL('details'));
          if (!detailsRes.ok) {
             throw new Error(`HTTP ${detailsRes.status}: ${await detailsRes.text()}`);
          }
          const details = normalizeDashboardData(await detailsRes.json());
          if (cancelled || requestSeq !== statsRequestSeqRef.current) return;
          setData(mergeDashboardDetails(summary, details));
        } catch (detailsError: unknown) {
          if (cancelled || requestSeq !== statsRequestSeqRef.current) return;
          console.warn('Failed to load stats details', detailsError);
        }
      } catch (e: unknown) {
        if (cancelled || requestSeq !== statsRequestSeqRef.current) return;
        console.error(e);
        setErrorMsg(e instanceof Error ? e.toString() : String(e));
      } finally {
        inFlight = false;
        if (!cancelled && requestSeq === statsRequestSeqRef.current) {
          setIsStatsLoading(false);
        }
      }
    };
    fetchData(true);
    const interval = setInterval(() => fetchData(false), 5000);
    return () => {
      cancelled = true;
      clearInterval(interval);
    };
  }, [selectedDevice, selectedSource]);

  const togglePause = async () => {
		try {
			const res = await fetch("/api/pause", { method: "POST" });
			if (res.ok) {
				const json = await res.json();
				setData(prev => prev ? { ...prev, is_paused: json.is_paused } : null);
			}
		} catch (e) {
			console.error("Failed to toggle pause", e);
		}
	};

  const toggleModelsCollapsed = (sourceName: string) => {
    setCollapsedModelSources((prev) => ({
      ...prev,
      [sourceName]: !prev[sourceName],
    }));
  };

  const openSystemLogs = async () => {
    try {
      const app = wailsWindow().go?.desktop?.App;
      if (app?.OpenSystemLogs) {
        await Promise.resolve(app.OpenSystemLogs());
        setSystemLogsNotice(t('systemLogsOpened'));
        return;
      }

      const res = await fetch('/api/system/logs');
      if (!res.ok) {
        throw new Error(`HTTP ${res.status}`);
      }
      const payload = asRecord(await res.json());
      const path = text(payload.path);
      setSystemLogsNotice(path ? `${t('systemLogsPath')}: ${path}` : t('systemLogsError'));
    } catch (e: unknown) {
      const message = e instanceof Error ? e.message : String(e);
      setSystemLogsNotice(`${t('systemLogsError')}: ${message}`);
    }
  };

  // Detect if we are running inside the Wails desktop app
  const isDesktop = typeof window !== 'undefined' && wailsWindow().go !== undefined;

  if (errorMsg) return (
    <div className={`min-h-screen bg-[#FFFFFF] text-[#FF0000] p-4 sm:p-6 md:p-10 flex items-center justify-center font-display text-xl sm:text-2xl uppercase border-[12px] border-[#FF0000] m-3 ${isDesktop ? 'wails-drag' : ''}`}>
      <div className="text-center">
        <div className="text-4xl md:text-5xl mb-4 text-[#000000] bg-[#FF0000] inline-block px-6 py-2">{t('systemError')}</div>
        <br/> {errorMsg}
      </div>
    </div>
  );

  if (!data) return (
    <div className={`min-h-screen bg-[#FFFFFF] text-[#000000] p-4 sm:p-6 md:p-10 flex items-center justify-center font-display text-4xl sm:text-5xl md:text-6xl uppercase border-[12px] border-[#000000] m-3 ${isDesktop ? 'wails-drag' : ''}`}>
      {t('systemInitializing')}
    </div>
  );

  const { periods = [], sources = [] } = data;
  const hasCostIntelligence = data.charts.daily_costs.length > 0 || data.charts.calendar_heatmap.length > 0 || data.charts.tool_share.length > 0;

  return (
    <div className={`bg-[#FFFFFF] text-[#000000] min-h-screen px-4 pb-4 sm:px-6 sm:pb-6 md:px-10 md:pb-10 font-sans selection:bg-[#000000] selection:text-[#FFFFFF] ${isDesktop ? 'pt-12' : 'pt-6 md:pt-10'}`}>
      
      {/* Dedicated invisible draggable titlebar for macOS native window controls */}
      {isDesktop && (
        <div className="h-10 w-full wails-drag fixed top-0 left-0 z-50 bg-[#FFFFFF]"></div>
      )}

      {isStatsLoading && (
        <div
          role="status"
          aria-live="polite"
          className={`fixed left-4 right-4 ${isDesktop ? 'top-14' : 'top-4'} z-[60] flex items-center gap-3 border-[5px] border-[#000000] bg-[#FFFFFF] px-4 py-3 font-display text-sm uppercase text-[#000000] sm:left-auto sm:right-6 sm:w-[260px]`}
          style={{ "--wails-draggable": "no-drag" } as React.CSSProperties}
        >
          <span className="h-4 w-4 shrink-0 animate-pulse bg-[#000000]" aria-hidden="true"></span>
          <span>{t('loading')}</span>
        </div>
      )}

      {/* Header Section */}
      <header className="flex flex-col md:flex-row justify-between items-start md:items-end gap-6 mb-16 border-b-[5px] border-[#000000] pb-6">
        <div>
          <h1 className="font-display text-5xl sm:text-6xl md:text-7xl leading-[1.0] uppercase tracking-tighter mb-4 break-words">
            {t('aiFlightDashboard').split(' ').map((word, i) => (
              <React.Fragment key={i}>{i > 0 && <br/>}{word}</React.Fragment>
            ))}
          </h1>
          <div className="flex flex-col sm:flex-row sm:items-center gap-4">
            <button
              onClick={togglePause}
              style={{ "--wails-draggable": "no-drag" } as React.CSSProperties}
              className={`border-[3px] px-3 py-1 font-sans text-xs font-semibold uppercase tracking-wider w-fit min-w-[100px] text-center cursor-pointer ${data?.is_paused ? 'border-[#FF0000] text-[#FF0000] hover:bg-[#FF0000] hover:text-[#FFFFFF]' : 'border-[#008000] text-[#008000] hover:bg-[#008000] hover:text-[#FFFFFF]'}`}
            >
              {data?.is_paused ? t('paused') : t('liveOperations')}
            </button>
            {data && (
              <select 
                value={selectedDevice} 
                onChange={e => setSelectedDevice(e.target.value)}
                className="bg-[#F0F0F0] text-[#000000] border-[3px] border-[#000000] rounded-none px-3 py-2 font-mono text-sm md:text-base outline-none focus:border-[5px] focus:m-[-2px] min-w-[140px]"
                style={{ "--wails-draggable": "no-drag" } as React.CSSProperties}
              >
                <option value="all">{t('allDevices')}</option>
                {data.devices?.map((d: DeviceStats) => (
                  <option key={d.id} value={d.id}>{(d.display_name || d.id).toUpperCase()}</option>
                ))}
              </select>
            )}
          </div>
        </div>
        <div className="font-mono text-sm md:text-base text-left md:text-right flex flex-col items-start md:items-end w-full md:w-auto mt-4 md:mt-0">
          <div>{t('dataRefreshRate')}</div>
          <div className="flex flex-wrap gap-4 mt-2">
            <button
              onClick={() => setIsSettingsOpen(true)}
              style={{ "--wails-draggable": "no-drag" } as React.CSSProperties}
              className="text-[#0000FF] uppercase underline decoration-[3px] underline-offset-4 cursor-pointer bg-transparent border-none p-0 hover:text-[#000000] whitespace-nowrap"
            >
              [ {t('settings')} ]
            </button>
            <button
              onClick={() => i18n.changeLanguage(i18n.language === 'zh' ? 'en' : 'zh')}
              style={{ "--wails-draggable": "no-drag" } as React.CSSProperties}
              className="text-[#0000FF] uppercase underline decoration-[3px] underline-offset-4 cursor-pointer bg-transparent border-none p-0 hover:text-[#000000] whitespace-nowrap"
            >
              [ {i18n.language === 'zh' ? 'EN' : '中'} ]
            </button>
            <button
              type="button"
              onClick={openSystemLogs}
              style={{ "--wails-draggable": "no-drag" } as React.CSSProperties}
              className="text-[#0000FF] uppercase underline decoration-[3px] underline-offset-4 cursor-pointer bg-transparent border-none p-0 hover:text-[#000000] whitespace-nowrap"
            >
              [ {t('systemLogs')} ]
            </button>
          </div>
          {systemLogsNotice && (
            <div role="status" className="mt-3 max-w-full md:max-w-[520px] break-all border-[3px] border-[#000000] bg-[#F0F0F0] p-2 text-xs text-[#000000]">
              {systemLogsNotice}
            </div>
          )}
        </div>
      </header>
      
      {isSettingsOpen && (
        <Suspense fallback={<LazyModalFallback />}>
          <SettingsModal onClose={() => setIsSettingsOpen(false)} />
        </Suspense>
      )}

      {/* Source Filter Tabs + PeriodCost Stats */}
      <section className="mb-12 md:mb-20">
        <div className="flex flex-wrap items-center gap-0 mb-6">
          {[
            { label: t('total'), value: '' },
            { label: 'CLAUDE', value: 'Claude Code' },
            { label: 'GEMINI', value: 'Gemini CLI' },
            { label: 'CODEX', value: 'Codex' },
            { label: 'ANTIGRAVITY', value: 'Antigravity' },
          ].map((tab) => (
            <button
              key={tab.value}
              onClick={() => setSelectedSource(tab.value)}
              className={`px-4 py-2 font-display text-sm uppercase tracking-wider border-[3px] border-[#000000] cursor-pointer transition-none -ml-[3px] first:ml-0 ${
                selectedSource === tab.value
                  ? 'bg-[#000000] text-[#FFFFFF]'
                  : 'bg-[#FFFFFF] text-[#000000] hover:bg-[#F0F0F0]'
              }`}
              style={{ "--wails-draggable": "no-drag" } as React.CSSProperties}
            >
              {tab.label}
            </button>
          ))}
        </div>
        <div className="grid grid-cols-1 sm:grid-cols-2 md:grid-cols-3 xl:grid-cols-5 gap-4 md:gap-6">
        {periods.map((p: PeriodStats) => {
          const isElevated = p.label === 'ALL';
          const cardClass = `bg-[#FFFFFF] border-[#000000] rounded-none p-4 md:p-6 flex flex-col justify-between shadow-none ${isElevated ? 'border-[5px]' : 'border-[3px]'}`;
          return (
            <div key={p.label} className={cardClass}>
              <div className="mb-4">
                <h3 className="font-display text-xl xl:text-2xl leading-[1.1] uppercase mb-2">{p.label}</h3>
                <div className="flex flex-col gap-1 font-mono text-xs xl:text-sm">
                  <span>{t('labelIn')}: {fmt(p.input_tokens)}</span>
                  <span>{t('cacheRead')}: {fmt(p.cached_tokens)}</span>
                  <span>{t('cacheWrite')}: {fmt(p.cache_creation_tokens)}</span>
                  <span>{t('labelOut')}: {fmt(p.output_tokens)}</span>
                  <span>{t('cacheHitRate')}: {fmtPercent(p.cache_hit_rate)}</span>
                </div>
              </div>
              <div className="font-mono text-2xl md:text-3xl leading-none mt-4 border-t-[3px] border-[#000000] pt-4 whitespace-nowrap overflow-hidden text-ellipsis">
                {fmtCost(p.cost)}
              </div>
            </div>
          )
        })}
        </div>
      </section>

      {hasCostIntelligence && (
        <CostIntelligenceSection charts={data.charts} t={t} />
      )}

      {/* LAN Radar Component */}
      <Suspense fallback={<LazyBlockFallback />}>
        <Radar />
      </Suspense>

      {/* Projects Section */}
      {data.projects && data.projects.length > 0 && (
        <section className="mb-12 md:mb-20">
          <article className="bg-[#FFFFFF] border-[5px] border-[#000000] rounded-none shadow-none flex flex-col">
            <div className="p-4 sm:p-6 border-b-[5px] border-[#000000] flex flex-col md:flex-row justify-between items-start md:items-end gap-4 bg-[#000000] text-[#FFFFFF]">
              <div className="break-words w-full">
                <h2 className="font-display text-3xl sm:text-4xl md:text-5xl uppercase leading-[1.05] break-words">
                  {t('projects') || 'PROJECT STATS'}
                </h2>
              </div>
              <button
                type="button"
                aria-label={projectsCollapsed ? t('expandProjects') : t('collapseProjects')}
                onClick={() => setProjectsCollapsed((collapsed) => !collapsed)}
                style={{ "--wails-draggable": "no-drag" } as React.CSSProperties}
                className="shrink-0 border-[3px] border-[#FFFFFF] bg-[#000000] px-3 py-2 font-display text-xs sm:text-sm uppercase text-[#FFFFFF] hover:bg-[#FFFFFF] hover:text-[#000000] cursor-pointer"
              >
                {projectsCollapsed ? t('expand') : t('collapse')}
              </button>
            </div>
            {!projectsCollapsed && (
              <div className="overflow-x-auto">
                <table className="w-full text-left font-mono text-xs sm:text-sm min-w-[600px]">
                  <thead>
                    <tr className="border-b-[5px] border-[#000000] bg-[#F0F0F0]">
                      <th className="px-3 py-3 sm:px-4 sm:py-4 font-display text-xs sm:text-sm uppercase">{t('project')}</th>
                      <th className="px-3 py-3 sm:px-4 sm:py-4 font-display text-xs sm:text-sm uppercase">{t('events')}</th>
                      <th className="px-3 py-3 sm:px-4 sm:py-4 font-display text-xs sm:text-sm uppercase">{t('tokens')}</th>
                      <th className="px-3 py-3 sm:px-4 sm:py-4 font-display text-xs sm:text-sm uppercase text-right">{t('subtotal')}</th>
                    </tr>
                  </thead>
                  <tbody>
                    {data.projects.map((p) => (
                      <tr key={p.project} className="border-b-[3px] border-[#000000] last:border-b-0 hover:bg-[#000000] hover:text-[#FFFFFF] transition-none group">
                        <td className="px-3 py-3 sm:px-4 sm:py-4 font-bold group-hover:text-[#FFFFFF] truncate max-w-[300px]" title={p.project}>{p.project}</td>
                        <td className="px-3 py-3 sm:px-4 sm:py-4 group-hover:text-[#FFFFFF]">{p.events}</td>
                        <td className="px-3 py-3 sm:px-4 sm:py-4 group-hover:text-[#FFFFFF]">
                          {t('labelIn')}: {fmt(p.input_tokens)} / {t('cacheRead')}: {fmt(p.cached_tokens)} / {t('cacheWrite')}: {fmt(p.cache_creation_tokens)} / {t('labelOut')}: {fmt(p.output_tokens)} / {t('cacheHitRate')}: {fmtPercent(p.cache_hit_rate)}
                        </td>
                        <td className="px-3 py-3 sm:px-4 sm:py-4 text-right font-bold group-hover:text-[#FFFFFF]">{fmtCost(p.total_cost)}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </article>
        </section>
      )}

      {/* Source Stats Grid */}
      <section className="grid grid-cols-1 2xl:grid-cols-2 gap-6 lg:gap-10">
        {sources.map((src: SourceStats) => {
           const baseInput = Math.max(0, num(src.total_input) - num(src.total_cached) - num(src.total_cache_creation));
           const totalTokens = baseInput + num(src.total_cached) + num(src.total_cache_creation) + num(src.total_output);
           
           const inPct = totalTokens > 0 ? (baseInput / totalTokens) * 100 : 0;
           const cachedPct = totalTokens > 0 ? (num(src.total_cached) / totalTokens) * 100 : 0;
           const cacheCreationPct = totalTokens > 0 ? (num(src.total_cache_creation) / totalTokens) * 100 : 0;
           const outPct = totalTokens > 0 ? (num(src.total_output) / totalTokens) * 100 : 0;

           const formatPct = (pct: number) => {
             if (pct > 0 && pct < 1) return '<1%';
             return pct.toFixed(0) + '%';
           };

           const sortedModels = [...(src.models || [])].sort((a: SourceModelStats, b: SourceModelStats) => num(b.total_cost) - num(a.total_cost));
           const modelsCollapsed = Boolean(collapsedModelSources[src.name]);

           return (
            <article key={src.name} className="bg-[#FFFFFF] border-[5px] border-[#000000] rounded-none shadow-none flex flex-col">
              <div className="p-4 sm:p-6 border-b-[5px] border-[#000000] flex flex-col md:flex-row justify-between items-start md:items-end gap-4 bg-[#000000] text-[#FFFFFF]">
                <div className="break-words w-full">
                  <h2 className="font-display text-3xl sm:text-4xl md:text-5xl uppercase leading-[1.05] break-words">
                    {src.name}
                  </h2>
                </div>
                <div className="text-left md:text-right shrink-0">
                  <span className="font-display text-xs sm:text-sm uppercase mb-1 block">{t('totalSpend')}</span>
                  <div className="font-mono text-3xl sm:text-4xl md:text-5xl leading-none">{fmtCost(src.total_cost)}</div>
                </div>
              </div>

              <div className="grid grid-cols-1 lg:grid-cols-2 border-b-[5px] border-[#000000]">
                {/* Token Distribution Grid */}
                <div className="p-4 sm:p-6 border-b-[5px] lg:border-b-0 lg:border-r-[5px] border-[#000000] grid grid-cols-2 sm:grid-cols-3 gap-4 sm:gap-6">
                  <div className="border-l-[5px] border-[#000000] pl-3">
                    <span className="font-display text-xs sm:text-sm uppercase block mb-1">{t('baseInput')}</span>
                    <div className="font-mono text-xl sm:text-2xl">{fmt(baseInput)}</div>
                  </div>
                  <div className="border-l-[5px] border-[#000000] pl-3">
                    <span className="font-display text-xs sm:text-sm uppercase block mb-1">{t('cacheRead')}</span>
                    <div className="font-mono text-xl sm:text-2xl">{fmt(src.total_cached)}</div>
                  </div>
                  <div className="border-l-[5px] border-[#000000] pl-3">
                    <span className="font-display text-xs sm:text-sm uppercase block mb-1">{t('cacheWrite')}</span>
                    <div className="font-mono text-xl sm:text-2xl">{fmt(src.total_cache_creation)}</div>
                  </div>
                  <div className="border-l-[5px] border-[#000000] pl-3">
                    <span className="font-display text-xs sm:text-sm uppercase block mb-1">{t('outputTokens')}</span>
                    <div className="font-mono text-xl sm:text-2xl">{fmt(src.total_output)}</div>
                  </div>
                  <div className="border-l-[5px] border-[#000000] pl-3">
                    <span className="font-display text-xs sm:text-sm uppercase block mb-1">{t('totalTokens')}</span>
                    <div className="font-mono text-xl sm:text-2xl">{fmt(totalTokens)}</div>
                  </div>
                  <div className="border-l-[5px] border-[#000000] pl-3">
                    <span className="font-display text-xs sm:text-sm uppercase block mb-1">{t('cacheHitRate')}</span>
                    <div className="font-mono text-xl sm:text-2xl">{fmtPercent(src.cache_hit_rate)}</div>
                  </div>
                </div>

                {/* Brutalist Progress Bar Area */}
                <div className="p-4 sm:p-6 flex flex-col justify-center">
                  <div className="w-full h-8 sm:h-10 border-[3px] border-[#000000] flex mb-4">
                    <div style={{width: `${inPct}%`}} className="bg-[#000000] h-full border-r-[3px] border-[#000000]"></div>
                    <div style={{width: `${cachedPct}%`}} className="bg-[#CCCCCC] h-full border-r-[3px] border-[#000000]"></div>
                    <div style={{width: `${cacheCreationPct}%`}} className="bg-[#888888] h-full border-r-[3px] border-[#000000]"></div>
                    <div style={{width: `${outPct}%`}} className="bg-[#FFFFFF] h-full"></div>
                  </div>
                  <div className="flex flex-col gap-2 font-mono text-xs sm:text-sm">
                    <div className="flex items-center gap-2">
                      <span className="w-4 h-4 bg-[#000000] border-[3px] border-[#000000] shrink-0"></span> {t('labelIn')} ({formatPct(inPct)})
                    </div>
                    <div className="flex items-center gap-2">
                      <span className="w-4 h-4 bg-[#CCCCCC] border-[3px] border-[#000000] shrink-0"></span> {t('cacheRead')} ({formatPct(cachedPct)})
                    </div>
                    <div className="flex items-center gap-2">
                      <span className="w-4 h-4 bg-[#888888] border-[3px] border-[#000000] shrink-0"></span> {t('cacheWrite')} ({formatPct(cacheCreationPct)})
                    </div>
                    <div className="flex items-center gap-2">
                      <span className="w-4 h-4 bg-[#FFFFFF] border-[3px] border-[#000000] shrink-0"></span> {t('labelOut')} ({formatPct(outPct)})
                    </div>
                  </div>
                </div>
              </div>

              {/* Model Table */}
              <div>
                <div className="flex items-center justify-between gap-4 bg-[#F0F0F0] px-3 py-3 sm:px-4 sm:py-4">
                  <h3 className="font-display text-xs sm:text-sm uppercase">
                    {t('models')} ({sortedModels.length})
                  </h3>
                  <button
                    type="button"
                    aria-label={modelsCollapsed ? t('expandModels') : t('collapseModels')}
                    onClick={() => toggleModelsCollapsed(src.name)}
                    style={{ "--wails-draggable": "no-drag" } as React.CSSProperties}
                    className="shrink-0 border-[3px] border-[#000000] bg-[#FFFFFF] px-3 py-2 font-display text-xs sm:text-sm uppercase text-[#000000] hover:bg-[#000000] hover:text-[#FFFFFF] cursor-pointer"
                  >
                    {modelsCollapsed ? t('expand') : t('collapse')}
                  </button>
                </div>
                {!modelsCollapsed && (
                  <div className="overflow-x-auto">
                    <table className="w-full text-left font-mono text-xs sm:text-sm min-w-[760px]">
                      <thead>
                        <tr className="border-y-[5px] border-[#000000] bg-[#F0F0F0]">
                          <th className="px-3 py-3 sm:px-4 sm:py-4 font-display text-xs sm:text-sm uppercase">{t('modelIdentifier')}</th>
                          <th className="px-3 py-3 sm:px-4 sm:py-4 font-display text-xs sm:text-sm uppercase">{t('rates1M')}</th>
                          <th className="px-3 py-3 sm:px-4 sm:py-4 font-display text-xs sm:text-sm uppercase">{t('tokens')}</th>
                          <th className="px-3 py-3 sm:px-4 sm:py-4 font-display text-xs sm:text-sm uppercase">{t('events')}</th>
                          <th className="px-3 py-3 sm:px-4 sm:py-4 font-display text-xs sm:text-sm uppercase text-right">{t('subtotal')}</th>
                        </tr>
                      </thead>
                      <tbody>
                        {sortedModels.map((m: SourceModelStats) => (
                          <tr key={m.model} className="border-b-[3px] border-[#000000] last:border-b-0 hover:bg-[#000000] hover:text-[#FFFFFF] transition-none group">
                            <td className="px-3 py-3 sm:px-4 sm:py-4 font-bold group-hover:text-[#FFFFFF] max-w-[200px] truncate" title={m.model}>{m.model}</td>
                            <td className="px-3 py-3 sm:px-4 sm:py-4 group-hover:text-[#FFFFFF]">
                              {t('labelIn')}: {fmtCost(m.input_price_per_m)} / {t('cacheRead')}: {fmtCost(m.cached_price_per_m)} / {t('cacheWrite')}: {fmtCost(m.cache_creation_price_per_m)} / {t('labelOut')}: {fmtCost(m.output_price_per_m)}
                            </td>
                            <td className="px-3 py-3 sm:px-4 sm:py-4 group-hover:text-[#FFFFFF]">
                              {t('labelIn')}: {fmt(m.input_tokens)} / {t('cacheRead')}: {fmt(m.cached_tokens)} / {t('cacheWrite')}: {fmt(m.cache_creation_tokens)} / {t('labelOut')}: {fmt(m.output_tokens)} / {t('cacheHitRate')}: {fmtPercent(m.cache_hit_rate)}
                            </td>
                            <td className="px-3 py-3 sm:px-4 sm:py-4 group-hover:text-[#FFFFFF]">{m.events}</td>
                            <td className="px-3 py-3 sm:px-4 sm:py-4 text-right font-bold group-hover:text-[#FFFFFF]">{fmtCost(m.total_cost)}</td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </div>
                )}
              </div>
            </article>
           );
        })}
      </section>
    </div>
  );
}
