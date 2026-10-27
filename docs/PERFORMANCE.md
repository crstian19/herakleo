# Performance

Measured on 2026-10-27 with a real library of **65,081 channels** (one M3U list, 17 MB channel
cache), release builds, AMD Ryzen 7 5700X + Radeon RX 6800, CachyOS, Hyprland, WebKitGTK 2.54.

## How to measure

| Layer | Command | What it measures |
|---|---|---|
| Backend only | `HERAKLEO_BENCH_DIR=<copy of ~/.config/herakleo> go test -v -run TestBenchRealLibrary ./internal/library/` (in `wails/`) | Go library code, no IPC |
| | `HERAKLEO_BENCH_DIR=<copy> cargo test --release bench_real_library -- --ignored --nocapture` (in `tauri/src-tauri/`) | Rust library code, no IPC |
| End to end | `just bench [library-dir] [runs]` | Both release apps on a copy of the library: window time, first page, API round trips timed **inside the webview** (`lib/bench.ts`), scroll smoothness, peak RSS per process |

`just bench` starts each app with `HERAKLEO_BENCH_OUT=<file>`; the frontend then runs the benchmark
by itself, writes the report through `benchReport` and quits. Always point it at a copy: the
script copies the library to a temp dir and sets `XDG_CONFIG_HOME`.

WebKitGTK clamps `performance.now()` to 1 ms, so `lib/bench.ts` times batches of 10 calls.

## Results

### Backend only (median, ms)

| Operation | Go before | Go after | Rust before | Rust after |
|---|---:|---:|---:|---:|
| Load library (`Open`) | 53 | 52–62 | 27 | 27–32 |
| First page (provider order) | 1.08 | 1.09 | 0.30 | 0.31 |
| Search, common term | 8.36 | **1.53** | 3.60 | **0.73** |
| Search, rare term | 8.38 | **1.69** | 3.41 | **0.67** |
| Group filter | 1.06 | 1.07 | 0.29 | 0.26 |
| **Sort A→Z, one page** | **137** | **0.51** | **31** | **1.44** |
| Group list | 1.06 | 1.09 | 1.42 | 1.33 |

The two fixes, both mirrored in Go and Rust:

1. **Sorting re-sorted all 65k channels for every page** of the infinite scroll (and Go's
   comparator lower-cased both names on each of ~2M comparisons). The A→Z order is now cached and
   rebuilt only when the channel lists change (`chanGen` / `chan_gen`, bumped at every mutation).
2. **Search lower-cased every channel name on every query.** The lower-case name is now computed
   once at load (`withKeys` / `with_keys`, not persisted).

### End to end, inside the app (median of 3 runs)

| | Tauri | Wails |
|---|---:|---:|
| Process start → window mapped | ~235 ms | ~245 ms |
| Process start → first page of channels on screen | ~575 ms | ~720 ms |
| `listChannels`, first page (UI round trip) | 1.0 ms | 1.7 ms |
| Search (UI round trip) | 1.4 ms | 2.0 ms |
| Sort A→Z page (UI round trip) | 2.1 ms | 1.1 ms |
| Group list (UI round trip) | 1.9 ms | 1.4 ms |
| Scroll: median frame / p95 / frames > 33 ms | 16 ms / 24 ms / 1 % | 16 ms / 24 ms / 1 % ¹ |
| Peak RSS, app + WebKit helpers | ~630 MiB | ~690 MiB |
| …of which the app process | 200 MiB | 245 MiB |
| …of which `WebKitWebProcess` (DOM, decoded logos) | 355 MiB | 365 MiB |

¹ One Wails run out of three had 10 % janky frames; the other two matched Tauri.

With an **empty library** the app process is already 165 MiB (Tauri) and 197 MiB (Wails): GTK,
the WebKit UI side and the GPU driver. The 65k channels add roughly **35 MiB in Rust** and **47 MiB
in Go** to the app process, and ~110 MiB to the web process (DOM and decoded logos).

## Reading the numbers

- **Every UI call is under a frame (16.7 ms), by a wide margin.** The IPC bridge costs about
  0.7–0.8 ms per call in both frameworks; the difference between Tauri and Wails is the backend
  code, not the bridge.
- **Rust is ~2–4× faster than Go on raw scans** (string search, page building), Go is faster on
  walking the cached sort order. Neither is noticeable in the UI after the fixes above.
- **Startup is dominated by WebKit**, not by loading the library (27–60 ms of ~600 ms).
- **Scrolling is smooth (60 fps)** since blur filters were removed (see `AGENTS.md`). The web
  process, not the backend, is where memory and paint time go.
- Wails also starts a sandboxed `glycin` image loader (via `bwrap`) to decode the window icon;
  it costs ~10 MiB.

## Why scrolling stays at 60 fps on a 120 Hz monitor

Measured on 2026-10-27 on a 120 Hz monitor (`just bench` prints `idle` and the scroll variants):

| Test | webkit2gtk-4.1 (GTK 3) | webkitgtk-6.0 (GTK 4) |
|---|---:|---:|
| Both apps, idle `requestAnimationFrame` | 120 fps | — |
| Both apps, scrolling the channel grid | ~60 fps (16–17 ms/frame) | — |
| `MiniBrowser`, trivial page, scrolling plain text | **59 fps** | **125 fps** |

- The apps are not capped: idle animation frames run at the monitor's 120 Hz.
- Scrolling drops to 60 in **any** page on the GTK 3 WebKit, even a page of plain text, while the
  GTK 4 WebKit of the same release scrolls at 120+. Removing logos, card backgrounds/glows or
  `content-visibility` changes nothing (`scrollVariants` in `lib/bench.ts`).
- So the limit is the GTK 3 port of WebKitGTK, which **both** frameworks use today:
  - Tauri 2.12 / wry 0.57 depend on `webkit2gtk` 2.0 (GTK 3) only.
  - Wails v2 links `webkit2gtk-4.1` (GTK 3).
  - **Wails v3** (beta.34) builds against GTK 4 + `webkitgtk-6.0` by default (`linux_cgo.go`; the
    `gtk3` build tag opts back into GTK 3). Moving the Wails build to v3 is the only way, today, to
    get 120 Hz scrolling in either app.

### After moving the Wails build to v3 (GTK 4)

`just bench` on 2026-10-27, same library, Wails v3.0.0 on GTK 4.24 + webkitgtk-6.0 2.54.2:

| Scrolling the channel grid (frame median) | Tauri (GTK 3) | Wails v3 (GTK 4) |
|---|---:|---:|
| Baseline | 17 ms (~60 fps) | **8–9 ms (~120 fps)** |
| No logos / flat cards / no `content-visibility` | 17 ms | 8–10 ms |

The GTK 4 build reaches the monitor's 120 Hz while scrolling. Caveats from the same runs:
`idle` fps in the Wails window was unstable (77 and 0.8 fps): GTK 4 stops the frame clock for a
window that is covered or not focused, so that number depends on what is on screen during the
run, not on the app. The p95 frame time is still 20–27 ms in both builds (occasional slow frames
while logos decode). UI round trips went from ~1.7 to ~2.0 ms per call in Wails v3, still far
under a frame; Wails v3 peak RSS is ~715 MiB (v2: ~690 MiB, Tauri: ~640 MiB).

## Scrolling with a real mouse wheel

A tester reported that opening the sidebar was smooth but scrolling was not. `just bench` scrolls
programmatically with no pointer over the grid, so it never saw this. `scripts/profile-scroll.sh`
(`just profile-scroll wails`) reproduces real use: ydotool puts the cursor on the grid and sends
wheel events, the app records frame times while scrolling (`wheelFrames` in `lib/bench.ts`), and
`--sysprof` captures WebKit/GTK marks for the run.

**Cause:** every card that slid under the still cursor started its hover transitions (`box-shadow`
with 48 px and 100 px blur, `border-color`, `transform`). `box-shadow` cannot be animated on the
GPU, so WebKit restyled and repainted on every frame of every running transition.

Wails v3, 120 Hz monitor, 65,081 channels, median of 2–3 runs:

| Variant (real wheel, cursor on the grid) | Frame median | p95 | Frames > 33 ms |
|---|---:|---:|---:|
| Before the fix | 24 ms (~42 fps) | 31–34 ms | 1–5 % |
| No transitions on cards | 9 ms | 25 ms | 1 % |
| No hover shadow | 10–12 ms | 26–28 ms | 1–2 % |
| No logos | 24 ms | 27–28 ms | 1–2 % |
| Flat cards (no background, border or glow) | 8 ms | 19 ms | 0 % |
| No `content-visibility` | 32 ms | 42 ms | 30–43 % |
| **After the fix** | **9 ms (~110 fps)** | 24–25 ms | 1 % |
| Tauri after the fix (GTK 3, for reference) | 17 ms (60 fps) | 35–36 ms | 11–12 % |

sysprof marks, before vs. without transitions (whole run): `RenderingUpdate` p95 14 → 10 ms,
`LayerTreeHostRenderingUpdate` p95 17 → 13 ms, `PerformLayout` 288 → 110 runs, `PaintTile`
15.8k → 9k tiles. Frames needing more than 8.3 ms of main-thread work missed two vsyncs, which is
why the median sat at exactly 3 × 8.3 ≈ 25 ms.

**Fix** (`app.css`, `App.svelte`, `ChannelCard.svelte`):

1. Only `transform` is transitioned on cards; `box-shadow` and `border-color` switch instantly.
2. While the grid scrolls (and 150 ms after), `.is-scrolling .card { pointer-events: none }`, so
   hovers don't start and stop as cards pass under the cursor.
3. The cursor-following spotlight writes its CSS variables at most once per animation frame.
4. `content-visibility: auto` stays: removing it is the worst variant of all.

> sysprof stores the environment of whoever launches it in the capture. The script starts it from a
> minimal environment so no API keys or tokens end up in `profiles/*.syscap`; still, treat captures
> as private.

## Long scrolls: the DOM grew without limit (virtualised grid)

After the hover fix, scrolling *far* (into channels never shown) was still slow. New scenarios in
`scripts/profile-scroll.sh`: `SCENARIO=deep` (250 fast wheel notches), `SCENARIO=hover` (sweep the
cursor, no scrolling), `FRESH_CACHE=1`/`FRESH_COLORS=1` (empty WebKit and logo-colour caches). Slow
frames are attributed to what happened while they were produced (`perfEvent` in `lib/bench.ts`).

> **Method fix.** Early `deep` runs did not scroll the same distance: when a run outpaced infinite
> scroll it hit the end of the loaded cards, scrolled less (19.7k vs 29k px), painted less and
> looked faster. Numbers for `decoding="async"` and the inset edge measured that way were too
> optimistic. Wheel mode now preloads ~1,500 cards first, so every run scrolls 31,125 px.

With ~1,560 cards in the page, **every** variant took **600–700 ms per frame** (with or without logos,
either border, either decoding): infinite scroll appended cards and never removed them, and the
cost grew with the number of cards in the DOM (840 cards: 17 ms; 1,560: 650 ms).

**Fix:** the channel grid is virtualised (`updateWindow` in `App.svelte`): only rows within 800 px of
the viewport are rendered (60–90 cards), with spacers above and below. Rows are square cards, so the
row height comes from the CSS grid's column width plus the gap; no card measuring. Keyboard
navigation works on list positions (`data-index`) and scrolls the target row into view first.

Wails v3, `deep`, 31,125 px, 1,500 cards preloaded, empty caches:

| | Frame median | p95 | Frames > 33 ms | Cards in DOM |
|---|---:|---:|---:|---:|
| Before virtualising | 600–700 ms | ~700 ms | 92–100 % | 1,560 |
| **Virtualised (current)** | **16–17 ms** | **19 ms** | **0 %** | 80–85 |
| Virtualised, old paint (sync decode + rounded CSS border) | 17 ms | 26 ms | 0–1 % | 80 |
| Tauri, virtualised (GTK 3) | 17 ms | 34 ms | 13 % | 80 |

The two paint changes still matter for spikes (~13 slow frames per run instead of ~140):

- `decoding="async"` on logos: WebKit decodes them off the main thread instead of during paint.
- The card edge is an inset `box-shadow`, not a rounded CSS `border`: with hundreds of cards entering
  per second, the rounded border alone cost ~3 ms per frame; a rounded background or inset shadow
  costs nothing measurable. (`border-radius: 0` would also fix it; changing the radius to 8 px did not.)

Other scenarios now: normal wheel scroll 9 ms median / 19 ms p95, hover sweep 8 / 10 ms. A `deep`
run feeds half a screen of new cards per frame, so ~16 ms (60 fps) is its practical ceiling.

## Pagination by default, and a "visual effects" switch

Settings → Appearance now has:

- **Browsing channels:** pages of 60 (default) or infinite scroll (virtualised), with a warning that
  long lists can be slower on modest computers. Pages: `«‹ Page X of Y ›»` under the grid, `[`/`]`,
  and ArrowDown on the last row turns the page. Benchmarks force infinite scroll so numbers stay
  comparable.
- **Visual effects** (default on, off if the system asks for reduced motion): off hides the ambient
  glow layer, hover zoom/shadows/spotlight/border glow, logo glows and all transitions
  (`:root[data-effects="off"]` in `app.css`).

Measured with the `deep` scenario (31,125 px, 1,500 cards preloaded):

| Wails v3 | Frame median | p95 | Frames > 33 ms |
|---|---:|---:|---:|
| Effects on | 16 ms (60 fps) | 18–19 ms | 0 % |
| **Effects off** | **9–10 ms (~110 fps)** | 18 ms | 0 % |

Hover sweep: 8 ms either way. So on fast long scrolls the decorative layers cost ~6 ms per frame;
on a 60 Hz screen or a normal scroll they are free.

## Ideas not done yet

- Lazy-load channel caches of disabled lists instead of keeping them in memory.
- Drop stream URLs from memory and read them from disk on play (they are ~40 % of the channel data).
