import React, { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Check, CheckCircle2, FileText, FolderSearch, Plus, RefreshCw } from "lucide-react";
import { fetchConfig, fetchSystemLogsPath, openSystemLogs, saveConfig } from "../../api/client";
import { AppConfig } from "../../types/telemetry";
import {
  DRAWER_INPUT_CLASS,
  DRAWER_PANEL_CLASS,
  DRAWER_PRIMARY_BTN_CLASS,
  DRAWER_SECONDARY_BTN_CLASS,
} from "./drawerStyles";

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

interface ConfigDrawerProps {
  onDataMutated: () => void;
}

interface StringListSectionProps {
  title: string;
  description: string;
  emptyText: string;
  items: string[];
  inputValue: string;
  placeholder: string;
  addLabel: string;
  removeLabel: string;
  icon?: React.ReactNode;
  onInputChange: (value: string) => void;
  onAdd: () => void;
  onRemove: (index: number) => void;
}

const StringListSection: React.FC<StringListSectionProps> = ({
  title,
  description,
  emptyText,
  items,
  inputValue,
  placeholder,
  addLabel,
  removeLabel,
  icon,
  onInputChange,
  onAdd,
  onRemove,
}) => (
  <div>
    <div className="mb-1.5 flex items-center gap-1.5 text-xs font-semibold text-slate-800 dark:text-slate-200">
      {icon}
      <span>{title}</span>
    </div>
    <p className="mb-2 text-[11px] text-slate-400 dark:text-slate-500">{description}</p>
    <div className="space-y-2">
      {items.length === 0 && (
        <div className="rounded-lg border border-dashed border-slate-200 dark:border-slate-700 bg-slate-50/70 dark:bg-slate-800/30 px-3 py-2 text-[11px] text-slate-400 dark:text-slate-500">
          {emptyText}
        </div>
      )}
      {items.map((item, index) => (
        <div
          key={`${item}-${index}`}
          className="flex items-center justify-between gap-2 rounded-lg border border-slate-200 dark:border-slate-700 bg-slate-50/70 dark:bg-slate-800/50 px-3 py-2"
        >
          <span className="font-mono-num min-w-0 truncate text-[11px] text-slate-600 dark:text-slate-300">
            {item}
          </span>
          <button
            type="button"
            onClick={() => onRemove(index)}
            className="shrink-0 text-[11px] font-medium text-rose-600 dark:text-rose-400 hover:text-rose-700 dark:hover:text-rose-300"
          >
            {removeLabel}
          </button>
        </div>
      ))}
    </div>
    <div className="mt-2 flex gap-2">
      <input
        type="text"
        value={inputValue}
        onChange={(event) => onInputChange(event.target.value)}
        onKeyDown={(event) => {
          if (event.key === "Enter") onAdd();
        }}
        placeholder={placeholder}
        className={`font-mono-num flex-1 ${DRAWER_INPUT_CLASS}`}
      />
      <button type="button" onClick={onAdd} className={DRAWER_PRIMARY_BTN_CLASS}>
        <Plus className="h-3.5 w-3.5" />
        {addLabel}
      </button>
    </div>
  </div>
);

export const ConfigDrawer: React.FC<ConfigDrawerProps> = ({ onDataMutated }) => {
  const { t } = useTranslation();
  const [config, setConfig] = useState<AppConfig | null>(null);
  const [newPath, setNewPath] = useState("");
  const [newPeerHost, setNewPeerHost] = useState("");
  const [logsPath, setLogsPath] = useState("");
  const [logsNotice, setLogsNotice] = useState("");
  const [configSaving, setConfigSaving] = useState(false);
  const [configSaved, setConfigSaved] = useState(false);

  const [versionInfo, setVersionInfo] = useState<VersionInfo | null>(null);
  const [updateStatus, setUpdateStatus] = useState<UpdateStatus | null>(null);
  const [checkingUpdate, setCheckingUpdate] = useState(false);
  const [applyingUpdate, setApplyingUpdate] = useState(false);
  const [updateResult, setUpdateResult] = useState<string | null>(null);
  const [updateError, setUpdateError] = useState<string | null>(null);
  const savedTimerRef = useRef<number | null>(null);

  useEffect(() => {
    let cancelled = false;
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

    return () => {
      cancelled = true;
      if (savedTimerRef.current !== null) {
        window.clearTimeout(savedTimerRef.current);
      }
    };
  }, []);

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
      if (savedTimerRef.current !== null) {
        window.clearTimeout(savedTimerRef.current);
      }
      savedTimerRef.current = window.setTimeout(() => setConfigSaved(false), 2200);
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
      setUpdateError(err instanceof Error ? err.message : String(err));
    } finally {
      setCheckingUpdate(false);
    }
  };

  const handleApplyUpdate = async () => {
    setApplyingUpdate(true);
    setUpdateResult(null);
    setUpdateError(null);
    try {
      const res = await fetch("/api/system/update/apply", { method: "POST" });
      const data = (await res.json().catch(() => null)) as UpdateApplyResponse | null;
      if (!res.ok && !data) throw new Error(`HTTP ${res.status}`);
      if (data?.success) {
        setUpdateResult(data.message || t("updateSuccess"));
      } else {
        setUpdateError(data?.message || t("updateFailed"));
      }
    } catch (err: unknown) {
      setUpdateError(err instanceof Error ? err.message : String(err));
    } finally {
      setApplyingUpdate(false);
    }
  };

  if (!config) {
    return (
      <div className="space-y-4" aria-busy="true">
        <div className="h-24 animate-pulse rounded-xl bg-slate-100 dark:bg-slate-800" />
        <div className="h-12 animate-pulse rounded-xl bg-slate-100 dark:bg-slate-800" />
        <div className="h-28 animate-pulse rounded-xl bg-slate-100 dark:bg-slate-800" />
      </div>
    );
  }

  return (
    <div className="space-y-5">
      {/* Version & Auto-Update Card */}
      <div className={`${DRAWER_PANEL_CLASS} p-4`}>
        <div className="flex items-center justify-between gap-3">
          <div>
            <div className="text-xs font-semibold text-slate-800 dark:text-slate-200">
              {t("versionAndUpdates")}
            </div>
            <div className="mt-1 flex items-center gap-2">
              <span className="font-mono-num rounded bg-slate-900 dark:bg-slate-100 px-2 py-0.5 text-xs font-bold text-white dark:text-slate-950">
                {versionInfo?.version || t("unknown")}
              </span>
              <span className="font-mono-num text-[11px] text-slate-500 dark:text-slate-400">
                {versionInfo ? `${versionInfo.goos}/${versionInfo.goarch}` : t("unknown")}
              </span>
            </div>
          </div>

          <button
            type="button"
            onClick={handleCheckUpdate}
            disabled={checkingUpdate || applyingUpdate}
            className={`inline-flex items-center gap-1.5 ${DRAWER_SECONDARY_BTN_CLASS}`}
          >
            <RefreshCw className={`h-3.5 w-3.5 ${checkingUpdate ? "animate-spin" : ""}`} />
            <span>{checkingUpdate ? t("checkingUpdates") : t("checkForUpdates")}</span>
          </button>
        </div>

        {updateError && (
          <div
            role="alert"
            className="mt-3 rounded-lg border border-rose-200 dark:border-rose-800/60 bg-rose-50 dark:bg-rose-950/40 p-2.5 text-xs text-rose-700 dark:text-rose-300"
          >
            {updateError}
          </div>
        )}

        {updateResult && (
          <div
            role="status"
            className="mt-3 rounded-lg border border-emerald-200 dark:border-emerald-800/60 bg-emerald-50 dark:bg-emerald-950/40 p-2.5 text-xs text-emerald-700 dark:text-emerald-300"
          >
            {updateResult}
          </div>
        )}

        {updateStatus && !updateStatus.has_update && !updateError && (
          <div className="mt-3 rounded-lg border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-900 p-2.5 text-xs text-slate-600 dark:text-slate-300">
            {t("alreadyLatest")}
          </div>
        )}

        {updateStatus?.has_update && (
          <div className="mt-3 space-y-2 rounded-lg border border-amber-200 dark:border-amber-800/60 bg-amber-50/80 dark:bg-amber-950/40 p-3 text-xs">
            <div className="flex items-center justify-between gap-2">
              <div className="font-semibold text-amber-900 dark:text-amber-200">
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
              <div className="font-mono-num text-[11px] text-amber-700 dark:text-amber-300">
                {updateStatus.asset_name}
              </div>
            )}

            {updateStatus.release_notes && (
              <div className="max-h-32 overflow-y-auto rounded border border-amber-200/80 dark:border-amber-800/60 bg-white/80 dark:bg-slate-950/60 p-2 font-mono text-[11px] whitespace-pre-wrap text-slate-700 dark:text-slate-200">
                {updateStatus.release_notes}
              </div>
            )}
          </div>
        )}
      </div>

      <label className="flex items-center justify-between rounded-lg border border-slate-200 dark:border-slate-800 px-3.5 py-2.5 text-xs font-medium text-slate-700 dark:text-slate-200">
        <span>{t("config.auto_start")}</span>
        <input
          type="checkbox"
          checked={config.auto_start}
          onChange={(event) => updateConfig({ auto_start: event.target.checked })}
          className="h-4 w-4 accent-slate-900 dark:accent-slate-200"
        />
      </label>

      <StringListSection
        title={t("config.extra_watch_dirs")}
        description={t("config.extra_watch_desc")}
        emptyText={t("config.no_paths")}
        items={config.extra_watch_dirs ?? []}
        inputValue={newPath}
        placeholder={t("config.add_path_placeholder")}
        addLabel={t("config.add_path")}
        removeLabel={t("config.remove")}
        icon={<FolderSearch className="h-4 w-4 text-slate-500 dark:text-slate-400" />}
        onInputChange={setNewPath}
        onAdd={handleAddPath}
        onRemove={handleRemovePath}
      />

      <label className="flex items-center justify-between rounded-lg border border-slate-200 dark:border-slate-800 px-3.5 py-2.5 text-xs font-medium text-slate-700 dark:text-slate-200">
        <span>{t("config.enable_lan")}</span>
        <input
          type="checkbox"
          checked={config.enable_lan !== false}
          onChange={(event) => updateConfig({ enable_lan: event.target.checked })}
          className="h-4 w-4 accent-slate-900 dark:accent-slate-200"
        />
      </label>

      <label className="flex items-center justify-between rounded-lg border border-slate-200 dark:border-slate-800 px-3.5 py-2.5 text-xs font-medium text-slate-700 dark:text-slate-200">
        <span>{t("config.tailscale")}</span>
        <input
          type="checkbox"
          checked={config.tailscale_discovery !== false}
          onChange={(event) => updateConfig({ tailscale_discovery: event.target.checked })}
          className="h-4 w-4 accent-slate-900 dark:accent-slate-200"
        />
      </label>

      <StringListSection
        title={t("config.extra_peer_hosts")}
        description={t("config.extra_peer_desc")}
        emptyText={t("config.no_peers")}
        items={config.extra_peer_hosts ?? []}
        inputValue={newPeerHost}
        placeholder={t("config.add_peer_placeholder")}
        addLabel={t("config.add_peer")}
        removeLabel={t("config.remove")}
        onInputChange={setNewPeerHost}
        onAdd={handleAddPeer}
        onRemove={handleRemovePeer}
      />

      <div className="rounded-lg border border-slate-200 dark:border-slate-800 bg-slate-50 dark:bg-slate-800/45 p-3">
        <div className="flex items-center gap-1.5 text-[11px] font-semibold text-slate-500 dark:text-slate-400">
          <FileText className="h-3.5 w-3.5" />
          <span>{t("config.log_path")}</span>
        </div>
        <div className="font-mono-num mt-1 break-all text-xs text-slate-800 dark:text-slate-200">
          {logsPath || "-"}
        </div>
        <button
          type="button"
          onClick={handleOpenLogs}
          className="mt-2 rounded-md border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-800 px-2.5 py-1 text-[11px] font-medium text-slate-700 dark:text-slate-200 hover:bg-slate-100 dark:hover:bg-slate-700"
        >
          {t("config.open_logs")}
        </button>
        {logsNotice && (
          <div role="status" className="mt-2 break-all text-[11px] text-slate-500 dark:text-slate-400">
            {logsNotice}
          </div>
        )}
      </div>

      <div className="flex items-center justify-between border-t border-slate-100 dark:border-slate-800 pt-4">
        {configSaved ? (
          <span className="inline-flex items-center gap-1.5 text-xs font-medium text-emerald-600 dark:text-emerald-400">
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
          className={`${DRAWER_PRIMARY_BTN_CLASS} gap-1.5 px-4 py-2`}
        >
          <Check className="h-3.5 w-3.5" />
          {t("config.save_btn")}
        </button>
      </div>
    </div>
  );
};
