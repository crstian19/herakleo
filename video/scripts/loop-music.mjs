#!/usr/bin/env node
/**
 * Extends a track to the length you need with no audible seam.
 *
 *   node scripts/loop-music.mjs --file track.mp3
 *   node scripts/loop-music.mjs --file track.mp3 --loop 12.227,19.846 --target 88 --out long.mp3
 *
 * Why it exists
 * -------------
 * The obvious approach is duplicating bars: work out the BPM, cut on an exact
 * bar and splice. It doesn't work. We tried 6 bars (breaks the phrase metre) and
 * 16 (a whole phrase, bar-exact, with micro-fades) and both times the seam was
 * audible. It wasn't a click: the music REWINDS to a point with different
 * energy. A crossfade can't fix that, because the problem is the jump in the
 * arrangement, not the transition.
 *
 * What works is ignoring the bar grid and looking for the track's natural GAPS.
 * Most produced music has periodic micro-silences at the same point in the
 * phrase. Cut in the middle of one gap and re-enter in the middle of another gap
 * of the same period, and the loop is inaudible: there is nothing to interrupt.
 *
 * The track this was developed on had 0.41 s gaps exactly every 7.619 s. All
 * four seams in the result measured -53 dB, i.e. silence.
 *
 * Without --loop the script only analyzes: it lists the gaps, infers the period
 * and suggests cut points. Choosing WHICH section repeats is a musical decision,
 * so a person makes it.
 */

import { execFile } from 'node:child_process';
import { mkdtempSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import path from 'node:path';
import { promisify } from 'node:util';

const run = promisify(execFile);

const arg = (name, fallback = null) => {
  const i = process.argv.indexOf(`--${name}`);
  return i >= 0 && process.argv[i + 1] && !process.argv[i + 1].startsWith('--') ? process.argv[i + 1] : fallback;
};

const file = arg('file');
if (!file) {
  console.error('Missing --file <track.mp3>. See the header of this script for the other flags.');
  process.exit(1);
}
const noise = arg('noise', '-32dB');
const minGap = Number(arg('min-gap', '0.06'));
const loopArg = arg('loop');
const target = Number(arg('target', '0'));
const out = arg('out', 'looped.mp3');
const bitrate = arg('bitrate', '192k');

const duration = async (f) => {
  const { stdout } = await run('ffprobe', ['-v', 'error', '-show_entries', 'format=duration', '-of', 'csv=p=0', f]);
  return Number(stdout.trim());
};

/** Mean level of a window, in dBFS. Used to verify the seams. */
const meanDb = async (f, at, len) => {
  const { stderr } = await run(
    'ffmpeg',
    ['-hide_banner', '-ss', String(Math.max(0, at)), '-t', String(len), '-i', f, '-af', 'volumedetect', '-f', 'null', '-'],
    { encoding: 'utf8' }
  ).catch((e) => ({ stderr: e.stderr ?? '' }));
  const m = /mean_volume:\s*(-?[\d.]+) dB/.exec(stderr);
  return m ? Number(m[1]) : NaN;
};

/** Gaps below the threshold: [{start, end, mid, dur}] */
const findGaps = async (f) => {
  const { stderr } = await run(
    'ffmpeg',
    ['-hide_banner', '-i', f, '-af', `silencedetect=noise=${noise}:d=${minGap}`, '-f', 'null', '-'],
    { encoding: 'utf8', maxBuffer: 1 << 26 }
  ).catch((e) => ({ stderr: e.stderr ?? '' }));

  const gaps = [];
  let open = null;
  for (const line of stderr.split('\n')) {
    const s = /silence_start:\s*(-?[\d.]+)/.exec(line);
    const e = /silence_end:\s*([\d.]+)/.exec(line);
    if (s) open = Number(s[1]);
    if (e && open !== null) {
      const start = open;
      const end = Number(e[1]);
      gaps.push({ start, end, mid: (start + end) / 2, dur: end - start });
      open = null;
    }
  }
  return gaps;
};

/**
 * Dominant period between gaps. Differences between gap midpoints are clustered
 * and the most frequent one wins: that is the phrase length.
 */
const dominantPeriod = (gaps) => {
  const diffs = [];
  for (let i = 0; i < gaps.length; i++) {
    for (let j = i + 1; j < gaps.length; j++) {
      const d = gaps[j].mid - gaps[i].mid;
      if (d > 2 && d < 40) diffs.push(d);
    }
  }
  if (!diffs.length) return null;

  const tol = 0.06; // 60 ms: below that nobody hears the difference

  // Every cluster of similar differences.
  const clusters = [];
  for (const d of diffs) {
    if (clusters.some((c) => Math.abs(c.period - d) <= tol)) continue;
    const members = diffs.filter((x) => Math.abs(x - d) <= tol);
    clusters.push({ period: members.reduce((a, b) => a + b, 0) / members.length, hits: members.length });
  }
  if (!clusters.length) return null;

  // A multiple of the real period also shows up as a strong cluster, so among
  // the well-supported ones pick the SHORTEST: a short loop gives finer control
  // over the final length.
  const maxHits = Math.max(...clusters.map((c) => c.hits));
  const strong = clusters.filter((c) => c.hits >= maxHits * 0.6).sort((a, b) => a.period - b.period);
  return strong[0];
};

const total = await duration(file);
const gaps = await findGaps(file);

console.log(`\n${path.basename(file)}  ${total.toFixed(2)} s`);
console.log(`gaps below ${noise} and at least ${minGap} s long: ${gaps.length}\n`);

if (!gaps.length) {
  console.error('No gaps found. Try raising the threshold: --noise -28dB');
  process.exit(1);
}

const wide = gaps.filter((g) => g.dur >= 0.2);
console.log('  start      end      dur    centre');
for (const g of gaps) {
  const mark = g.dur >= 0.2 ? ' <-' : '';
  console.log(
    `  ${g.start.toFixed(3).padStart(7)}  ${g.end.toFixed(3).padStart(7)}  ${g.dur.toFixed(3)}  ${g.mid.toFixed(3)}${mark}`
  );
}

const dom = dominantPeriod(wide.length >= 3 ? wide : gaps);
if (dom) {
  console.log(`\ndominant period between gaps: ${dom.period.toFixed(3)} s  (${dom.hits} matches)`);
  const usable = (wide.length >= 3 ? wide : gaps).filter((g, i, a) =>
    a.some((h) => Math.abs(h.mid - g.mid - dom.period) <= 0.06)
  );
  if (usable.length) {
    const a = usable[Math.floor(usable.length / 3)];
    const b = (wide.length >= 3 ? wide : gaps).find((h) => Math.abs(h.mid - a.mid - dom.period) <= 0.06);
    if (b) {
      console.log(`suggestion:  --loop ${a.mid.toFixed(3)},${b.mid.toFixed(3)}`);
      if (target) {
        const need = Math.ceil((target - total) / dom.period);
        console.log(`reaching ${target} s takes ${need} repeats`);
      }
    }
  }
}

if (!loopArg) {
  console.log('\nPass --loop <centreA>,<centreB> --target <seconds> --out <file> to build it.\n');
  process.exit(0);
}

// -------------------------------------------------------------------- build

const [loopFrom, loopTo] = loopArg.split(',').map(Number);
if (!(loopTo > loopFrom)) {
  console.error('--loop needs two increasing times, in seconds.');
  process.exit(1);
}
const loopLen = loopTo - loopFrom;
if (!target) {
  console.error('Missing --target <seconds>.');
  process.exit(1);
}

const repeats = Math.max(1, Math.ceil((target - total) / loopLen));
const finalLen = total + repeats * loopLen;

console.log(`\nloop of ${loopLen.toFixed(3)} s x ${repeats}  ->  ${finalLen.toFixed(2)} s`);

const tmp = mkdtempSync(path.join(tmpdir(), 'loopmusic-'));
try {
  const A = path.join(tmp, 'a.wav');
  const B = path.join(tmp, 'b.wav');
  const Cc = path.join(tmp, 'c.wav');

  // No fades: the cuts land in silence, so there is no click to hide. It also
  // keeps the length exact, which is what lets the video lock to the bars.
  await run('ffmpeg', ['-v', 'error', '-y', '-i', file, '-t', String(loopTo), '-ar', '44100', '-c:a', 'pcm_s16le', A]);
  await run('ffmpeg', ['-v', 'error', '-y', '-ss', String(loopFrom), '-i', file, '-t', String(loopLen), '-ar', '44100', '-c:a', 'pcm_s16le', B]);
  await run('ffmpeg', ['-v', 'error', '-y', '-ss', String(loopTo), '-i', file, '-ar', '44100', '-c:a', 'pcm_s16le', Cc]);

  const list = path.join(tmp, 'list.txt');
  writeFileSync(list, [`file '${A}'`, ...Array(repeats).fill(`file '${B}'`), `file '${Cc}'`].join('\n').replace(/\\/g, '/'));

  await run('ffmpeg', ['-v', 'error', '-y', '-f', 'concat', '-safe', '0', '-i', list, '-c:a', 'libmp3lame', '-b:a', bitrate, out]);

  const got = await duration(out);
  console.log(`\n${out}  ${got.toFixed(3)} s`);

  // Check that every seam lands in silence.
  console.log('\nlevel at each seam (should be near silence):');
  let ok = true;
  for (let i = 0; i <= repeats; i++) {
    const at = loopTo + i * loopLen;
    const db = await meanDb(out, at - 0.1, 0.2);
    const good = db < -40;
    ok = ok && good;
    console.log(`  ${at.toFixed(3)}s  ${db.toFixed(1)} dB  ${good ? 'ok' : 'WARNING: audible'}`);
  }
  if (!ok) {
    console.log('\nAt least one seam is not in silence. Try two other gap centres,');
    console.log('or lower the threshold with --noise -36dB to keep only the clean gaps.');
  }
  console.log();
} finally {
  rmSync(tmp, { recursive: true, force: true });
}
