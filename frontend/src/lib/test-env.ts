import { vi } from "vitest";

// Browser globals for modules that read them at import time (prefs, i18n).
// Call before a dynamic import, after vi.resetModules().
export interface FakeEnv {
  languages?: string[];
  /** Initial localStorage contents; null makes every storage call throw. */
  storage?: Record<string, string> | null;
  reducedMotion?: boolean;
}

export function stubEnv({ languages = ["en-US"], storage = {}, reducedMotion = false }: FakeEnv = {}) {
  const html = { lang: "", dataset: {} as Record<string, string> };
  const store = new Map(Object.entries(storage ?? {}));
  const fail = () => {
    throw new Error("storage unavailable");
  };
  vi.stubGlobal("navigator", { languages, language: languages[0] });
  vi.stubGlobal("document", { documentElement: html, hidden: false });
  vi.stubGlobal("matchMedia", () => ({ matches: reducedMotion }));
  vi.stubGlobal(
    "localStorage",
    storage === null
      ? { getItem: fail, setItem: fail }
      : { getItem: (k: string) => store.get(k) ?? null, setItem: (k: string, v: string) => void store.set(k, v) },
  );
  return { html, store };
}
