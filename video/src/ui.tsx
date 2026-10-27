import React from "react";
import { Img, staticFile } from "remotion";
import { CHANNELS, type DemoChannel, GROUPS } from "./data";
import { C, SANS } from "./theme";

// A reconstruction of Herakleo's UI in a fixed 1920×1080 design space, so the
// camera can frame any part of it and text stays sharp at any zoom. Sizes and
// colours follow frontend/src/app.css; positions below are referenced by the
// cursor path in scenes/Product.tsx.

export const LAYOUT = {
  sidebar: 320,
  addButton: { x: 258, y: 412 },
  search: { x: 1500, y: 68 },
  grid: { x: 360, y: 140, col: 200, gap: 20, cols: 7 },
  dialog: { x: 640, y: 300, w: 640, h: 440 },
  dialogField: { x: 960, y: 548 },
  dialogAdd: { x: 1196, y: 690 },
};

export const cardCenter = (index: number) => {
  const g = LAYOUT.grid;
  const col = index % g.cols;
  const row = Math.floor(index / g.cols);
  return { x: g.x + col * (g.col + g.gap) + g.col / 2, y: g.y + row * (g.col + g.gap) + g.col / 2 };
};

const chip = (active: boolean): React.CSSProperties => ({
  display: "flex",
  alignItems: "center",
  justifyContent: "space-between",
  height: 40,
  padding: "0 14px",
  borderRadius: 12,
  fontSize: 17,
  color: active ? C.text : C.muted,
  background: active ? "rgba(224,176,74,0.16)" : "transparent",
  border: `1px solid ${active ? "rgba(224,176,74,0.45)" : "transparent"}`,
});

const sectionTitle: React.CSSProperties = {
  fontSize: 13,
  fontWeight: 700,
  letterSpacing: "0.12em",
  textTransform: "uppercase",
  color: C.muted,
  borderLeft: `3px solid ${C.accent}`,
  paddingLeft: 10,
};

/** `rest` fades everything but the logo and title, so scenes can hand the logo over. */
export const Sidebar: React.FC<{ rest?: number; hasList?: boolean }> = ({ rest = 1, hasList = true }) => (
  <div
    style={{
      position: "absolute",
      inset: "0 auto 0 0",
      width: LAYOUT.sidebar,
      background: `rgba(12,12,18,${0.7 * rest})`,
      borderRight: `1px solid rgba(255,255,255,${0.08 * rest})`,
      padding: "34px 18px",
      display: "flex",
      flexDirection: "column",
      gap: 26,
    }}
  >
    <div style={{ display: "flex", flexDirection: "column", alignItems: "center", gap: 10 }}>
      <Img src={staticFile("logo-wails.png")} style={{ width: 128, height: 128, imageRendering: "pixelated" }} />
      <div style={{ fontSize: 32, fontWeight: 700, letterSpacing: "-0.02em" }}>Herakleo</div>
    </div>
    <div style={{ display: "flex", flexDirection: "column", gap: 4, opacity: rest }}>
      <div style={chip(true)}>All</div>
      <div style={chip(false)}>★ Favourites</div>
      <div style={chip(false)}>↺ Recent</div>
    </div>
    <div style={{ opacity: rest }}>
      <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", marginBottom: 10 }}>
        <span style={sectionTitle}>Lists</span>
        <span style={{ fontSize: 14, color: C.muted, border: `1px solid ${C.line}`, borderRadius: 10, padding: "4px 10px" }}>+ Add</span>
      </div>
      {hasList ? (
        <div style={chip(false)}>
          <span>My list</span>
          <span style={{ fontSize: 14 }}>65,081</span>
        </div>
      ) : (
        <div style={{ fontSize: 15, color: C.muted, padding: "8px 14px" }}>No lists yet</div>
      )}
    </div>
    <div style={{ opacity: rest * (hasList ? 1 : 0) }}>
      <div style={{ ...sectionTitle, marginBottom: 10 }}>Groups</div>
      {GROUPS.map((g, i) => (
        <div key={g} style={chip(i === -1)}>
          <span>{g}</span>
          <span style={{ fontSize: 14 }}>{[412, 263, 1880, 940, 305, 512, 377, 288][i]}</span>
        </div>
      ))}
    </div>
  </div>
);

export const Header: React.FC<{ query: string; caret: boolean; total: number; opacity?: number }> = ({ query, caret, total, opacity = 1 }) => (
  <div style={{ position: "absolute", left: LAYOUT.sidebar + 40, right: 40, top: 34, height: 70, display: "flex", alignItems: "center", gap: 18, opacity }}>
    <div style={{ flex: 1 }}>
      <div style={{ display: "flex", alignItems: "center", gap: 12 }}>
        <span style={{ fontSize: 34, fontWeight: 650, letterSpacing: "-0.02em" }}>All channels</span>
      </div>
      <div style={{ fontSize: 17, color: C.muted, marginTop: 2 }}>{total ? `${total.toLocaleString("en")} channels` : "No channels yet"}</div>
    </div>
    <div style={{ width: 420, height: 52, borderRadius: 14, border: `1px solid ${caret ? "rgba(224,176,74,0.6)" : C.lineHi}`, background: "rgba(255,255,255,0.05)", display: "flex", alignItems: "center", gap: 10, padding: "0 16px", fontSize: 19 }}>
      <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke={C.muted} strokeWidth="2"><circle cx="11" cy="11" r="7" /><path d="m20 20-3.5-3.5" /></svg>
      <span style={{ color: query ? C.text : "rgba(148,163,184,0.8)" }}>{query || "Search channels…"}</span>
      {caret ? <span style={{ width: 2, height: 24, background: C.accent, marginLeft: -6 }} /> : null}
    </div>
    <div style={{ height: 52, borderRadius: 14, border: `1px solid ${C.lineHi}`, display: "flex", alignItems: "center", padding: "0 16px", fontSize: 18, color: C.muted }}>⇅ List order</div>
  </div>
);

export const ChannelCard: React.FC<{ c: DemoChannel; hover?: number; playing?: boolean }> = ({ c, hover = 0, playing }) => {
  const color = `hsl(${c.hue} 70% 60%)`;
  return (
    <div
      style={{
        position: "relative",
        width: LAYOUT.grid.col,
        height: LAYOUT.grid.col,
        borderRadius: 22,
        background: C.surface,
        boxShadow: `inset 0 0 0 1px ${hover || playing ? `hsl(${c.hue} 70% 60% / 0.6)` : C.line}${hover ? `, 0 24px 48px -12px hsl(${c.hue} 70% 60% / ${0.35 * hover})` : ""}`,
        transform: `scale(${1 + 0.04 * hover}) translateY(${-3 * hover}px)`,
        display: "flex",
        flexDirection: "column",
        alignItems: "center",
        justifyContent: "center",
        gap: 16,
        overflow: "hidden",
      }}
    >
      {hover || playing ? (
        <div style={{ position: "absolute", inset: 0, background: `radial-gradient(circle at 50% 40%, hsl(${c.hue} 70% 60% / 0.18), transparent 60%)`, opacity: playing ? 1 : hover }} />
      ) : null}
      <div style={{ width: 84, height: 84, borderRadius: 24, display: "grid", placeItems: "center", background: `hsl(${c.hue} 70% 60% / 0.14)`, color, fontSize: 40 }}>{c.mark}</div>
      <div style={{ fontSize: 17, fontWeight: 600, color: "rgba(248,250,252,0.9)", textAlign: "center", padding: "0 14px", lineHeight: 1.25 }}>{c.name}</div>
    </div>
  );
};

export const Grid: React.FC<{
  channels: DemoChannel[];
  scroll: number;
  hoverIndex?: number;
  hover?: number;
  playingIndex?: number;
  cardOpacity?: (i: number) => number;
}> = ({ channels, scroll, hoverIndex = -1, hover = 0, playingIndex = -1, cardOpacity = () => 1 }) => {
  const g = LAYOUT.grid;
  return (
    <div style={{ position: "absolute", left: g.x, top: g.y, right: 40, bottom: 0, overflow: "hidden" }}>
      <div
        style={{
          display: "grid",
          gridTemplateColumns: `repeat(${g.cols}, ${g.col}px)`,
          gap: g.gap,
          transform: `translateY(${-scroll}px)`,
        }}
      >
        {channels.map((c, i) => (
          <div key={`${c.name}-${i}`} style={{ opacity: cardOpacity(i), transform: `translateY(${(1 - cardOpacity(i)) * 16}px)` }}>
            <ChannelCard c={c} hover={i === hoverIndex ? hover : 0} playing={i === playingIndex} />
          </div>
        ))}
      </div>
    </div>
  );
};

export const AddDialog: React.FC<{ opacity: number; scale: number; url: string; caret: boolean; pressed: number }> = ({ opacity, scale, url, caret, pressed }) => {
  const d = LAYOUT.dialog;
  return (
    <div style={{ position: "absolute", inset: 0, background: `rgba(0,0,0,${0.75 * opacity})` }}>
      <div
        style={{
          position: "absolute",
          left: d.x,
          top: d.y,
          width: d.w,
          height: d.h,
          opacity,
          transform: `scale(${scale})`,
          borderRadius: 28,
          border: `1px solid ${C.line}`,
          background: "#11111a",
          padding: 34,
          boxShadow: "0 30px 80px -20px rgba(0,0,0,0.8)",
        }}
      >
        <div style={{ fontSize: 26, fontWeight: 650 }}>Add list</div>
        <div style={{ fontSize: 17, color: C.muted, marginTop: 4 }}>Downloaded once and kept locally.</div>
        <div style={{ display: "grid", gridTemplateColumns: "repeat(3, 1fr)", gap: 6, padding: 6, marginTop: 24, borderRadius: 14, background: "rgba(255,255,255,0.05)" }}>
          {["M3U by URL", "M3U file", "Xtream Codes"].map((t, i) => (
            <div key={t} style={{ ...chip(i === 0), justifyContent: "center", fontSize: 15 }}>{t}</div>
          ))}
        </div>
        <div style={{ marginTop: 22, height: 50, borderRadius: 14, border: `1px solid ${C.lineHi}`, display: "flex", alignItems: "center", padding: "0 16px", fontSize: 18, color: C.text }}>My list</div>
        <div style={{ marginTop: 12, height: 50, borderRadius: 14, border: `1px solid ${caret ? "rgba(224,176,74,0.6)" : C.lineHi}`, display: "flex", alignItems: "center", padding: "0 16px", fontSize: 18 }}>
          <span style={{ color: url ? C.text : "rgba(148,163,184,0.8)" }}>{url || "https://provider.tv/list.m3u"}</span>
          {caret ? <span style={{ width: 2, height: 24, background: C.accent, marginLeft: 2 }} /> : null}
        </div>
        <div style={{ position: "absolute", right: 34, bottom: 30, display: "flex", gap: 10 }}>
          <div style={{ height: 48, padding: "0 20px", borderRadius: 14, border: `1px solid ${C.line}`, display: "flex", alignItems: "center", color: C.muted, fontSize: 17 }}>Cancel</div>
          <div
            style={{
              height: 48,
              padding: "0 26px",
              borderRadius: 14,
              display: "flex",
              alignItems: "center",
              fontSize: 17,
              fontWeight: 650,
              color: "#1a1206",
              background: `linear-gradient(135deg, ${C.accent}, ${C.accentDeep})`,
              transform: `scale(${1 - 0.04 * pressed})`,
            }}
          >
            Add
          </div>
        </div>
      </div>
    </div>
  );
};

export const Toast: React.FC<{ text: string; opacity: number }> = ({ text, opacity }) => (
  <div
    style={{
      position: "absolute",
      left: "50%",
      bottom: 50,
      transform: `translate(-50%, ${(1 - opacity) * 14}px)`,
      opacity,
      padding: "14px 22px",
      borderRadius: 18,
      border: "1px solid rgba(224,176,74,0.3)",
      background: "#111118",
      fontSize: 18,
      whiteSpace: "nowrap",
    }}
  >
    {text}
  </div>
);

/** A pointer that keeps its size whatever the camera zoom is. */
export const Cursor: React.FC<{ x: number; y: number; camScale: number; click: number }> = ({ x, y, camScale, click }) => (
  <div style={{ position: "absolute", left: x, top: y, transform: `scale(${1 / camScale})`, transformOrigin: "0 0", pointerEvents: "none" }}>
    {click > 0 ? (
      <div
        style={{
          position: "absolute",
          left: -22,
          top: -22,
          width: 44,
          height: 44,
          borderRadius: "50%",
          border: `2px solid ${C.accent}`,
          opacity: 1 - click,
          transform: `scale(${0.4 + click})`,
        }}
      />
    ) : null}
    <svg width="34" height="34" viewBox="0 0 24 24" style={{ filter: "drop-shadow(0 2px 4px rgba(0,0,0,0.6))" }}>
      <path d="M4 2.5v17.2l4.6-4.2 3 6.6 3-1.4-3-6.5h6.2z" fill="#fff" stroke="#111" strokeWidth="1.2" strokeLinejoin="round" />
    </svg>
  </div>
);

/** The external player: a fictional broadcast in an mpv-style window. */
export const MpvWindow: React.FC<{ x: number; frame: number; c: DemoChannel }> = ({ x, frame, c }) => {
  const color = `hsl(${c.hue} 70% 55%)`;
  return (
    <div
      style={{
        position: "absolute",
        left: x,
        top: 170,
        width: 1180,
        height: 700,
        borderRadius: 16,
        overflow: "hidden",
        boxShadow: "0 40px 120px -20px rgba(0,0,0,0.9), 0 0 0 1px rgba(255,255,255,0.08)",
        background: "#000",
        fontFamily: SANS,
      }}
    >
      <div style={{ height: 44, background: "#1c1c22", display: "flex", alignItems: "center", gap: 10, padding: "0 16px", fontSize: 16, color: "#d4d4d8" }}>
        <span style={{ width: 12, height: 12, borderRadius: 6, background: "#3f3f46" }} />
        <span style={{ width: 12, height: 12, borderRadius: 6, background: "#3f3f46" }} />
        <span style={{ marginLeft: 10 }}>Herakleo · {c.name} — mpv</span>
      </div>
      <div
        style={{
          position: "relative",
          height: 656,
          background: `radial-gradient(900px 500px at ${30 + 10 * Math.sin(frame / 40)}% 30%, hsl(${c.hue} 60% 30%), #05070d 70%)`,
          overflow: "hidden",
        }}
      >
        <div style={{ position: "absolute", left: 60, top: 60, fontSize: 18, fontWeight: 700, letterSpacing: "0.2em", color: "#fff", background: "#dc2626", borderRadius: 6, padding: "4px 10px" }}>LIVE</div>
        <div style={{ position: "absolute", right: 60, top: 54, fontSize: 30, fontWeight: 800, color, letterSpacing: "-0.02em" }}>{c.mark} {c.name.toUpperCase()}</div>
        <div style={{ position: "absolute", left: 0, right: 0, bottom: 70, padding: "22px 60px", background: "linear-gradient(90deg, rgba(0,0,0,0.85), rgba(0,0,0,0.3))" }}>
          <div style={{ fontSize: 40, fontWeight: 700, color: "#fff" }}>Morning on the Nile</div>
          <div style={{ fontSize: 22, color: "#cbd5e1", marginTop: 6 }}>Weather, river traffic and the day ahead</div>
        </div>
        <div style={{ position: "absolute", left: 0, right: 0, bottom: 0, height: 70, background: color, display: "flex", alignItems: "center", overflow: "hidden" }}>
          <div style={{ whiteSpace: "nowrap", fontSize: 24, fontWeight: 600, color: "#0b0b10", transform: `translateX(${1180 - frame * 6}px)` }}>
            Sunny over Luxor · 31°C · Feluccas back on schedule · Papyrus Kids at 10:00 · Sphinx Cinema tonight: The Golden Reel ·
          </div>
        </div>
      </div>
    </div>
  );
};

export { CHANNELS };
