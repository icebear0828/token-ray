import React, { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { AlertCircle, RefreshCw, Server, Wifi, WifiOff } from "lucide-react";
import { fetchLanScan, fetchLanStatus, joinLan, leaveLan } from "../../api/client";
import { CurrencyCode, LanPeer } from "../../types/telemetry";
import { formatCompactNumber, formatCost } from "../../utils/formatters";
import { SourceBadge } from "../SourceBadge";

interface LanDrawerProps {
  currency: CurrencyCode;
  onDataMutated: () => void;
}

export const LanDrawer: React.FC<LanDrawerProps> = ({ currency, onDataMutated }) => {
  const { t } = useTranslation();
  const [lanEnabled, setLanEnabled] = useState(true);
  const [lanPeers, setLanPeers] = useState<LanPeer[]>([]);
  const [lanBusy, setLanBusy] = useState(false);

  useEffect(() => {
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
  }, []);

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
  );
};
