import React from "react";
import { AbsoluteFill, useCurrentFrame } from "remotion";
import { blink, clickAt, ease, fade, path, typed } from "../anim";
import { CHANNELS, MANY } from "../data";
import { moves, useCamera } from "../kit/camera";
import { ambient, C } from "../theme";
import { HEIGHT, SCENES, WIDTH } from "../timing";
import { AddDialog, cardCenter, Cursor, Grid, Header, LAYOUT, MpvWindow, Sidebar, Toast } from "../ui";
import { Brand, SIDEBAR_BRAND } from "./Brand";

// One continuous shot of the app: add a list, search, play, scroll.
// All times are frames from the start of the scene (VO 04–07 land at
// 30, 222, 352 and 520, see timing.ts).

const URL = "https://iptv.example/my-list.m3u";
const QUERY = "nile";
const NILE = CHANNELS.filter((c) => c.name.toLowerCase().includes(QUERY));

const T = {
  addClick: 46,
  dialogIn: 48,
  typeUrl: [62, 96],
  confirm: 110,
  loaded: 116,
  searchClick: 204,
  typeQuery: [214, 230],
  playClick: 344,
  mpvIn: [352, 378],
  mpvOut: [470, 492],
  cleared: 492,
  scroll: [500, 628],
} as const;

const LEN = SCENES.product.to - SCENES.product.from;
const DESIGN = { width: WIDTH, height: HEIGHT };

const camera = moves([
  { at: 0, fx: 960, fy: 540, s: 1 },
  { at: 60, fx: 960, fy: 520, s: 1.3 },
  { at: 128, fx: 960, fy: 540, s: 1 },
  { at: 252, fx: 700, fy: 330, s: 1.45 },
  { at: 364, fx: 960, fy: 540, s: 1 },
]);

const add = { x: 272, y: 405 };
const search = { x: 1500, y: 69 };
const card0 = cardCenter(0);

const cursorKeys = [
  { at: 0, x: 980, y: 760 },
  { at: 14, x: 980, y: 760 },
  { at: 40, ...add },
  { at: 74, ...add },
  { at: 102, x: LAYOUT.dialogAdd.x, y: LAYOUT.dialogAdd.y },
  { at: 182, x: LAYOUT.dialogAdd.x, y: LAYOUT.dialogAdd.y },
  { at: 200, ...search },
  { at: 268, ...search },
  { at: 304, x: card0.x + 10, y: card0.y + 14 },
];

export const Product: React.FC = () => {
  const f = useCurrentFrame();
  const cam = useCamera(camera, f, DESIGN);

  const rest = fade(f, 0, 16) * (1 - fade(f, LEN - 22, LEN - 4));
  const loaded = f >= T.loaded;

  const typedQuery = f >= T.cleared ? "" : typed(f, QUERY, T.typeQuery[0], T.typeQuery[1]);
  const filtered = typedQuery.length >= 3;
  const channels = !loaded ? [] : filtered ? NILE : MANY;
  const total = !loaded ? 0 : filtered ? NILE.length : 65081;
  const scroll = ease(f, T.scroll as unknown as number[], [0, 7600]);

  const dialog = fade(f, T.dialogIn, T.dialogIn + 10) * (1 - fade(f, T.confirm + 2, T.confirm + 12));
  const hover = fade(f, 300, 312) * (1 - fade(f, T.mpvOut[0], T.mpvOut[1]));
  const mpvX = f < T.mpvOut[0] ? ease(f, T.mpvIn as unknown as number[], [WIDTH + 40, 640]) : ease(f, T.mpvOut as unknown as number[], [640, WIDTH + 40]);

  const cursor = path(f, cursorKeys);
  const cursorOpacity = fade(f, 10, 18) * (1 - fade(f, T.cleared, T.cleared + 8));
  const click = clickAt(f, [T.addClick, T.confirm, T.searchClick, T.playClick]);

  // Cards pop in row by row once the list has loaded, and again when the filter changes.
  const since = filtered ? f - T.typeQuery[1] : f >= T.cleared ? f - T.cleared : f - T.loaded;
  const delay = (i: number) => Math.min(Math.floor(i / 7), 5) * 3 + (i % 7);
  const cardOpacity = (i: number) => fade(since, delay(i), delay(i) + 10);

  return (
    <AbsoluteFill style={{ background: ambient, overflow: "hidden" }}>
      <div style={{ position: "absolute", width: WIDTH, height: HEIGHT, transform: cam.transform, transformOrigin: "0 0" }}>
        <Sidebar rest={rest} hasList={loaded} />
        <Brand {...SIDEBAR_BRAND} />
        <Header query={typedQuery} caret={f >= T.searchClick && f < T.cleared && blink(f)} total={total} opacity={rest} />
        <div style={{ opacity: rest }}>
          <Grid channels={channels} scroll={scroll} hoverIndex={0} hover={hover} playingIndex={f >= T.playClick && f < T.cleared ? 0 : -1} cardOpacity={cardOpacity} />
          {!loaded ? (
            <div style={{ position: "absolute", left: LAYOUT.sidebar, right: 0, top: 380, textAlign: "center", color: C.muted }}>
              <div style={{ fontSize: 34, fontWeight: 600, color: C.text }}>No channels yet</div>
              <div style={{ fontSize: 22, marginTop: 10 }}>Add an M3U link or an Xtream account to start.</div>
            </div>
          ) : null}
        </div>
        {dialog > 0 ? (
          <AddDialog
            opacity={dialog}
            scale={0.96 + 0.04 * dialog}
            url={typed(f, URL, T.typeUrl[0], T.typeUrl[1])}
            caret={f < T.confirm && blink(f)}
            pressed={clickAt(f, [T.confirm]) > 0 ? 1 : 0}
          />
        ) : null}
        <div style={{ opacity: cursorOpacity }}>
          <Cursor x={cursor.x} y={cursor.y} camScale={cam.s} click={click} />
        </div>
      </div>
      {mpvX < WIDTH ? <MpvWindow x={mpvX} frame={f} c={NILE[0]!} /> : null}
      <Toast text="✓ My list · 65,081 channels imported" opacity={fade(f, T.confirm + 10, T.confirm + 20) * (1 - fade(f, 186, 196))} />
    </AbsoluteFill>
  );
};
