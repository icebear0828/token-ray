import React, { useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Check, CheckCircle2, Plus } from "lucide-react";
import { fetchPricing, savePricing } from "../../api/client";
import { PricingEntry } from "../../types/telemetry";
import {
  DRAWER_CARD_CLASS,
  DRAWER_EMPTY_CLASS,
  DRAWER_INPUT_CLASS,
  DRAWER_PANEL_CLASS,
  DRAWER_PRIMARY_BTN_CLASS,
} from "./drawerStyles";

interface PricingDrawerProps {
  onDataMutated: () => void;
}

type NumericPricingField = keyof Omit<PricingEntry, "model">;

const PRICING_FIELDS: ReadonlyArray<{ field: NumericPricingField; labelKey: string }> = [
  { field: "input_price_per_m", labelKey: "pricing.input_m" },
  { field: "cached_price_per_m", labelKey: "pricing.cached_m" },
  { field: "cache_creation_price_per_m", labelKey: "pricing.write_m" },
  { field: "output_price_per_m", labelKey: "pricing.output_m" },
];

const emptyPricingEntry = (model: string): PricingEntry => ({
  model,
  input_price_per_m: 0,
  cached_price_per_m: 0,
  cache_creation_price_per_m: 0,
  output_price_per_m: 0,
});

export const PricingDrawer: React.FC<PricingDrawerProps> = ({ onDataMutated }) => {
  const { t } = useTranslation();
  const [pricingList, setPricingList] = useState<PricingEntry[]>([]);
  const [newModelName, setNewModelName] = useState("");
  const [pricingSaving, setPricingSaving] = useState(false);
  const [pricingSaved, setPricingSaved] = useState(false);
  const savedTimerRef = useRef<number | null>(null);

  useEffect(() => {
    let cancelled = false;
    fetchPricing()
      .then((list) => {
        if (!cancelled) setPricingList(list);
      })
      .catch(() => undefined);

    return () => {
      cancelled = true;
      if (savedTimerRef.current !== null) {
        window.clearTimeout(savedTimerRef.current);
      }
    };
  }, []);

  const sortedPricing = useMemo(
    () => [...pricingList].sort((a, b) => a.model.localeCompare(b.model)),
    [pricingList]
  );

  const handlePricingChange = (model: string, field: NumericPricingField, raw: string) => {
    const value = parseFloat(raw);
    const sanitized = Number.isFinite(value) ? Math.max(0, value) : 0;
    setPricingList((prev) =>
      prev.map((item) => (item.model === model ? { ...item, [field]: sanitized } : item))
    );
  };

  const handleAddModel = () => {
    const model = newModelName.trim();
    if (!model) return;
    if (pricingList.some((entry) => entry.model === model)) return;
    setPricingList((prev) => [emptyPricingEntry(model), ...prev]);
    setNewModelName("");
  };

  const handleSavePricing = async () => {
    setPricingSaving(true);
    try {
      await savePricing(pricingList);
      setPricingSaved(true);
      onDataMutated();
      if (savedTimerRef.current !== null) {
        window.clearTimeout(savedTimerRef.current);
      }
      savedTimerRef.current = window.setTimeout(() => setPricingSaved(false), 2200);
    } finally {
      setPricingSaving(false);
    }
  };

  return (
    <div className="space-y-4">
      <div className={`${DRAWER_PANEL_CLASS} p-3.5`}>
        <label
          htmlFor="pricing-add-model-input"
          className="block text-[11px] font-semibold text-slate-500 dark:text-slate-400"
        >
          {t("pricing.add_model")}
        </label>
        <div className="mt-1.5 flex gap-2">
          <input
            id="pricing-add-model-input"
            type="text"
            value={newModelName}
            onChange={(event) => setNewModelName(event.target.value)}
            onKeyDown={(event) => {
              if (event.key === "Enter") handleAddModel();
            }}
            placeholder={t("pricing.model_placeholder")}
            className={`font-mono-num flex-1 ${DRAWER_INPUT_CLASS}`}
          />
          <button type="button" onClick={handleAddModel} className={DRAWER_PRIMARY_BTN_CLASS}>
            <Plus className="h-3.5 w-3.5" />
            {t("pricing.add_btn")}
          </button>
        </div>
      </div>

      {sortedPricing.length === 0 && (
        <div className={DRAWER_EMPTY_CLASS}>{t("pricing.no_entries")}</div>
      )}

      <div className="space-y-3">
        {sortedPricing.map((entry) => (
          <div key={entry.model} className={`${DRAWER_CARD_CLASS} p-3.5`}>
            <div className="mb-2.5 flex items-center justify-between gap-2">
              <span className="font-mono-num text-xs font-semibold text-slate-900 dark:text-slate-100">
                {entry.model}
              </span>
            </div>

            <div className="grid grid-cols-2 gap-2.5 sm:grid-cols-4">
              {PRICING_FIELDS.map(({ field, labelKey }) => {
                const inputId = `pricing-${entry.model}-${field}`;
                return (
                  <div key={field}>
                    <label
                      htmlFor={inputId}
                      className="block text-[10px] font-medium text-slate-400 dark:text-slate-500"
                    >
                      {t(labelKey)}
                    </label>
                    <input
                      id={inputId}
                      type="number"
                      step="0.01"
                      value={entry[field]}
                      onChange={(event) =>
                        handlePricingChange(entry.model, field, event.target.value)
                      }
                      className={`font-mono-num mt-1 w-full ${DRAWER_INPUT_CLASS}`}
                    />
                  </div>
                );
              })}
            </div>
          </div>
        ))}
      </div>

      <div className="sticky bottom-0 flex items-center justify-between border-t border-slate-100 dark:border-slate-800 bg-white dark:bg-slate-900 pt-4">
        {pricingSaved ? (
          <span className="inline-flex items-center gap-1.5 text-xs font-medium text-emerald-600 dark:text-emerald-400">
            <CheckCircle2 className="h-4 w-4" />
            {t("pricing.saved_toast")}
          </span>
        ) : (
          <span />
        )}
        <button
          type="button"
          disabled={pricingSaving}
          onClick={handleSavePricing}
          className={`${DRAWER_PRIMARY_BTN_CLASS} gap-1.5 px-4 py-2`}
        >
          <Check className="h-3.5 w-3.5" />
          {t("pricing.save_btn")}
        </button>
      </div>
    </div>
  );
};
