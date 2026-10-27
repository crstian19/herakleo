<p align="center">
  <img src="assets/herakleo-wails.png" alt="Herakleo mascot" width="160">
</p>

<h1 align="center">Herakleo</h1>

<p align="center">A fast desktop IPTV player for Linux. Load an M3U list or an Xtream Codes account,
find a channel among tens of thousands, and watch it in mpv.</p>

## Features

- **M3U and Xtream Codes**: by URL, from a local file, or with an Xtream account (shows its expiry
  date). Several lists at once, each one can be refreshed or turned off without deleting it.
- **Built for big lists**: 65,000 channels search, filter and scroll smoothly. Searching is a
  single pass over a precomputed index and the channel grid only renders what is on screen.
- **Plays in your player**: mpv by default; VLC, Celluloid, Haruna and SMPlayer also work. Stream
  caching, hardware decoding and start volume can be set from the app.
- **Favourites, history and hiding**: hide channels or whole groups you never watch, keep a list of
  recent channels, back up favourites (they survive re-adding a list).
- **Keyboard first**: every view works with arrows, Enter and Esc, so a TV remote is enough. Press
  `?` for the shortcuts.
- **Six languages** (English, Spanish, French, Portuguese, German, Italian), picked from the
  system, and six accent colours.
- **Light on old machines**: visual effects can be turned off, and the grid can page (default) or
  scroll infinitely.
- Everything stays on your machine, in `~/.config/herakleo`. No accounts, no telemetry.

## Install

Arch Linux packages are built from [`packaging/arch`](packaging/arch/README.md):

```bash
just package-arch wails
sudo pacman -U packaging/arch/herakleo-wails/herakleo-wails-*.pkg.tar.zst
sudo pacman -S mpv   # or any supported player
```

Other distributions: build from source (below). Herakleo needs GTK 4 and `webkitgtk-6.0` at run
time.

## Build from source

Requirements: [Go](https://go.dev) 1.27+, [Bun](https://bun.sh), [just](https://just.systems),
`gtk4`, `webkitgtk-6.0` and `pkgconf`.

```bash
just setup         # frontend dependencies and git hooks
just build-wails   # → wails/build/bin/herakleo-wails
just dev-wails     # live-reloading development build
just dev-mock      # the UI alone, in a browser, with fake data
just test          # svelte-check, Go and Rust tests
```

## How it is built

The UI is Svelte 5 + Tailwind 4, rendered by the system WebKit. The app you install uses a Go
backend on [Wails v3](https://v3.wails.io) (GTK 4).

Herakleo was written twice on purpose: the same frontend also runs on a Rust backend with
[Tauri 2](https://v2.tauri.app), to compare both frameworks on a real app. The Tauri build is
kept in the repo (`just build-tauri`) and both backends implement the same API, error codes and
on-disk format, so they can share a library. On Linux, Tauri still uses the GTK 3 WebKit, which
caps scrolling at 60 fps; Wails v3 on GTK 4 does not, which is why Wails is the main build.
Measurements are in [`docs/PERFORMANCE.md`](docs/PERFORMANCE.md).

```
frontend/          Svelte UI, shared by both apps (src/lib/types.ts is the API contract)
wails/             Go backend (Wails v3): playlist parsing, library, player
tauri/src-tauri/   Rust backend (Tauri 2), same API
packaging/arch/    PKGBUILDs
scripts/           benchmark and scroll-profiling harnesses
docs/              performance notes and backlog
video/             Remotion project for the launch video
```

Contributor notes (parity rules between the two backends, performance gotchas) are in
[`AGENTS.md`](AGENTS.md); planned work is in [`docs/BACKLOG.md`](docs/BACKLOG.md).

## Support

Herakleo is a side project. If it is useful to you, you can support it on
[GitHub Sponsors](https://github.com/sponsors/crstian19), [Ko-fi](https://ko-fi.com/crstian) or
[PayPal](https://paypal.me/CrstianG).

## License

[GPL-3.0-or-later](LICENSE). You can use, study, change and share Herakleo; copies and
modified versions you distribute must stay under the same license, with their source.
