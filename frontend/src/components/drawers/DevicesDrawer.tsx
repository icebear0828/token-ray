import React, { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { Cpu, Plus, Trash2 } from "lucide-react";
import {
  deleteDeviceAlias,
  fetchDevices,
  fetchLANSelf,
  setDeviceAlias,
  softDeleteDevice,
} from "../../api/client";
import { CurrencyCode, DeviceSummary } from "../../types/telemetry";
import { formatCompactNumber, formatCost } from "../../utils/formatters";
import {
  DRAWER_CARD_CLASS,
  DRAWER_DANGER_BTN_CLASS,
  DRAWER_EMPTY_CLASS,
  DRAWER_INPUT_CLASS,
  DRAWER_PANEL_CLASS,
  DRAWER_PRIMARY_BTN_CLASS,
  DRAWER_SECONDARY_BTN_CLASS,
} from "./drawerStyles";

interface DevicesDrawerProps {
  currency: CurrencyCode;
  onDataMutated: () => void;
}

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

export const DevicesDrawer: React.FC<DevicesDrawerProps> = ({ currency, onDataMutated }) => {
  const { t } = useTranslation();
  const [devices, setDevices] = useState<DeviceSummary[]>([]);
  const [localDeviceId, setLocalDeviceId] = useState("");
  const [aliasDrafts, setAliasDrafts] = useState<Record<string, string>>({});
  const [newDeviceId, setNewDeviceId] = useState("");
  const [newDeviceName, setNewDeviceName] = useState("");
  const [deviceBusyId, setDeviceBusyId] = useState<string | null>(null);
  const [confirmingDeleteId, setConfirmingDeleteId] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
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

    return () => {
      cancelled = true;
    };
  }, []);

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

  return (
    <div className="space-y-4 overflow-x-hidden" data-testid="device-management-list">
      <div className="rounded-lg border border-amber-200/80 dark:border-amber-800/60 bg-amber-50/70 dark:bg-amber-950/40 px-3.5 py-2.5 text-xs text-amber-800 dark:text-amber-300">
        {t("devices.soft_delete_hint")}
      </div>

      <div className={`${DRAWER_PANEL_CLASS} p-3.5`}>
        <div className="text-[11px] font-semibold text-slate-500 dark:text-slate-400">
          {t("devices.add_alias")}
        </div>
        <div className="mt-1.5 grid grid-cols-1 gap-2 sm:grid-cols-[1fr_1fr_auto]">
          <input
            type="text"
            value={newDeviceId}
            onChange={(event) => setNewDeviceId(event.target.value)}
            placeholder={t("devices.device_id_placeholder")}
            className={`font-mono-num ${DRAWER_INPUT_CLASS}`}
          />
          <input
            type="text"
            value={newDeviceName}
            onChange={(event) => setNewDeviceName(event.target.value)}
            placeholder={t("devices.add_alias_name_placeholder")}
            className={DRAWER_INPUT_CLASS}
          />
          <button
            type="button"
            disabled={!newDeviceId.trim() || !newDeviceName.trim() || Boolean(deviceBusyId)}
            onClick={handleAddAlias}
            className={DRAWER_PRIMARY_BTN_CLASS}
          >
            <Plus className="h-3.5 w-3.5" />
            {t("devices.add_alias_btn")}
          </button>
        </div>
      </div>

      {devices.length === 0 && (
        <div className={DRAWER_EMPTY_CLASS}>{t("devices.no_devices")}</div>
      )}

      {devices.map((device) => {
        const isLocal = device.id === localDeviceId;
        const isConfirming = confirmingDeleteId === device.id;
        return (
          <div key={device.id} data-testid="device-row" className={`${DRAWER_CARD_CLASS} p-4`}>
            <div className="flex flex-wrap items-center justify-between gap-2">
              <div className="flex min-w-0 flex-wrap items-center gap-2">
                <Cpu className="h-4 w-4 shrink-0 text-slate-500 dark:text-slate-400" />
                <span className="font-mono-num break-all text-xs font-semibold text-slate-800 dark:text-slate-100">
                  {device.display_name || device.id}
                </span>
                {isLocal && (
                  <span className="rounded bg-slate-900 dark:bg-slate-100 px-1.5 py-0.5 text-[9px] font-bold tracking-wider text-white dark:text-slate-950 uppercase">
                    {t("devices.local_badge")}
                  </span>
                )}
              </div>

              <div className="font-mono-num text-xs font-semibold text-slate-700 dark:text-slate-200">
                {formatCompactNumber(deviceTotalTokens(device))} tok ·{" "}
                {formatCost(device.total_cost, currency)}
              </div>
            </div>

            <div className="mt-1 flex flex-wrap gap-x-3 gap-y-1 text-[11px] text-slate-400 dark:text-slate-500">
              <span className="font-mono-num break-all">{device.id}</span>
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
                className={`min-w-[140px] flex-1 ${DRAWER_INPUT_CLASS}`}
              />
              <button
                type="button"
                disabled={deviceBusyId === device.id}
                onClick={() => handleSaveAlias(device.id)}
                className={DRAWER_PRIMARY_BTN_CLASS}
              >
                {t("devices.save_alias")}
              </button>
              {device.display_name && device.display_name !== device.id && (
                <button
                  type="button"
                  disabled={deviceBusyId === device.id}
                  onClick={() => handleClearAlias(device.id)}
                  className={DRAWER_SECONDARY_BTN_CLASS}
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
                  className={`ml-auto ${DRAWER_DANGER_BTN_CLASS}`}
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
                    className={DRAWER_SECONDARY_BTN_CLASS}
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
  );
};
