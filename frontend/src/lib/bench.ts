import type { Backend, ChannelQuery } from "./types";

// Times the backend as the UI sees it: IPC bridge + (de)serialisation +
// backend work. Same scenarios as the Go/Rust micro-benchmarks
// (bench_test.go / tests.rs), so the difference between the two is the cost
// of the bridge itself.

export interface BenchRow {
  name: string;
  median: number;
  p95: number;
}

export interface BenchResult {
  backend: string;
  channels: number;
  runs: number;
  rows: BenchRow[];
}

const page = (q: Partial<ChannelQuery>): ChannelQuery => ({
  sourceId: "",
  group: "",
  search: "",
  view: "",
  sort: "",
  offset: 0,
  limit: 120,
  ...q,
});

// WebKitGTK clamps performance.now() to whole milliseconds, too coarse for
// sub-millisecond calls, so each sample times a batch and divides.
const BATCH = 10;

export async function runBench(api: Backend, runs = 20, onprogress?: (name: string) => void): Promise<BenchResult> {
  const groups = await api.listGroups("");
  const group = groups[0]?.name ?? "";
  const first = await api.listChannels(page({}));
  const scenarios: [string, () => Promise<unknown>][] = [
    ["listSources", () => api.listSources()],
    ["page", () => api.listChannels(page({}))],
    ["page_2", () => api.listChannels(page({ offset: 120 }))],
    ["search_common", () => api.listChannels(page({ search: "es" }))],
    ["search_rare", () => api.listChannels(page({ search: "zzqx" }))],
    ["group", () => api.listChannels(page({ group }))],
    ["sort_az", () => api.listChannels(page({ sort: "az" }))],
    ["sort_az_2", () => api.listChannels(page({ sort: "az", offset: 120 }))],
    ["groups", () => api.listGroups("")],
  ];

  const rows: BenchRow[] = [];
  for (const [name, call] of scenarios) {
    onprogress?.(name);
    await call(); // warm-up
    const times: number[] = [];
    for (let i = 0; i < runs; i++) {
      const start = performance.now();
      for (let j = 0; j < BATCH; j++) await call();
      times.push((performance.now() - start) / BATCH);
    }
    times.sort((a, b) => a - b);
    rows.push({ name, median: times[Math.floor(times.length / 2)]!, p95: times[Math.floor(times.length * 0.95)]! });
  }
  return { backend: api.name, channels: first.total, runs, rows };
}

export interface ScrollResult {
  frames: number;
  /** Median and p95 time between frames, in ms (16.7 = 60 fps). */
  median: number;
  p95: number;
  /** Frames that took longer than 2 × 16.7 ms: visible stutter. */
  janky: number;
  cards: number;
}

/**
 * Scrolls `el` at a steady speed for `ms`, letting infinite scroll load
 * pages as it goes, and records the time between animation frames. This is
 * what "it feels sluggish" means: paint and layout cost in WebKitGTK.
 */
export function scrollBench(el: HTMLElement, ms = 4000, pxPerFrame = 40): Promise<ScrollResult> {
  return new Promise((resolve) => {
    const gaps: number[] = [];
    let last = performance.now();
    const end = last + ms;
    const step = (now: number) => {
      gaps.push(now - last);
      last = now;
      el.scrollTop += pxPerFrame;
      if (now < end) return requestAnimationFrame(step);
      gaps.sort((a, b) => a - b);
      resolve({
        frames: gaps.length,
        median: gaps[Math.floor(gaps.length / 2)]!,
        p95: gaps[Math.floor(gaps.length * 0.95)]!,
        janky: gaps.filter((g) => g > 33.4).length,
        cards: el.querySelectorAll(".card").length,
      });
    };
    requestAnimationFrame(step);
  });
}

/** Animation frames per second with nothing else going on: the refresh rate the webview runs at. */
export function idleFps(ms = 2000): Promise<number> {
  return new Promise((resolve) => {
    let n = 0;
    let start = 0;
    const step = (now: number) => {
      if (!start) start = now;
      else n++;
      if (now - start < ms) requestAnimationFrame(step);
      else resolve((n * 1000) / (now - start));
    };
    requestAnimationFrame(step);
  });
}

/** Bench-only switches that CSS cannot express (set before cards render). */
export const benchFlags = { syncDecode: false };

/**
 * Diagnostic variants for scrollBench: each strips one suspect from the card
 * paint so `just bench` shows what costs frames. Injected only in bench mode.
 */
export const scrollVariants: Record<string, string> = {
  baseline: "",
  // Hover suspects: with a real wheel the card under the cursor changes
  // every few frames, so its hover styles start and stop constantly.
  "no-hover":
    ".card:hover { transform: none !important; box-shadow: none !important; border-color: var(--color-line) !important; } .card:hover .spotlight, .card:hover .border-glow { opacity: 0 !important; } .card, .card * { transition: none !important; }",
  "no-transitions": ".card, .card * { transition: none !important; }",
  "no-shadow": ".card:hover { box-shadow: none !important; }",
  "no-logos": ".card-logo { display: none !important; }",
  "flat-cards":
    ".card { background: none !important; border-color: transparent !important; box-shadow: none !important; } .card .spotlight, .card .border-glow { display: none !important; }",
  "no-content-visibility": ".card { content-visibility: visible !important; }",
  "no-glow-layers": ".card .spotlight, .card .border-glow { display: none !important; }",
  "no-card-surface": ".card { background: none !important; border-color: transparent !important; }",
  "opaque-surface": ".card { background: #101016 !important; }",
  "no-radius": ".card { border-radius: 0 !important; }",
  "radius-8": ".card { border-radius: 8px !important; }",
  "radius-no-border": ".card { border-color: transparent !important; border-width: 0 !important; }",
  "radius-no-background": ".card { background: none !important; }",
  // The card edge as it was before: a real rounded CSS border.
  "old-border": ".card { box-shadow: none !important; border: 1px solid var(--color-line) !important; }",
  // "sync-decode" variants also set benchFlags.syncDecode (App.svelte).
  "sync-decode": "",
  // Settings → "Visual effects" off (App.svelte sets data-effects).
  "effects-off": "",
  "sync-decode+old-border": ".card { box-shadow: none !important; border: 1px solid var(--color-line) !important; }",
  "border-as-background":
    ".card { border-color: transparent !important; background: linear-gradient(#ffffff0a, #ffffff0a) padding-box, linear-gradient(#ffffff14, #ffffff14) border-box !important; }",
  "border-inset-shadow": ".card { border-color: transparent !important; box-shadow: inset 0 0 0 1px #ffffff14 !important; }",
  "border-outline": ".card { border-color: transparent !important; outline: 1px solid #ffffff14 !important; outline-offset: -1px !important; }",
  "no-clip": ".card { overflow: visible !important; isolation: auto !important; }",
};

export async function scrollVariantsBench(el: HTMLElement): Promise<Record<string, ScrollResult>> {
  const out: Record<string, ScrollResult> = {};
  const style = document.createElement("style");
  document.head.append(style);
  for (const [name, css] of Object.entries(scrollVariants)) {
    style.textContent = css;
    el.scrollTop = 0;
    await new Promise((r) => setTimeout(r, 300));
    out[name] = await scrollBench(el, 3000);
  }
  style.remove();
  return out;
}

/**
 * Records animation-frame gaps only while the user (or ydotool) is active on
 * `el`: a frame counts if a scroll or pointer-move event fired within the last
 * 150 ms (so both wheel scrolling and sweeping the cursor over cards count).
 * Ends 2 s after activity stops, or after `maxMs`.
 */
// Things that may cost a frame, recorded only during wheelFrames so slow
// frames can be attributed ("page" appended, logo "colors" applied, …).
const events: { t: number; what: string }[] = [];
let recording = false;
export function perfEvent(what: string) {
  if (recording) events.push({ t: performance.now(), what });
}

export function wheelFrames(
  el: HTMLElement,
  variant: string,
  maxMs = 20000,
): Promise<Omit<ScrollResult, "cards"> & { cards: number; scrolled: number; causes: Record<string, number> }> {
  events.length = 0;
  recording = true;
  const slow: { start: number; end: number }[] = [];
  const style = document.createElement("style");
  style.textContent = scrollVariants[variant] ?? "";
  document.head.append(style);
  return new Promise((resolve) => {
    const gaps: number[] = [];
    let lastScroll = -Infinity;
    let started = false;
    const top0 = el.scrollTop;
    const onScroll = () => {
      lastScroll = performance.now();
      started = true;
    };
    el.addEventListener("scroll", onScroll, { passive: true });
    el.addEventListener("pointermove", onScroll, { passive: true });
    const begin = performance.now();
    let last = begin;
    const step = (now: number) => {
      if (now - lastScroll < 150) {
        gaps.push(now - last);
        if (now - last > 20) slow.push({ start: last, end: now });
      }
      last = now;
      const idle = started && now - lastScroll > 2000;
      if (!idle && now - begin < maxMs) return requestAnimationFrame(step);
      el.removeEventListener("scroll", onScroll);
      el.removeEventListener("pointermove", onScroll);
      style.remove();
      recording = false;
      // A slow frame is blamed on whatever happened while it was being made.
      const causes: Record<string, number> = {};
      for (const f of slow) {
        const hit = [...new Set(events.filter((e) => e.t >= f.start - 2 && e.t <= f.end).map((e) => e.what))];
        const key = hit.length ? hit.sort().join("+") : "none";
        causes[key] = (causes[key] ?? 0) + 1;
      }
      gaps.sort((a, b) => a - b);
      const at = (q: number) => gaps[Math.min(gaps.length - 1, Math.floor(gaps.length * q))] ?? 0;
      resolve({
        frames: gaps.length,
        median: at(0.5),
        p95: at(0.95),
        janky: gaps.filter((g) => g > 33.4).length,
        cards: el.querySelectorAll(".card").length,
        scrolled: Math.round(el.scrollTop - top0),
        causes,
      });
    };
    requestAnimationFrame(step);
  });
}
