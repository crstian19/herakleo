import { afterEach, describe, expect, it, vi } from "vitest";
import { stubEnv, type FakeEnv } from "./test-env";

const KEY = "herakleo:prefs";

async function load(env: FakeEnv = {}) {
  vi.resetModules();
  const fake = stubEnv(env);
  const mod = await import("./prefs.svelte");
  return { ...fake, ...mod };
}

afterEach(() => vi.unstubAllGlobals());

describe("prefs defaults", () => {
  it("starts with the documented defaults", async () => {
    const { prefs } = await load();
    expect(prefs).toEqual({
      logoColors: true,
      sort: "",
      accent: "gold",
      language: "auto",
      browse: "pages",
      effects: true,
      effectsChosen: false,
    });
  });

  it("turns effects off by default when the system asks for reduced motion", async () => {
    const { prefs } = await load({ reducedMotion: true });
    expect(prefs.effects).toBe(false);
    expect(prefs.effectsChosen).toBe(false);
  });

  it("merges saved values over the defaults", async () => {
    const { prefs } = await load({ storage: { [KEY]: JSON.stringify({ sort: "az", accent: "nile" }) } });
    expect(prefs.sort).toBe("az");
    expect(prefs.accent).toBe("nile");
    expect(prefs.browse).toBe("pages");
  });

  it("maps the old Spanish accent IDs", async () => {
    const { prefs } = await load({ storage: { [KEY]: JSON.stringify({ accent: "esmeralda" }) } });
    expect(prefs.accent).toBe("emerald");
  });

  it("replaces invalid saved accent and browse values", async () => {
    const { prefs } = await load({ storage: { [KEY]: JSON.stringify({ accent: "pink", browse: "wall" }) } });
    expect(prefs.accent).toBe("gold");
    expect(prefs.browse).toBe("pages");
  });
});

describe("prefs storage failures", () => {
  it("falls back to defaults when saved JSON is corrupt", async () => {
    const { prefs } = await load({ storage: { [KEY]: "{not json" } });
    expect(prefs.accent).toBe("gold");
  });

  it("does not throw when localStorage is unavailable", async () => {
    const { prefs, savePrefs, setAccent, setEffects } = await load({ storage: null });
    expect(prefs.accent).toBe("gold");
    expect(() => savePrefs()).not.toThrow();
    expect(() => setAccent("lapis")).not.toThrow();
    expect(() => setEffects(false)).not.toThrow();
    expect(prefs.accent).toBe("lapis");
    expect(prefs.effects).toBe(false);
  });
});

describe("prefs setters", () => {
  it("setAccent persists and sets the data attribute", async () => {
    const { setAccent, html, store } = await load();
    setAccent("crimson");
    expect(html.dataset.accent).toBe("crimson");
    expect(JSON.parse(store.get(KEY)!).accent).toBe("crimson");
  });

  it("setEffects marks the choice unless told otherwise", async () => {
    const { setEffects, prefs, html } = await load();
    setEffects(false, false);
    expect(prefs.effects).toBe(false);
    expect(prefs.effectsChosen).toBe(false);
    expect(html.dataset.effects).toBe("off");
    setEffects(true);
    expect(prefs.effectsChosen).toBe(true);
    expect(html.dataset.effects).toBe("on");
  });
});
