import React, { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Package, Trash2 } from "lucide-react";
import {
  fetchConfig,
  fetchPluginIndex,
  fetchPlugins,
  installPluginURL,
  installPluginZip,
  removePlugin,
  saveConfig,
  setPluginEnabled,
} from "../../api/client";
import { AppConfig, InstalledPlugin, PluginIndexEntry } from "../../types/telemetry";
import {
  DRAWER_CARD_CLASS,
  DRAWER_DANGER_BTN_CLASS,
  DRAWER_EMPTY_CLASS,
  DRAWER_INPUT_CLASS,
  DRAWER_PANEL_CLASS,
  DRAWER_PRIMARY_BTN_CLASS,
  DRAWER_SECONDARY_BTN_CLASS,
} from "./drawerStyles";

interface PluginsDrawerProps {
  onDataMutated: () => void;
}

export const PluginsDrawer: React.FC<PluginsDrawerProps> = ({ onDataMutated }) => {
  const { t } = useTranslation();
  const [pluginList, setPluginList] = useState<InstalledPlugin[]>([]);
  const [config, setConfig] = useState<AppConfig | null>(null);
  const [pluginIndex, setPluginIndex] = useState<PluginIndexEntry[] | null>(null);
  const [pluginBusy, setPluginBusy] = useState<string | null>(null);
  const [pluginMessage, setPluginMessage] = useState<string | null>(null);
  const [pluginError, setPluginError] = useState<string | null>(null);
  const pluginFileRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    let cancelled = false;
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

    return () => {
      cancelled = true;
    };
  }, []);

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

  if (!config) {
    return (
      <div className="space-y-4">
        {pluginError ? (
          <div
            role="alert"
            className="rounded-lg border border-rose-200 dark:border-rose-800/60 bg-rose-50 dark:bg-rose-950/40 p-2.5 text-xs text-rose-700 dark:text-rose-300"
          >
            {pluginError}
          </div>
        ) : (
          <div className="space-y-3" aria-busy="true">
            <div className="h-24 animate-pulse rounded-xl bg-slate-100 dark:bg-slate-800" />
            <div className="h-28 animate-pulse rounded-xl bg-slate-100 dark:bg-slate-800" />
          </div>
        )}
      </div>
    );
  }

  return (
    <div className="space-y-5">
      {pluginMessage && (
        <div
          role="status"
          className="rounded-lg border border-emerald-200 dark:border-emerald-800/60 bg-emerald-50 dark:bg-emerald-950/40 p-2.5 text-xs text-emerald-700 dark:text-emerald-300"
        >
          {pluginMessage}
        </div>
      )}
      {pluginError && (
        <div
          role="alert"
          className="rounded-lg border border-rose-200 dark:border-rose-800/60 bg-rose-50 dark:bg-rose-950/40 p-2.5 text-xs text-rose-700 dark:text-rose-300"
        >
          {pluginError}
        </div>
      )}

      <div>
        <div className="mb-2 text-xs font-semibold text-slate-800 dark:text-slate-200">
          {t("plugins.installed")}
        </div>
        <div className="space-y-3">
          {pluginList.length === 0 && (
            <div className={`${DRAWER_EMPTY_CLASS} p-6`}>{t("plugins.empty")}</div>
          )}
          {pluginList.map((plugin) => (
            <div key={plugin.manifest.id} className={`${DRAWER_CARD_CLASS} p-4`}>
              <div className="flex flex-wrap items-start justify-between gap-3">
                <div className="min-w-0">
                  <div className="flex flex-wrap items-center gap-2">
                    <span className="text-sm font-semibold text-slate-900 dark:text-slate-100">
                      {plugin.manifest.name}
                    </span>
                    <span className="font-mono-num rounded bg-slate-900 dark:bg-slate-100 px-1.5 py-0.5 text-[10px] font-bold text-white dark:text-slate-950">
                      v{plugin.manifest.version}
                    </span>
                    <span
                      className={`rounded px-1.5 py-0.5 text-[10px] font-semibold uppercase ${
                        plugin.enabled
                          ? "bg-emerald-50 dark:bg-emerald-950/60 text-emerald-700 dark:text-emerald-300"
                          : "bg-slate-100 dark:bg-slate-800 text-slate-500 dark:text-slate-400"
                      }`}
                    >
                      {plugin.enabled ? t("plugins.enabled") : t("plugins.disabled")}
                    </span>
                  </div>
                  <p className="mt-1 text-[11px] text-slate-500 dark:text-slate-400">
                    {plugin.manifest.description || plugin.manifest.source}
                  </p>
                  <div className="font-mono-num mt-1 break-all text-[10px] text-slate-400 dark:text-slate-500">
                    {plugin.path}
                  </div>
                </div>
                <div className="flex shrink-0 gap-2">
                  <button
                    type="button"
                    disabled={pluginBusy === `plugin:${plugin.manifest.id}`}
                    onClick={() => handleTogglePlugin(plugin)}
                    className={DRAWER_SECONDARY_BTN_CLASS}
                  >
                    {plugin.enabled ? t("plugins.disable") : t("plugins.enable")}
                  </button>
                  <button
                    type="button"
                    disabled={pluginBusy === `plugin:${plugin.manifest.id}`}
                    onClick={() => handleRemovePlugin(plugin)}
                    className={`${DRAWER_DANGER_BTN_CLASS} px-3 font-semibold`}
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

      <div className={`${DRAWER_PANEL_CLASS} p-4`}>
        <div className="flex items-center gap-1.5 text-xs font-semibold text-slate-800 dark:text-slate-200">
          <Package className="h-4 w-4 text-slate-500 dark:text-slate-400" />
          <span>{t("plugins.install_file")}</span>
        </div>
        <p className="mt-1 text-[11px] text-slate-400 dark:text-slate-500">
          {t("plugins.install_file_desc")}
        </p>
        <div className="mt-3 flex flex-col gap-2 sm:flex-row">
          <input
            ref={pluginFileRef}
            type="file"
            accept=".zip,application/zip"
            className="min-w-0 flex-1 rounded-lg border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-950/70 px-2.5 py-1.5 text-xs text-slate-700 dark:text-slate-200 file:mr-2 file:rounded file:border-0 file:bg-slate-100 dark:file:bg-slate-800 file:px-2 file:py-1 file:text-[11px] file:font-semibold file:text-slate-700 dark:file:text-slate-200"
          />
          <button
            type="button"
            disabled={pluginBusy === "install-file"}
            onClick={handleInstallPluginFile}
            className={DRAWER_PRIMARY_BTN_CLASS}
          >
            {pluginBusy === "install-file" ? t("plugins.installing") : t("plugins.install")}
          </button>
        </div>
      </div>

      <div className={`${DRAWER_PANEL_CLASS} p-4`}>
        <div className="text-xs font-semibold text-slate-800 dark:text-slate-200">
          {t("plugins.index")}
        </div>
        <p className="mt-1 text-[11px] text-slate-400 dark:text-slate-500">
          {t("plugins.index_desc")}
        </p>
        <div className="mt-3 flex flex-col gap-2">
          <input
            type="text"
            value={config.plugin_index_url ?? ""}
            onChange={(event) =>
              setConfig((prev) => (prev ? { ...prev, plugin_index_url: event.target.value } : prev))
            }
            placeholder="https://example.com/plugins/index.json"
            className={`font-mono-num ${DRAWER_INPUT_CLASS}`}
          />
          <button
            type="button"
            disabled={pluginBusy === "load-index"}
            onClick={handleLoadPluginIndex}
            className={`self-start ${DRAWER_SECONDARY_BTN_CLASS}`}
          >
            {pluginBusy === "load-index" ? t("app.loading") : t("plugins.load_index")}
          </button>
        </div>

        {pluginIndex && (
          <div className="mt-3 space-y-2">
            {pluginIndex.map((entry) => (
              <div
                key={entry.id}
                className="flex flex-wrap items-center justify-between gap-2 rounded-lg border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-900 p-3"
              >
                <div className="min-w-0">
                  <div className="flex items-center gap-2">
                    <span className="text-xs font-semibold text-slate-800 dark:text-slate-100">
                      {entry.name}
                    </span>
                    <span className="font-mono-num rounded bg-slate-900 dark:bg-slate-100 px-1.5 py-0.5 text-[10px] font-bold text-white dark:text-slate-950">
                      v{entry.version}
                    </span>
                  </div>
                  <p className="mt-0.5 text-[11px] text-slate-500 dark:text-slate-400">
                    {entry.description || entry.id}
                  </p>
                </div>
                <button
                  type="button"
                  disabled={pluginBusy === `remote:${entry.id}`}
                  onClick={() => handleInstallPluginRemote(entry)}
                  className={DRAWER_PRIMARY_BTN_CLASS}
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
  );
};
