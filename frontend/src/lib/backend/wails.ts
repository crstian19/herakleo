import { Call } from "@wailsio/runtime";
import type { Backend } from "../types";

// Wails v3 calls Go service methods by name ("<package>.<Service>.<Method>")
// through its runtime. Calling by name keeps the shared frontend free of
// generated bindings. Go methods that take a context get it from Wails, so
// it is not passed here.
const call = (method: string, ...args: unknown[]): Promise<any> => Call.ByName(`main.App.${method}`, ...args);

// Same shape as v2's window.go.main.App, so the method list below stays readable.
const app = () =>
  new Proxy({} as Record<string, (...args: unknown[]) => Promise<any>>, {
    get: (_, method: string) => (...args: unknown[]) => call(method, ...args),
  });

export const backend: Backend = {
  name: "wails",
  listSources: () => app().ListSources(),
  addSource: (req) => app().AddSource(req),
  updateSource: (id, req) => app().UpdateSource(id, req),
  refreshSource: (id) => app().RefreshSource(id),
  setSourceEnabled: (id, enabled) => app().SetSourceEnabled(id, enabled),
  removeSource: (id) => app().RemoveSource(id),
  listGroups: (sourceId, media) => app().ListGroups(sourceId, media),
  listChannels: (req) => app().ListChannels(req),
  setFavorite: (channelId, favorite) => app().SetFavorite(channelId, favorite),
  setChannelHidden: (channelId, hidden) => app().SetChannelHidden(channelId, hidden),
  setGroupHidden: (name, hidden) => app().SetGroupHidden(name, hidden),
  listHiddenGroups: () => app().ListHiddenGroups(),
  play: (channelId) => app().Play(channelId),
  clearHistory: () => app().ClearHistory(),
  refreshGuide: (id) => app().RefreshGuide(id),
  setGuideUrl: (id, url) => app().SetGuideURL(id, url),
  nowNext: (ids) => app().NowNext(ids),
  // Go sends nil slices as null.
  channelGuide: async (id) => (await app().ChannelGuide(id)) ?? [],
  setReminder: async (r, on) => (await app().SetReminder(r, on)) ?? [],
  listReminders: async () => (await app().ListReminders()) ?? [],
  seriesEpisodes: async (id) => (await app().SeriesEpisodes(id)) ?? [],
  playEpisode: (id, episodeId) => app().PlayEpisode(id, episodeId),
  logoColors: (urls) => app().LogoColors(urls),
  getSettings: () => app().GetSettings(),
  updateSettings: (patch) => app().UpdateSettings(patch),
  // Go has no null: "" and -1 mean the dialog was cancelled.
  exportFavorites: async (title) => (await app().ExportFavorites(title)) || null,
  importFavorites: async (title) => {
    const n: number = await app().ImportFavorites(title);
    return n < 0 ? null : n;
  },
  pickPlaylistFile: async (title) => (await app().PickPlaylistFile(title)) || null,
  benchMode: () => app().BenchMode(),
  diagnostics: () => app().Diagnostics(),
  openLink: (id) => app().OpenLink(id),
  benchReport: (report) => app().BenchReport(report),
};
