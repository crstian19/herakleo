import { Audio } from "@remotion/media";
import React from "react";
import { AbsoluteFill, Sequence, staticFile } from "remotion";
import { DEFAULT_DUCK, duckVolume } from "./kit/audio";
import { Subtitles } from "./kit/Subtitle";
import { Close } from "./scenes/Close";
import { ColdOpen, Reveal, Wall } from "./scenes/Intro";
import { Product } from "./scenes/Product";
import { C, SANS } from "./theme";
import { SCENES, SUBTITLES, TOTAL, VO } from "./timing";

// Measured with volumedetect: at the kit's defaults the music still added
// 2.6 dB under the voice and the mix peaked at -0.1 dB.
const DUCK = { ...DEFAULT_DUCK, base: 0.75, under: 0.12 };

const scene = (s: { from: number; to: number }) => ({ from: s.from, durationInFrames: s.to - s.from });

export const Video: React.FC = () => (
  <AbsoluteFill style={{ background: C.bg, fontFamily: SANS, color: C.text }}>
    <Sequence {...scene(SCENES.cold)}>
      <ColdOpen />
    </Sequence>
    <Sequence {...scene(SCENES.wall)}>
      <Wall />
    </Sequence>
    <Sequence {...scene(SCENES.reveal)}>
      <Reveal />
    </Sequence>
    <Sequence {...scene(SCENES.product)}>
      <Product />
    </Sequence>
    <Sequence {...scene(SCENES.close)}>
      <Close />
    </Sequence>

    <Subtitles cues={SUBTITLES} fontFamily={SANS} />

    <Audio
      src={staticFile("music/aco1.mp3")}
      trimAfter={TOTAL}
      volume={(f) => duckVolume(f, VO, DUCK, { total: TOTAL })}
    />
    {VO.map((v) => (
      <Sequence key={v.id} from={v.at} durationInFrames={v.frames + 6}>
        <Audio src={staticFile(`vo/vo-${v.id}.wav`)} />
      </Sequence>
    ))}
  </AbsoluteFill>
);
