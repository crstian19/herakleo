# Backlog

Feature gap against Fred TV (formerly Open TV, GPL-2.0), reviewed on 2026-10-26 from its README and
the names of its settings and commands only. **Implement from public format docs (M3U, XMLTV, the
Xtream player API), never from Fred TV's source**: copying or translating its code would make
Herakleo a GPL-2.0 derivative.

Every item lands in both backends (Tauri/Rust and Wails/Go) with matching tests; see `AGENTS.md`.

## Block 1 — quick wins (done 2026-10-26)

- [x] Watch history ("Recientes") and clearing it
- [x] Hide channels and groups, plus a view to unhide them
- [x] Sort: provider order, A→Z, Z→A
- [x] Enable/disable a list without deleting it
- [x] Refresh all lists; optional refresh on start
- [x] Xtream account expiry date and status
- [x] Back up and restore favourites (survives re-adding a list)
- [x] Fix: both apps open at once overwrite each other's `library.json`; clean orphan channel files

## Block 2 — keyboard and player

- [x] Keyboard shortcuts (search, favourites, all, groups, back) and arrow/Tab navigation for TV remotes (done 2026-10-27; `?` shows them)
- [x] Player options: stream caching, hardware decoding, start volume (fixed options, not free-form args; mpv all three, VLC the first two) (done 2026-10-27)

## Block 3 — EPG

- [ ] XMLTV guide per source (Xtream `xmltv.php` or a custom URL), streamed parsing for large files
- [ ] Now/next on cards and a programme view per channel
- [ ] Reminders for upcoming programmes

## Block 4 — VOD

- [ ] Xtream movies (`get_vod_streams`) and series (`get_series`, `get_series_info`) with seasons/episodes
- [ ] Media-type filter: live / movies / series

## Later or maybe never

- [ ] Record while watching and download movies (ffmpeg)
- [ ] Custom channels, groups and lists; share them
- [ ] Re-stream a channel to other devices on the LAN
- [ ] Stalker portal (MAG) sources
- [ ] System tray, UI zoom, default view on start
- [x] Pick a licence for Herakleo itself: GPL-3.0-or-later (done 2026-10-27)
