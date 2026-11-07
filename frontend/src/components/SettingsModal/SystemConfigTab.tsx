import React, { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { AppConfig } from '../../SettingsModal';

interface SystemConfigTabProps {
  config: AppConfig;
  newPath: string;
  setNewPath: (path: string) => void;
  handleAddPath: () => void;
  handleRemovePath: (index: number) => void;
  handleToggleLAN: () => void;
  newPeerHost: string;
  setNewPeerHost: (host: string) => void;
  handleAddPeer: () => void;
  handleRemovePeer: (index: number) => void;
  handleToggleTailscale: () => void;
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
}

interface UpdateApplyResponse {
  success: boolean;
  message: string;
  version?: string;
}

export default function SystemConfigTab({
  config,
  newPath,
  setNewPath,
  handleAddPath,
  handleRemovePath,
  handleToggleLAN,
  newPeerHost,
  setNewPeerHost,
  handleAddPeer,
  handleRemovePeer,
  handleToggleTailscale,
}: SystemConfigTabProps) {
  const { t } = useTranslation();
  const tailscaleOn = config.tailscale_discovery !== false;

  const [versionInfo, setVersionInfo] = useState<VersionInfo | null>(null);
  const [updateStatus, setUpdateStatus] = useState<UpdateStatus | null>(null);
  const [checkingUpdate, setCheckingUpdate] = useState<boolean>(false);
  const [applyingUpdate, setApplyingUpdate] = useState<boolean>(false);
  const [updateResult, setUpdateResult] = useState<string | null>(null);
  const [updateError, setUpdateError] = useState<string | null>(null);

  useEffect(() => {
    fetch('/api/system/version')
      .then((res) => (res.ok ? res.json() : null))
      .then((data: VersionInfo | null) => {
        if (data) setVersionInfo(data);
      })
      .catch(() => {
        // ignore fallback
      });
  }, []);

  const handleCheckUpdate = async () => {
    setCheckingUpdate(true);
    setUpdateResult(null);
    setUpdateError(null);
    try {
      const res = await fetch('/api/system/update/check');
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      const data = (await res.json()) as UpdateStatus;
      setUpdateStatus(data);
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
      const res = await fetch('/api/system/update/apply', {
        method: 'POST',
      });
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      const data = (await res.json()) as UpdateApplyResponse;
      if (data.success) {
        setUpdateResult(data.message || t('updateSuccess'));
      } else {
        setUpdateError(data.message || t('updateFailed'));
      }
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : String(err);
      setUpdateError(msg);
    } finally {
      setApplyingUpdate(false);
    }
  };

  return (
    <div className="flex flex-col gap-8 mb-8">
      {/* Version & Auto-Update Section */}
      <section>
        <h3 className="font-display text-xl sm:text-2xl uppercase border-b-[3px] border-[#000000] pb-2 mb-4">
          {t('versionAndUpdates')}
        </h3>
        <div className="border-[3px] border-[#000000] p-4 bg-[#F9F9F9] flex flex-col gap-4">
          <div className="flex flex-col sm:flex-row sm:justify-between sm:items-center gap-2">
            <div>
              <div className="font-bold uppercase text-base">
                {t('currentVersion')}:{' '}
                <span className="font-mono bg-[#000000] text-[#FFFFFF] px-2 py-0.5">
                  {versionInfo?.version || 'v1.8.1'}
                </span>
              </div>
              <div className="text-sm text-[#666666] font-mono mt-1">
                {t('platform')}: {versionInfo ? `${versionInfo.goos}/${versionInfo.goarch}` : 'unknown'}
              </div>
            </div>
            <button
              onClick={handleCheckUpdate}
              disabled={checkingUpdate || applyingUpdate}
              className="border-[3px] border-[#000000] bg-[#FFFFFF] px-4 py-2 font-bold uppercase hover:bg-[#000000] hover:text-[#FFFFFF] transition-none disabled:opacity-50 text-sm whitespace-nowrap self-start sm:self-auto"
            >
              {checkingUpdate ? t('checkingUpdates') : t('checkForUpdates')}
            </button>
          </div>

          {updateError && (
            <div className="border-[2px] border-[#FF0000] bg-[#FFEEEE] text-[#FF0000] p-3 text-sm font-mono">
              ⚠️ {t('updateFailed')}: {updateError}
            </div>
          )}

          {updateResult && (
            <div className="border-[2px] border-[#008000] bg-[#EEFFEE] text-[#008000] p-3 text-sm font-mono font-bold">
              ✓ {updateResult}
            </div>
          )}

          {updateStatus && !updateStatus.has_update && !updateResult && (
            <div className="text-sm text-[#008000] font-mono font-bold flex items-center gap-2">
              <span>✓</span> {t('alreadyLatest')} ({updateStatus.latest_version})
            </div>
          )}

          {updateStatus && updateStatus.has_update && (
            <div className="border-[3px] border-[#000000] bg-[#FFFFEE] p-4 flex flex-col gap-3">
              <div className="flex flex-col sm:flex-row justify-between sm:items-center gap-2">
                <div>
                  <div className="font-bold uppercase text-[#000000] flex items-center gap-2">
                    <span className="text-lg">🚀</span> {t('newVersionAvailable')}:{' '}
                    <span className="font-mono bg-[#008000] text-[#FFFFFF] px-2 py-0.5">
                      {updateStatus.latest_version}
                    </span>
                  </div>
                  {updateStatus.asset_name && (
                    <div className="text-xs text-[#666666] font-mono mt-1">
                      Asset: {updateStatus.asset_name}
                    </div>
                  )}
                </div>
                <button
                  onClick={handleApplyUpdate}
                  disabled={applyingUpdate}
                  className="border-[3px] border-[#000000] bg-[#008000] text-[#FFFFFF] px-6 py-2 font-bold uppercase hover:opacity-90 transition-none disabled:opacity-50 text-sm whitespace-nowrap"
                >
                  {applyingUpdate ? t('updating') : t('applyUpdate')}
                </button>
              </div>

              {updateStatus.release_notes && (
                <div className="border-[2px] border-[#CCCCCC] bg-[#FFFFFF] p-3 text-xs font-mono max-h-40 overflow-y-auto whitespace-pre-wrap">
                  <div className="font-bold uppercase text-[#666666] mb-1">{t('releaseNotes')}:</div>
                  {updateStatus.release_notes}
                </div>
              )}
            </div>
          )}
        </div>
      </section>

      {/* Network & Radar Section */}
      <section>
        <h3 className="font-display text-xl sm:text-2xl uppercase border-b-[3px] border-[#000000] pb-2 mb-4">
          {t('networkSettings', 'Network Settings')}
        </h3>
        <div className="flex flex-col gap-3">
          <div className="flex justify-between items-center border-[3px] border-[#000000] p-4 bg-[#F9F9F9]">
            <div>
              <div className="font-bold uppercase mb-1">
                {t('enableLanDiscovery', 'Enable LAN Discovery')}
              </div>
              <div className="text-sm text-[#666666]">
                {t('enableLanDesc', 'Broadcast and receive real-time token usage across local network devices. Requires restart.')}
              </div>
            </div>
            <button
              onClick={handleToggleLAN}
              className={`font-bold px-4 py-2 border-[3px] border-[#000000] transition-none w-24 text-center ${
                config.enable_lan !== false ? 'bg-[#008000] text-[#FFFFFF]' : 'bg-[#FF0000] text-[#FFFFFF]'
              }`}
            >
              {config.enable_lan !== false ? t('on', 'ON') : t('off', 'OFF')}
            </button>
          </div>

          <div className="flex justify-between items-center border-[3px] border-[#000000] p-4 bg-[#F9F9F9]">
            <div>
              <div className="font-bold uppercase mb-1">
                {t('tailscaleDiscovery', 'Tailscale Auto-Discovery')}
              </div>
              <div className="text-sm text-[#666666]">
                {t('tailscaleDiscoveryDesc', 'When the tailscale CLI is installed, automatically probe online Tailscale peers. Multicast/broadcast does not work over Tailscale.')}
              </div>
            </div>
            <button
              onClick={handleToggleTailscale}
              className={`font-bold px-4 py-2 border-[3px] border-[#000000] transition-none w-24 text-center ${
                tailscaleOn ? 'bg-[#008000] text-[#FFFFFF]' : 'bg-[#FF0000] text-[#FFFFFF]'
              }`}
            >
              {tailscaleOn ? t('on', 'ON') : t('off', 'OFF')}
            </button>
          </div>
        </div>
      </section>

      {/* Extra Peer Hosts Section */}
      <section>
        <h3 className="font-display text-xl sm:text-2xl uppercase border-b-[3px] border-[#000000] pb-2 mb-4">
          {t('extraPeerHosts', 'Extra Peer Hosts')}
        </h3>
        <p className="mb-4 text-[#666666]">
          {t('extraPeerHostsDesc', 'Add hostnames or IPs that should be probed in addition to the local subnet. Useful for Tailscale, VPN, or cross-subnet peers.')}
        </p>

        <ul className="mb-4 flex flex-col gap-2">
          {(config.extra_peer_hosts || []).map((host, idx) => (
            <li
              key={idx}
              className="flex flex-col sm:flex-row sm:justify-between items-start sm:items-center gap-2 sm:gap-4 border-[3px] border-[#000000] bg-[#F9F9F9] p-3"
            >
              <span className="font-mono break-all sm:truncate w-full">{host}</span>
              <button
                onClick={() => handleRemovePeer(idx)}
                className="bg-[#FF0000] text-[#FFFFFF] font-bold px-3 py-1 border-[2px] border-[#FF0000] hover:opacity-80 shrink-0"
              >
                {t('remove')}
              </button>
            </li>
          ))}
          {(config.extra_peer_hosts || []).length === 0 && (
            <li className="italic text-[#666666] border-[3px] border-dashed border-[#CCCCCC] p-3 text-center">
              {t('noExtraPeerHosts', 'No extra peer hosts configured.')}
            </li>
          )}
        </ul>

        <div className="flex flex-col sm:flex-row gap-2">
          <input
            type="text"
            value={newPeerHost}
            onChange={(e) => setNewPeerHost(e.target.value)}
            placeholder={t('addPeerHostPlaceholder', 'IP or hostname e.g. 100.64.0.5 or mac.tailnet.ts.net')}
            className="flex-1 border-[3px] border-[#000000] px-3 py-2 outline-none focus:bg-[#000000] focus:text-[#FFFFFF] transition-none w-full"
          />
          <button
            onClick={handleAddPeer}
            className="border-[3px] border-[#000000] px-6 py-2 font-bold uppercase hover:bg-[#000000] hover:text-[#FFFFFF] transition-none whitespace-nowrap w-full sm:w-auto"
          >
            {t('addPeerHost', '+ ADD HOST')}
          </button>
        </div>
      </section>

      {/* Log Directories Section */}
      <section>
        <h3 className="font-display text-xl sm:text-2xl uppercase border-b-[3px] border-[#000000] pb-2 mb-4">
          {t('extraWatchDirectories')}
        </h3>
        <p className="mb-4 text-[#666666]">{t('extraWatchDesc')}</p>

        <ul className="mb-4 flex flex-col gap-2">
          {(config.extra_watch_dirs || []).map((dir, idx) => (
            <li
              key={idx}
              className="flex flex-col sm:flex-row sm:justify-between items-start sm:items-center gap-2 sm:gap-4 border-[3px] border-[#000000] bg-[#F9F9F9] p-3"
            >
              <span className="font-mono break-all sm:truncate w-full">{dir}</span>
              <button
                onClick={() => handleRemovePath(idx)}
                className="bg-[#FF0000] text-[#FFFFFF] font-bold px-3 py-1 border-[2px] border-[#FF0000] hover:opacity-80 shrink-0"
              >
                {t('remove')}
              </button>
            </li>
          ))}
          {(config.extra_watch_dirs || []).length === 0 && (
            <li className="italic text-[#666666] border-[3px] border-dashed border-[#CCCCCC] p-3 text-center">
              {t('noExtraDirectories')}
            </li>
          )}
        </ul>

        <div className="flex flex-col sm:flex-row gap-2">
          <input
            type="text"
            value={newPath}
            onChange={(e) => setNewPath(e.target.value)}
            placeholder={t('addPathPlaceholder')}
            className="flex-1 border-[3px] border-[#000000] px-3 py-2 outline-none focus:bg-[#000000] focus:text-[#FFFFFF] transition-none w-full"
          />
          <button
            onClick={handleAddPath}
            className="border-[3px] border-[#000000] px-6 py-2 font-bold uppercase hover:bg-[#000000] hover:text-[#FFFFFF] transition-none whitespace-nowrap w-full sm:w-auto"
          >
            {t('addPath')}
          </button>
        </div>
      </section>
    </div>
  );
}
