// Contract between the UI and its backends: the Go app (wails) and fake data (mock).
// Field names are camelCase on the wire in both implementations.

export type SourceKind = "m3u_url" | "m3u_file" | "xtream";

export interface SourceInfo {
  id: string;
  name: string;
  kind: SourceKind;
  /** URL, file path or Xtream server; never includes credentials. */
  location: string;
  channelCount: number;
  /** Unix seconds of the last successful refresh. */
  updatedAt: number;
  /** Disabled lists keep their cache but are left out of channels and groups. */
  enabled: boolean;
  /** Xtream only: unix seconds when the subscription ends; 0 when unknown or unlimited. */
  expiresAt: number;
  /** Xtream only: status as the panel words it ("Active", "Expired"…); "" when unknown. */
  accountStatus: string;
  /** The XMLTV guide URL the user set, without query or credentials; "" when none. */
  guideUrl: string;
  /** A guide can be fetched: the user's URL, the Xtream panel or the list's header. */
  hasGuide: boolean;
  /** Unix seconds of the last guide download; 0 when never. */
  guideUpdatedAt: number;
  /** Channels the last guide matched. */
  guideChannels: number;
}

/** One guide entry; times are unix seconds. */
export interface Programme {
  start: number;
  stop: number;
  title: string;
  desc?: string;
}

export interface NowNext {
  now?: Programme;
  next?: Programme;
}

/** Announced by the UI when the programme starts. */
export interface Reminder {
  channelId: string;
  /** Channel name, so the reminder reads well even if the list changes. */
  channel: string;
  title: string;
  start: number;
  stop: number;
}

export interface Channel {
  id: string;
  sourceId: string;
  name: string;
  group: string;
  logo: string;
  favorite: boolean;
  hidden: boolean;
}

export interface Group {
  name: string;
  count: number;
}

/** "" = everything visible; "history" is newest first and ignores sort; "hidden" lists hidden channels. */
export type ChannelView = "" | "favorites" | "history" | "hidden";
/** "" = provider order. */
export type ChannelSort = "" | "az" | "za";

export interface ChannelQuery {
  /** Empty string means all sources. */
  sourceId: string;
  /** Empty string means all groups. */
  group: string;
  search: string;
  view: ChannelView;
  sort: ChannelSort;
  offset: number;
  limit: number;
}

export interface ChannelPage {
  items: Channel[];
  total: number;
}

/** guideUrl is an optional XMLTV URL, only read when adding (setGuideUrl changes it). */
export type NewSource = (
  | { kind: "m3u_url"; name: string; url: string }
  | { kind: "m3u_file"; name: string; path: string }
  | { kind: "xtream"; name: string; server: string; username: string; password: string }
) & { guideUrl?: string };

export interface PlayerInfo {
  id: string;
  name: string;
  /** Found on PATH on this machine. */
  installed: boolean;
  /** Playback options this player honours (keys of PlaybackOptions). */
  options: (keyof PlaybackOptions)[];
}

/** Closed set of playback tweaks; 0 means "player default". */
export interface PlaybackOptions {
  softwareDecoding: boolean;
  /** One of AppSettings.cacheChoices. */
  cacheSecs: number;
  /** 1-100. */
  volume: number;
}

/** Preferences stored by the backend, shared by both builds. */
export interface AppSettings extends PlaybackOptions {
  player: string;
  players: PlayerInfo[];
  refreshOnStart: boolean;
  cacheChoices: number[];
  dataDir: string;
  version: string;
}

export interface SettingsPatch extends Partial<PlaybackOptions> {
  /** Only IDs from AppSettings.players are accepted. */
  player?: string;
  refreshOnStart?: boolean;
}

export interface Backend {
  /** Which implementation is running, shown in the UI for the comparison. */
  readonly name: "wails" | "mock";
  listSources(): Promise<SourceInfo[]>;
  addSource(source: NewSource): Promise<SourceInfo>;
  /** Empty fields keep their current value; changing location or credentials re-downloads. */
  updateSource(id: string, source: NewSource): Promise<SourceInfo>;
  refreshSource(id: string): Promise<SourceInfo>;
  setSourceEnabled(id: string, enabled: boolean): Promise<SourceInfo>;
  removeSource(id: string): Promise<void>;
  listGroups(sourceId: string): Promise<Group[]>;
  listChannels(query: ChannelQuery): Promise<ChannelPage>;
  setFavorite(channelId: string, favorite: boolean): Promise<void>;
  setChannelHidden(channelId: string, hidden: boolean): Promise<void>;
  /** Groups are hidden by name, across all lists. */
  setGroupHidden(name: string, hidden: boolean): Promise<void>;
  listHiddenGroups(): Promise<string[]>;
  /** Opens the channel in the chosen player and records it in the history. */
  play(channelId: string): Promise<void>;
  clearHistory(): Promise<void>;
  /** Downloads the list's guide and matches it to its channels. */
  refreshGuide(sourceId: string): Promise<SourceInfo>;
  /** "" goes back to the panel's or the list's own guide. */
  setGuideUrl(sourceId: string, url: string): Promise<SourceInfo>;
  /** What is on now and next, for up to 300 channels; channels without a guide are left out. */
  nowNext(channelIds: string[]): Promise<Record<string, NowNext>>;
  /** The channel's programmes from the one on air onwards. */
  channelGuide(channelId: string): Promise<Programme[]>;
  /** Adds (on) or removes a reminder; resolves to the upcoming reminders. */
  setReminder(reminder: Reminder, on: boolean): Promise<Reminder[]>;
  listReminders(): Promise<Reminder[]>;
  /** Accent colour per logo URL ("#rrggbb", or "" when unusable). Max 64 per call. */
  logoColors(urls: string[]): Promise<Record<string, string>>;
  getSettings(): Promise<AppSettings>;
  updateSettings(patch: SettingsPatch): Promise<AppSettings>;
  // Native dialogs take their title from the UI so it follows the UI language.
  /** Save dialog; resolves to the saved path, or null when cancelled. */
  exportFavorites(title: string): Promise<string | null>;
  /** Open dialog; resolves to how many favourites matched, or null when cancelled. */
  importFavorites(title: string): Promise<number | null>;
  /** File picker for .m3u files; null when cancelled. */
  pickPlaylistFile(title: string): Promise<string | null>;
  /** Opens a project link (repo or donation page) in the system browser; IDs only, never URLs. */
  openLink(id: "github" | "sponsors" | "kofi" | "paypal"): Promise<void>;
  /** Machine/renderer facts for the in-app performance report (no secrets). */
  diagnostics(): Promise<Record<string, string>>;
  /** "" normally; "full" or "wheel[:variant]" when started by just bench / scripts/profile-scroll.sh. */
  benchMode(): Promise<string>;
  /** Writes the JSON report to HERAKLEO_BENCH_OUT and quits. Only works in bench mode. */
  benchReport(report: string): Promise<void>;
}
