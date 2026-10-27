import { Easing, interpolate } from 'remotion';

/**
 * Virtual camera over a UI rebuilt in HTML.
 *
 * You lay out the interface in a fixed design space (say 1920x1080) and then
 * move a "camera" that centres a point (fx, fy) at scale s. That gives you
 * close-ups of one area without touching the layout.
 *
 * The motion style took three rounds of review against the rendered output:
 *
 *  - CONTINUOUS pans and zooms inside a scene read as handheld footage and pull
 *    attention away from what the interface is doing.
 *  - HARD cuts between framings read as jumps, especially across scenes.
 *
 * `moves()` is what works: the camera holds still most of the time and only
 * travels for a few frames right before the next framing. One rule matters more
 * than the helper itself: every scene must START on the framing where the
 * previous one ended. That is what keeps a scene change from reading as a cut.
 */
export type Cam = { f: number[]; fx: number[]; fy: number[]; s: number[] };

export type Framing = { at: number; fx: number; fy: number; s: number };

/** A single framing with no movement. */
export const fixed = (fx: number, fy: number, s: number): Cam => ({
  f: [0, 1],
  fx: [fx, fx],
  fy: [fy, fy],
  s: [s, s],
});

/**
 * Framings joined by a short move. Each one holds still, and the camera travels
 * for `ease` frames before reaching the next.
 */
export const moves = (list: readonly Framing[], ease = 24): Cam => {
  const f: number[] = [];
  const fx: number[] = [];
  const fy: number[] = [];
  const s: number[] = [];
  list.forEach((k, i) => {
    if (i > 0) {
      const prev = list[i - 1];
      f.push(Math.max(prev.at + 1, k.at - ease));
      fx.push(prev.fx);
      fy.push(prev.fy);
      s.push(prev.s);
    }
    f.push(k.at);
    fx.push(k.fx);
    fy.push(k.fy);
    s.push(k.s);
  });
  return { f, fx, fy, s };
};

/** Framings joined by hard cuts. Use only when the cut is intended. */
export const cuts = (list: readonly Framing[]): Cam => {
  const f: number[] = [];
  const fx: number[] = [];
  const fy: number[] = [];
  const s: number[] = [];
  list.forEach((k, i) => {
    const next = list[i + 1];
    f.push(k.at);
    fx.push(k.fx);
    fy.push(k.fy);
    s.push(k.s);
    if (next) {
      f.push(next.at - 1);
      fx.push(k.fx);
      fy.push(k.fy);
      s.push(k.s);
    }
  });
  return { f, fx, fy, s };
};

/**
 * Returns the transform to apply to the design-space container. The focus point
 * is clamped so the canvas never shows empty space outside the design; without
 * this, a close-up near an edge leaves half the frame black.
 *
 *   const cam = useCamera(moves([...]), frame, { width: 1920, height: 1080 });
 *   <div style={{ width: 1920, height: 1080, transform: cam.transform,
 *                 transformOrigin: '0 0' }}>
 */
export const useCamera = (
  cam: Cam,
  frame: number,
  design: { width: number; height: number }
) => {
  const o = {
    extrapolateLeft: 'clamp',
    extrapolateRight: 'clamp',
    easing: Easing.inOut(Easing.cubic),
  } as const;

  const s = interpolate(frame, cam.f, cam.s, o);
  const hw = design.width / 2 / s;
  const hh = design.height / 2 / s;
  const clamp = (v: number, lo: number, hi: number) =>
    lo > hi ? (lo + hi) / 2 : Math.min(Math.max(v, lo), hi);

  const x = clamp(interpolate(frame, cam.f, cam.fx, o), hw, design.width - hw);
  const y = clamp(interpolate(frame, cam.f, cam.fy, o), hh, design.height - hh);

  return {
    s,
    transform: `translate(${design.width / 2 - x * s}px, ${design.height / 2 - y * s}px) scale(${s})`,
  };
};

/**
 * Scales the design space to the real canvas. The cursor and anything else that
 * lives inside the UI space must compensate for `cam.s` so it doesn't grow with
 * the zoom: `transform: scale(1 / camScale)`.
 */
export const fitScale = (
  canvas: { width: number; height: number },
  design: { width: number; height: number }
) => Math.min(canvas.width / design.width, canvas.height / design.height);
