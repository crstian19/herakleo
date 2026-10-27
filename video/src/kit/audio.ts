import { interpolate } from 'remotion';

export type VoLine = { at: number; frames: number };
export type Hush = { at: number; frames: number; level: number };

export type DuckConfig = {
  /** Music level when there is no voice. */
  base: number;
  /** Music level under the voice. */
  under: number;
  /** Ramp length in frames when ducking starts and ends. */
  ramp: number;
  /** Padding before and after each line, in frames. */
  pad: number;
};

export const DEFAULT_DUCK: DuckConfig = { base: 0.85, under: 0.24, ramp: 9, pad: 6 };

/**
 * Music volume curve: drops under each voice line and comes back up as soon as
 * the line ends.
 *
 *   <Audio src={music} volume={(f) => duckVolume(f, VO, DEFAULT_DUCK, {
 *     musicAt: 0, total: DURATION_IN_FRAMES, hush: HUSH,
 *   })} />
 *
 * Two things learned by measuring, not by ear:
 *
 *  - `base` above ~0.85 clips. Library tracks are mastered to 0 dB, so any
 *    effect on top hits the ceiling.
 *  - Whether the ducking is enough has an objective test: voice over music
 *    should measure about the same as the voice alone. If it measures higher,
 *    the music is still adding energy and buries the narration.
 *
 *      ffmpeg -hide_banner -ss 27 -t 4 -i out.mp4 -af volumedetect -f null -
 *
 * `hush` is for deliberate dips that don't come from the voice, such as a
 * silence before a key line. The music backing off and returning with the line
 * is what gives the line its weight.
 */
export const duckVolume = (
  frame: number,
  vo: readonly VoLine[],
  cfg: DuckConfig = DEFAULT_DUCK,
  opts: { musicAt?: number; total: number; hush?: readonly Hush[]; fadeOut?: number } = { total: 0 }
) => {
  const { musicAt = 0, total, hush = [], fadeOut = 45 } = opts;

  const env = interpolate(frame, [musicAt, musicAt + 10, total - fadeOut, total - 2], [0, 1, 1, 0], {
    extrapolateLeft: 'clamp',
    extrapolateRight: 'clamp',
  });

  let level = cfg.base;

  for (const line of vo) {
    const a = line.at - cfg.pad;
    const b = line.at + line.frames + cfg.pad;
    level = Math.min(
      level,
      interpolate(frame, [a - cfg.ramp, a, b, b + cfg.ramp], [cfg.base, cfg.under, cfg.under, cfg.base], {
        extrapolateLeft: 'clamp',
        extrapolateRight: 'clamp',
      })
    );
  }

  for (const h of hush) {
    level = Math.min(
      level,
      interpolate(frame, [h.at - 14, h.at, h.at + h.frames, h.at + h.frames + 8], [cfg.base, h.level, h.level, cfg.base], {
        extrapolateLeft: 'clamp',
        extrapolateRight: 'clamp',
      })
    );
  }

  return level * env;
};
