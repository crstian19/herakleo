set shell := ["bash", "-euo", "pipefail", "-c"]

# Wails v3 needs no CLI: it is a plain cgo build against GTK 4 + webkitgtk-6.0.
# -tags production embeds the UI; the C flag hides deprecation warnings from
# Wails' own X11 code under GTK 4.24.
wails_build := 'CGO_CFLAGS="-Wno-deprecated-declarations" go build -tags production -trimpath -ldflags "-s -w"'

default:
    @just --list

# Install frontend deps and git hooks
setup:
    cd frontend && bun install
    lefthook install

# UI only, in a browser, against fake data
dev-mock:
    cd frontend && bun run dev:mock

# Vite dev server + the app pointed at it (FRONTEND_DEVSERVER_URL, non-production build)
dev-wails:
    #!/usr/bin/env bash
    set -euo pipefail
    (cd frontend && exec bun run dev:wails) & vite=$!
    trap 'kill $vite 2>/dev/null' EXIT
    until curl -s -o /dev/null http://localhost:5173; do sleep 0.2; done
    cd wails && CGO_CFLAGS="-Wno-deprecated-declarations" FRONTEND_DEVSERVER_URL=http://localhost:5173 go run .

build-wails:
    cd frontend && bun install && bun run build:wails
    cd wails && {{ wails_build }} -o build/bin/herakleo .

build: build-wails

test:
    cd frontend && bun run check
    cd frontend && bun run build:wails
    cd wails && go test ./...

# svelte-check, then wails/.golangci.yml (govet included) and its formatters
lint:
    cd frontend && bun run check
    cd frontend && bun run build:wails
    cd wails && CGO_CFLAGS=-Wno-deprecated-declarations golangci-lint run ./...
    cd wails && golangci-lint fmt --diff

# Start the release build, wait, and sum RSS of it plus its WebKit helpers
mem secs="5":
    #!/usr/bin/env bash
    set -euo pipefail
    wails/build/bin/herakleo >/dev/null 2>&1 & pid=$!
    sleep {{ secs }}
    pids=$(echo "$pid" $(pgrep -P "$pid" || true) | tr ' ' ',')
    kb=$(ps -o rss= -p "$pids" | awk '{s+=$1} END {print s}')
    echo "$((kb / 1024)) MiB RSS across processes $pids"
    kill "$pid"

# Tars the working tree (uncommitted changes included) and runs makepkg in each
# PKGBUILD dir. No -s: install the makedepends yourself (go, bun, nodejs, pkgconf).
# Arch package from the working tree (packaging/arch)
package-arch pkgs="herakleo":
    #!/usr/bin/env bash
    set -euo pipefail
    ver=$(sed -n 's/^pkgver=//p' packaging/arch/herakleo/PKGBUILD)
    tarball="packaging/arch/herakleo-$ver.tar.gz"
    # Only what git would publish (tracked + untracked, minus .gitignore), so
    # local-only files like profiles/ (sysprof captures hold the environment)
    # can never end up in a source tarball.
    git ls-files -z --cached --others --exclude-standard \
      | while IFS= read -r -d '' f; do [ -e "$f" ] && printf '%s\0' "$f"; done \
      | tar -czf "$tarball" --null -T - --transform "s,^,herakleo-$ver/,"
    for pkg in {{ pkgs }}; do
      cp "$tarball" "packaging/arch/$pkg/"
      (cd "packaging/arch/$pkg" && makepkg -f --noconfirm)
    done
    for pkg in {{ pkgs }}; do ls -1 "$PWD"/packaging/arch/$pkg/$pkg-*.pkg.tar.zst; done

# The release workflow refuses tags whose version differs from these files.
# Set the release version (Arch pkgver, AppStream entry, CHANGELOG); then commit and tag
bump version:
    #!/usr/bin/env bash
    set -euo pipefail
    v="{{ version }}"
    [[ "$v" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || { echo "version must be x.y.z"; exit 1; }
    sed -i "s/^pkgver=.*/pkgver=$v/; s/^pkgrel=.*/pkgrel=1/" packaging/arch/herakleo/PKGBUILD
    meta=packaging/linux/me.crstian.Herakleo.metainfo.xml
    grep -q "<release version=\"$v\"" "$meta" \
      || sed -i "s|  <releases>|  <releases>\n    <release version=\"$v\" date=\"$(date +%F)\"/>|" "$meta"
    # Unreleased becomes the new version; a fresh empty Unreleased goes on top.
    if ! grep -q "^## \[$v\]" CHANGELOG.md; then
      prev=$(sed -n 's/^## \[\([0-9][^]]*\)\].*/\1/p' CHANGELOG.md | head -1)
      sed -i "s|^## \[Unreleased\]|## [Unreleased]\n\n## [$v] - $(date +%F)|" CHANGELOG.md
      sed -i "s|^\[Unreleased\]: .*|[Unreleased]: https://github.com/crstian19/herakleo/compare/v$v...HEAD\n[$v]: https://github.com/crstian19/herakleo/compare/v$prev...v$v|" CHANGELOG.md
    fi
    git diff --stat
    echo "Next: git commit -am 'chore: release v$v' && git tag v$v && git push --follow-tags"

# End-to-end benchmark of the release build on a copy of a library
# (window time, first page, in-webview API round-trips, peak RSS)
bench library="" runs="3":
    scripts/bench.sh "{{ library }}" "{{ runs }}"

# Real mouse-wheel scroll profile (needs ydotoold; --sysprof adds a capture in profiles/)
profile-scroll variant="baseline" *flags="":
    scripts/profile-scroll.sh "{{ variant }}" {{ flags }}
