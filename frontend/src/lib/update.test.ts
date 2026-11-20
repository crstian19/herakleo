import { afterEach, describe, expect, it, vi } from "vitest";
import { stubEnv } from "./test-env";
import { releaseNotes, skippedVersion, skipVersion } from "./update";

describe("releaseNotes", () => {
  it("turns a releaser-pleaser body into plain text", () => {
    const md = [
      "## Fixes",
      "",
      "- update module github.com/wailsapp/wails/v3 to v3.1.0 (#17)",
      "- **deps**: bump Go ([e0dc638](https://github.com/crstian19/herakleo/commit/e0dc638f9ebe))",
      "",
      "",
      "",
      "**Full changes**: https://github.com/crstian19/herakleo/compare/v0.4.1...v0.4.2",
    ].join("\n");
    expect(releaseNotes(md)).toBe(
      ["Fixes", "", "• update module github.com/wailsapp/wails/v3 to v3.1.0 (#17)", "• deps: bump Go"].join("\n"),
    );
  });

  it("keeps link text and never leaves markup", () => {
    expect(releaseNotes("See [the docs](https://x.y) and `just build`")).toBe("See the docs and just build");
    expect(releaseNotes("[Compare to previous version](https://x)\n### Features")).toBe("Features");
  });

  it("returns HTML as inert text (rendered with text interpolation)", () => {
    expect(releaseNotes("<img src=x onerror=alert(1)>")).toBe("<img src=x onerror=alert(1)>");
  });
});

describe("skipped version", () => {
  afterEach(() => vi.unstubAllGlobals());

  it("round-trips through localStorage", () => {
    const { store } = stubEnv();
    expect(skippedVersion()).toBe("");
    skipVersion("0.5.0");
    expect(skippedVersion()).toBe("0.5.0");
    expect(store.get("herakleo:update-skipped")).toBe("0.5.0");
  });

  it("does not throw when localStorage is unavailable", () => {
    stubEnv({ storage: null });
    expect(() => skipVersion("0.5.0")).not.toThrow();
    expect(skippedVersion()).toBe("");
  });
});
