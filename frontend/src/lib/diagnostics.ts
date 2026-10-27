import { prefs } from "./prefs.svelte";
import type { Backend } from "./types";

// The in-app performance report (Settings → Performance): lets a tester on
// another machine measure real wheel scrolling and send the result as text,
// without a terminal or profiling tools. Keys are English on purpose: the
// report is meant to be pasted to the developer.

export interface ScrollStats {
  frames: number;
  median: number;
  p95: number;
  janky: number;
  cards: number;
  scrolled: number;
}

/** GPU as WebGL reports it; "llvmpipe"/"softpipe" means WebKit is painting in software. */
function gpu(): string {
  try {
    const gl = document.createElement("canvas").getContext("webgl");
    if (!gl) return "no WebGL";
    const info = gl.getExtension("WEBGL_debug_renderer_info");
    const renderer = info ? gl.getParameter(info.UNMASKED_RENDERER_WEBGL) : gl.getParameter(gl.RENDERER);
    const vendor = info ? gl.getParameter(info.UNMASKED_VENDOR_WEBGL) : gl.getParameter(gl.VENDOR);
    return `${vendor} / ${renderer}`;
  } catch {
    return "unknown";
  }
}

export async function buildReport(
  api: Backend,
  idleFps: number,
  scroll: ScrollStats,
  channelsTotal: number,
): Promise<string> {
  let machine: Record<string, string> = {};
  try {
    machine = await api.diagnostics();
  } catch {
    // The report is still useful without the backend part.
  }
  const fps = (ms: number) => (ms > 0 ? Math.round(1000 / ms) : 0);
  const lines = [
    `Herakleo performance report (${api.name})`,
    `webview:       ${machine.webview ?? "?"}`,
    `os:            ${machine.os || "?"}`,
    `session:       ${machine.session || "?"} / ${machine.desktop || "?"}`,
    `gpu (webgl):   ${gpu()}`,
    `screen:        ${screen.width}x${screen.height} @ ${devicePixelRatio}x scale, window ${innerWidth}x${innerHeight}`,
    `idle:          ${idleFps.toFixed(0)} fps (≈ monitor refresh rate)`,
    `settings:      browse=${prefs.browse}, effects=${prefs.effects ? "on" : "off"}`,
    scroll.frames
      ? `wheel scroll:  median ${scroll.median.toFixed(1)} ms (${fps(scroll.median)} fps), p95 ${scroll.p95.toFixed(1)} ms, ` +
        `janky ${scroll.janky}/${scroll.frames} (${Math.round((100 * scroll.janky) / scroll.frames)} %), scrolled ${scroll.scrolled} px`
      : "wheel scroll:  not measured (no scrolling detected)",
    `channels:      ${channelsTotal} in view, ${scroll.cards} cards in the page`,
  ];
  const env = Object.entries(machine).filter(([k]) => k.startsWith("env."));
  lines.push(env.length ? `env:           ${env.map(([k, v]) => `${k.slice(4)}=${v}`).join(" ")}` : "env:           (none set)");
  lines.push(`user agent:    ${navigator.userAgent}`);
  return lines.join("\n");
}
