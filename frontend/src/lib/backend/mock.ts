import type { AppSettings, Backend, Channel, Episode, MediaType, Programme, Reminder, SourceInfo } from "../types";

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
    guideUrl: "",
    hasGuide: true,
    guideUpdatedAt: now - 3600,
    guideChannels: count - Math.floor(count / 5),
    movieCount: 24,
    seriesCount: 8,
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
    guideUrl: "",
    hasGuide: true,
    guideUpdatedAt: 0,
    guideChannels: 0,
    movieCount: 0,
    seriesCount: 0,
  },
];

const films = ["Viaje a la Luna", "El último tren", "Mar adentro", "La isla", "Noche en Madrid", "El faro", "Tierra roja", "Cuarto oscuro"];
const shows2 = ["Los vecinos", "Distrito 9", "La oficina", "Agentes", "Hospital central", "El internado", "Mareas", "Sombras"];
const vod = (media: MediaType, i: number, name: string, group: string): Channel => ({
  id: `${media}-${i}`,
  sourceId: "demo",
  name,
  group,
  logo: "",
  favorite: false,
  hidden: false,
  media,
  container: media === "series",
});
const channels: Channel[] = [
  ...Array.from({ length: count }, (_, i) => ({
    id: `demo-${i}`,
    sourceId: "demo",
    name: channelName(i),
    group: groups[i % groups.length]!,
    logo: "",
    favorite: i % 7 === 0,
    hidden: false,
    media: "live" as const,
  })),
  ...Array.from({ length: 24 }, (_, i) => vod("movie", i, `${films[i % films.length]}${i >= films.length ? ` ${Math.floor(i / films.length) + 1}` : ""}`, ["Estrenos", "Drama", "Comedia"][i % 3]!)),
  ...Array.from({ length: 8 }, (_, i) => vod("series", i, shows2[i]!, ["Drama", "Comedia"][i % 2]!)),
];
const episodesOf = (id: string): Episode[] =>
  Array.from({ length: 3 }, (_, s) =>
    Array.from({ length: 6 }, (_, e) => ({ id: `${id}-${s + 1}-${e + 1}`, season: s + 1, number: e + 1, title: `Capítulo ${e + 1}`, plot: e % 2 ? "" : "Episodio de prueba del modo demo." })),
  ).flat();
const hiddenGroups = new Set<string>();
let reminders: Reminder[] = [];

// A fake schedule: blocks of 30 to 90 minutes from the last full hour, the
// same for a channel every time. One channel in five has no guide.
const shows = ["Telediario", "El tiempo", "Cine de barrio", "Documental", "Fútbol: previa", "Concierto", "Noticias 24h", "Serie: capítulo 3", "Magazine", "Dibujos"];
function guideFor(id: string): Programme[] {
  const i = Number(id.split("-")[1]);
  if (!Number.isInteger(i) || i % 5 === 4) return [];
  const out: Programme[] = [];
  let start = Math.floor(Date.now() / 3_600_000) * 3600 - 3600;
  for (let k = 0; out.length < 30; k++) {
    const stop = start + 1800 * (1 + ((i + k) % 3));
    if (stop > Date.now() / 1000) out.push({ start, stop, title: shows[(i + k) % shows.length]!, desc: k % 2 ? "" : "Programa de prueba del modo demo." });
    start = stop;
  }
  return out;
}
let history: string[] = [];

const settings: AppSettings = {
  player: "mpv",
  refreshOnStart: false,
  softwareDecoding: false,
  cacheSecs: 0,
  volume: 0,
  cacheChoices: [0, 10, 30, 60, 120],
  dataDir: "~/.config/herakleo",
  version: "0.0.0-mock",
  telemetry: "on",
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
  listGroups: (_, media) => {
    const ofMedia = channels.filter((c) => visible(c) && (!media || c.media === media));
    const names = [...new Set(ofMedia.map((c) => c.group))];
    return delay(names.map((name) => ({ name, count: ofMedia.filter((c) => c.group === name).length })));
  },
  listChannels: (q) => {
    const needle = q.search.toLowerCase();
    let hits =
      q.view === "history"
        ? history.map((id) => channels.find((c) => c.id === id)!).filter(visible)
        : channels.filter((c) =>
            q.view === "hidden" ? c.hidden : visible(c) && (q.view !== "favorites" || c.favorite),
          );
    const anyMedia = !q.media || q.view === "history" || q.view === "hidden";
    hits = hits.filter(
      (c) => (anyMedia || c.media === q.media) && (!q.group || c.group === q.group) && (!needle || c.name.toLowerCase().includes(needle)),
    );
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
    if (channels.find((c) => c.id === id)?.container) return Promise.reject("not_playable");
    history = [id, ...history.filter((x) => x !== id)].slice(0, 50);
    return delay(undefined);
  },
  clearHistory: () => {
    history = [];
    return delay(undefined);
  },
  logoColors: () => delay({}),
  refreshGuide: (id) => {
    const s = sources.find((x) => x.id === id)!;
    s.guideUpdatedAt = Math.floor(Date.now() / 1000);
    return delay(s);
  },
  setGuideUrl: (id, url) => {
    const s = sources.find((x) => x.id === id)!;
    s.guideUrl = url.split("?")[0]!;
    return delay(s);
  },
  nowNext: (ids) => {
    const t = Date.now() / 1000;
    const out: Record<string, { now?: Programme; next?: Programme }> = {};
    for (const id of ids.slice(0, 300)) {
      const g = guideFor(id);
      if (!g.length) continue;
      const on = g[0]!.start <= t;
      out[id] = { now: on ? g[0] : undefined, next: on ? g[1] : g[0] };
    }
    return delay(out);
  },
  channelGuide: (id) => delay(guideFor(id)),
  setReminder: (r, on) => {
    reminders = reminders.filter((x) => !(x.channelId === r.channelId && x.start === r.start));
    if (on) reminders = [...reminders, r].sort((a, b) => a.start - b.start);
    return delay(reminders);
  },
  listReminders: () => delay(reminders),
  seriesEpisodes: (id) => (channels.find((c) => c.id === id)?.container ? delay(episodesOf(id)) : Promise.reject("not_a_series")),
  playEpisode: (id) => {
    history = [id, ...history.filter((x) => x !== id)].slice(0, 50);
    return delay(undefined);
  },
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
