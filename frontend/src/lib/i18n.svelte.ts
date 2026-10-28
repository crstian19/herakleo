// UI translations. The language follows the system (navigator.languages,
// which WebKitGTK takes from the desktop locale) unless the user picks one in
// Settings. Numbers, dates and "x minutes ago" come from Intl, so catalogues
// only hold words.
import de from "./locales/de";
import en from "./locales/en";
import es, { type Messages } from "./locales/es";
import fr from "./locales/fr";
import it from "./locales/it";
import pt from "./locales/pt";
import { prefs, savePrefs } from "./prefs.svelte";

const catalogues = { es, en, fr, pt, de, it } satisfies Record<string, Messages>;

export type Locale = keyof typeof catalogues;
export type MessageKey = keyof Messages;

/** Native names, shown as-is whatever the current language. */
export const localeNames: Record<Locale, string> = {
  es: "Español",
  en: "English",
  fr: "Français",
  pt: "Português",
  de: "Deutsch",
  it: "Italiano",
};

const isLocale = (v: string): v is Locale => v in catalogues;

/** The first system language we have a catalogue for; English otherwise. */
export function systemLocale(): Locale {
  const wanted = typeof navigator === "undefined" ? [] : navigator.languages?.length ? navigator.languages : [navigator.language];
  for (const tag of wanted) {
    const base = tag?.toLowerCase().split("-")[0] ?? "";
    if (isLocale(base)) return base;
  }
  return "en";
}

function resolve(pref: string): Locale {
  return pref !== "auto" && isLocale(pref) ? pref : systemLocale();
}

const state = $state({ locale: resolve(prefs.language) });

export function currentLocale(): Locale {
  return state.locale;
}

/** Sets "auto" or a fixed language and persists the choice. */
export function setLanguage(pref: "auto" | Locale) {
  prefs.language = pref;
  savePrefs();
  state.locale = resolve(pref);
  document.documentElement.lang = state.locale;
}

/** Translates a key; {placeholders} are replaced from params. Reactive in templates. */
export function t(key: MessageKey, params: Record<string, string | number> = {}): string {
  const msg: string = catalogues[state.locale][key] ?? es[key] ?? key;
  return msg.replace(/\{(\w+)\}/g, (_, name: string) => {
    const v = params[name];
    return v === undefined ? `{${name}}` : typeof v === "number" ? num(v) : v;
  });
}

export function hasKey(key: string): key is MessageKey {
  return Object.hasOwn(es, key);
}

export function num(n: number): string {
  return n.toLocaleString(state.locale);
}

export function date(unix: number): string {
  return new Date(unix * 1000).toLocaleDateString(state.locale, { day: "numeric", month: "short", year: "numeric" });
}

/** "5 minutes ago"-style text for recent times, a date for older ones. */
export function ago(unix: number): string {
  const secs = Date.now() / 1000 - unix;
  const rtf = new Intl.RelativeTimeFormat(state.locale, { numeric: "auto" });
  if (secs < 60) return rtf.format(0, "second");
  if (secs < 3600) return rtf.format(-Math.round(secs / 60), "minute");
  if (secs < 86400) return rtf.format(-Math.round(secs / 3600), "hour");
  return date(unix);
}

document.documentElement.lang = state.locale;
