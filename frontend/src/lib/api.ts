import { hasKey, t } from "./i18n.svelte";

// $backend is a Vite alias resolved per build mode (see vite.config.ts).
export { backend as api } from "$backend";
export type * from "./types";

/**
 * Turns a backend rejection into text in the UI language. Both backends
 * reject with a string: either an error code ("source_not_found") optionally
 * followed by technical detail ("download_failed: connection refused"), or a
 * raw message, shown as-is.
 */
export function errorMessage(err: unknown): string {
  const raw = typeof err === "string" ? err : err instanceof Error ? err.message : String(err);
  const m = /^([a-z_]+)(?::\s*([\s\S]*))?$/.exec(raw.trim());
  const key = m && `err.${m[1]}`;
  return key && hasKey(key) ? t(key, { detail: m[2] ?? "" }) : raw;
}

/** Stable accent colour per channel, since logo pixels are CORS-tainted. */
export function accentFor(name: string): string {
  let h = 0;
  for (let i = 0; i < name.length; i++) h = (h * 31 + name.charCodeAt(i)) >>> 0;
  return `hsl(${h % 360} 70% 60%)`;
}
