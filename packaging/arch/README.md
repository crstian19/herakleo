# Herakleo for Arch Linux

Two packages of the same IPTV player, one built with Tauri and one with Wails. They can be
installed side by side: each has its own binary, icon and menu entry ("Herakleo (Tauri)" and
"Herakleo (Wails)"). Both share the library in `~/.config/herakleo`.

## Install

```bash
sudo pacman -U herakleo-wails-*.pkg.tar.zst
# and/or
sudo pacman -U herakleo-tauri-*.pkg.tar.zst
```

Channels open in an external player. The default is **mpv**:

```bash
sudo pacman -S mpv
```

VLC, Celluloid, Haruna or SMPlayer also work (pick one in the app's settings).

## Uninstall

```bash
sudo pacman -R herakleo-wails herakleo-tauri
```

Your lists and favourites stay in `~/.config/herakleo`; delete it by hand if you don't want them.

## Building the packages (for the author)

From the repo root: `just package-arch` (both) or `just package-arch wails` (one). You need
`go`, `bun`, `nodejs`, `pkgconf` and `imagemagick`, plus `rust`/`rustup` and `webkit2gtk-4.1` for
Tauri, and `webkitgtk-6.0` + `gtk4` for Wails (v3, GTK 4).
