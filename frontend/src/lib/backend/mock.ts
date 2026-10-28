import type { AppSettings, Backend, Channel, SourceInfo } from "../types";

// In-memory backend for working on the UI in a plain browser (`bun run dev:mock`).
// Enough groups to exercise the sidebar filter; real lists often have hundreds.
const groups = [
  "News", "Sports", "Movies", "Kids", "Music", "Documentaries",
  "ES | Noticias", "ES | Deportes", "ES | Cine", "ES | Series", "ES | Infantil", "ES | Música",
  "UK | News", "UK | Sport", "UK | Entertainment", "FR | Actualités", "FR | Sport", "FR | Cinéma",
  "DE | Nachrichten", "DE | Sport", "DE | Filme", "IT | Notizie", "IT | Sport", "IT | Cinema",
  "PT | Notícias", "PT | Desporto", "LaLiga", "Premier League", "Champions League", "Formula 1",
  "Tennis", "Golf", "Nature", "History", "Cooking", "Travel", "Religion", "Shopping", "Radio", "4K UHD",
];
const names = [
  "La 1", "La 2", "Antena 3", "Cuatro", "Telecinco", "laSexta", "24h", "Teledeporte",
  "Clan", "Boing", "Paramount", "Cosmo", "AXN", "TNT", "Calle 13", "Discovery",
  "National Geographic", "Historia", "Odisea", "MTV", "Sol Música", "Euronews",
  "BBC World", "CNN", "France 24", "DW", "Eurosport", "Real Madrid TV", "Barça TV",
];

const now = Math.floor(Date.now() / 1000);
// `?channels=5000` in the URL fakes a big list (e.g. to exercise the virtualised grid).
const count = Number(new URLSearchParams(globalThis.location?.search ?? "").get("channels")) || groups.length * 3;
const channelName = (i: number) => (i < names.length ? names[i]! : `${names[i % names.length]} ${Math.floor(i / names.length) + 1}`);

const sources: SourceInfo[] = [
  {
    id: "demo",
    name: "Demo",
    kind: "m3u_url",
    location: "https://example.com/demo.m3u",
    channelCount: count,
    updatedAt: now,
    enabled: true,
    expiresAt: 0,
    accountStatus: "",
  },
  {
    id: "xtream",
    name: "Xtream provider",
    kind: "xtream",
    location: "http://panel.example:8080",
    channelCount: 0,
    updatedAt: now - 86400,
    enabled: false,
    expiresAt: now + 20 * 86400,
    accountStatus: "Active",
  },
];

const channels: Channel[] = Array.from({ length: count }, (_, i) => ({
  id: `demo-${i}`,
  sourceId: "demo",
  name: channelName(i),
  group: groups[i % groups.length]!,
  logo: "",
  favorite: i % 7 === 0,
  hidden: false,
}));
const hiddenGroups = new Set<string>();
let history: string[] = [];

const settings: AppSettings = {
  player: "mpv",
  refreshOnStart: false,
  softwareDecoding: false,
  cacheSecs: 0,
  volume: 0,
  cacheChoices: [0, 10, 30, 60, 120],
  dataDir: "~/.config/herakleo",
  players: [
    { id: "mpv", name: "mpv", installed: true, options: ["softwareDecoding", "cacheSecs", "volume"] },
    { id: "vlc", name: "VLC", installed: true, options: ["softwareDecoding", "cacheSecs"] },
    { id: "celluloid", name: "Celluloid", installed: false, options: [] },
    { id: "haruna", name: "Haruna", installed: false, options: [] },
  ],
};

const delay = <T>(v: T) => new Promise<T>((r) => setTimeout(() => r(structuredClone(v)), 60));
const visible = (c: Channel) => !c.hidden && !hiddenGroups.has(c.group);

export const backend: Backend = {
  name: "mock",
  listSources: () => delay(sources),
  addSource: () => Promise.reject("demo_unavailable"),
  updateSource: (id, req) => {
    const s = sources.find((x) => x.id === id)!;
    if (req.name) s.name = req.name;
    return delay(s);
  },
  refreshSource: (id) => delay(sources.find((x) => x.id === id)!),
  setSourceEnabled: (id, enabled) => {
    const s = sources.find((x) => x.id === id)!;
    s.enabled = enabled;
    return delay(s);
  },
  removeSource: () => Promise.reject("demo_unavailable"),
  listGroups: () =>
    delay(
      groups
        .filter((g) => !hiddenGroups.has(g))
        .map((name) => ({ name, count: channels.filter((c) => c.group === name && visible(c)).length })),
    ),
  listChannels: (q) => {
    const needle = q.search.toLowerCase();
    let hits =
      q.view === "history"
        ? history.map((id) => channels.find((c) => c.id === id)!).filter(visible)
        : channels.filter((c) =>
            q.view === "hidden" ? c.hidden : visible(c) && (q.view !== "favorites" || c.favorite),
          );
    hits = hits.filter((c) => (!q.group || c.group === q.group) && (!needle || c.name.toLowerCase().includes(needle)));
    if (q.view !== "history" && q.sort) {
      hits = [...hits].sort((a, b) => a.name.toLowerCase().localeCompare(b.name.toLowerCase()));
      if (q.sort === "za") hits.reverse();
    }
    return delay({ items: hits.slice(q.offset, q.offset + q.limit), total: hits.length });
  },
  setFavorite: (id, fav) => {
    channels.find((x) => x.id === id)!.favorite = fav;
    return delay(undefined);
  },
  setChannelHidden: (id, hidden) => {
    channels.find((x) => x.id === id)!.hidden = hidden;
    return delay(undefined);
  },
  setGroupHidden: (name, hidden) => {
    if (hidden) hiddenGroups.add(name);
    else hiddenGroups.delete(name);
    return delay(undefined);
  },
  listHiddenGroups: () => delay([...hiddenGroups].sort()),
  play: (id) => {
    history = [id, ...history.filter((x) => x !== id)].slice(0, 50);
    return delay(undefined);
  },
  clearHistory: () => {
    history = [];
    return delay(undefined);
  },
  logoColors: () => delay({}),
  getSettings: () => delay(settings),
  updateSettings: (patch) => {
    Object.assign(settings, patch);
    return delay(settings);
  },
  exportFavorites: () => delay("/tmp/herakleo-favorites.json"),
  importFavorites: () => delay(0),
  pickPlaylistFile: () => delay(null),
  benchMode: () => delay(""),
  openLink: () => delay(undefined),
  diagnostics: () => delay({ webview: "browser (mock)", session: "", desktop: "", os: "" }),
  benchReport: () => delay(undefined),
};
