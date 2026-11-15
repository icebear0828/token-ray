import React, { useEffect, useState } from "react";
import { CurrencyCode } from "../types/telemetry";
import { ConfigDrawer } from "./drawers/ConfigDrawer";
import { DevicesDrawer } from "./drawers/DevicesDrawer";
import { DrawerKind, DrawerShell } from "./drawers/DrawerShell";
import { LanDrawer } from "./drawers/LanDrawer";
import { PluginsDrawer } from "./drawers/PluginsDrawer";
import { PricingDrawer } from "./drawers/PricingDrawer";

export type DrawerType = DrawerKind | null;

interface SlideOverDrawersProps {
  activeDrawer: DrawerType;
  onClose: () => void;
  currency: CurrencyCode;
  onDataMutated: () => void;
}

export const SlideOverDrawers: React.FC<SlideOverDrawersProps> = ({
  activeDrawer,
  onClose,
  currency,
  onDataMutated,
}) => {
  // Keep the panel mounted briefly after closing so the exit transition can
  // play. Each drawer owns its data loading and resets when it unmounts.
  const [renderedDrawer, setRenderedDrawer] = useState<DrawerType>(activeDrawer);
  const [isClosing, setIsClosing] = useState(false);

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

  if (!renderedDrawer) return null;

  return (
    <DrawerShell drawer={renderedDrawer} isClosing={isClosing} onClose={onClose}>
      {renderedDrawer === "lan" && (
        <LanDrawer currency={currency} onDataMutated={onDataMutated} />
      )}
      {renderedDrawer === "devices" && (
        <DevicesDrawer currency={currency} onDataMutated={onDataMutated} />
      )}
      {renderedDrawer === "pricing" && <PricingDrawer onDataMutated={onDataMutated} />}
      {renderedDrawer === "plugins" && <PluginsDrawer onDataMutated={onDataMutated} />}
      {renderedDrawer === "config" && <ConfigDrawer onDataMutated={onDataMutated} />}
    </DrawerShell>
  );
};
