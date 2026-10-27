import React from "react";
import { AbsoluteFill, useCurrentFrame } from "remotion";
import { ease, fade } from "../anim";
import { ambient, C } from "../theme";
import { SCENES } from "../timing";
import { Brand, SIDEBAR_BRAND } from "./Brand";

const Heart = () => (
  <svg width="30" height="30" viewBox="0 0 24 24" style={{ verticalAlign: "-6px" }}>
    <path
      fill="#ef4444"
      d="M20.84 4.61a5.5 5.5 0 0 0-7.78 0L12 5.67l-1.06-1.06a5.5 5.5 0 0 0-7.78 7.78l1.06 1.06L12 21.23l7.78-7.78 1.06-1.06a5.5 5.5 0 0 0 0-7.78z"
    />
  </svg>
);

// Drawn rather than an emoji: headless Chrome may not ship a flag font.
const Spain = () => (
  <svg width="36" height="24" viewBox="0 0 3 2" style={{ verticalAlign: "-4px", borderRadius: 3 }}>
    <rect width="3" height="2" fill="#c60b1e" />
    <rect y="0.5" width="3" height="1" fill="#ffc400" />
  </svg>
);

/** The logo leaves the sidebar slot for the centre: free, open source, where to get it. */
export const Close: React.FC = () => {
  const f = useCurrentFrame();
  const len = SCENES.close.to - SCENES.close.from;
  const go = [4, 30];
  const end = 1 - fade(f, len - 20, len - 2);
  const line = (a: number) => ({ opacity: fade(f, a, a + 12), transform: `translateY(${(1 - fade(f, a, a + 12)) * 14}px)` });

  return (
    <AbsoluteFill style={{ background: ambient, opacity: end }}>
      <Brand
        cx={ease(f, go, [SIDEBAR_BRAND.cx, 960])}
        cy={ease(f, go, [SIDEBAR_BRAND.cy, 330])}
        size={ease(f, go, [SIDEBAR_BRAND.size, 240])}
        title={ease(f, go, [SIDEBAR_BRAND.title, 84])}
        glow={fade(f, go[0]!, go[1]!)}
      />
      <div style={{ position: "absolute", left: 0, right: 0, top: 590, textAlign: "center" }}>
        <div style={{ fontSize: 52, fontWeight: 600, color: C.text, ...line(30) }}>Free and open source.</div>
        <div style={{ fontSize: 34, fontWeight: 500, color: C.accent, marginTop: 26, ...line(52) }}>github.com/crstian19/herakleo</div>
        <div style={{ fontSize: 26, color: C.muted, marginTop: 70, ...line(80) }}>
          Made with <Heart /> from <Spain /> with Wails
        </div>
      </div>
    </AbsoluteFill>
  );
};
