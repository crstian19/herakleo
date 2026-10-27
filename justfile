set shell := ["bash", "-euo", "pipefail", "-c"]

# Wails v3 needs no CLI: it is a plain cgo build against GTK 4 + webkitgtk-6.0.
# -tags production embeds the UI; the C flag hides deprecation warnings from
# Wails' own X11 code under GTK 4.24.
wails_build := 'CGO_CFLAGS="-Wno-deprecated-declarations" go build -tags production -trimpath -ldflags "-s -w"'

default:
    @just --list

# Install frontend + Tauri CLI deps and git hooks
setup:
    cd frontend && bun install
    cd tauri && bun install
    lefthook install

# UI only, in a browser, against fake data
dev-mock:
    cd frontend && bun run dev:mock

dev-tauri:
    cd tauri && bunx tauri dev

# Vite dev server + the app pointed at it (FRONTEND_DEVSERVER_URL, non-production build)
dev-wails:
    #!/usr/bin/env bash
    set -euo pipefail
    (cd frontend && exec bun run dev:wails) & vite=$!
    trap 'kill $vite 2>/dev/null' EXIT
    until curl -s -o /dev/null http://localhost:5173; do sleep 0.2; done
    cd wails && CGO_CFLAGS="-Wno-deprecated-declarations" FRONTEND_DEVSERVER_URL=http://localhost:5173 go run .

build-tauri:
    cd tauri && bunx tauri build --no-bundle

# Builds .deb, .rpm and AppImage
bundle-tauri:
    cd tauri && bunx tauri build

build-wails:
    cd frontend && bun install && bun run build:wails
    cd wails && {{ wails_build }} -o build/bin/herakleo-wails .

build: build-tauri build-wails

test:
    cd frontend && bun run check
    cd frontend && bun run build:wails
    cd wails && go test ./...
    cd tauri/src-tauri && cargo test

lint:
    cd frontend && bun run check
    cd frontend && bun run build:wails
    cd wails && go vet ./...
    cd tauri/src-tauri && cargo clippy --all-targets -- -D warnings

# Clean release builds of both, then binary size and build time side by side
compare:
    #!/usr/bin/env bash
    set -euo pipefail
    cd tauri/src-tauri && cargo clean -q && cd ../..
    t0=$(date +%s); (cd tauri && bunx tauri build --no-bundle >/dev/null 2>&1); t1=$(date +%s)
    (cd wails && go clean -cache >/dev/null && just build-wails >/dev/null 2>&1); t2=$(date +%s)
    printf "%-8s %10s %12s\n" app binary "clean build"
    printf "%-8s %10s %11ss\n" tauri "$(du -h tauri/src-tauri/target/release/herakleo-tauri | cut -f1)" $((t1 - t0))
    printf "%-8s %10s %11ss\n" wails "$(du -h wails/build/bin/herakleo-wails | cut -f1)" $((t2 - t1))

# Start one app (tauri|wails), wait, and sum RSS of it plus its WebKit helpers
mem app="tauri" secs="5":
    #!/usr/bin/env bash
    set -euo pipefail
    case "{{ app }}" in
      tauri) bin=tauri/src-tauri/target/release/herakleo-tauri ;;
      wails) bin=wails/build/bin/herakleo-wails ;;
      *) echo "app must be tauri or wails"; exit 1 ;;
    esac
    "$bin" >/dev/null 2>&1 & pid=$!
    sleep {{ secs }}
    pids=$(echo "$pid" $(pgrep -P "$pid" || true) | tr ' ' ',')
    kb=$(ps -o rss= -p "$pids" | awk '{s+=$1} END {print s}')
    echo "{{ app }}: $((kb / 1024)) MiB RSS across processes $pids"
    kill "$pid"

# Tars the working tree (uncommitted changes included) and runs makepkg in each
# PKGBUILD dir. No -s: install the makedepends yourself (cargo, go, bun,
# nodejs, pkgconf, imagemagick).
# Arch packages (packaging/arch): both apps, or one with `just package-arch wails`
package-arch flavours="tauri wails":
    #!/usr/bin/env bash
    set -euo pipefail
    ver=$(sed -n 's/^pkgver=//p' packaging/arch/herakleo-tauri/PKGBUILD)
    tarball="packaging/arch/herakleo-$ver.tar.gz"
    tar -czf "$tarball" \
      --exclude-vcs \
      --exclude=node_modules --exclude=target --exclude=dist --exclude=dist-mock \
      --exclude=./wails/frontend --exclude=./wails/build/bin --exclude=./tauri/src-tauri/gen \
      --exclude='./packaging/arch/*/src' --exclude='./packaging/arch/*/pkg' \
      --exclude='*.pkg.tar.zst' --exclude='herakleo-*.tar.gz' --exclude=.env \
      --transform "s,^\.,herakleo-$ver," \
      .
    for flavour in {{ flavours }}; do
      cp "$tarball" "packaging/arch/herakleo-$flavour/"
      (cd "packaging/arch/herakleo-$flavour" && makepkg -f --noconfirm)
    done
    for flavour in {{ flavours }}; do ls -1 "$PWD"/packaging/arch/herakleo-$flavour/herakleo-*.pkg.tar.zst; done

# End-to-end benchmark of both release builds on a copy of a library
# (window time, first page, in-webview API round-trips, peak RSS)
bench library="" runs="3":
    scripts/bench.sh "{{ library }}" "{{ runs }}"

# Real mouse-wheel scroll profile (needs ydotoold; --sysprof adds a capture in profiles/)
profile-scroll app="wails" variant="baseline" *flags="":
    scripts/profile-scroll.sh "{{ app }}" "{{ variant }}" {{ flags }}
