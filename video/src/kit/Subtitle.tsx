import React from 'react';
import { AbsoluteFill, interpolate, Sequence, useCurrentFrame } from 'remotion';

/**
 * Subtitle for a product video that will be watched WITHOUT SOUND.
 *
 * The first version was 38 px text with a drop shadow and was unreadable,
 * because it competes with an interface already full of text. What works is
 * dimming the background with a gradient and raising the text to 54 px at
 * medium weight. Checked over both dark and light UI.
 *
 * A scripting rule that cuts noise: only subtitle lines that are NOT already
 * written on screen. If the line appears on a full-screen title card,
 * subtitling it puts the same text twice in the same place.
 */
export const Subtitle: React.FC<{
  text: string;
  opacity: number;
  fontFamily?: string;
  /** Gradient height as a fraction of the canvas height. */
  scrim?: number;
  fontSize?: number;
  color?: string;
  /** Gradient base colour, as rgb without alpha. */
  scrimColor?: string;
}> = ({
  text,
  opacity,
  fontFamily = 'system-ui, sans-serif',
  scrim = 0.32,
  fontSize = 54,
  color = '#ffffff',
  scrimColor = '5,7,12',
}) => (
  <AbsoluteFill style={{ opacity, zIndex: 90 }}>
    <div
      style={{
        position: 'absolute',
        left: 0,
        right: 0,
        bottom: 0,
        height: `${scrim * 100}%`,
        background: `linear-gradient(to top, rgba(${scrimColor},0.94) 0%, rgba(${scrimColor},0.86) 38%, rgba(${scrimColor},0.55) 68%, rgba(${scrimColor},0) 100%)`,
      }}
    />
    <div style={{ position: 'absolute', left: '8%', right: '8%', bottom: '6.4%', textAlign: 'center' }}>
      <div
        style={{
          fontFamily,
          fontSize,
          fontWeight: 500,
          lineHeight: 1.24,
          color,
          letterSpacing: '-0.018em',
          textShadow: '0 2px 22px rgba(0,0,0,0.9)',
        }}
      >
        {text}
      </div>
    </div>
  </AbsoluteFill>
);

export type SubtitleCue = {
  /** Frame where the cue appears. */
  at: number;
  /** Frames ON SCREEN, not the voice duration: the cue enters earlier and stays longer. */
  dur: number;
  text: string;
};

/**
 * Renders a list of cues. Trim the ranges by hand so no cue overlaps the next
 * one or the title card after it; that is the easiest mistake to make.
 */
export const Subtitles: React.FC<{
  cues: readonly SubtitleCue[];
  fontFamily?: string;
}> = ({ cues, fontFamily }) => (
  <>
    {cues.map((c) => (
      <Sequence key={`${c.at}-${c.text}`} from={c.at} durationInFrames={c.dur} layout="none">
        <Cue frames={c.dur} text={c.text} fontFamily={fontFamily} />
      </Sequence>
    ))}
  </>
);

const Cue: React.FC<{ frames: number; text: string; fontFamily?: string }> = ({ frames, text, fontFamily }) => {
  const f = useCurrentFrame();
  const opacity = interpolate(f, [0, 6, frames - 10, frames - 2], [0, 1, 1, 0], {
    extrapolateLeft: 'clamp',
    extrapolateRight: 'clamp',
  });
  return <Subtitle text={text} opacity={opacity} fontFamily={fontFamily} />;
};
