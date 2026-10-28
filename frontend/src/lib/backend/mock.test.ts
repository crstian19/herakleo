import { beforeEach, describe, expect, it, vi } from "vitest";
import type { Backend, ChannelQuery } from "../types";

// The mock keeps its state at module level, so every test gets a fresh copy.
let api: Backend;

beforeEach(async () => {
  vi.resetModules();
  api = (await import("./mock")).backend;
});

const query = (over: Partial<ChannelQuery> = {}): ChannelQuery => ({
  sourceId: "",
  group: "",
  search: "",
  view: "",
  sort: "",
  offset: 0,
  limit: 1000,
  ...over,
});

// Every method of the Backend interface (lib/types.ts). The compiler checks
// the mock has them; this list keeps the runtime check in step when one is added.
const methods: (keyof Backend)[] = [
  "listSources", "addSource", "updateSource", "refreshSource", "setSourceEnabled", "removeSource",
  "listGroups", "listChannels", "setFavorite", "setChannelHidden", "setGroupHidden", "listHiddenGroups",
  "play", "clearHistory", "logoColors", "getSettings", "updateSettings", "exportFavorites",
  "importFavorites", "pickPlaylistFile", "benchMode", "openLink", "diagnostics", "benchReport",
];

describe("mock backend", () => {
  it("identifies itself and implements every Backend method", () => {
    expect(api.name).toBe("mock");
    for (const m of methods) expect(api[m], m).toBeTypeOf("function");
  });

  it("rejects with an error code when a source would be added or removed", async () => {
    await expect(api.addSource({ kind: "m3u_url", name: "x", url: "http://x" })).rejects.toBe("demo_unavailable");
    await expect(api.removeSource("demo")).rejects.toBe("demo_unavailable");
  });

  it("returns copies, so callers cannot corrupt its state", async () => {
    const first = await api.listSources();
    first[0]!.name = "changed";
    expect((await api.listSources())[0]!.name).toBe("Demo");
  });
});

describe("listChannels", () => {
  it("pages with offset and limit, reporting the total", async () => {
    const all = await api.listChannels(query());
    const page = await api.listChannels(query({ offset: 10, limit: 5 }));
    expect(page.total).toBe(all.total);
    expect(page.items.map((c) => c.id)).toEqual(all.items.slice(10, 15).map((c) => c.id));
    expect((await api.listChannels(query({ offset: all.total, limit: 5 }))).items).toEqual([]);
  });

  it("searches names case-insensitively", async () => {
    const { items, total } = await api.listChannels(query({ search: "BBC" }));
    expect(total).toBe(items.length);
    expect(items.length).toBeGreaterThan(0);
    expect(items.every((c) => c.name.toLowerCase().includes("bbc"))).toBe(true);
  });

  it("filters by group", async () => {
    const { items } = await api.listChannels(query({ group: "Sports" }));
    expect(items.length).toBeGreaterThan(0);
    expect(items.every((c) => c.group === "Sports")).toBe(true);
  });

  it("sorts A-Z and Z-A, and keeps provider order without a sort", async () => {
    const names = async (sort: ChannelQuery["sort"]) =>
      (await api.listChannels(query({ sort }))).items.map((c) => c.name.toLowerCase());
    const az = await names("az");
    expect(az).toEqual([...az].sort((a, b) => a.localeCompare(b)));
    expect(await names("za")).toEqual([...az].reverse());
    expect(await names("")).not.toEqual(az);
  });

  it("lists only favourites in the favourites view", async () => {
    const { items } = await api.listChannels(query({ view: "favorites" }));
    expect(items.length).toBeGreaterThan(0);
    expect(items.every((c) => c.favorite)).toBe(true);
  });

  it("moves a channel in and out of the favourites view", async () => {
    const before = (await api.listChannels(query({ view: "favorites" }))).total;
    await api.setFavorite("demo-1", true);
    expect((await api.listChannels(query({ view: "favorites" }))).total).toBe(before + 1);
    await api.setFavorite("demo-1", false);
    expect((await api.listChannels(query({ view: "favorites" }))).total).toBe(before);
  });

  it("hides channels from the main view and lists them under hidden", async () => {
    const total = (await api.listChannels(query())).total;
    await api.setChannelHidden("demo-2", true);
    expect((await api.listChannels(query())).total).toBe(total - 1);
    expect((await api.listChannels(query({ view: "hidden" }))).items.map((c) => c.id)).toEqual(["demo-2"]);
  });

  it("hides whole groups, also from the group list", async () => {
    await api.setGroupHidden("Sports", true);
    expect((await api.listChannels(query({ group: "Sports" }))).total).toBe(0);
    expect((await api.listGroups("")).some((g) => g.name === "Sports")).toBe(false);
    expect(await api.listHiddenGroups()).toEqual(["Sports"]);
    await api.setGroupHidden("Sports", false);
    expect(await api.listHiddenGroups()).toEqual([]);
  });

  it("keeps history newest first without duplicates, and clears it", async () => {
    await api.play("demo-3");
    await api.play("demo-4");
    await api.play("demo-3");
    const ids = async () => (await api.listChannels(query({ view: "history" }))).items.map((c) => c.id);
    expect(await ids()).toEqual(["demo-3", "demo-4"]);
    await api.clearHistory();
    expect(await ids()).toEqual([]);
  });
});

describe("settings", () => {
  it("merges updates into the stored settings", async () => {
    const next = await api.updateSettings({ player: "vlc", volume: 40 });
    expect(next.player).toBe("vlc");
    expect((await api.getSettings()).volume).toBe(40);
  });
});
