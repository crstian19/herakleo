#!/usr/bin/env bash
# Profiles scrolling the way a person does it: real mouse-wheel events (ydotool)
# with the cursor over the channel grid, so hover effects churn as cards pass
# under the pointer. The app records frame times while scrolling (wheel mode,
# lib/bench.ts) and quits; optionally the whole run is captured with sysprof,
# whose WebKit/GTK marks show where each frame's time went.
#
# Usage: scripts/profile-scroll.sh <tauri|wails> [variant] [--sysprof]
#   variant: a key of scrollVariants in frontend/src/lib/bench.ts (default baseline)
# Env:
#   SCENARIO=scroll  fast flick down + slower scroll up (default)
#   SCENARIO=deep    long fast scroll into channels never shown before
#   SCENARIO=hover   no scrolling: sweep the cursor across the cards
#   FRESH_COLORS=1   start without the logo-colour cache
#   FRESH_CACHE=1    start with an empty WebKit cache (logos download and decode again)
# Needs: hyprctl (Hyprland), ydotool + ydotoold on $YDOTOOL_SOCKET, sysprof for --sysprof.
set -euo pipefail

app=${1:?tauri or wails}
variant=${2:-baseline}
use_sysprof=false
[ "${3:-}" = "--sysprof" ] && use_sysprof=true
root=$(cd "$(dirname "$0")/.." && pwd)
export YDOTOOL_SOCKET=${YDOTOOL_SOCKET:-/run/user/$(id -u)/.ydotool_socket}
[ -S "$YDOTOOL_SOCKET" ] || { echo "ydotoold is not running ($YDOTOOL_SOCKET)" >&2; exit 1; }

case "$app" in
  tauri) bin="$root/tauri/src-tauri/target/release/herakleo-tauri" class="herakleo-tauri" ;;
  wails) bin="$root/wails/build/bin/herakleo-wails" class="me.crstian.herakleo-wails" ;;
  *) echo "app must be tauri or wails" >&2; exit 1 ;;
esac
[ -x "$bin" ] || { echo "missing $bin (run: just build)" >&2; exit 1; }

out_dir="$root/profiles"
mkdir -p "$out_dir"
stamp="$app-${SCENARIO:-scroll}-$variant-$(date +%H%M%S)"
report="$out_dir/$stamp.json"

# Work on a copy of the library: the app rewrites library.json and colors.json.
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
mkdir -p "$work/config"
cp -r "${XDG_CONFIG_HOME:-$HOME/.config}/herakleo" "$work/config/herakleo"
# FRESH_COLORS=1: start without the logo-colour cache, like scrolling into
# channels for the first time (colours then arrive and repaint mid-scroll).
[ "${FRESH_COLORS:-}" = 1 ] && rm -f "$work/config/herakleo/colors.json"

# Windows of the same app that were already open (e.g. your everyday one): the
# profile must target only the window it starts.
before=$(hyprctl clients -j | python3 -I -c '
import json, sys
print(" ".join(c["address"] for c in json.load(sys.stdin) if c["class"] == sys.argv[1]))' "$class")

cache_env=()
[ "${FRESH_CACHE:-}" = 1 ] && cache_env=(XDG_CACHE_HOME="$work/cache")
envs=("${cache_env[@]}" XDG_CONFIG_HOME="$work/config" HERAKLEO_BENCH_OUT="$report" HERAKLEO_BENCH_MODE="wheel:$variant")
if $use_sysprof; then
  capture="$out_dir/$stamp.syscap"
  sysprof_env=()
  for e in "${envs[@]}"; do sysprof_env+=(-e "$e"); done
  # sysprof stores its own environment in the capture ("spawnable.environ"),
  # so start it from a minimal one: no API keys or tokens end up in the file.
  env -i HOME="$HOME" USER="$USER" PATH=/usr/bin:/bin LANG="${LANG:-C.UTF-8}" \
    XDG_RUNTIME_DIR="$XDG_RUNTIME_DIR" WAYLAND_DISPLAY="${WAYLAND_DISPLAY:-}" DISPLAY="${DISPLAY:-}" \
    DBUS_SESSION_BUS_ADDRESS="${DBUS_SESSION_BUS_ADDRESS:-}" XDG_SESSION_TYPE="${XDG_SESSION_TYPE:-}" \
    XDG_CURRENT_DESKTOP="${XDG_CURRENT_DESKTOP:-}" \
    sysprof-cli --gtk --use-trace-fd --no-perf -f "${sysprof_env[@]}" "$capture" -- "$bin" >"$work/sysprof.log" 2>&1 &
else
  env "${envs[@]}" "$bin" >/dev/null 2>&1 &
fi
runner=$!

# Wait for the window, then for the first page of channels to render.
geo=""
for _ in $(seq 100); do
  geo=$(hyprctl clients -j | python3 -I -c '
import json, sys
for c in json.load(sys.stdin):
    if c["class"] == sys.argv[1] and c["address"] not in sys.argv[2].split():
        x, y = c["at"]; w, h = c["size"]
        print(x + int(w * 0.62), y + int(h * 0.55)); break' "$class" "$before")
  [ -n "$geo" ] && break
  sleep 0.1
done
[ -n "$geo" ] || { echo "window did not appear" >&2; kill "$runner" 2>/dev/null; exit 1; }
sleep 2

# ydotool only moves relatively: walk the cursor to the middle of the grid.
read -r tx ty <<<"$geo"
for _ in 1 2 3; do
  IFS=', ' read -r cx cy <<<"$(hyprctl cursorpos)"
  [ "$cx" = "$tx" ] && [ "$cy" = "$ty" ] && break
  ydotool mousemove -x $((tx - cx)) -y $((ty - cy)) >/dev/null
  sleep 0.1
done

case "${SCENARIO:-scroll}" in
  scroll) # a fast flick down, a pause, and a slower scroll back up
    for _ in $(seq 60); do ydotool mousemove -w -x 0 -y -1 >/dev/null; sleep 0.03; done
    sleep 0.5
    for _ in $(seq 30); do ydotool mousemove -w -x 0 -y 1 >/dev/null; sleep 0.08; done ;;
  deep) # keep going down, well past the first pages
    for _ in $(seq 250); do ydotool mousemove -w -x 0 -y -1 >/dev/null; sleep 0.025; done ;;
  hover) # sweep left and right across a row of cards, then diagonally
    for _ in 1 2 3; do
      for _ in $(seq 40); do ydotool mousemove -x -15 -y 0 >/dev/null; sleep 0.016; done
      for _ in $(seq 40); do ydotool mousemove -x 15 -y 0 >/dev/null; sleep 0.016; done
    done
    for _ in $(seq 40); do ydotool mousemove -x -10 -y 6 >/dev/null; sleep 0.016; done ;;
esac

for _ in $(seq 300); do [ -s "$report" ] && break; sleep 0.1; done
wait "$runner" 2>/dev/null || true
[ -s "$report" ] || { echo "no report (see $work/sysprof.log)" >&2; cat "$work/sysprof.log" 2>/dev/null | tail -5 >&2; exit 1; }

python3 -I - "$report" "$app" "${SCENARIO:-scroll}" <<'PY'
import json, sys
r = json.load(open(sys.argv[1]))
print(f"{sys.argv[2]:<6} {sys.argv[3]:<6} {r['variant']:<15} frames {r['frames']:4}  median {r['median']:5.1f} ms  "
      f"p95 {r['p95']:5.1f} ms  janky {r['janky']:3} ({100 * r['janky'] / max(r['frames'], 1):3.0f} %)  "
      f"scrolled {r['scrolled']} px  cards {r['cards']}")
if r.get("causes"):
    print("        slow frames by cause:", ", ".join(f"{k}={v}" for k, v in sorted(r["causes"].items(), key=lambda kv: -kv[1])))
PY
$use_sysprof && echo "capture: $capture"
exit 0
