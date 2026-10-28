import { describe, expect, it } from "vitest";
import de from "./de";
import en from "./en";
import es from "./es";
import fr from "./fr";
import it_ from "./it";
import pt from "./pt";

const locales: Record<string, Record<string, string>> = { en, fr, pt, de, it: it_ };
const base: Record<string, string> = es;
const placeholders = (s: string) => [...s.matchAll(/\{(\w+)\}/g)].map((m) => m[1]!).sort();

describe.each(Object.entries(locales))("locale %s", (_name, messages) => {
  it("has exactly the keys of es", () => {
    expect(Object.keys(messages).sort()).toEqual(Object.keys(base).sort());
  });

  it("uses the same {placeholders} as es for every key", () => {
    const bad = Object.keys(base).filter(
      (k) => placeholders(messages[k] ?? "").join() !== placeholders(base[k]!).join(),
    );
    expect(bad).toEqual([]);
  });

  it("has no empty messages", () => {
    expect(Object.entries(messages).filter(([, v]) => v.trim() === "")).toEqual([]);
  });
});
