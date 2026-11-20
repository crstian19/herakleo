import type { PlayerExit } from "./types";

/** The player error as plain text, for the alert's Copy button. */
export function exitText(e: PlayerExit): string {
  return [`${e.title} · ${e.player} · exit ${e.code} · ${e.seconds}s`, ...(e.log ?? [])].join("\n");
}
