import { beforeAll, describe, expect, it, vi } from "vitest";
import { stubEnv } from "./test-env";

type Api = typeof import("./api");
let api: Api;

beforeAll(async () => {
  vi.resetModules();
  stubEnv({ languages: ["en"] });
  api = await import("./api");
});

describe("errorMessage", () => {
  it("translates a bare error code", () => {
    expect(api.errorMessage("source_not_found")).toBe("List not found.");
  });

  it("splits `code: detail` and fills the detail in", () => {
    expect(api.errorMessage("download_failed: connection refused")).toBe(
      "Couldn't download the list: connection refused",
    );
  });

  it("keeps colons and newlines inside the detail", () => {
    expect(api.errorMessage("download_failed: dial tcp 1.2.3.4:80:\ntimeout")).toBe(
      "Couldn't download the list: dial tcp 1.2.3.4:80:\ntimeout",
    );
  });

  it("trims the raw string", () => {
    expect(api.errorMessage("  source_not_found\n")).toBe("List not found.");
  });

  it("shows unknown codes and free text raw", () => {
    expect(api.errorMessage("brand_new_code")).toBe("brand_new_code");
    expect(api.errorMessage("Something went wrong!")).toBe("Something went wrong!");
  });

  it("does not treat inherited object keys as codes", () => {
    expect(api.errorMessage("constructor")).toBe("constructor");
  });

  it("accepts Error objects and other values", () => {
    expect(api.errorMessage(new Error("source_not_found"))).toBe("List not found.");
    expect(api.errorMessage(42)).toBe("42");
    expect(api.errorMessage(undefined)).toBe("undefined");
  });

  it("leaves an empty detail for codes without one", () => {
    expect(api.errorMessage("server_status")).toBe("The server answered ");
  });
});

describe("accentFor", () => {
  it("is stable for the same name", () => {
    expect(api.accentFor("La 1")).toBe(api.accentFor("La 1"));
  });

  it("returns an hsl colour with a hue in range", () => {
    for (const name of ["", "CNN", "Real Madrid TV", "Sol Música", "x".repeat(500)]) {
      const m = /^hsl\((\d+) 70% 60%\)$/.exec(api.accentFor(name));
      expect(m).not.toBeNull();
      expect(Number(m![1])).toBeLessThan(360);
    }
  });

  it("gives different names different hues", () => {
    const hues = new Set(["La 1", "La 2", "Antena 3", "Cuatro", "Telecinco", "laSexta"].map(api.accentFor));
    expect(hues.size).toBeGreaterThan(3);
  });
});
