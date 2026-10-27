import React from "react";
import { Img, staticFile } from "remotion";

// The logo with the title under it, laid out exactly like the app's sidebar
// header (logo, 10px gap, title) so it can travel between the reveal, the
// sidebar and the closing card without a visible cut.
export const SIDEBAR_BRAND = { cx: 160, cy: 98, size: 128, title: 32 };

export const Brand: React.FC<{ cx: number; cy: number; size: number; title: number; titleOpacity?: number; glow?: number }> = ({
  cx,
  cy,
  size,
  title,
  titleOpacity = 1,
  glow = 0,
}) => (
  <>
    {glow > 0 ? (
      <div
        style={{
          position: "absolute",
          left: cx - size * 1.4,
          top: cy - size * 1.4,
          width: size * 2.8,
          height: size * 2.8,
          borderRadius: "50%",
          background: `radial-gradient(circle, rgba(224,176,74,${0.35 * glow}), transparent 65%)`,
        }}
      />
    ) : null}
    <Img
      src={staticFile("logo-wails.png")}
      style={{ position: "absolute", left: cx - size / 2, top: cy - size / 2, width: size, height: size, imageRendering: "pixelated" }}
    />
    <div
      style={{
        position: "absolute",
        left: cx - 600,
        width: 1200,
        top: cy + size / 2 + 10 * (size / 128),
        textAlign: "center",
        fontSize: title,
        fontWeight: 700,
        letterSpacing: "-0.02em",
        opacity: titleOpacity,
      }}
    >
      Herakleo
    </div>
  </>
);
