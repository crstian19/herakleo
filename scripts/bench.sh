#!/usr/bin/env bash
# Benchmarks both release builds end to end against a copy of a library:
# time until the window maps, time until the first page of channels is on
# screen, API round-trips measured inside the webview (lib/bench.ts) and peak
# RSS of the app plus its WebKit helpers. Usage: scripts/bench.sh [library-dir] [runs]
set -euo pipefail

src=${1:-"${XDG_CONFIG_HOME:-$HOME/.config}/herakleo"}
runs=${2:-3}
root=$(cd "$(dirname "$0")/.." && pwd)
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

# Work on a copy: the apps rewrite library.json and colors.json.
mkdir -p "$work/config"
cp -r "$src" "$work/config/herakleo"

declare -A bins=(
  [tauri]="$root/tauri/src-tauri/target/release/herakleo-tauri"
  [wails]="$root/wails/build/bin/herakleo"
)

tree_rss_kb() { # RSS of a process and all its descendants
  local pids
  # -T: no threads, which would count the same process's memory many times.
  pids=$(pstree -pT "$1" 2>/dev/null | grep -o '([0-9]\+)' | tr -d '()' | paste -sd, -)
  # awk always prints exactly one number, even if a process exits mid-read.
  if [ -n "$pids" ]; then ps -o rss= -p "$pids" 2>/dev/null | awk '{s+=$1} END {print s+0}'; else echo 0; fi
}

per_process() { # "name=MiB ..." for the app and each WebKit helper
  local pids
  pids=$(pstree -pT "$1" 2>/dev/null | grep -o '([0-9]\+)' | tr -d '()' | paste -sd, -)
  ps -o comm=,rss= -p "$pids" 2>/dev/null | awk '{printf "%s=%d ", $1, $2/1024}'
}

for app in tauri wails; do
  bin=${bins[$app]}
  [ -x "$bin" ] || { echo "missing $bin (run: just build)" >&2; exit 1; }
  for run in $(seq "$runs"); do
    out="$work/$app-$run.json"
    start=$(date +%s%N)
    start_ms=$(( start / 1000000 ))
    XDG_CONFIG_HOME="$work/config" HERAKLEO_BENCH_OUT="$out" "$bin" >/dev/null 2>&1 &
    pid=$!
    window_ms="" peak=0
    breakdown=""
    while kill -0 "$pid" 2>/dev/null; do
      if [ -z "$window_ms" ] && command -v hyprctl >/dev/null &&
        hyprctl clients -j | grep -q "\"pid\": $pid,"; then
        window_ms=$(( ($(date +%s%N) - start) / 1000000 ))
      fi
      rss=$(tree_rss_kb "$pid"); (( rss > peak )) && peak=$rss && breakdown=$(per_process "$pid")
      sleep 0.05
      (( ($(date +%s%N) - start) / 1000000000 > 180 )) && { kill "$pid"; echo "$app timed out" >&2; break; }
    done
    python3 -I - "$out" "$app" "$run" "${window_ms:-?}" "$start_ms" "$peak" "$breakdown" <<'PY'
import json, sys
path, app, run, window, start_ms, peak, breakdown = sys.argv[1:]
r = json.load(open(path))
rows = " ".join(f"{x['name']}={x['median']:.2f}" for x in r["rows"])
first = r["firstPageAt"] - int(start_ms)
print(f"{app} run {run}: window {window} ms | first page {first} ms | "
      f"peak RSS {int(peak)//1024} MiB ({breakdown.strip()}) | {r['channels']} channels | median ms: {rows}")
s = r.get("scroll")
if "fps" in r:
    print(f"    idle: {r['fps']:.1f} fps")
for name, v in (r.get("variants") or {}).items():
    print(f"    {name:<22} frame median {v['median']:5.1f} ms  p95 {v['p95']:5.1f} ms  janky {100*v['janky']/v['frames']:3.0f} %  cards {v['cards']}")
if s:
    print(f"    scroll: {s['frames']} frames, frame median {s['median']:.1f} ms, p95 {s['p95']:.1f} ms, "
          f"janky {s['janky']} ({100*s['janky']/s['frames']:.0f} %), {s['cards']} cards in DOM")
PY
  done
done
