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

interface PluginsDrawerProps {
  onDataMutated: () => void;
}

export const PluginsDrawer: React.FC<PluginsDrawerProps> = ({ onDataMutated }) => {
  const { t } = useTranslation();
  const [config, setConfig] = useState<AppConfig | null>(null);
  const [pluginList, setPluginList] = useState<InstalledPlugin[]>([]);
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

  const updateConfig = (patch: Partial<AppConfig>) => {
    setConfig((prev) => (prev ? { ...prev, ...patch } : prev));
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

  if (!config) return null;

  return (
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
  );
};
