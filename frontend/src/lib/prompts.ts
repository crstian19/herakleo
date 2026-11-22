// Which "help the project" popup a play opens, if any. Pure so the rules can
// be tested; App.svelte keeps the counters in localStorage.
//
// - support (donations): never before SUPPORT_AFTER plays; on that play if
//   it was never shown, after that by chance (SUPPORT_ODDS per play).
// - star (GitHub star): once, from STAR_AFTER plays on, and never again once
//   closed. It waits for a play that does not open the support popup, so the
//   two never stack.

export const SUPPORT_AFTER = 10;
export const SUPPORT_ODDS = 1 / 200;
export const STAR_AFTER = 30;

export type Prompt = "support" | "star" | null;

export interface PromptState {
  plays: number;
  supportSeen: boolean;
  starSeen: boolean;
}

export function promptFor(s: PromptState, random: () => number = Math.random): Prompt {
  if (s.plays < SUPPORT_AFTER) return null;
  if (!s.supportSeen || random() < SUPPORT_ODDS) return "support";
  if (s.plays >= STAR_AFTER && !s.starSeen) return "star";
  return null;
}
