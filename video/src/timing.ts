// Every number here was measured:
// - VO frame lengths are what scripts/gen-voice.mjs printed for public/vo.
// - Scene cuts sit on the bar grid of public/music/aco1.mp3 as printed by
//   scripts/analyze-music.mjs (157 BPM, first beat 0.325 s, 1 bar = 45.9 frames).
//   Bar 9 (frame 377, 12.6 s) is where the track reaches full energy: the logo
//   bursts there.

export const FPS = 30;
export const WIDTH = 1920;
export const HEIGHT = 1080;

/** Frame where bar `n` (1-based) starts. */
export const bar = (n: number) => Math.round(9.75 + (n - 1) * 45.93);

export const SCENES = {
  cold: { from: 0, to: bar(5) }, // 0–193
  wall: { from: bar(5), to: bar(8) }, // 193–331
  reveal: { from: bar(8), to: bar(12) }, // 331–515
  product: { from: bar(12), to: bar(26) }, // 515–1158
  close: { from: bar(26), to: bar(26) + 182 }, // 1158–1340
} as const;

export const TOTAL = SCENES.close.to;

/** The logo bursts on the drop. */
export const DROP = bar(9);

/** Voice-over placements: `frames` are the real durations of public/vo/vo-NN.wav. */
export const VO = [
  { id: "01", at: 45, frames: 60 },
  { id: "02", at: 225, frames: 52 },
  { id: "03", at: DROP + 18, frames: 30 },
  { id: "04", at: SCENES.product.from + 30, frames: 80 },
  { id: "05", at: SCENES.product.from + 222, frames: 39 },
  { id: "06", at: SCENES.product.from + 352, frames: 64 },
  { id: "07", at: SCENES.product.from + 520, frames: 79 },
  { id: "08", at: SCENES.close.from + 30, frames: 41 },
] as const;

/**
 * Subtitles only for lines whose words are not already on screen ("Meet
 * Herakleo" sits under the logo, "Free, and open source" is the closing
 * title). `dur` is reading time: in a bit before, out a bit after.
 */
export const SUBTITLES = [
  { at: 41, dur: 72, text: "Your playlist has thousands of channels." },
  { at: 221, dur: 66, text: "Finding one shouldn't feel like work." },
  { at: VO[3].at - 4, dur: 92, text: "Add an M3U link, or your Xtream account." },
  { at: VO[4].at - 4, dur: 56, text: "Search, and it's right there." },
  { at: VO[5].at - 4, dur: 76, text: "Press play. It opens in mpv." },
  { at: VO[6].at - 4, dur: 92, text: "Smooth, even with sixty-five thousand channels." },
] as const;
