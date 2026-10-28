import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { watchSlowScroll } from "./slowscroll";

// A fake clock plus a manual requestAnimationFrame queue, so a scroll burst
// is just "N frames, `gap` ms apart".
let now = 0;
let queue: FrameRequestCallback[] = [];
const doc = { hidden: false };

beforeEach(() => {
  now = 1000;
  queue = [];
  doc.hidden = false;
  vi.stubGlobal("document", doc);
  vi.stubGlobal("performance", { now: () => now });
  vi.stubGlobal("requestAnimationFrame", (cb: FrameRequestCallback) => queue.push(cb));
});

afterEach(() => vi.unstubAllGlobals());

/** One scroll event followed by the animation frame it schedules. */
function step(scroll: () => void, gap: number) {
  now += gap;
  scroll();
  queue.shift()?.(now);
}

/** A burst of `frames` frames, then idle long enough for the burst to end. */
function burst(scroll: () => void, frames: number, gap: number) {
  for (let i = 0; i < frames; i++) step(scroll, gap);
  now += 500;
  queue.shift()?.(now);
}

describe("watchSlowScroll", () => {
  it("fires after 3 slow bursts", () => {
    const onSlow = vi.fn();
    const scroll = watchSlowScroll(onSlow);
    burst(scroll, 25, 40);
    burst(scroll, 25, 40);
    expect(onSlow).not.toHaveBeenCalled();
    burst(scroll, 25, 40);
    expect(onSlow).toHaveBeenCalledTimes(1);
  });

  it("does not fire for fast scrolling", () => {
    const onSlow = vi.fn();
    const scroll = watchSlowScroll(onSlow);
    for (let i = 0; i < 10; i++) burst(scroll, 30, 16);
    expect(onSlow).not.toHaveBeenCalled();
  });

  it("ignores bursts shorter than the minimum frame count", () => {
    const onSlow = vi.fn();
    const scroll = watchSlowScroll(onSlow);
    for (let i = 0; i < 10; i++) burst(scroll, 10, 80);
    expect(onSlow).not.toHaveBeenCalled();
  });

  it("only counts slow bursts among the last five", () => {
    const onSlow = vi.fn();
    const scroll = watchSlowScroll(onSlow);
    burst(scroll, 25, 40);
    burst(scroll, 25, 40);
    // Four fast bursts push the first slow one out of the window.
    for (let i = 0; i < 4; i++) burst(scroll, 25, 16);
    burst(scroll, 25, 40);
    expect(onSlow).not.toHaveBeenCalled();
  });

  it("judges a burst by its median frame, not by one slow frame", () => {
    const onSlow = vi.fn();
    const scroll = watchSlowScroll(onSlow);
    for (let i = 0; i < 5; i++) {
      for (let j = 0; j < 25; j++) step(scroll, j === 10 ? 300 : 16);
      now += 500;
      queue.shift()?.(now);
    }
    expect(onSlow).not.toHaveBeenCalled();
  });

  it("fires at most once and stops scheduling frames", () => {
    const onSlow = vi.fn();
    const scroll = watchSlowScroll(onSlow);
    for (let i = 0; i < 6; i++) burst(scroll, 25, 40);
    expect(onSlow).toHaveBeenCalledTimes(1);
    scroll();
    expect(queue).toHaveLength(0);
  });

  it("ends the burst when the document is hidden", () => {
    const onSlow = vi.fn();
    const scroll = watchSlowScroll(onSlow);
    for (let i = 0; i < 3; i++) {
      for (let j = 0; j < 25; j++) step(scroll, 40);
      doc.hidden = true;
      step(scroll, 40);
      doc.hidden = false;
    }
    expect(onSlow).toHaveBeenCalledTimes(1);
  });
});
