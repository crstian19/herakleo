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
  media: "live",
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
  "refreshGuide", "setGuideUrl", "nowNext", "channelGuide", "setReminder", "listReminders",
  "seriesEpisodes", "playEpisode",
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
    expect((await api.listGroups("", "live")).some((g) => g.name === "Sports")).toBe(false);
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

describe("guide", () => {
  it("answers now/next only for channels with a schedule, capped at 300", async () => {
    const ids = Array.from({ length: 400 }, (_, i) => `demo-${i}`);
    const got = await api.nowNext(ids);
    const now = Date.now() / 1000;
    expect(Object.keys(got).length).toBeLessThanOrEqual(300);
    expect(got["demo-4"]).toBeUndefined(); // one channel in five has no guide
    const nn = got["demo-0"]!;
    expect(nn.now!.start).toBeLessThanOrEqual(now);
    expect(nn.now!.stop).toBeGreaterThan(now);
    expect(nn.next!.start).toBe(nn.now!.stop);
  });

  it("lists a channel's programmes in order, from the one on air", async () => {
    const g = await api.channelGuide("demo-1");
    expect(g.length).toBeGreaterThan(1);
    expect(g[0]!.stop).toBeGreaterThan(Date.now() / 1000);
    g.slice(1).forEach((p, i) => expect(p.start).toBe(g[i]!.stop));
    expect(await api.channelGuide("demo-4")).toEqual([]);
  });

  it("adds and removes reminders without duplicates", async () => {
    const r = { channelId: "demo-0", channel: "La 1", title: "Cine", start: 2000, stop: 3000 };
    expect(await api.setReminder(r, true)).toHaveLength(1);
    expect(await api.setReminder({ ...r, start: 1000 }, true)).toHaveLength(2);
    expect((await api.setReminder(r, true)).map((x) => x.start)).toEqual([1000, 2000]);
    expect(await api.setReminder(r, false)).toHaveLength(1);
    expect(await api.listReminders()).toHaveLength(1);
  });

  it("drops the query from a guide URL it shows", async () => {
    const s = await api.setGuideUrl("demo", "https://epg.example/guide.xml?token=secret");
    expect(s.guideUrl).toBe("https://epg.example/guide.xml");
  });
});

describe("films and series", () => {
  it("filters by media type, and every type when media is empty", async () => {
    const live = await api.listChannels(query());
    const movies = await api.listChannels(query({ media: "movie" }));
    const all = await api.listChannels(query({ media: "" }));
    expect(movies.total).toBeGreaterThan(0);
    expect(movies.items.every((c) => c.media === "movie")).toBe(true);
    expect(all.total).toBe(live.total + movies.total + (await api.listChannels(query({ media: "series" }))).total);
    const groups = await api.listGroups("", "movie");
    expect(groups.reduce((n, g) => n + g.count, 0)).toBe(movies.total);
  });

  it("lists a series' episodes and plays them, but never the series itself", async () => {
    const show = (await api.listChannels(query({ media: "series" }))).items[0]!;
    expect(show.container).toBe(true);
    await expect(api.play(show.id)).rejects.toBe("not_playable");
    const eps = await api.seriesEpisodes(show.id);
    expect(eps.length).toBeGreaterThan(1);
    await api.playEpisode(show.id, eps[0]!.id);
    expect((await api.listChannels(query({ view: "history" }))).items[0]!.id).toBe(show.id);
    await expect(api.seriesEpisodes("demo-0")).rejects.toBe("not_a_series");
  });
});
