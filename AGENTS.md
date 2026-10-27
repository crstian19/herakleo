# Herakleo — Agent Guide

Desktop IPTV player (M3U + Xtream Codes, playback in external mpv), built **twice on purpose**
to compare Tauri and Wails. Named after Herakleopolis, the ancient rival of Thebes.

## Layout

| Path | What |
|---|---|
| `frontend/` | Svelte 5 + Vite 8 + Tailwind 4, **shared** by both apps |
| `frontend/src/lib/types.ts` | The API contract both backends implement |
| `frontend/src/lib/backend/{tauri,wails,mock}.ts` | One adapter per backend, picked by Vite `--mode` via the `$backend` alias |
| `tauri/src-tauri/` | Rust backend (Tauri v2): `playlist.rs`, `library.rs`, `player.rs`, commands in `lib.rs` |
| `wails/` | Go backend (Wails **v3**, GTK 4 + webkitgtk-6.0): `internal/{playlist,library,player}`, service methods in `app.go` |

## Parity rules (the point of the project)

- Every change to the backend goes into **both** implementations, in the same commit.
- The on-disk library (`~/.config/herakleo/library.json` + `channels/<sourceId>.json`) is shared:
  both apps must read and write the same JSON.
- Channel IDs are `<sourceId>-<fnv64a(streamURL) as 16 hex>`. Tests in both languages pin the same
  vectors (`library_test.go`, `library.rs`); keep them in sync.
- Both apps may run at once on the same files: every library call first reloads `library.json` if its
  mtime changed (`sync.go` / `library/sync.rs`). Keep that call at the top of any new public method.
- `library.json` also holds hidden channels/groups and the history; the Go `library` package and the
  Rust `library/` module are split into the same files (core, sync, query, curation, colors).
- Shared preferences (the chosen player) live in `library.json` under `settings`; purely visual ones
  (logo colours on/off) live in the frontend's localStorage. Logo colours are cached in `colors.json`.
- Players are a fixed allowlist (`player.go` / `player.rs`: mpv, VLC, Celluloid, Haruna, SMPlayer).
  Never accept a free-form command from the UI: a webview bug would become code execution.
- **Errors are codes, not prose.** Backends return `code` or `code: detail` (e.g. `source_not_found`,
  `download_failed: connection refused`); the UI translates `err.<code>` from its catalogue
  (`frontend/src/lib/locales/*.ts`) and shows unknown strings raw. A new error = same code in both
  backends + an `err.<code>` key in `es.ts` (the source catalogue; other locales are type-checked
  against it). Native dialog titles are passed in by the UI for the same reason.
- **i18n:** `lib/i18n.svelte.ts` picks the system language (navigator.languages, which WebKitGTK takes
  from the desktop locale) unless the user chose one; es, en, fr, pt, de, it. Numbers/dates/relative
  times go through Intl, never hand-formatted.
- **Accent themes** are `:root[data-accent=…]` blocks in `app.css` overriding `--color-accent`,
  `--color-accent-deep` and `--color-on-accent`; use `accent` utilities (`bg-accent/10`,
  `border-accent/50`, `btn-accent`), never a literal colour. Visual prefs (accent, language, sort,
  logo colours) live in localStorage, so each build keeps its own.
- Xtream stream URLs use Go's `url.PathEscape` rules; `playlist.rs::path_escape` mirrors them.
- Tauri commands take named args mapped from camelCase (`channelId` → `channel_id`); the Wails
  adapter calls `Call.ByName("main.App.<Method>")` from `@wailsio/runtime`, without generated
  bindings. Go methods whose first parameter is `context.Context` get it from Wails (cancelled if
  the call is), so the frontend does not pass it.

## Commands

```bash
just setup        # bun install (frontend + tauri CLI), lefthook install
just dev-mock     # UI in a browser with fake data, no backend needed
just dev-tauri    # / just dev-wails
just build        # release binaries of both
just test         # svelte-check + go test + cargo test
just lint
just compare      # clean builds: binary size + build time
just mem tauri    # RSS of a running app incl. WebKit helper processes
just bench        # end-to-end benchmark of both apps on a copy of the library (docs/PERFORMANCE.md)
```

## Gotchas

- **Wails is v3 on GTK 4 + webkitgtk-6.0** (migrated from v2 on 2026-10-27): the GTK 3 WebKit that
  Tauri and Wails v2 use scrolls at 60 fps at most (docs/PERFORMANCE.md). No Wails CLI is needed:
  `just build-wails` is a plain `go build -tags production`; without that tag the binary serves
  `FRONTEND_DEVSERVER_URL` instead of the embedded UI (`just dev-wails`). Under GTK 4 the window
  class is the `ApplicationID` (`me.crstian.herakleo-wails`), not `ProgramName`.
- `wails/frontend/dist` is embedded with `go:embed`; run `bun run build:wails` in `frontend/` before
  `go vet`/`go test` on a fresh clone.
- Don't set `WEBKIT_DISABLE_DMABUF_RENDERER=1` by default: it is only a workaround for blank windows
  (mostly NVIDIA) and pushes WebKitGTK onto a slower paint path. On AMD/Intel it isn't needed.
- WebKitGTK paints `filter: blur()` / `backdrop-filter` very slowly. With thousands of channel cards
  they made the UI sluggish, so the tesdash look is approximated with radial gradients and flat
  translucent cards; cards use `content-visibility: auto`. Keep blurs out of anything repeated.
- svelte-check doesn't support TypeScript 7 (tsgo) yet, so `frontend` pins TypeScript 6.
- Tauri sync commands run on the main thread: anything blocking (the file dialog) goes through
  `spawn_blocking` from an async command.
- EPG (XMLTV) is not implemented yet.
- **The channel grid is virtualised** (`updateWindow` in `App.svelte`): never render the whole
  `channels` list; cards carry `data-index` (list position) and keyboard navigation works on it.
  Without this the DOM grew with infinite scroll and frames hit 600+ ms past ~1,500 cards.
- **Dialogs** close with the shared `CloseButton` (X, top right) plus Esc; keep text buttons only for
  real actions (Cancel/Save, Not now, Copy). The support popup plays the Lottie heart from tesdash
  (`lottie_light`, no eval, fine under Tauri's CSP) after a play button traces and flashes (`.support-hero` in `app.css`), plus confetti on donation clicks, both skipped
  when visual effects are off.
- **External links** (repo, donations) open through `openLink(id)`: the UI sends an ID from a fixed
  list (`projectLinks` in `app.go` / `PROJECT_LINKS` in `lib.rs`), never a URL. Add a link to both.
  On Linux both open it through a GDK launch context (`openurl_linux.go` / `open_url` in `lib.rs`),
  not xdg-open, so the browser gets an XDG activation token and the compositor may focus it.
  The support popup never shows on first launch: on the 10th channel played, then by chance (1 in 200 plays)
  (`SUPPORT_AFTER`; `localStorage` keys `herakleo:plays` and `herakleo:support-seen`).
- **Browse mode** is a visual pref (`prefs.browse`): `"pages"` (default, `PAGE_SIZE` 60, `goToPage`) or
  `"scroll"` (virtualised infinite scroll). `loadChannels(false)` is a no-op in pages mode. Bench and
  wheel sessions switch to scroll mode without saving.
- **Visual effects** (`prefs.effects`, `:root[data-effects="off"]`): anything decorative and costly to
  paint must be covered by the effects-off rules in `app.css` (mark images/glows with `fx-glow`).
- Card edge = inset `box-shadow` (`--card-line`), not a CSS border; logos use `decoding="async"`.
- **Scrolling performance:** never transition `box-shadow`, `filter` or colours on the channel cards
  (only `transform`/`opacity`); hover is disabled while the grid scrolls (`.is-scrolling`). Check
  with `just profile-scroll wails` (real wheel events via ydotool), not just `just bench`.
- **Performance:** queries must stay a single linear pass. Never sort or lower-case per query: use
  the cached A→Z order and the precomputed `lower` key, and bump `chanGen`/`chan_gen` wherever the
  channel lists change or the cache goes stale. Check with `just bench` (docs/PERFORMANCE.md).
- **Keyboard:** one `onkey` handler in `App.svelte`; helpers and the help list in `lib/shortcuts.ts`.
  Single-key shortcuts never fire while typing (`isTyping`). Everything must stay reachable with
  arrows + Enter + Esc/Backspace (TV remotes). A new shortcut goes in both `onkey` and
  `shortcutGroups`, with its label in the locales.
- Player options are a closed set (`player.Options` / `player::Options`) validated in both backends;
  add a new one as a typed field plus per-player args, never as a free-form string.
