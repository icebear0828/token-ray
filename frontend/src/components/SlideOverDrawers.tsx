import React, { useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import {
  AlertCircle,
  Check,
  CheckCircle2,
  Cpu,
  DollarSign,
  FileText,
  FolderSearch,
  Laptop,
  Package,
  Plus,
  Radio,
  RefreshCw,
  Server,
  Sliders,
  Trash2,
  Wifi,
  WifiOff,
  X,
} from "lucide-react";
import {
  deleteDeviceAlias,
  fetchConfig,
  fetchDevices,
  fetchLanScan,
  fetchLanStatus,
  fetchLANSelf,
  fetchPluginIndex,
  fetchPlugins,
  fetchPricing,
  fetchSystemLogsPath,
  installPluginURL,
  installPluginZip,
  joinLan,
  leaveLan,
  openSystemLogs,
  removePlugin,
  saveConfig,
  savePricing,
  setDeviceAlias,
  setPluginEnabled,
  softDeleteDevice,
} from "../api/client";
import {
  AppConfig,
  CurrencyCode,
  DeviceSummary,
  InstalledPlugin,
  LanPeer,
  PluginIndexEntry,
  PricingEntry,
} from "../types/telemetry";
import { formatCompactNumber, formatCost } from "../utils/formatters";
import { SourceBadge } from "./SourceBadge";

export type DrawerType = "lan" | "devices" | "pricing" | "plugins" | "config" | null;

interface VersionInfo {
  version: string;
  goos: string;
  goarch: string;
}

interface UpdateStatus {
  current_version: string;
  latest_version: string;
  has_update: boolean;
  release_notes?: string;
  published_at?: string;
  download_url?: string;
  asset_name?: string;
  error?: string;
}

interface UpdateApplyResponse {
  success: boolean;
  message: string;
  version?: string;
}

interface SlideOverDrawersProps {
  activeDrawer: DrawerType;
  onClose: () => void;
  currency: CurrencyCode;
  onDataMutated: () => void;
}

const emptyPricingEntry = (model: string): PricingEntry => ({
  model,
  input_price_per_m: 0,
  cached_price_per_m: 0,
  cache_creation_price_per_m: 0,
  output_price_per_m: 0,
});

const deviceTotalTokens = (device: DeviceSummary): number =>
  device.input_tokens +
  device.cached_tokens +
  device.cache_creation_tokens +
  device.output_tokens;

const formatTimestamp = (value?: string): string => {
  if (!value) return "";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString();
};

export const SlideOverDrawers: React.FC<SlideOverDrawersProps> = ({
  activeDrawer,
  onClose,
  currency,
  onDataMutated,
}) => {
  const { t } = useTranslation();

  // LAN Radar state
  const [lanEnabled, setLanEnabled] = useState(true);
  const [lanPeers, setLanPeers] = useState<LanPeer[]>([]);
  const [lanBusy, setLanBusy] = useState(false);

  // Devices state
  const [devices, setDevices] = useState<DeviceSummary[]>([]);
  const [localDeviceId, setLocalDeviceId] = useState("");
  const [aliasDrafts, setAliasDrafts] = useState<Record<string, string>>({});
  const [newDeviceId, setNewDeviceId] = useState("");
  const [newDeviceName, setNewDeviceName] = useState("");
  const [deviceBusyId, setDeviceBusyId] = useState<string | null>(null);
  const [confirmingDeleteId, setConfirmingDeleteId] = useState<string | null>(null);

  // Pricing state
  const [pricingList, setPricingList] = useState<PricingEntry[]>([]);
  const [newModelName, setNewModelName] = useState("");
  const [pricingSaving, setPricingSaving] = useState(false);
  const [pricingSaved, setPricingSaved] = useState(false);

  // Config state
  const [config, setConfig] = useState<AppConfig | null>(null);
  const [newPath, setNewPath] = useState("");
  const [newPeerHost, setNewPeerHost] = useState("");
  const [logsPath, setLogsPath] = useState<string>("");
  const [logsNotice, setLogsNotice] = useState<string>("");
  const [configSaving, setConfigSaving] = useState(false);
  const [configSaved, setConfigSaved] = useState(false);

  // Plugin state
  const [pluginList, setPluginList] = useState<InstalledPlugin[]>([]);
  const [pluginIndex, setPluginIndex] = useState<PluginIndexEntry[] | null>(null);
  const [pluginBusy, setPluginBusy] = useState<string | null>(null);
  const [pluginMessage, setPluginMessage] = useState<string | null>(null);
  const [pluginError, setPluginError] = useState<string | null>(null);
  const pluginFileRef = useRef<HTMLInputElement>(null);

  // Update & Version state
  const [versionInfo, setVersionInfo] = useState<VersionInfo | null>(null);
  const [updateStatus, setUpdateStatus] = useState<UpdateStatus | null>(null);
  const [checkingUpdate, setCheckingUpdate] = useState<boolean>(false);
  const [applyingUpdate, setApplyingUpdate] = useState<boolean>(false);
  const [updateResult, setUpdateResult] = useState<string | null>(null);
  const [updateError, setUpdateError] = useState<string | null>(null);

  // LAN 3-second polling when drawer is open
  useEffect(() => {
    if (activeDrawer !== "lan") return;
    let cancelled = false;

    const pollLan = async () => {
      try {
        const [statusRes, scanRes] = await Promise.all([fetchLanStatus(), fetchLanScan()]);
        if (cancelled) return;
        setLanEnabled(statusRes.enabled);
        setLanPeers(scanRes.peer_infos ?? []);
      } catch {
        // Keep the last known state; the drawer is a secondary surface.
      }
    };

    pollLan();
    const timer = setInterval(pollLan, 3000);
    return () => {
      cancelled = true;
      clearInterval(timer);
    };
  }, [activeDrawer]);

  // Load drawer-specific datasets on open
  useEffect(() => {
    let cancelled = false;

    if (activeDrawer === "devices") {
      Promise.all([fetchDevices(), fetchLANSelf().catch(() => ({ device_id: "" }))])
        .then(([list, self]) => {
          if (cancelled) return;
          setDevices(list);
          setLocalDeviceId(self.device_id);
          const drafts: Record<string, string> = {};
          list.forEach((device) => {
            drafts[device.id] = device.display_name ?? "";
          });
          setAliasDrafts(drafts);
        })
        .catch(() => undefined);
    } else if (activeDrawer === "pricing") {
      fetchPricing()
        .then((list) => {
          if (!cancelled) setPricingList(list);
        })
        .catch(() => undefined);
    } else if (activeDrawer === "plugins") {
      Promise.all([fetchPlugins(), fetchConfig()])
        .then(([list, cfg]) => {
          if (cancelled) return;
          setPluginList(list);
          setConfig(cfg);
        })
        .catch((err: unknown) => {
          if (!cancelled) {
            setPluginError(err instanceof Error ? err.message : String(err));
          }
        });
    } else if (activeDrawer === "config") {
      fetch("/api/system/version")
        .then((res) => (res.ok ? res.json() : null))
        .then((v: VersionInfo | null) => {
          if (!cancelled && v) setVersionInfo(v);
        })
        .catch(() => undefined);
      Promise.all([fetchConfig(), fetchSystemLogsPath().catch(() => ({ path: "" }))])
        .then(([cfg, logObj]) => {
          if (cancelled) return;
          setConfig(cfg);
          setLogsPath(logObj.path);
        })
        .catch(() => undefined);
    }

    return () => {
      cancelled = true;
    };
  }, [activeDrawer]);

  const sortedPricing = useMemo(
    () => [...pricingList].sort((a, b) => a.model.localeCompare(b.model)),
    [pricingList]
  );

  if (!activeDrawer) return null;

  const handleToggleLan = async () => {
    setLanBusy(true);
    try {
      if (lanEnabled) {
        await leaveLan();
      } else {
        await joinLan();
      }
      const [statusRes, scanRes] = await Promise.all([fetchLanStatus(), fetchLanScan()]);
      setLanEnabled(statusRes.enabled);
      setLanPeers(scanRes.peer_infos ?? []);
      onDataMutated();
    } finally {
      setLanBusy(false);
    }
  };

  const handleSaveAlias = async (deviceId: string) => {
    setDeviceBusyId(deviceId);
    try {
      const value = (aliasDrafts[deviceId] ?? "").trim();
      if (value) {
        await setDeviceAlias(deviceId, value);
      } else {
        await deleteDeviceAlias(deviceId);
      }
      const updated = await fetchDevices();
      setDevices(updated);
      onDataMutated();
    } finally {
      setDeviceBusyId(null);
    }
  };

  const handleClearAlias = async (deviceId: string) => {
    setDeviceBusyId(deviceId);
    try {
      await deleteDeviceAlias(deviceId);
      setAliasDrafts((prev) => ({ ...prev, [deviceId]: "" }));
      const updated = await fetchDevices();
      setDevices(updated);
      onDataMutated();
    } finally {
      setDeviceBusyId(null);
    }
  };

  const handleAddAlias = async () => {
    const deviceId = newDeviceId.trim();
    const displayName = newDeviceName.trim();
    if (!deviceId || !displayName) return;
    setDeviceBusyId(deviceId);
    try {
      await setDeviceAlias(deviceId, displayName);
      setNewDeviceId("");
      setNewDeviceName("");
      const updated = await fetchDevices();
      setDevices(updated);
      setAliasDrafts((prev) => ({ ...prev, [deviceId]: displayName }));
      onDataMutated();
    } finally {
      setDeviceBusyId(null);
    }
  };

  const handleSoftDelete = async (deviceId: string) => {
    setDeviceBusyId(deviceId);
    try {
      await softDeleteDevice(deviceId);
      setConfirmingDeleteId(null);
      const updated = await fetchDevices();
      setDevices(updated);
      onDataMutated();
    } finally {
      setDeviceBusyId(null);
    }
  };

  const handlePricingChange = (
    idx: number,
    field: keyof Omit<PricingEntry, "model">,
    raw: string
  ) => {
    const value = parseFloat(raw);
    setPricingList((prev) =>
      prev.map((item, i) =>
        i === idx ? { ...item, [field]: Number.isFinite(value) ? Math.max(0, value) : 0 } : item
      )
    );
  };

  const handleAddModel = () => {
    const model = newModelName.trim();
    if (!model) return;
    if (pricingList.some((entry) => entry.model === model)) return;
    setPricingList((prev) => [emptyPricingEntry(model), ...prev]);
    setNewModelName("");
  };

  const handleSavePricing = async () => {
    setPricingSaving(true);
    try {
      await savePricing(pricingList);
      setPricingSaved(true);
      onDataMutated();
      setTimeout(() => setPricingSaved(false), 2200);
    } finally {
      setPricingSaving(false);
    }
  };

  const updateConfig = (patch: Partial<AppConfig>) => {
    setConfig((prev) => (prev ? { ...prev, ...patch } : prev));
  };

  const handleAddPath = () => {
    const value = newPath.trim();
    if (!value || !config) return;
    if ((config.extra_watch_dirs ?? []).includes(value)) return;
    updateConfig({ extra_watch_dirs: [...(config.extra_watch_dirs ?? []), value] });
    setNewPath("");
  };

  const handleRemovePath = (index: number) => {
    if (!config) return;
    const next = [...(config.extra_watch_dirs ?? [])];
    next.splice(index, 1);
    updateConfig({ extra_watch_dirs: next });
  };

  const handleAddPeer = () => {
    const value = newPeerHost.trim();
    if (!value || !config) return;
    if ((config.extra_peer_hosts ?? []).includes(value)) return;
    updateConfig({ extra_peer_hosts: [...(config.extra_peer_hosts ?? []), value] });
    setNewPeerHost("");
  };

  const handleRemovePeer = (index: number) => {
    if (!config) return;
    const next = [...(config.extra_peer_hosts ?? [])];
    next.splice(index, 1);
    updateConfig({ extra_peer_hosts: next });
  };

  const handleOpenLogs = async () => {
    setLogsNotice("");
    try {
      const path = await openSystemLogs();
      setLogsNotice(path ? `${t("config.log_path")}: ${path}` : t("config.logs_opened"));
    } catch {
      setLogsNotice(logsPath || t("config.logs_unavailable"));
    }
  };

  const handleSaveConfig = async () => {
    if (!config) return;
    setConfigSaving(true);
    try {
      await saveConfig(config);
      setConfigSaved(true);
      onDataMutated();
      setTimeout(() => setConfigSaved(false), 2200);
    } finally {
      setConfigSaving(false);
    }
  };

  const handleCheckUpdate = async () => {
    setCheckingUpdate(true);
    setUpdateResult(null);
    setUpdateError(null);
    try {
      const res = await fetch("/api/system/update/check");
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      const data = (await res.json()) as UpdateStatus;
      setUpdateStatus(data);
      if (data.error) setUpdateError(data.error);
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : String(err);
      setUpdateError(msg);
    } finally {
      setCheckingUpdate(false);
    }
  };

  const handleApplyUpdate = async () => {
    setApplyingUpdate(true);
    setUpdateResult(null);
    setUpdateError(null);
    try {
      const res = await fetch("/api/system/update/apply", {
        method: "POST",
      });
      const data = (await res.json().catch(() => null)) as UpdateApplyResponse | null;
      if (!res.ok && !data) throw new Error(`HTTP ${res.status}`);
      if (data?.success) {
        setUpdateResult(data.message || t("updateSuccess"));
      } else {
        setUpdateError(data?.message || t("updateFailed"));
      }
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : String(err);
      setUpdateError(msg);
    } finally {
      setApplyingUpdate(false);
    }
  };

  const refreshPlugins = async () => {
    const list = await fetchPlugins();
    setPluginList(list);
    onDataMutated();
  };

  const handleInstallPluginFile = async () => {
    const file = pluginFileRef.current?.files?.[0];
    if (!file) {
      setPluginError(t("plugins.select_file"));
      return;
    }
    setPluginBusy("install-file");
    setPluginError(null);
    setPluginMessage(null);
    try {
      const manifest = await installPluginZip(file);
      setPluginMessage(t("plugins.installed_toast", { name: manifest.name }));
      if (pluginFileRef.current) pluginFileRef.current.value = "";
      await refreshPlugins();
    } catch (err: unknown) {
      setPluginError(err instanceof Error ? err.message : String(err));
    } finally {
      setPluginBusy(null);
    }
  };

  const handleInstallPluginRemote = async (entry: PluginIndexEntry) => {
    setPluginBusy(`remote:${entry.id}`);
    setPluginError(null);
    setPluginMessage(null);
    try {
      const manifest = await installPluginURL(entry.download_url, entry.sha256);
      setPluginMessage(t("plugins.installed_toast", { name: manifest.name }));
      await refreshPlugins();
    } catch (err: unknown) {
      setPluginError(err instanceof Error ? err.message : String(err));
    } finally {
      setPluginBusy(null);
    }
  };

  const handleTogglePlugin = async (plugin: InstalledPlugin) => {
    setPluginBusy(`plugin:${plugin.manifest.id}`);
    setPluginError(null);
    setPluginMessage(null);
    try {
      await setPluginEnabled(plugin.manifest.id, !plugin.enabled);
      await refreshPlugins();
    } catch (err: unknown) {
      setPluginError(err instanceof Error ? err.message : String(err));
    } finally {
      setPluginBusy(null);
    }
  };

  const handleRemovePlugin = async (plugin: InstalledPlugin) => {
    if (!window.confirm(t("plugins.remove_confirm", { name: plugin.manifest.name }))) return;
    setPluginBusy(`plugin:${plugin.manifest.id}`);
    setPluginError(null);
    setPluginMessage(null);
    try {
      await removePlugin(plugin.manifest.id);
      await refreshPlugins();
    } catch (err: unknown) {
      setPluginError(err instanceof Error ? err.message : String(err));
    } finally {
      setPluginBusy(null);
    }
  };

  const handleLoadPluginIndex = async () => {
    setPluginBusy("load-index");
    setPluginError(null);
    setPluginMessage(null);
    try {
      if (config) {
        await saveConfig(config);
      }
      const rows = await fetchPluginIndex();
      setPluginIndex(rows);
      if (rows.length === 0) setPluginMessage(t("plugins.index_empty"));
    } catch (err: unknown) {
      setPluginError(err instanceof Error ? err.message : String(err));
    } finally {
      setPluginBusy(null);
    }
  };

  const renderSyncBadge = (status?: string) => {
    switch (status) {
      case "ok":
        return (
          <span className="inline-flex items-center gap-1 rounded-full bg-emerald-50 px-2 py-0.5 text-[10px] font-semibold text-emerald-700 uppercase">
            <span className="h-1.5 w-1.5 rounded-full bg-emerald-500" />
            OK
          </span>
        );
      case "syncing":
        return (
          <span className="inline-flex items-center gap-1 rounded-full bg-blue-50 px-2 py-0.5 text-[10px] font-semibold text-blue-700 uppercase">
            <RefreshCw className="h-2.5 w-2.5 animate-spin" />
            SYNCING
          </span>
        );
      case "error":
        return (
          <span className="inline-flex items-center gap-1 rounded-full bg-rose-50 px-2 py-0.5 text-[10px] font-semibold text-rose-700 uppercase">
            <AlertCircle className="h-2.5 w-2.5" />
            ERROR
          </span>
        );
      default:
        return (
          <span className="inline-flex items-center gap-1 rounded-full bg-slate-100 px-2 py-0.5 text-[10px] font-semibold text-slate-600 uppercase">
            {(status || "DISCOVERY").toUpperCase()}
          </span>
        );
    }
  };

  return (
    <div className="fixed inset-0 z-50 flex justify-end bg-slate-900/25 backdrop-blur-[2px] transition-opacity">
      <div className="flex-1" onClick={onClose} />

      <aside className="flex h-full w-full max-w-xl flex-col border-l border-slate-200 bg-white shadow-2xl">
        <div className="flex items-center justify-between border-b border-slate-100 px-6 py-4">
          <div className="flex items-center gap-3">
            <div className="flex h-9 w-9 items-center justify-center rounded-lg border border-slate-200 bg-slate-50 text-slate-800">
              {activeDrawer === "lan" && <Radio className="h-4 w-4" />}
              {activeDrawer === "devices" && <Laptop className="h-4 w-4" />}
              {activeDrawer === "pricing" && <DollarSign className="h-4 w-4" />}
              {activeDrawer === "plugins" && <Package className="h-4 w-4" />}
              {activeDrawer === "config" && <Sliders className="h-4 w-4" />}
            </div>
            <div>
              <div className="text-[10px] font-semibold tracking-wider text-slate-400 uppercase">
                {activeDrawer === "lan" && t("lan.subtitle")}
                {activeDrawer === "devices" && t("devices.subtitle")}
                {activeDrawer === "pricing" && t("pricing.subtitle")}
                {activeDrawer === "plugins" && t("plugins.subtitle")}
                {activeDrawer === "config" && t("config.subtitle")}
              </div>
              <h2 className="text-base font-semibold text-slate-900">
                {activeDrawer === "lan" && t("lan.title")}
                {activeDrawer === "devices" && t("devices.title")}
                {activeDrawer === "pricing" && t("pricing.title")}
                {activeDrawer === "plugins" && t("plugins.title")}
                {activeDrawer === "config" && (
                  <span>
                    {t("config.title")}
                    <span className="sr-only"> - {t("settingsPanel")} / SETTINGS PANEL</span>
                  </span>
                )}
              </h2>
            </div>
          </div>

          <button
            type="button"
            onClick={onClose}
            aria-label={t("drawers.close")}
            className="rounded-lg border border-slate-200 p-1.5 text-slate-500 transition-colors hover:bg-slate-100 hover:text-slate-900"
          >
            <X className="h-4 w-4" />
          </button>
        </div>

        <div className="flex-1 overflow-y-auto p-6">
          {/* 1. LAN RADAR PANEL */}
          {activeDrawer === "lan" && (
            <div className="space-y-6">
              <div className="relative overflow-hidden rounded-xl border border-slate-200 bg-slate-950 p-5 text-white">
                <div className="flex flex-wrap items-center justify-between gap-4">
                  <div className="flex items-center gap-4">
                    <div className="relative flex h-16 w-16 shrink-0 items-center justify-center">
                      <svg viewBox="0 0 80 80" className="h-full w-full">
                        <circle cx="40" cy="40" r="36" fill="none" stroke="rgba(148,163,184,0.2)" strokeWidth="1" />
                        <circle cx="40" cy="40" r="24" fill="none" stroke="rgba(148,163,184,0.2)" strokeWidth="1" />
                        <circle cx="40" cy="40" r="12" fill="none" stroke="rgba(148,163,184,0.25)" strokeWidth="1" />
                        <line x1="40" y1="4" x2="40" y2="76" stroke="rgba(148,163,184,0.15)" strokeWidth="1" />
                        <line x1="4" y1="40" x2="76" y2="40" stroke="rgba(148,163,184,0.15)" strokeWidth="1" />
                        {lanEnabled && (
                          <g className="origin-center animate-radar-sweep">
                            <path d="M40 40 L40 4 A36 36 0 0 1 71 22 Z" fill="rgba(16,185,129,0.28)" />
                            <line x1="40" y1="40" x2="40" y2="4" stroke="#34D399" strokeWidth="1.5" />
                          </g>
                        )}
                        <circle cx="40" cy="40" r="3.5" fill={lanEnabled ? "#10B981" : "#64748B"} />
                        {lanEnabled && lanPeers.length > 0 && (
                          <>
                            <circle cx="56" cy="24" r="3" fill="#38BDF8" />
                            <circle cx="24" cy="52" r="3" fill="#A78BFA" />
                          </>
                        )}
                      </svg>
                    </div>

                    <div>
                      <div className="flex items-center gap-2">
                        {lanEnabled ? (
                          <Wifi className="h-4 w-4 text-emerald-400" />
                        ) : (
                          <WifiOff className="h-4 w-4 text-slate-400" />
                        )}
                        <span className="text-sm font-semibold">
                          {lanEnabled ? t("lan.status_active") : t("lan.status_inactive")}
                        </span>
                      </div>
                      <p className="mt-1 text-xs text-slate-400">
                        {t("lan.peers_found")}:{" "}
                        <strong className="font-mono-num text-white">{lanPeers.length}</strong> ·
                        UDP/HTTP Local Mesh
                      </p>
                    </div>
                  </div>

                  <button
                    type="button"
                    disabled={lanBusy}
                    onClick={handleToggleLan}
                    className={`rounded-lg px-3.5 py-2 text-xs font-semibold transition-all ${
                      lanEnabled
                        ? "border border-slate-700 bg-slate-800 text-slate-200 hover:bg-slate-700"
                        : "bg-emerald-500 text-slate-950 hover:bg-emerald-400"
                    }`}
                  >
                    {lanEnabled ? t("lan.leave_btn") : t("lan.join_btn")}
                  </button>
                </div>
              </div>

              {lanPeers.length === 0 ? (
                <div className="rounded-xl border border-dashed border-slate-200 bg-slate-50/70 p-8 text-center">
                  <Server className="mx-auto h-7 w-7 text-slate-300" />
                  <div className="mt-2 text-xs font-semibold text-slate-700">{t("lan.no_peers")}</div>
                  <p className="mt-1 text-[11px] text-slate-400">{t("lan.no_peers_hint")}</p>
                </div>
              ) : (
                <div className="space-y-3">
                  {lanPeers.map((peer) => (
                    <div key={peer.id} className="rounded-xl border border-slate-200 bg-white p-4 shadow-2xs">
                      <div className="flex items-start justify-between gap-2">
                        <div>
                          <div className="flex items-center gap-2">
                            <span className="text-sm font-semibold text-slate-900">
                              {peer.display_name || peer.id}
                            </span>
                            {renderSyncBadge(peer.sync_status)}
                          </div>
                          <div className="font-mono-num mt-0.5 text-[11px] text-slate-400">
                            {peer.ip || "127.0.0.1"}:{peer.http_port || 19100} · ID: {peer.id}
                          </div>
                        </div>

                        <div className="text-right">
                          <div className="font-mono-num text-sm font-bold text-slate-900">
                            {formatCost(peer.cost_total ?? 0, currency)}
                          </div>
                          <div className="text-[10px] text-slate-400">{t("lan.cost_total")}</div>
                        </div>
                      </div>

                      {peer.sync_error && (
                        <div className="mt-2 rounded-md bg-rose-50 px-2.5 py-1.5 text-[11px] text-rose-700">
                          {peer.sync_error}
                        </div>
                      )}

                      <div className="mt-3 grid grid-cols-2 gap-2 rounded-lg bg-slate-50 p-2.5 text-xs">
                        <div>
                          <span className="text-[10px] font-semibold text-slate-400 uppercase">
                            {t("lan.tokens_24h")}
                          </span>
                          <div className="font-mono-num font-semibold text-slate-800">
                            {formatCompactNumber(peer.tokens_24h ?? 0)}
                          </div>
                        </div>
                        <div>
                          <span className="text-[10px] font-semibold text-slate-400 uppercase">
                            {t("lan.tokens_total")}
                          </span>
                          <div className="font-mono-num font-semibold text-slate-800">
                            {formatCompactNumber(peer.tokens_total ?? 0)}
                          </div>
                        </div>
                      </div>

                      {peer.sources && peer.sources.length > 0 && (
                        <div className="mt-3 flex flex-wrap gap-1.5">
                          {peer.sources.map((source) => (
                            <div
                              key={source.source}
                              className="inline-flex items-center gap-1.5 rounded-md border border-slate-200 bg-white px-2 py-1 text-[11px]"
                            >
                              <SourceBadge source={source.source} />
                              <span className="font-mono-num font-medium text-slate-700">
                                {formatCompactNumber(source.tokens_total ?? 0)}
                              </span>
                            </div>
                          ))}
                        </div>
                      )}
                    </div>
                  ))}
                </div>
              )}
            </div>
          )}

          {/* 2. DEVICES & ALIASES PANEL */}
          {activeDrawer === "devices" && (
            <div className="space-y-4" data-testid="device-management-list">
              <div className="rounded-lg border border-amber-200/80 bg-amber-50/70 px-3.5 py-2.5 text-xs text-amber-800">
                {t("devices.soft_delete_hint")}
              </div>

              <div className="rounded-xl border border-slate-200 bg-slate-50/70 p-3.5">
                <div className="text-[11px] font-semibold text-slate-500">
                  {t("devices.add_alias")}
                </div>
                <div className="mt-1.5 grid grid-cols-1 gap-2 sm:grid-cols-[1fr_1fr_auto]">
                  <input
                    type="text"
                    value={newDeviceId}
                    onChange={(event) => setNewDeviceId(event.target.value)}
                    placeholder={t("devices.device_id_placeholder")}
                    className="font-mono-num min-w-0 rounded-lg border border-slate-200 bg-white px-2.5 py-1.5 text-xs text-slate-900 focus:border-slate-900 focus:outline-none"
                  />
                  <input
                    type="text"
                    value={newDeviceName}
                    onChange={(event) => setNewDeviceName(event.target.value)}
                    placeholder={t("devices.add_alias_name_placeholder")}
                    className="min-w-0 rounded-lg border border-slate-200 bg-white px-2.5 py-1.5 text-xs text-slate-900 focus:border-slate-900 focus:outline-none"
                  />
                  <button
                    type="button"
                    disabled={!newDeviceId.trim() || !newDeviceName.trim() || Boolean(deviceBusyId)}
                    onClick={handleAddAlias}
                    className="inline-flex items-center justify-center gap-1 rounded-lg bg-slate-900 px-3 py-1.5 text-xs font-semibold text-white hover:bg-slate-800 disabled:opacity-50"
                  >
                    <Plus className="h-3.5 w-3.5" />
                    {t("devices.add_alias_btn")}
                  </button>
                </div>
              </div>

              {devices.length === 0 && (
                <div className="rounded-xl border border-dashed border-slate-200 bg-slate-50/70 p-8 text-center text-xs text-slate-500">
                  {t("devices.no_devices")}
                </div>
              )}

              {devices.map((device) => {
                const isLocal = device.id === localDeviceId;
                const isConfirming = confirmingDeleteId === device.id;
                return (
                  <div
                    key={device.id}
                    data-testid="device-row"
                    className="rounded-xl border border-slate-200 bg-white p-4"
                  >
                    <div className="flex flex-wrap items-center justify-between gap-2">
                      <div className="flex flex-wrap items-center gap-2">
                        <Cpu className="h-4 w-4 text-slate-500" />
                        <span className="font-mono-num text-xs font-semibold text-slate-800">
                          {device.display_name || device.id}
                        </span>
                        {isLocal && (
                          <span className="rounded bg-slate-900 px-1.5 py-0.5 text-[9px] font-bold tracking-wider text-white uppercase">
                            {t("devices.local_badge")}
                          </span>
                        )}
                      </div>

                      <div className="font-mono-num text-xs font-semibold text-slate-700">
                        {formatCompactNumber(deviceTotalTokens(device))} tok ·{" "}
                        {formatCost(device.total_cost, currency)}
                      </div>
                    </div>

                    <div className="mt-1 flex flex-wrap gap-x-3 gap-y-1 text-[11px] text-slate-400">
                      <span className="font-mono-num">{device.id}</span>
                      <span>
                        {t("devices.events")}: {formatCompactNumber(device.events)}
                      </span>
                      <span>
                        {t("devices.last_seen")}:{" "}
                        {device.last_seen ? formatTimestamp(device.last_seen) : t("devices.never")}
                      </span>
                    </div>

                    <div className="mt-3 flex flex-wrap items-center gap-2">
                      <input
                        type="text"
                        value={aliasDrafts[device.id] ?? ""}
                        onChange={(event) =>
                          setAliasDrafts((prev) => ({ ...prev, [device.id]: event.target.value }))
                        }
                        placeholder={t("devices.alias_placeholder")}
                        className="min-w-[160px] flex-1 rounded-lg border border-slate-200 px-2.5 py-1.5 text-xs text-slate-800 focus:border-slate-900 focus:outline-none"
                      />
                      <button
                        type="button"
                        disabled={deviceBusyId === device.id}
                        onClick={() => handleSaveAlias(device.id)}
                        className="rounded-lg bg-slate-900 px-3 py-1.5 text-xs font-medium text-white hover:bg-slate-800 disabled:opacity-50"
                      >
                        {t("devices.save_alias")}
                      </button>
                      {device.display_name && device.display_name !== device.id && (
                        <button
                          type="button"
                          disabled={deviceBusyId === device.id}
                          onClick={() => handleClearAlias(device.id)}
                          className="rounded-lg border border-slate-200 px-2.5 py-1.5 text-xs font-medium text-slate-600 hover:bg-slate-50 disabled:opacity-50"
                        >
                          {t("devices.clear_alias")}
                        </button>
                      )}
                      {!isLocal && !isConfirming && (
                        <button
                          type="button"
                          disabled={deviceBusyId === device.id}
                          onClick={() => setConfirmingDeleteId(device.id)}
                          title={t("devices.soft_delete")}
                          className="ml-auto inline-flex items-center gap-1 rounded-lg border border-rose-200 bg-rose-50 px-2.5 py-1.5 text-xs font-medium text-rose-700 hover:bg-rose-100 disabled:opacity-50"
                        >
                          <Trash2 className="h-3.5 w-3.5" />
                          <span>{t("devices.soft_delete")}</span>
                        </button>
                      )}
                      {isConfirming && (
                        <div className="ml-auto inline-flex items-center gap-1.5">
                          <button
                            type="button"
                            disabled={deviceBusyId === device.id}
                            onClick={() => handleSoftDelete(device.id)}
                            className="rounded-lg bg-rose-600 px-2.5 py-1.5 text-xs font-medium text-white hover:bg-rose-700 disabled:opacity-50"
                          >
                            {t("devices.confirm_delete")}
                          </button>
                          <button
                            type="button"
                            onClick={() => setConfirmingDeleteId(null)}
                            className="rounded-lg border border-slate-200 px-2.5 py-1.5 text-xs font-medium text-slate-600 hover:bg-slate-50"
                          >
                            {t("devices.cancel")}
                          </button>
                        </div>
                      )}
                    </div>
                  </div>
                );
              })}
            </div>
          )}

          {/* 3. MODEL PRICING EDITOR PANEL */}
          {activeDrawer === "pricing" && (
            <div className="space-y-4">
              <div className="rounded-xl border border-slate-200 bg-slate-50/70 p-3.5">
                <label className="block text-[11px] font-semibold text-slate-500">
                  {t("pricing.add_model")}
                </label>
                <div className="mt-1.5 flex gap-2">
                  <input
                    type="text"
                    value={newModelName}
                    onChange={(event) => setNewModelName(event.target.value)}
                    onKeyDown={(event) => {
                      if (event.key === "Enter") handleAddModel();
                    }}
                    placeholder={t("pricing.model_placeholder")}
                    className="font-mono-num min-w-0 flex-1 rounded-lg border border-slate-200 bg-white px-2.5 py-1.5 text-xs text-slate-900 focus:border-slate-900 focus:outline-none"
                  />
                  <button
                    type="button"
                    onClick={handleAddModel}
                    className="inline-flex items-center gap-1 rounded-lg bg-slate-900 px-3 py-1.5 text-xs font-semibold text-white hover:bg-slate-800"
                  >
                    <Plus className="h-3.5 w-3.5" />
                    {t("pricing.add_btn")}
                  </button>
                </div>
              </div>

              {sortedPricing.length === 0 && (
                <div className="rounded-xl border border-dashed border-slate-200 bg-slate-50/70 p-8 text-center text-xs text-slate-500">
                  {t("pricing.no_entries")}
                </div>
              )}

              <div className="space-y-3">
                {sortedPricing.map((entry) => {
                  const idx = pricingList.findIndex((item) => item.model === entry.model);
                  return (
                    <div key={entry.model} className="rounded-xl border border-slate-200 bg-white p-3.5">
                      <div className="mb-2.5 flex items-center justify-between gap-2">
                        <span className="font-mono-num text-xs font-semibold text-slate-900">
                          {entry.model}
                        </span>
                      </div>

                      <div className="grid grid-cols-2 gap-2.5 sm:grid-cols-4">
                        <div>
                          <label className="block text-[10px] font-medium text-slate-400">
                            {t("pricing.input_m")}
                          </label>
                          <input
                            type="number"
                            step="0.01"
                            value={entry.input_price_per_m}
                            onChange={(event) =>
                              handlePricingChange(idx, "input_price_per_m", event.target.value)
                            }
                            className="font-mono-num mt-1 w-full rounded-md border border-slate-200 px-2 py-1 text-xs text-slate-900 focus:border-slate-900 focus:outline-none"
                          />
                        </div>
                        <div>
                          <label className="block text-[10px] font-medium text-slate-400">
                            {t("pricing.cached_m")}
                          </label>
                          <input
                            type="number"
                            step="0.01"
                            value={entry.cached_price_per_m}
                            onChange={(event) =>
                              handlePricingChange(idx, "cached_price_per_m", event.target.value)
                            }
                            className="font-mono-num mt-1 w-full rounded-md border border-slate-200 px-2 py-1 text-xs text-slate-900 focus:border-slate-900 focus:outline-none"
                          />
                        </div>
                        <div>
                          <label className="block text-[10px] font-medium text-slate-400">
                            {t("pricing.write_m")}
                          </label>
                          <input
                            type="number"
                            step="0.01"
                            value={entry.cache_creation_price_per_m}
                            onChange={(event) =>
                              handlePricingChange(idx, "cache_creation_price_per_m", event.target.value)
                            }
                            className="font-mono-num mt-1 w-full rounded-md border border-slate-200 px-2 py-1 text-xs text-slate-900 focus:border-slate-900 focus:outline-none"
                          />
                        </div>
                        <div>
                          <label className="block text-[10px] font-medium text-slate-400">
                            {t("pricing.output_m")}
                          </label>
                          <input
                            type="number"
                            step="0.01"
                            value={entry.output_price_per_m}
                            onChange={(event) =>
                              handlePricingChange(idx, "output_price_per_m", event.target.value)
                            }
                            className="font-mono-num mt-1 w-full rounded-md border border-slate-200 px-2 py-1 text-xs text-slate-900 focus:border-slate-900 focus:outline-none"
                          />
                        </div>
                      </div>
                    </div>
                  );
                })}
              </div>

              <div className="sticky bottom-0 flex items-center justify-between border-t border-slate-100 bg-white pt-4">
                {pricingSaved ? (
                  <span className="inline-flex items-center gap-1.5 text-xs font-medium text-emerald-600">
                    <CheckCircle2 className="h-4 w-4" />
                    {t("pricing.saved_toast")}
                  </span>
                ) : (
                  <span />
                )}
                <button
                  type="button"
                  disabled={pricingSaving}
                  onClick={handleSavePricing}
                  className="inline-flex items-center gap-1.5 rounded-lg bg-slate-900 px-4 py-2 text-xs font-semibold text-white hover:bg-slate-800 disabled:opacity-50"
                >
                  <Check className="h-3.5 w-3.5" />
                  {t("pricing.save_btn")}
                </button>
              </div>
            </div>
          )}

          {/* 4. SYSTEM & SCANNER CONFIG PANEL */}
          {/* 4. SOURCE PLUGINS PANEL */}
          {activeDrawer === "plugins" && config && (
            <div className="space-y-5">
              {pluginMessage && (
                <div
                  role="status"
                  className="rounded-lg border border-emerald-200 bg-emerald-50 p-2.5 text-xs text-emerald-700"
                >
                  {pluginMessage}
                </div>
              )}
              {pluginError && (
                <div
                  role="alert"
                  className="rounded-lg border border-rose-200 bg-rose-50 p-2.5 text-xs text-rose-700"
                >
                  {pluginError}
                </div>
              )}

              <div>
                <div className="mb-2 text-xs font-semibold text-slate-800">
                  {t("plugins.installed")}
                </div>
                <div className="space-y-3">
                  {pluginList.length === 0 && (
                    <div className="rounded-xl border border-dashed border-slate-200 bg-slate-50/70 p-6 text-center text-xs text-slate-500">
                      {t("plugins.empty")}
                    </div>
                  )}
                  {pluginList.map((plugin) => (
                    <div
                      key={plugin.manifest.id}
                      className="rounded-xl border border-slate-200 bg-white p-4"
                    >
                      <div className="flex flex-wrap items-start justify-between gap-3">
                        <div className="min-w-0">
                          <div className="flex flex-wrap items-center gap-2">
                            <span className="text-sm font-semibold text-slate-900">
                              {plugin.manifest.name}
                            </span>
                            <span className="font-mono-num rounded bg-slate-900 px-1.5 py-0.5 text-[10px] font-bold text-white">
                              v{plugin.manifest.version}
                            </span>
                            <span
                              className={`rounded px-1.5 py-0.5 text-[10px] font-semibold uppercase ${
                                plugin.enabled
                                  ? "bg-emerald-50 text-emerald-700"
                                  : "bg-slate-100 text-slate-500"
                              }`}
                            >
                              {plugin.enabled ? t("plugins.enabled") : t("plugins.disabled")}
                            </span>
                          </div>
                          <p className="mt-1 text-[11px] text-slate-500">
                            {plugin.manifest.description || plugin.manifest.source}
                          </p>
                          <div className="font-mono-num mt-1 break-all text-[10px] text-slate-400">
                            {plugin.path}
                          </div>
                        </div>
                        <div className="flex shrink-0 gap-2">
                          <button
                            type="button"
                            disabled={pluginBusy === `plugin:${plugin.manifest.id}`}
                            onClick={() => handleTogglePlugin(plugin)}
                            className="rounded-lg border border-slate-200 bg-white px-3 py-1.5 text-xs font-semibold text-slate-700 hover:bg-slate-100 disabled:opacity-50"
                          >
                            {plugin.enabled ? t("plugins.disable") : t("plugins.enable")}
                          </button>
                          <button
                            type="button"
                            disabled={pluginBusy === `plugin:${plugin.manifest.id}`}
                            onClick={() => handleRemovePlugin(plugin)}
                            className="inline-flex items-center gap-1 rounded-lg border border-rose-200 bg-rose-50 px-3 py-1.5 text-xs font-semibold text-rose-700 hover:bg-rose-100 disabled:opacity-50"
                          >
                            <Trash2 className="h-3.5 w-3.5" />
                            {t("plugins.remove")}
                          </button>
                        </div>
                      </div>
                    </div>
                  ))}
                </div>
              </div>

              <div className="rounded-xl border border-slate-200 bg-slate-50/70 p-4">
                <div className="flex items-center gap-1.5 text-xs font-semibold text-slate-800">
                  <Package className="h-4 w-4 text-slate-500" />
                  <span>{t("plugins.install_file")}</span>
                </div>
                <p className="mt-1 text-[11px] text-slate-400">{t("plugins.install_file_desc")}</p>
                <div className="mt-3 flex flex-col gap-2 sm:flex-row">
                  <input
                    ref={pluginFileRef}
                    type="file"
                    accept=".zip,application/zip"
                    className="min-w-0 flex-1 rounded-lg border border-slate-200 bg-white px-2.5 py-1.5 text-xs text-slate-700 file:mr-2 file:rounded file:border-0 file:bg-slate-100 file:px-2 file:py-1 file:text-[11px] file:font-semibold file:text-slate-700"
                  />
                  <button
                    type="button"
                    disabled={pluginBusy === "install-file"}
                    onClick={handleInstallPluginFile}
                    className="rounded-lg bg-slate-900 px-3 py-1.5 text-xs font-semibold text-white hover:bg-slate-800 disabled:opacity-50"
                  >
                    {pluginBusy === "install-file" ? t("plugins.installing") : t("plugins.install")}
                  </button>
                </div>
              </div>

              <div className="rounded-xl border border-slate-200 bg-slate-50/70 p-4">
                <div className="text-xs font-semibold text-slate-800">{t("plugins.index")}</div>
                <p className="mt-1 text-[11px] text-slate-400">{t("plugins.index_desc")}</p>
                <div className="mt-3 flex flex-col gap-2">
                  <input
                    type="text"
                    value={config.plugin_index_url ?? ""}
                    onChange={(event) => updateConfig({ plugin_index_url: event.target.value })}
                    placeholder="https://example.com/plugins/index.json"
                    className="font-mono-num min-w-0 rounded-lg border border-slate-200 bg-white px-2.5 py-1.5 text-xs text-slate-900 focus:border-slate-900 focus:outline-none"
                  />
                  <button
                    type="button"
                    disabled={pluginBusy === "load-index"}
                    onClick={handleLoadPluginIndex}
                    className="self-start rounded-lg border border-slate-200 bg-white px-3 py-1.5 text-xs font-semibold text-slate-700 hover:bg-slate-100 disabled:opacity-50"
                  >
                    {pluginBusy === "load-index" ? t("app.loading") : t("plugins.load_index")}
                  </button>
                </div>

                {pluginIndex && (
                  <div className="mt-3 space-y-2">
                    {pluginIndex.map((entry) => (
                      <div
                        key={entry.id}
                        className="flex flex-wrap items-center justify-between gap-2 rounded-lg border border-slate-200 bg-white p-3"
                      >
                        <div className="min-w-0">
                          <div className="flex items-center gap-2">
                            <span className="text-xs font-semibold text-slate-800">
                              {entry.name}
                            </span>
                            <span className="font-mono-num rounded bg-slate-900 px-1.5 py-0.5 text-[10px] font-bold text-white">
                              v{entry.version}
                            </span>
                          </div>
                          <p className="mt-0.5 text-[11px] text-slate-500">
                            {entry.description || entry.id}
                          </p>
                        </div>
                        <button
                          type="button"
                          disabled={pluginBusy === `remote:${entry.id}`}
                          onClick={() => handleInstallPluginRemote(entry)}
                          className="rounded-lg bg-slate-900 px-3 py-1.5 text-xs font-semibold text-white hover:bg-slate-800 disabled:opacity-50"
                        >
                          {pluginBusy === `remote:${entry.id}`
                            ? t("plugins.installing")
                            : t("plugins.install")}
                        </button>
                      </div>
                    ))}
                  </div>
                )}
              </div>
            </div>
          )}

          {activeDrawer === "config" && config && (
            <div className="space-y-5">
              {/* Version & Auto-Update Card */}
              <div className="rounded-xl border border-slate-200 bg-slate-50/70 p-4">
                <div className="flex items-center justify-between gap-3">
                  <div>
                    <div className="text-xs font-semibold text-slate-800">
                      {t("versionAndUpdates")}
                    </div>
                    <div className="mt-1 flex items-center gap-2">
                      <span className="font-mono-num rounded bg-slate-900 px-2 py-0.5 text-xs font-bold text-white">
                        {versionInfo?.version || t("unknown")}
                      </span>
                      <span className="font-mono-num text-[11px] text-slate-500">
                        {versionInfo ? `${versionInfo.goos}/${versionInfo.goarch}` : t("unknown")}
                      </span>
                    </div>
                  </div>

                  <button
                    type="button"
                    onClick={handleCheckUpdate}
                    disabled={checkingUpdate || applyingUpdate}
                    className="inline-flex items-center gap-1.5 rounded-lg border border-slate-200 bg-white px-3 py-1.5 text-xs font-semibold text-slate-700 hover:bg-slate-100 disabled:opacity-50"
                  >
                    <RefreshCw className={`h-3.5 w-3.5 ${checkingUpdate ? "animate-spin" : ""}`} />
                    <span>{checkingUpdate ? t("checkingUpdates") : t("checkForUpdates")}</span>
                  </button>
                </div>

                {updateError && (
                  <div role="alert" className="mt-3 rounded-lg border border-rose-200 bg-rose-50 p-2.5 text-xs text-rose-700">
                    {updateError}
                  </div>
                )}

                {updateResult && (
                  <div role="status" className="mt-3 rounded-lg border border-emerald-200 bg-emerald-50 p-2.5 text-xs text-emerald-700">
                    {updateResult}
                  </div>
                )}

                {updateStatus && !updateStatus.has_update && !updateError && (
                  <div className="mt-3 rounded-lg border border-slate-200 bg-white p-2.5 text-xs text-slate-600">
                    {t("alreadyLatest")}
                  </div>
                )}

                {updateStatus?.has_update && (
                  <div className="mt-3 space-y-2 rounded-lg border border-amber-200 bg-amber-50/80 p-3 text-xs">
                    <div className="flex items-center justify-between">
                      <div className="font-semibold text-amber-900">
                        🚀 {t("newVersionAvailable")}:{" "}
                        <span className="font-mono-num font-bold underline">
                          {updateStatus.latest_version}
                        </span>
                      </div>
                      <button
                        type="button"
                        onClick={handleApplyUpdate}
                        disabled={applyingUpdate}
                        className="inline-flex items-center gap-1 rounded-lg bg-amber-600 px-3 py-1.5 font-semibold text-white hover:bg-amber-700 disabled:opacity-50"
                      >
                        {applyingUpdate ? t("updating") : t("applyUpdate")}
                      </button>
                    </div>

                    {updateStatus.asset_name && (
                      <div className="font-mono-num text-[11px] text-amber-700">
                        {updateStatus.asset_name}
                      </div>
                    )}

                    {updateStatus.release_notes && (
                      <div className="max-h-32 overflow-y-auto rounded border border-amber-200/80 bg-white/80 p-2 font-mono text-[11px] text-slate-700 whitespace-pre-wrap">
                        {updateStatus.release_notes}
                      </div>
                    )}
                  </div>
                )}
              </div>

              <label className="flex items-center justify-between rounded-lg border border-slate-200 px-3.5 py-2.5 text-xs font-medium text-slate-700">
                <span>{t("config.auto_start")}</span>
                <input
                  type="checkbox"
                  checked={config.auto_start}
                  onChange={(event) => updateConfig({ auto_start: event.target.checked })}
                  className="h-4 w-4 accent-slate-900"
                />
              </label>

              <div>
                <div className="mb-1.5 flex items-center gap-1.5 text-xs font-semibold text-slate-800">
                  <FolderSearch className="h-4 w-4 text-slate-500" />
                  <span>{t("config.extra_watch_dirs")}</span>
                </div>
                <p className="mb-2 text-[11px] text-slate-400">{t("config.extra_watch_desc")}</p>
                <div className="space-y-2">
                  {(config.extra_watch_dirs ?? []).length === 0 && (
                    <div className="rounded-lg border border-dashed border-slate-200 bg-slate-50/70 px-3 py-2 text-[11px] text-slate-400">
                      {t("config.no_paths")}
                    </div>
                  )}
                  {(config.extra_watch_dirs ?? []).map((dir, index) => (
                    <div
                      key={`${dir}-${index}`}
                      className="flex items-center justify-between gap-2 rounded-lg border border-slate-200 bg-slate-50/70 px-3 py-2"
                    >
                      <span className="font-mono-num min-w-0 truncate text-[11px] text-slate-600">
                        {dir}
                      </span>
                      <button
                        type="button"
                        onClick={() => handleRemovePath(index)}
                        className="shrink-0 text-[11px] font-medium text-rose-600 hover:text-rose-700"
                      >
                        {t("config.remove")}
                      </button>
                    </div>
                  ))}
                </div>
                <div className="mt-2 flex gap-2">
                  <input
                    type="text"
                    value={newPath}
                    onChange={(event) => setNewPath(event.target.value)}
                    onKeyDown={(event) => {
                      if (event.key === "Enter") handleAddPath();
                    }}
                    placeholder={t("config.add_path_placeholder")}
                    className="font-mono-num min-w-0 flex-1 rounded-lg border border-slate-200 px-2.5 py-1.5 text-xs text-slate-900 focus:border-slate-900 focus:outline-none"
                  />
                  <button
                    type="button"
                    onClick={handleAddPath}
                    className="inline-flex items-center gap-1 rounded-lg bg-slate-900 px-3 py-1.5 text-xs font-semibold text-white hover:bg-slate-800"
                  >
                    <Plus className="h-3.5 w-3.5" />
                    {t("config.add_path")}
                  </button>
                </div>
              </div>

              <label className="flex items-center justify-between rounded-lg border border-slate-200 px-3.5 py-2.5 text-xs font-medium text-slate-700">
                <span>{t("config.enable_lan")}</span>
                <input
                  type="checkbox"
                  checked={config.enable_lan !== false}
                  onChange={(event) => updateConfig({ enable_lan: event.target.checked })}
                  className="h-4 w-4 accent-slate-900"
                />
              </label>

              <label className="flex items-center justify-between rounded-lg border border-slate-200 px-3.5 py-2.5 text-xs font-medium text-slate-700">
                <span>{t("config.tailscale")}</span>
                <input
                  type="checkbox"
                  checked={config.tailscale_discovery !== false}
                  onChange={(event) => updateConfig({ tailscale_discovery: event.target.checked })}
                  className="h-4 w-4 accent-slate-900"
                />
              </label>

              <div>
                <div className="mb-1.5 text-xs font-semibold text-slate-800">
                  {t("config.extra_peer_hosts")}
                </div>
                <p className="mb-2 text-[11px] text-slate-400">{t("config.extra_peer_desc")}</p>
                <div className="space-y-2">
                  {(config.extra_peer_hosts ?? []).length === 0 && (
                    <div className="rounded-lg border border-dashed border-slate-200 bg-slate-50/70 px-3 py-2 text-[11px] text-slate-400">
                      {t("config.no_peers")}
                    </div>
                  )}
                  {(config.extra_peer_hosts ?? []).map((host, index) => (
                    <div
                      key={`${host}-${index}`}
                      className="flex items-center justify-between gap-2 rounded-lg border border-slate-200 bg-slate-50/70 px-3 py-2"
                    >
                      <span className="font-mono-num min-w-0 truncate text-[11px] text-slate-600">
                        {host}
                      </span>
                      <button
                        type="button"
                        onClick={() => handleRemovePeer(index)}
                        className="shrink-0 text-[11px] font-medium text-rose-600 hover:text-rose-700"
                      >
                        {t("config.remove")}
                      </button>
                    </div>
                  ))}
                </div>
                <div className="mt-2 flex gap-2">
                  <input
                    type="text"
                    value={newPeerHost}
                    onChange={(event) => setNewPeerHost(event.target.value)}
                    onKeyDown={(event) => {
                      if (event.key === "Enter") handleAddPeer();
                    }}
                    placeholder={t("config.add_peer_placeholder")}
                    className="font-mono-num min-w-0 flex-1 rounded-lg border border-slate-200 px-2.5 py-1.5 text-xs text-slate-900 focus:border-slate-900 focus:outline-none"
                  />
                  <button
                    type="button"
                    onClick={handleAddPeer}
                    className="inline-flex items-center gap-1 rounded-lg bg-slate-900 px-3 py-1.5 text-xs font-semibold text-white hover:bg-slate-800"
                  >
                    <Plus className="h-3.5 w-3.5" />
                    {t("config.add_peer")}
                  </button>
                </div>
              </div>

              <div className="rounded-lg border border-slate-200 bg-slate-50 p-3">
                <div className="flex items-center gap-1.5 text-[11px] font-semibold text-slate-500">
                  <FileText className="h-3.5 w-3.5" />
                  <span>{t("config.log_path")}</span>
                </div>
                <div className="font-mono-num mt-1 break-all text-xs text-slate-800">
                  {logsPath || "-"}
                </div>
                <button
                  type="button"
                  onClick={handleOpenLogs}
                  className="mt-2 rounded-md border border-slate-200 bg-white px-2.5 py-1 text-[11px] font-medium text-slate-700 hover:bg-slate-100"
                >
                  {t("config.open_logs")}
                </button>
                {logsNotice && (
                  <div role="status" className="mt-2 break-all text-[11px] text-slate-500">
                    {logsNotice}
                  </div>
                )}
              </div>

              <div className="flex items-center justify-between border-t border-slate-100 pt-4">
                {configSaved ? (
                  <span className="inline-flex items-center gap-1.5 text-xs font-medium text-emerald-600">
                    <CheckCircle2 className="h-4 w-4" />
                    {t("config.saved_toast")}
                  </span>
                ) : (
                  <span />
                )}
                <button
                  type="button"
                  disabled={configSaving}
                  onClick={handleSaveConfig}
                  className="inline-flex items-center gap-1.5 rounded-lg bg-slate-900 px-4 py-2 text-xs font-semibold text-white hover:bg-slate-800 disabled:opacity-50"
                >
                  <Check className="h-3.5 w-3.5" />
                  {t("config.save_btn")}
                </button>
              </div>
            </div>
          )}
        </div>
      </aside>
    </div>
  );
};
