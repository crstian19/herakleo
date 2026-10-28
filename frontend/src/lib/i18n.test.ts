import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { stubEnv, type FakeEnv } from "./test-env";

async function load(env: FakeEnv = {}) {
  vi.resetModules();
  const fake = stubEnv(env);
  const i18n = await import("./i18n.svelte");
  return { ...fake, ...i18n };
}

beforeEach(() => vi.useRealTimers());
afterEach(() => {
  vi.unstubAllGlobals();
  vi.useRealTimers();
});

describe("language pick", () => {
  it("uses the first system language that has a catalogue", async () => {
    const { currentLocale } = await load({ languages: ["ja-JP", "pt-BR", "de"] });
    expect(currentLocale()).toBe("pt");
  });

  it("matches the base language case-insensitively", async () => {
    const { currentLocale } = await load({ languages: ["FR-ca"] });
    expect(currentLocale()).toBe("fr");
  });

  it("falls back to English when nothing matches or the list is empty", async () => {
    expect((await load({ languages: ["ja", "zh-CN"] })).currentLocale()).toBe("en");
    expect((await load({ languages: [] })).currentLocale()).toBe("en");
  });

  it("sets the document language", async () => {
    const { html } = await load({ languages: ["it-IT"] });
    expect(html.lang).toBe("it");
  });

  it("prefers a saved language over the system one", async () => {
    const { currentLocale } = await load({
      languages: ["en-US"],
      storage: { "herakleo:prefs": JSON.stringify({ language: "de" }) },
    });
    expect(currentLocale()).toBe("de");
  });

  it("ignores a saved language it has no catalogue for", async () => {
    const { currentLocale } = await load({
      languages: ["fr"],
      storage: { "herakleo:prefs": JSON.stringify({ language: "xx" }) },
    });
    expect(currentLocale()).toBe("fr");
  });

  it("setLanguage switches, persists, and auto returns to the system language", async () => {
    const { setLanguage, currentLocale, html, store } = await load({ languages: ["es-ES"] });
    setLanguage("en");
    expect(currentLocale()).toBe("en");
    expect(html.lang).toBe("en");
    expect(JSON.parse(store.get("herakleo:prefs")!).language).toBe("en");
    setLanguage("auto");
    expect(currentLocale()).toBe("es");
  });
});

describe("t", () => {
  it("translates and interpolates", async () => {
    const { t } = await load({ languages: ["en"] });
    expect(t("nav.hideGroupNamed", { name: "News" })).toBe("Hide group News");
  });

  it("follows the current language", async () => {
    const { t, setLanguage } = await load({ languages: ["en"] });
    const en = t("nav.all");
    setLanguage("es");
    expect(t("nav.all")).toBe("Todos");
    expect(t("nav.all")).not.toBe(en);
  });

  it("leaves a placeholder untouched when its param is missing", async () => {
    const { t } = await load({ languages: ["en"] });
    expect(t("nav.hideGroupNamed")).toBe("Hide group {name}");
  });

  it("formats numeric params with the locale", async () => {
    const { t, num } = await load({ languages: ["de"] });
    expect(t("err.server_status", { detail: 1234567 })).toContain(num(1234567));
    expect(num(1234567)).toBe("1.234.567");
  });

  it("returns the key itself for an unknown key", async () => {
    const { t } = await load();
    expect(t("no.such.key" as never)).toBe("no.such.key");
  });
});

describe("hasKey", () => {
  it("tells catalogue keys from anything else", async () => {
    const { hasKey } = await load();
    expect(hasKey("err.source_not_found")).toBe(true);
    expect(hasKey("err.nope")).toBe(false);
  });

  it("is false for inherited object property names", async () => {
    const { hasKey } = await load();
    expect(hasKey("toString")).toBe(false);
  });
});

describe("ago and date", () => {
  const T = new Date("2026-06-15T12:00:00Z");

  it("uses relative text for recent times", async () => {
    vi.useFakeTimers();
    vi.setSystemTime(T);
    const { ago } = await load({ languages: ["en"] });
    const now = T.getTime() / 1000;
    expect(ago(now - 10)).toBe("now");
    expect(ago(now - 5 * 60)).toBe("5 minutes ago");
    expect(ago(now - 3 * 3600)).toBe("3 hours ago");
  });

  it("uses a date once it is a day old or more", async () => {
    vi.useFakeTimers();
    vi.setSystemTime(T);
    const { ago, date } = await load({ languages: ["en"] });
    const old = T.getTime() / 1000 - 3 * 86400;
    expect(ago(old)).toBe(date(old));
    expect(ago(old)).toMatch(/2026/);
  });
});
