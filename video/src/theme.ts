import { loadFont } from "@remotion/fonts";
import { staticFile } from "remotion";

// The app's palette (frontend/src/app.css), gold accent.
export const C = {
  bg: "#0a0a0f",
  panel: "#0c0c12",
  surface: "rgba(255,255,255,0.04)",
  line: "rgba(255,255,255,0.08)",
  lineHi: "rgba(255,255,255,0.15)",
  text: "#f8fafc",
  muted: "#94a3b8",
  accent: "#e0b04a",
  accentDeep: "#b8862b",
};

export const SANS = "Geist";

// Same Geist the app ships (variable weight, latin subset).
loadFont({ family: SANS, url: staticFile("fonts/geist.woff2"), weight: "100 900" });

/** The app's ambient background: three soft radial glows, no blur filters. */
export const ambient = `radial-gradient(600px 600px at 10% -5%, rgb(224 176 74 / 0.13), transparent 70%),
  radial-gradient(520px 520px at 100% 105%, rgb(49 46 129 / 0.22), transparent 70%),
  radial-gradient(420px 420px at 55% 45%, rgb(13 148 136 / 0.06), transparent 70%), ${C.bg}`;
