// Turns the visual effects off once when wheel scrolling is clearly slow on
// this machine, so people on weak GPUs get the light look without finding the
// switch. Frames are timed only while the grid is scrolling, the same way the
// Settings → Performance meter does (bench.ts wheelFrames).

/** Median frame time above this counts as a slow burst (≈ under 40 fps). */
const SLOW_MS = 25;
/** Shorter bursts are too noisy to judge. */
const MIN_FRAMES = 20;
/** Effects go off when this many of the last WINDOW bursts were slow. */
const SLOW_BURSTS = 3;
const WINDOW = 5;
/** A burst ends once the grid has not scrolled for this long. */
const IDLE_MS = 150;

/** Returns the function to call on every scroll event. */
export function watchSlowScroll(onSlow: () => void): () => void {
  let gaps: number[] = [];
  let last = 0;
  let lastScroll = 0;
  let running = false;
  let done = false;
  const recent: boolean[] = [];

  function endBurst() {
    if (gaps.length >= MIN_FRAMES) {
      gaps.sort((a, b) => a - b);
      recent.push(gaps[Math.floor(gaps.length / 2)]! > SLOW_MS);
      if (recent.length > WINDOW) recent.shift();
      if (recent.filter(Boolean).length >= SLOW_BURSTS) {
        done = true;
        onSlow();
      }
    }
    gaps = [];
    last = 0;
  }

  function frame(now: number) {
    if (now - lastScroll > IDLE_MS || document.hidden) {
      running = false;
      endBurst();
      return;
    }
    // The first frame of a burst follows an idle gap, so it is not timed.
    if (last) gaps.push(now - last);
    last = now;
    requestAnimationFrame(frame);
  }

  return () => {
    if (done) return;
    lastScroll = performance.now();
    if (!running) {
      running = true;
      requestAnimationFrame(frame);
    }
  };
}
