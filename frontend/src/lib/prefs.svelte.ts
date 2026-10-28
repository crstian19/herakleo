import type { ChannelSort } from "./types";

// UI-only preferences, per machine. Backend data (lists, favourites) lives in
// ~/.config/herakleo; these are just display choices, so localStorage is fine.
const KEY = "herakleo:prefs";

/** Accent themes defined in app.css (`:root[data-accent=…]`); "gold" is the default. */
export const accents = ["gold", "nile", "lapis", "crimson", "amethyst", "emerald"] as const;

// Accent IDs used to be Spanish; map old saved values so nobody loses theirs.
const legacyAccents: Record<string, Accent> = {
  oro: "gold",
  nilo: "nile",
  carmesi: "crimson",
  amatista: "amethyst",
  esmeralda: "emerald",
};
export type Accent = (typeof accents)[number];

interface Prefs {
  /** Tint each card with its logo's colour instead of a name-based hue. */
  logoColors: boolean;
  /** Last sort picked in the header; history ignores it. */
  sort: ChannelSort;
  accent: Accent;
  /** "auto" follows the system language; otherwise a locale code (see i18n). */
  language: string;
  /** "pages": fixed pages of channels (default, always light); "scroll": infinite scroll. */
  browse: "pages" | "scroll";
  /** Decorative effects (ambient glow, hover zoom/shadow/spotlight, logo glows). Off for slow PCs. */
  effects: boolean;
  /** The user set `effects` in Settings: slowscroll.ts no longer turns them off. */
  effectsChosen: boolean;
}

const reducedMotion = () => {
  try {
    return matchMedia("(prefers-reduced-motion: reduce)").matches;
  } catch {
    return false;
  }
};

const defaults: Prefs = {
  logoColors: true,
  sort: "",
  accent: "gold",
  language: "auto",
  browse: "pages",
  // Follow the system's "reduce motion" setting until the user chooses.
  effects: !reducedMotion(),
  effectsChosen: false,
};

function load(): Prefs {
  try {
    const p = { ...defaults, ...JSON.parse(localStorage.getItem(KEY) ?? "{}") };
    p.accent = legacyAccents[p.accent] ?? p.accent;
    if (!accents.includes(p.accent)) p.accent = defaults.accent;
    if (p.browse !== "pages" && p.browse !== "scroll") p.browse = defaults.browse;
    return p;
  } catch {
    return { ...defaults };
  }
}

export const prefs = $state<Prefs>(load());

export function savePrefs() {
  try {
    localStorage.setItem(KEY, JSON.stringify(prefs));
  } catch {
    // Storage can be unavailable; preferences then last for this session only.
  }
}

/** Applies and persists an accent theme. */
export function setAccent(accent: Accent) {
  prefs.accent = accent;
  savePrefs();
  applyAccent();
}

export function applyAccent() {
  document.documentElement.dataset.accent = prefs.accent;
}

/** Applies and persists the effects switch (app.css `:root[data-effects="off"]`). */
export function setEffects(on: boolean, chosen = true) {
  prefs.effects = on;
  if (chosen) prefs.effectsChosen = true;
  savePrefs();
  applyEffects();
}

export function applyEffects() {
  document.documentElement.dataset.effects = prefs.effects ? "on" : "off";
}
