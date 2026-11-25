import React, { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { DollarSign, Laptop, Package, Radio, Sliders, X } from "lucide-react";
import { CurrencyCode } from "../types/telemetry";
import { ConfigDrawer } from "./drawers/ConfigDrawer";
import { DevicesDrawer } from "./drawers/DevicesDrawer";
import { LanDrawer } from "./drawers/LanDrawer";
import { PluginsDrawer } from "./drawers/PluginsDrawer";
import { PricingDrawer } from "./drawers/PricingDrawer";

export type DrawerType = "lan" | "devices" | "pricing" | "plugins" | "config" | null;

type ActiveDrawerType = Exclude<DrawerType, null>;

interface SlideOverDrawersProps {
  activeDrawer: DrawerType;
  onClose: () => void;
  currency: CurrencyCode;
  onDataMutated: () => void;
}

const DRAWER_META: Record<
  ActiveDrawerType,
  {
    Icon: React.ComponentType<{ className?: string }>;
    subtitleKey: string;
    titleKey: string;
  }
> = {
  lan: { Icon: Radio, subtitleKey: "lan.subtitle", titleKey: "lan.title" },
  devices: { Icon: Laptop, subtitleKey: "devices.subtitle", titleKey: "devices.title" },
  pricing: { Icon: DollarSign, subtitleKey: "pricing.subtitle", titleKey: "pricing.title" },
  plugins: { Icon: Package, subtitleKey: "plugins.subtitle", titleKey: "plugins.title" },
  config: { Icon: Sliders, subtitleKey: "config.subtitle", titleKey: "config.title" },
};

export const SlideOverDrawers: React.FC<SlideOverDrawersProps> = ({
  activeDrawer,
  onClose,
  currency,
  onDataMutated,
}) => {
  const { t } = useTranslation();

  // Keep the panel mounted briefly after closing so the exit transition can play.
  const [renderedDrawer, setRenderedDrawer] = useState<DrawerType>(activeDrawer);
  const [isClosing, setIsClosing] = useState(false);
  const onCloseRef = useRef(onClose);
  onCloseRef.current = onClose;
  const closeButtonRef = useRef<HTMLButtonElement>(null);

  useEffect(() => {
    if (activeDrawer) {
      setRenderedDrawer(activeDrawer);
      setIsClosing(false);
      return;
    }
    if (!renderedDrawer) return;
    setIsClosing(true);
    const timer = window.setTimeout(() => {
      setRenderedDrawer(null);
      setIsClosing(false);
    }, 200);
    return () => window.clearTimeout(timer);
  }, [activeDrawer, renderedDrawer]);

  // Close the active drawer on Escape.
  useEffect(() => {
    if (!activeDrawer) return;
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        event.preventDefault();
        onCloseRef.current();
      }
    };
    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [activeDrawer]);

  // Lock body scrolling while any drawer is open (or transitioning out).
  const isOpen = renderedDrawer !== null;
  useEffect(() => {
    if (!isOpen) return;
    const prevOverflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    return () => {
      document.body.style.overflow = prevOverflow;
    };
  }, [isOpen]);

  // Move focus to the close control when a drawer opens.
  useEffect(() => {
    if (activeDrawer) {
      closeButtonRef.current?.focus({ preventScroll: true });
    }
  }, [activeDrawer]);

  if (!renderedDrawer) return null;

  const { Icon, subtitleKey, titleKey } = DRAWER_META[renderedDrawer];

  return (
    <div
      className={`fixed inset-0 z-50 flex justify-end bg-slate-900/25 dark:bg-slate-950/60 backdrop-blur-[2px] transition-opacity duration-200 ${
        isClosing ? "opacity-0" : "animate-[overlay-in_200ms_ease-out] opacity-100"
      }`}
    >
      <div className="flex-1" onClick={onClose} />

      <aside
        role="dialog"
        aria-modal="true"
        aria-labelledby="slide-over-drawer-title"
        className={`flex h-full w-full max-w-xl flex-col border-l border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900 text-slate-900 dark:text-slate-100 shadow-2xl transition-transform duration-200 ease-out ${
          isClosing
            ? "translate-x-full"
            : "translate-x-0 animate-[drawer-in_200ms_ease-out]"
        }`}
      >
        <div className="flex items-center justify-between border-b border-slate-100 dark:border-slate-800 bg-slate-50/40 dark:bg-slate-900/95 px-6 py-4">
          <div className="flex items-center gap-3">
            <div className="flex h-9 w-9 items-center justify-center rounded-lg border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-800 text-slate-800 dark:text-slate-200">
              <Icon className="h-4 w-4" />
            </div>
            <div>
              <div className="text-[10px] font-semibold tracking-wider text-slate-400 dark:text-slate-500 uppercase">
                {t(subtitleKey)}
              </div>
              <h2
                id="slide-over-drawer-title"
                className="text-base font-semibold text-slate-900 dark:text-slate-100"
              >
                {renderedDrawer === "config" ? (
                  <span>
                    {t(titleKey)}
                    <span className="sr-only"> - {t("settingsPanel")} / SETTINGS PANEL</span>
                  </span>
                ) : (
                  t(titleKey)
                )}
              </h2>
            </div>
          </div>

          <button
            ref={closeButtonRef}
            type="button"
            onClick={onClose}
            aria-label={t("drawers.close")}
            className="rounded-lg border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-800 p-1.5 text-slate-500 dark:text-slate-400 transition-colors hover:bg-slate-100 dark:hover:bg-slate-700 hover:text-slate-900 dark:hover:text-slate-100 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-slate-900 dark:focus-visible:outline-slate-200"
          >
            <X className="h-4 w-4" />
          </button>
        </div>

        <div className="flex-1 overflow-y-auto p-6">
          {renderedDrawer === "lan" && (
            <LanDrawer currency={currency} onDataMutated={onDataMutated} />
          )}
          {renderedDrawer === "devices" && (
            <DevicesDrawer currency={currency} onDataMutated={onDataMutated} />
          )}
          {renderedDrawer === "pricing" && <PricingDrawer onDataMutated={onDataMutated} />}
          {renderedDrawer === "plugins" && <PluginsDrawer onDataMutated={onDataMutated} />}
          {renderedDrawer === "config" && <ConfigDrawer onDataMutated={onDataMutated} />}
        </div>
      </aside>
    </div>
  );
};
