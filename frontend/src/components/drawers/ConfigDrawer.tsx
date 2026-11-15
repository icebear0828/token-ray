import React, { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { Check, CheckCircle2, FileText, FolderSearch, Plus, RefreshCw } from "lucide-react";
import {
  fetchConfig,
  fetchSystemLogsPath,
  openSystemLogs,
  saveConfig,
} from "../../api/client";
import { AppConfig } from "../../types/telemetry";

interface ConfigDrawerProps {
  onDataMutated: () => void;
}

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

export const ConfigDrawer: React.FC<ConfigDrawerProps> = ({ onDataMutated }) => {
  const { t } = useTranslation();
  const [config, setConfig] = useState<AppConfig | null>(null);
  const [newPath, setNewPath] = useState("");
  const [newPeerHost, setNewPeerHost] = useState("");
  const [logsPath, setLogsPath] = useState<string>("");
  const [logsNotice, setLogsNotice] = useState<string>("");
  const [configSaving, setConfigSaving] = useState(false);
  const [configSaved, setConfigSaved] = useState(false);
  const [versionInfo, setVersionInfo] = useState<VersionInfo | null>(null);
  const [updateStatus, setUpdateStatus] = useState<UpdateStatus | null>(null);
  const [checkingUpdate, setCheckingUpdate] = useState<boolean>(false);
  const [applyingUpdate, setApplyingUpdate] = useState<boolean>(false);
  const [updateResult, setUpdateResult] = useState<string | null>(null);
  const [updateError, setUpdateError] = useState<string | null>(null);

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

  if (!config) return null;

  return (
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
  );
};
