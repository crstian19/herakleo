import React from "react";
import { AbsoluteFill, Easing, useCurrentFrame } from "remotion";
import { ease, fade } from "../anim";
import { MANY } from "../data";
import { C, ambient } from "../theme";
import { DROP, SCENES } from "../timing";
import { Brand, SIDEBAR_BRAND } from "./Brand";

/** 0 → 65,081: the size of a real provider list. */
export const ColdOpen: React.FC = () => {
  const f = useCurrentFrame();
  const len = SCENES.cold.to - SCENES.cold.from;
  const n = Math.round(ease(f, [12, 150], [0, 65081], Easing.out(Easing.cubic)));
  const out = 1 - fade(f, len - 16, len);
  return (
    <AbsoluteFill style={{ background: C.bg, alignItems: "center", justifyContent: "center", opacity: out }}>
      <div style={{ transform: `scale(${ease(f, [0, len], [0.94, 1.06], Easing.linear)})`, textAlign: "center", opacity: fade(f, 0, 12) }}>
        <div style={{ fontSize: 220, fontWeight: 800, letterSpacing: "-0.05em", fontVariantNumeric: "tabular-nums", color: C.text }}>
          {n.toLocaleString("en")}
        </div>
        <div style={{ fontSize: 44, fontWeight: 500, color: C.muted, marginTop: -10, letterSpacing: "0.02em" }}>channels</div>
      </div>
    </AbsoluteFill>
  );
};

/** The problem: an endless wall of names flying past. */
export const Wall: React.FC = () => {
  const f = useCurrentFrame();
  const len = SCENES.wall.to - SCENES.wall.from;
  const cols = 7;
  return (
    <AbsoluteFill style={{ background: C.bg, overflow: "hidden", opacity: fade(f, 0, 14) * (1 - fade(f, len - 14, len)) }}>
      <div style={{ position: "absolute", inset: 0, display: "flex", gap: 36, padding: "0 60px", transform: "rotate(-6deg) scale(1.2)" }}>
        {Array.from({ length: cols }, (_, c) => {
          const speed = 9 + ((c * 37) % 7) * 2.2;
          return (
            <div key={c} style={{ flex: 1, display: "flex", flexDirection: "column", gap: 18, transform: `translateY(${-((f * speed) % 2400) - c * 140}px)` }}>
              {Array.from({ length: 90 }, (_, i) => {
                const ch = MANY[(i * 11 + c * 29) % MANY.length]!;
                return (
                  <div key={i} style={{ fontSize: 30, fontWeight: 600, whiteSpace: "nowrap", color: `rgba(248,250,252,${0.18 + ((i * 7 + c) % 5) * 0.08})` }}>
                    {ch.name}
                  </div>
                );
              })}
            </div>
          );
        })}
      </div>
      <AbsoluteFill style={{ background: `radial-gradient(ellipse at center, transparent 30%, ${C.bg} 85%)` }} />
    </AbsoluteFill>
  );
};

const CX = 960;
const CY = 420;

/** A play button is traced, fills, flashes and bursts into the logo on the drop. */
export const Reveal: React.FC = () => {
  const f = useCurrentFrame();
  const len = SCENES.reveal.to - SCENES.reveal.from;
  const drop = DROP - SCENES.reveal.from;

  const trace = ease(f, [4, drop - 8], [1, 0]);
  const fill = fade(f, drop - 10, drop - 2);
  const playScale = ease(f, [drop - 4, drop], [1, 1.25]) * (f >= drop ? 0 : 1);
  const flash = f >= drop ? 1 - fade(f, drop, drop + 16) : 0;
  const burst = fade(f, drop, drop + 26);

  // Logo pops in on the drop, then travels into the sidebar slot.
  const settle = [len - 40, len - 4];
  const size0 = ease(f, [drop, drop + 10], [180, 300], Easing.out(Easing.back(1.6)));
  const cx = ease(f, settle, [CX, SIDEBAR_BRAND.cx]);
  const cy = ease(f, settle, [CY, SIDEBAR_BRAND.cy]);
  const size = f < settle[0]! ? size0 : ease(f, settle, [300, SIDEBAR_BRAND.size]);
  const title = ease(f, settle, [96, SIDEBAR_BRAND.title]);

  return (
    <AbsoluteFill style={{ background: f < drop ? C.bg : ambient, overflow: "hidden" }}>
      {f >= drop ? (
        <AbsoluteFill style={{ background: C.bg, opacity: 1 - fade(f, drop, drop + 20) }} />
      ) : null}
      {f < drop ? (
        <svg width="300" height="300" viewBox="0 0 100 100" style={{ position: "absolute", left: CX - 150, top: CY - 150, transform: `scale(${playScale})` }}>
          <path
            d="M34 22 L80 50 L34 78 Z"
            fill={`rgba(224,176,74,${fill})`}
            stroke={C.accent}
            strokeWidth={3}
            strokeLinejoin="round"
            pathLength={1}
            strokeDasharray={1}
            strokeDashoffset={trace}
          />
        </svg>
      ) : null}
      {f >= drop && burst < 1
        ? Array.from({ length: 18 }, (_, i) => {
            const a = (i / 18) * Math.PI * 2 + 0.2;
            const r = 140 + 300 * burst;
            const s = (1 - burst) * (i % 3 === 0 ? 14 : 9);
            return (
              <div
                key={i}
                style={{
                  position: "absolute",
                  left: CX + Math.cos(a) * r - s / 2,
                  top: CY + Math.sin(a) * r - s / 2,
                  width: s,
                  height: s,
                  borderRadius: "50%",
                  background: i % 2 ? C.accent : "#f8e3b0",
                }}
              />
            );
          })
        : null}
      {f >= drop ? (
        <Brand cx={cx} cy={cy} size={size} title={title} titleOpacity={fade(f, drop + 10, drop + 22)} glow={1 - fade(f, settle[0]!, settle[1]!)} />
      ) : null}
      <AbsoluteFill style={{ background: "#fff8e6", opacity: flash * 0.85 }} />
    </AbsoluteFill>
  );
};
