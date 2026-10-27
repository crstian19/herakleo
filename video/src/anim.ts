import { Easing, interpolate } from "remotion";

const smooth = Easing.inOut(Easing.cubic);

/** Clamped, eased interpolation: every animation in the video goes through here. */
export const ease = (f: number, input: readonly number[], output: readonly number[], easing = smooth) =>
  interpolate(f, input as number[], output as number[], { extrapolateLeft: "clamp", extrapolateRight: "clamp", easing });

export const fade = (f: number, a: number, b: number) => ease(f, [a, b], [0, 1]);

/** Characters of `text` typed between frames a and b. */
export const typed = (f: number, text: string, a: number, b: number) =>
  text.slice(0, Math.round(ease(f, [a, b], [0, text.length], Easing.linear)));

/** Blinking caret, 15 frames per cycle. */
export const blink = (f: number) => Math.floor(f / 15) % 2 === 0;

export type Key = { at: number; x: number; y: number };

/** Cursor position: rests on each key and glides to the next. */
export const path = (f: number, keys: readonly Key[]) => {
  const at = keys.map((k) => k.at);
  return { x: ease(f, at, keys.map((k) => k.x)), y: ease(f, at, keys.map((k) => k.y)) };
};

/** Click ripple progress 0→1 over 12 frames after any of `clicks`, 0 otherwise. */
export const clickAt = (f: number, clicks: readonly number[]) => {
  for (const c of clicks) if (f >= c && f < c + 12) return (f - c) / 12;
  return 0;
};
