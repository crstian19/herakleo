import { describe, expect, it } from "vitest";
import { exitText } from "./playerexit";

describe("exitText", () => {
  it("puts the player's lines under a one-line header", () => {
    const e = { player: "mpv", title: "La 1", code: 2, seconds: 1, at: 0, log: ["[curl] HTTP error 403", "Failed to open http://h/…."] };
    expect(exitText(e)).toBe("La 1 · mpv · exit 2 · 1s\n[curl] HTTP error 403\nFailed to open http://h/….");
    expect(exitText({ ...e, log: null })).toBe("La 1 · mpv · exit 2 · 1s");
  });
});
