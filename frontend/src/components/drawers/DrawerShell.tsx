import React from "react";
import { useTranslation } from "react-i18next";
import { DollarSign, Laptop, Package, Radio, Sliders, X } from "lucide-react";

export type DrawerKind = "lan" | "devices" | "pricing" | "plugins" | "config";

interface DrawerShellProps {
  drawer: DrawerKind;
  isClosing: boolean;
  onClose: () => void;
  children: React.ReactNode;
}

const drawerIcons: Record<DrawerKind, React.ReactNode> = {
  lan: <Radio className="h-4 w-4" />,
  devices: <Laptop className="h-4 w-4" />,
  pricing: <DollarSign className="h-4 w-4" />,
  plugins: <Package className="h-4 w-4" />,
  config: <Sliders className="h-4 w-4" />,
};

export const DrawerShell: React.FC<DrawerShellProps> = ({
  drawer,
  isClosing,
  onClose,
  children,
}) => {
  const { t } = useTranslation();

  return (
    <div
      className={`fixed inset-0 z-50 flex justify-end bg-slate-900/25 backdrop-blur-[2px] transition-opacity duration-200 ${
        isClosing ? "opacity-0" : "animate-[overlay-in_200ms_ease-out] opacity-100"
      }`}
    >
      <div className="flex-1" onClick={onClose} />

      <aside
        className={`flex h-full w-full max-w-xl flex-col border-l border-slate-200 bg-white shadow-2xl transition-transform duration-200 ease-out ${
          isClosing
            ? "translate-x-full"
            : "translate-x-0 animate-[drawer-in_200ms_ease-out]"
        }`}
      >
        <div className="flex items-center justify-between border-b border-slate-100 px-6 py-4">
          <div className="flex items-center gap-3">
            <div className="flex h-9 w-9 items-center justify-center rounded-lg border border-slate-200 bg-slate-50 text-slate-800">
              {drawerIcons[drawer]}
            </div>
            <div>
              <div className="text-[10px] font-semibold tracking-wider text-slate-400 uppercase">
                {t(`${drawer}.subtitle`)}
              </div>
              <h2 className="text-base font-semibold text-slate-900">
                {drawer === "config" ? (
                  <span>
                    {t("config.title")}
                    <span className="sr-only"> - {t("settingsPanel")} / SETTINGS PANEL</span>
                  </span>
                ) : (
                  t(`${drawer}.title`)
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

        <div className="flex-1 overflow-y-auto p-6">{children}</div>
      </aside>
    </div>
  );
};
