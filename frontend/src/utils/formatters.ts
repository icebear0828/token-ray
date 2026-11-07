import { CurrencyCode } from "../types/telemetry";

export const EXCHANGE_RATES: Record<CurrencyCode, { symbol: string; rate: number; label: string }> = {
  USD: { symbol: "$", rate: 1, label: "USD ($)" },
  CNY: { symbol: "¥", rate: 7.24, label: "CNY (¥)" },
  EUR: { symbol: "€", rate: 0.92, label: "EUR (€)" },
  JPY: { symbol: "¥", rate: 154.5, label: "JPY (¥)" },
};

/**
 * Formats large token / event counts with exact domain abbreviation rules:
 * >= 1e9 -> 1.23B
 * >= 1e6 -> 1.23M
 * >= 1e3 -> 1.2K
 */
export function formatCompactNumber(num: number | undefined | null): string {
  const n = Number(num ?? 0);
  if (!Number.isFinite(n) || n === 0) return "0";
  const abs = Math.abs(n);
  const sign = n < 0 ? "-" : "";
  if (abs >= 1e9) {
    return `${sign}${(abs / 1e9).toFixed(2)}B`;
  }
  if (abs >= 1e6) {
    return `${sign}${(abs / 1e6).toFixed(2)}M`;
  }
  if (abs >= 1e3) {
    return `${sign}${(abs / 1e3).toFixed(1)}K`;
  }
  return `${sign}${Math.round(abs)}`;
}

export function formatExactNumber(num: number | undefined | null): string {
  const n = Math.round(Number(num ?? 0));
  return n.toLocaleString("en-US");
}

/**
 * Formats cost in USD (or converted multi-currency).
 * Rule: Less than $0.01 displays 4 decimal places; otherwise 2 decimal places.
 */
export function formatCost(usdAmount: number | undefined | null, currency: CurrencyCode = "USD"): string {
  const rawUsd = Number(usdAmount ?? 0);
  if (!Number.isFinite(rawUsd)) return "$0.00";
  const { symbol, rate } = EXCHANGE_RATES[currency] ?? EXCHANGE_RATES.USD;
  const converted = rawUsd * rate;

  if (converted === 0) {
    return `${symbol}0.00`;
  }
  if (Math.abs(rawUsd) < 0.01 || Math.abs(converted) < 0.01) {
    return `${symbol}${converted.toFixed(4)}`;
  }
  if (currency === "JPY" && Math.abs(converted) >= 100) {
    return `${symbol}${Math.round(converted).toLocaleString("en-US")}`;
  }
  return `${symbol}${converted.toFixed(2)}`;
}

export function formatUnitPrice(usdPerMillion: number | undefined | null, currency: CurrencyCode = "USD"): string {
  const rawUsd = Number(usdPerMillion ?? 0);
  const { symbol, rate } = EXCHANGE_RATES[currency] ?? EXCHANGE_RATES.USD;
  const converted = rawUsd * rate;
  if (converted === 0) return `${symbol}0`;
  if (converted < 0.01) return `${symbol}${converted.toFixed(4)}`;
  if (converted < 1) return `${symbol}${converted.toFixed(3)}`;
  return `${symbol}${converted.toFixed(2)}`;
}

/**
 * Computes billable uncached fresh input ("BASE INPUT"):
 * input_tokens - cached_tokens - cache_creation_tokens
 */
export function calcBaseInput(
  inputTokens: number,
  cachedTokens: number,
  cacheCreationTokens: number
): number {
  return Math.max(0, (inputTokens || 0) - (cachedTokens || 0) - (cacheCreationTokens || 0));
}

export function formatPercent(rate: number | undefined | null): string {
  const r = Math.min(100, Math.max(0, Number(rate ?? 0)));
  return `${r.toFixed(1)}%`;
}

export interface SourceVisualTheme {
  id: string;
  color: string;       // Primary hex
  bgTint: string;      // Soft background class
  textClass: string;   // Text color class
  borderClass: string; // Border color class
  dotClass: string;    // Indicator dot class
  shortCode: string;   // 2-3 char telemetry badge
}

const KNOWN_SOURCE_THEMES: Record<string, SourceVisualTheme> = {
  "Claude Code": {
    id: "Claude Code",
    color: "#D97757",
    bgTint: "bg-[#FDF4F0]",
    textClass: "text-[#B85636]",
    borderClass: "border-[#F3D3C6]",
    dotClass: "bg-[#D97757]",
    shortCode: "CLD",
  },
  "Gemini CLI": {
    id: "Gemini CLI",
    color: "#2563EB",
    bgTint: "bg-[#EFF6FF]",
    textClass: "text-[#1D4ED8]",
    borderClass: "border-[#BFDBFE]",
    dotClass: "bg-[#2563EB]",
    shortCode: "GEM",
  },
  "Codex": {
    id: "Codex",
    color: "#059669",
    bgTint: "bg-[#ECFDF5]",
    textClass: "text-[#047857]",
    borderClass: "border-[#A7F3D0]",
    dotClass: "bg-[#059669]",
    shortCode: "CDX",
  },
  "Antigravity": {
    id: "Antigravity",
    color: "#7C3AED",
    bgTint: "bg-[#F5F3FF]",
    textClass: "text-[#6D28D9]",
    borderClass: "border-[#DDD6FE]",
    dotClass: "bg-[#7C3AED]",
    shortCode: "AGV",
  },
};

const FALLBACK_THEMES: SourceVisualTheme[] = [
  {
    id: "custom-1",
    color: "#0891B2",
    bgTint: "bg-cyan-50",
    textClass: "text-cyan-700",
    borderClass: "border-cyan-200",
    dotClass: "bg-cyan-600",
    shortCode: "CLI",
  },
  {
    id: "custom-2",
    color: "#DB2777",
    bgTint: "bg-pink-50",
    textClass: "text-pink-700",
    borderClass: "border-pink-200",
    dotClass: "bg-pink-600",
    shortCode: "AI",
  },
  {
    id: "custom-3",
    color: "#D97706",
    bgTint: "bg-amber-50",
    textClass: "text-amber-700",
    borderClass: "border-amber-200",
    dotClass: "bg-amber-600",
    shortCode: "AGT",
  },
];

export function getSourceTheme(sourceName: string): SourceVisualTheme {
  if (KNOWN_SOURCE_THEMES[sourceName]) {
    return KNOWN_SOURCE_THEMES[sourceName];
  }
  let hash = 0;
  for (let i = 0; i < sourceName.length; i++) {
    hash = sourceName.charCodeAt(i) + ((hash << 5) - hash);
  }
  const idx = Math.abs(hash) % FALLBACK_THEMES.length;
  return {
    ...FALLBACK_THEMES[idx],
    id: sourceName,
    shortCode: sourceName.slice(0, 3).toUpperCase(),
  };
}
