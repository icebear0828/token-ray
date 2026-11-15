import React, { useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { Check, CheckCircle2, Plus } from "lucide-react";
import { fetchPricing, savePricing } from "../../api/client";
import { PricingEntry } from "../../types/telemetry";
import { usePagination } from "../../utils/usePagination";

interface PricingDrawerProps {
  onDataMutated: () => void;
}

const PRICING_PAGE_SIZE = 20;

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

  useEffect(() => {
    let cancelled = false;
    fetchPricing()
      .then((list) => {
        if (!cancelled) setPricingList(list);
      })
      .catch(() => undefined);
    return () => {
      cancelled = true;
    };
  }, []);

  const sortedPricing = useMemo(
    () => [...pricingList].sort((a, b) => a.model.localeCompare(b.model)),
    [pricingList]
  );

  const pricingPagination = usePagination(sortedPricing, PRICING_PAGE_SIZE);

  const handlePricingChange = (
    idx: number,
    field: keyof Omit<PricingEntry, "model">,
    raw: string
  ) => {
    const value = parseFloat(raw);
    setPricingList((prev) =>
      prev.map((item, i) =>
        i === idx ? { ...item, [field]: Number.isFinite(value) ? Math.max(0, value) : 0 } : item
      )
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
      setTimeout(() => setPricingSaved(false), 2200);
    } finally {
      setPricingSaving(false);
    }
  };

  return (
    <div className="space-y-4">
      <div className="rounded-xl border border-slate-200 bg-slate-50/70 p-3.5">
        <label className="block text-[11px] font-semibold text-slate-500">
          {t("pricing.add_model")}
        </label>
        <div className="mt-1.5 flex gap-2">
          <input
            type="text"
            value={newModelName}
            onChange={(event) => setNewModelName(event.target.value)}
            onKeyDown={(event) => {
              if (event.key === "Enter") handleAddModel();
            }}
            placeholder={t("pricing.model_placeholder")}
            className="font-mono-num min-w-0 flex-1 rounded-lg border border-slate-200 bg-white px-2.5 py-1.5 text-xs text-slate-900 focus:border-slate-900 focus:outline-none"
          />
          <button
            type="button"
            onClick={handleAddModel}
            className="inline-flex items-center gap-1 rounded-lg bg-slate-900 px-3 py-1.5 text-xs font-semibold text-white hover:bg-slate-800"
          >
            <Plus className="h-3.5 w-3.5" />
            {t("pricing.add_btn")}
          </button>
        </div>
      </div>

      {sortedPricing.length === 0 && (
        <div className="rounded-xl border border-dashed border-slate-200 bg-slate-50/70 p-8 text-center text-xs text-slate-500">
          {t("pricing.no_entries")}
        </div>
      )}

      <div className="space-y-3">
        {pricingPagination.pageItems.map((entry) => {
          const idx = pricingList.findIndex((item) => item.model === entry.model);
          return (
            <div key={entry.model} className="rounded-xl border border-slate-200 bg-white p-3.5">
              <div className="mb-2.5 flex items-center justify-between gap-2">
                <span className="font-mono-num text-xs font-semibold text-slate-900">
                  {entry.model}
                </span>
              </div>

              <div className="grid grid-cols-2 gap-2.5 sm:grid-cols-4">
                <div>
                  <label className="block text-[10px] font-medium text-slate-400">
                    {t("pricing.input_m")}
                  </label>
                  <input
                    type="number"
                    step="0.01"
                    value={entry.input_price_per_m}
                    onChange={(event) =>
                      handlePricingChange(idx, "input_price_per_m", event.target.value)
                    }
                    className="font-mono-num mt-1 w-full rounded-md border border-slate-200 px-2 py-1 text-xs text-slate-900 focus:border-slate-900 focus:outline-none"
                  />
                </div>
                <div>
                  <label className="block text-[10px] font-medium text-slate-400">
                    {t("pricing.cached_m")}
                  </label>
                  <input
                    type="number"
                    step="0.01"
                    value={entry.cached_price_per_m}
                    onChange={(event) =>
                      handlePricingChange(idx, "cached_price_per_m", event.target.value)
                    }
                    className="font-mono-num mt-1 w-full rounded-md border border-slate-200 px-2 py-1 text-xs text-slate-900 focus:border-slate-900 focus:outline-none"
                  />
                </div>
                <div>
                  <label className="block text-[10px] font-medium text-slate-400">
                    {t("pricing.write_m")}
                  </label>
                  <input
                    type="number"
                    step="0.01"
                    value={entry.cache_creation_price_per_m}
                    onChange={(event) =>
                      handlePricingChange(idx, "cache_creation_price_per_m", event.target.value)
                    }
                    className="font-mono-num mt-1 w-full rounded-md border border-slate-200 px-2 py-1 text-xs text-slate-900 focus:border-slate-900 focus:outline-none"
                  />
                </div>
                <div>
                  <label className="block text-[10px] font-medium text-slate-400">
                    {t("pricing.output_m")}
                  </label>
                  <input
                    type="number"
                    step="0.01"
                    value={entry.output_price_per_m}
                    onChange={(event) =>
                      handlePricingChange(idx, "output_price_per_m", event.target.value)
                    }
                    className="font-mono-num mt-1 w-full rounded-md border border-slate-200 px-2 py-1 text-xs text-slate-900 focus:border-slate-900 focus:outline-none"
                  />
                </div>
              </div>
            </div>
          );
        })}
      </div>

      {pricingPagination.total > PRICING_PAGE_SIZE && (
        <div className="flex items-center justify-between text-[11px] text-slate-500">
          <span className="font-mono-num">
            {t("pagination.page_of", {
              page: pricingPagination.page + 1,
              total: pricingPagination.pageCount,
            })}
          </span>
          <div className="flex items-center gap-1.5">
            <button
              type="button"
              disabled={pricingPagination.page === 0}
              onClick={() => pricingPagination.setPage(pricingPagination.page - 1)}
              className="rounded-md border border-slate-200 px-2 py-1 font-medium text-slate-600 transition-colors hover:bg-slate-50 disabled:cursor-not-allowed disabled:opacity-40"
            >
              {t("pagination.prev")}
            </button>
            <button
              type="button"
              disabled={pricingPagination.page >= pricingPagination.pageCount - 1}
              onClick={() => pricingPagination.setPage(pricingPagination.page + 1)}
              className="rounded-md border border-slate-200 px-2 py-1 font-medium text-slate-600 transition-colors hover:bg-slate-50 disabled:cursor-not-allowed disabled:opacity-40"
            >
              {t("pagination.next")}
            </button>
          </div>
        </div>
      )}

      <div className="sticky bottom-0 flex items-center justify-between border-t border-slate-100 bg-white pt-4">
        {pricingSaved ? (
          <span className="inline-flex items-center gap-1.5 text-xs font-medium text-emerald-600">
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
          className="inline-flex items-center gap-1.5 rounded-lg bg-slate-900 px-4 py-2 text-xs font-semibold text-white hover:bg-slate-800 disabled:opacity-50"
        >
          <Check className="h-3.5 w-3.5" />
          {t("pricing.save_btn")}
        </button>
      </div>
    </div>
  );
};
