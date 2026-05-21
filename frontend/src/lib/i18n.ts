import { addMessages, init, getLocaleFromNavigator, locale, _ } from "svelte-i18n";
import en from "./locales/en.json";
import de from "./locales/de.json";

export const SUPPORTED_LOCALES = ["en", "de"] as const;
export type SupportedLocale = (typeof SUPPORTED_LOCALES)[number];
const STORAGE_KEY = "steelpage_locale";

addMessages("en", en);
addMessages("de", de);

function normalizeLocale(raw: string | null | undefined): SupportedLocale {
  if (!raw) return "en";
  const head = raw.toLowerCase().split(/[-_]/)[0];
  return (SUPPORTED_LOCALES as readonly string[]).includes(head)
    ? (head as SupportedLocale)
    : "en";
}

function pickInitial(): SupportedLocale {
  try {
    const stored = localStorage.getItem(STORAGE_KEY);
    if (stored) return normalizeLocale(stored);
  } catch {
    // ignore (e.g. private mode)
  }
  return normalizeLocale(getLocaleFromNavigator());
}

init({
  fallbackLocale: "en",
  initialLocale: pickInitial(),
});

locale.subscribe((l) => {
  if (l && typeof document !== "undefined") {
    document.documentElement.lang = l.split(/[-_]/)[0];
  }
});

export function setLocale(next: SupportedLocale): void {
  locale.set(next);
  try {
    localStorage.setItem(STORAGE_KEY, next);
  } catch {
    // ignore
  }
}

export { locale, _ };
