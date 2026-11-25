import { useCallback, useSyncExternalStore } from "react";
import zh from "./locales/zh.json";
import en from "./locales/en.json";

export type Language = "zh" | "en";
export type InterpolationValues = Record<string, string | number | boolean | null | undefined>;
export type TranslateFn = (key: string, values?: InterpolationValues) => string;

type LocaleTree = string | { readonly [key: string]: LocaleTree };

const STORAGE_KEY = "ai-flight-dashboard_lang";
const RESOURCES: Record<Language, LocaleTree> = { zh, en };
const listeners = new Set<() => void>();

const normalizeLang = (lng: string | null | undefined): Language => (lng === "en" ? "en" : "zh");

let currentLang: Language = (() => {
  if (typeof window === "undefined") return "zh";
  try {
    return normalizeLang(window.localStorage.getItem(STORAGE_KEY));
  } catch {
    return "zh";
  }
})();

function lookup(tree: LocaleTree, path: string): string | undefined {
  let cursor: LocaleTree | undefined = tree;
  for (const segment of path.split(".")) {
    if (typeof cursor !== "object" || cursor === null || !(segment in cursor)) {
      return undefined;
    }
    cursor = cursor[segment];
  }
  return typeof cursor === "string" ? cursor : undefined;
}

function translate(lang: Language, key: string, values?: InterpolationValues): string {
  const raw =
    lookup(RESOURCES[lang], key) ??
    (lang !== "en" ? lookup(RESOURCES.en, key) : undefined) ??
    key;
  if (!values) return raw;
  return raw.replace(/\{\{\s*([a-zA-Z0-9_]+)\s*\}\}/g, (match, token: string) => {
    const replacement = values[token];
    return replacement === undefined || replacement === null ? match : String(replacement);
  });
}

const subscribe = (listener: () => void) => {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
};

const getSnapshot = (): Language => currentLang;
const getServerSnapshot = (): Language => "zh";

const i18n = {
  get language(): Language {
    return currentLang;
  },
  changeLanguage(lng: string): Promise<void> {
    const next = normalizeLang(lng);
    if (typeof window !== "undefined") {
      try {
        window.localStorage.setItem(STORAGE_KEY, next);
      } catch {
        // Ignore storage errors in restricted contexts.
      }
    }
    if (next !== currentLang) {
      currentLang = next;
      listeners.forEach((listener) => listener());
    }
    return Promise.resolve();
  },
  t(key: string, values?: InterpolationValues): string {
    return translate(currentLang, key, values);
  },
};

export const initReactI18next = { type: "3rdParty" as const, init: () => undefined };

export function useTranslation(): { t: TranslateFn; i18n: typeof i18n } {
  const lang = useSyncExternalStore(subscribe, getSnapshot, getServerSnapshot);
  const t = useCallback<TranslateFn>((key, values) => translate(lang, key, values), [lang]);
  return { t, i18n };
}

export default i18n;
