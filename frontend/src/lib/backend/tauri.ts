import { invoke } from "@tauri-apps/api/core";
import type { Backend } from "../types";

// Struct payloads go as a single `req`/`patch`; scalar args use Tauri's
// camelCase to snake_case mapping (channelId -> channel_id on the Rust side).
export const backend: Backend = {
  name: "tauri",
  listSources: () => invoke("list_sources"),
  addSource: (req) => invoke("add_source", { req }),
  updateSource: (id, req) => invoke("update_source", { id, req }),
  refreshSource: (id) => invoke("refresh_source", { id }),
  setSourceEnabled: (id, enabled) => invoke("set_source_enabled", { id, enabled }),
  removeSource: (id) => invoke("remove_source", { id }),
  listGroups: (sourceId) => invoke("list_groups", { sourceId }),
  listChannels: (req) => invoke("list_channels", { req }),
  setFavorite: (channelId, favorite) => invoke("set_favorite", { channelId, favorite }),
  setChannelHidden: (channelId, hidden) => invoke("set_channel_hidden", { channelId, hidden }),
  setGroupHidden: (name, hidden) => invoke("set_group_hidden", { name, hidden }),
  listHiddenGroups: () => invoke("list_hidden_groups"),
  play: (channelId) => invoke("play", { channelId }),
  clearHistory: () => invoke("clear_history"),
  logoColors: (urls) => invoke("logo_colors", { urls }),
  getSettings: () => invoke("get_settings"),
  updateSettings: (patch) => invoke("update_settings", { patch }),
  exportFavorites: (title) => invoke("export_favorites", { title }),
  importFavorites: (title) => invoke("import_favorites", { title }),
  pickPlaylistFile: (title) => invoke("pick_playlist_file", { title }),
  benchMode: () => invoke("bench_mode"),
  diagnostics: () => invoke("diagnostics"),
  openLink: (id) => invoke("open_link", { id }),
  benchReport: (report) => invoke("bench_report", { report }),
};
