#!/usr/bin/env node
/**
 * Generates a video's voice-over against any OpenAI-compatible TTS endpoint
 * (`POST /audio/speech`). Built against Kokoro served by Helmcode, but any other
 * provider works: change TTS_BASE_URL and the model name.
 *
 *   node --env-file-if-exists=.env scripts/gen-voice.mjs
 *   node --env-file-if-exists=.env scripts/gen-voice.mjs --force
 *   node --env-file-if-exists=.env scripts/gen-voice.mjs --only 03,09
 *   node --env-file-if-exists=.env scripts/gen-voice.mjs --voice bf_emma --force
 *   node --env-file-if-exists=.env scripts/gen-voice.mjs --config voice.json --out public/vo
 *
 * Three decisions that look like details but aren't:
 *
 * 1. ONE FILE PER LINE, never one long take. With separate files you place each
 *    line on an exact frame. With a single take, moving one line means redoing
 *    the whole edit.
 *
 * 2. NORMALIZE. Kokoro outputs around -25 dB on average and a library music
 *    track sits near -14. With a 12 dB gap the voice can't cut through even
 *    with the music ducked. loudnorm fixes that and also evens out the lines
 *    against each other.
 *
 * 3. TRIM SILENCE at head and tail, so the frame where you place a line is where
 *    the speech actually starts, not a variable gap.
 *
 * When done it prints each line's real duration in seconds and frames. Those
 * numbers drive the edit: record the voice BEFORE editing.
 */

import { execFile } from 'node:child_process';
import { existsSync, mkdirSync, readFileSync, unlinkSync, writeFileSync } from 'node:fs';
import { promisify } from 'node:util';

const run = promisify(execFile);

const BASE_URL = process.env.TTS_BASE_URL ?? 'https://api.helmcode.com/v1';
const API_KEY = process.env.TTS_API_KEY ?? process.env.HELMCODE_API_KEY;

const argv = process.argv.slice(2);
const flag = (name, fallback = null) => {
  const i = argv.indexOf(`--${name}`);
  return i >= 0 && argv[i + 1] && !argv[i + 1].startsWith('--') ? argv[i + 1] : fallback;
};
const force = argv.includes('--force');
const only = flag('only');
const configPath = flag('config', 'voiceover.json');
const outDir = flag('out', 'public/vo');
const fps = Number(flag('fps', '30'));

if (!existsSync(configPath)) {
  console.error(`Cannot find ${configPath}. Copy voiceover.json and edit it.`);
  process.exit(1);
}
if (!API_KEY) {
  console.error('Missing TTS_API_KEY (or HELMCODE_API_KEY). Set it in .env; see .env.example.');
  process.exit(1);
}

const cfg = JSON.parse(readFileSync(configPath, 'utf8'));
const voice = flag('voice', cfg.voice);
const speed = Number(flag('speed', String(cfg.speed ?? 1)));

mkdirSync(outDir, { recursive: true });

const wanted = only ? new Set(only.split(',').map((s) => s.trim())) : null;
const lines = cfg.lines.filter((l) => !wanted || wanted.has(l.id));

/**
 * The API returns a RIFF with size 0xFFFFFFFF (written while streaming), so it
 * has to be remuxed. The same pass trims silence and normalizes.
 */
const normalize = async (raw, out) =>
  run('ffmpeg', [
    '-v', 'error', '-y', '-i', raw,
    '-af',
    'silenceremove=start_periods=1:start_silence=0.03:start_threshold=-50dB,' +
      'areverse,silenceremove=start_periods=1:start_silence=0.06:start_threshold=-50dB,areverse,' +
      `loudnorm=I=${cfg.loudnorm?.I ?? -16}:TP=${cfg.loudnorm?.TP ?? -1.5}:LRA=${cfg.loudnorm?.LRA ?? 11}`,
    '-ar', '48000', '-ac', '1', '-c:a', 'pcm_s16le',
    out,
  ]);

const duration = async (file) => {
  const { stdout } = await run('ffprobe', ['-v', 'error', '-show_entries', 'format=duration', '-of', 'csv=p=0', file]);
  return Number(stdout.trim());
};

console.log(`\n${cfg.model} · voice ${voice} · speed ${speed} · ${BASE_URL}\n`);

const report = [];
for (const line of lines) {
  const out = `${outDir}/vo-${line.id}.wav`;

  if (existsSync(out) && !force) {
    report.push([line.id, await duration(out), line.text, 'exists']);
    continue;
  }

  const res = await fetch(`${BASE_URL}/audio/speech`, {
    method: 'POST',
    headers: { Authorization: `Bearer ${API_KEY}`, 'Content-Type': 'application/json' },
    body: JSON.stringify({ model: cfg.model, input: line.text, voice, speed, response_format: 'wav' }),
  });

  if (!res.ok) {
    console.error(`vo-${line.id}: HTTP ${res.status} ${await res.text()}`);
    process.exit(1);
  }

  const raw = `${outDir}/.raw-${line.id}.wav`;
  writeFileSync(raw, Buffer.from(await res.arrayBuffer()));
  await normalize(raw, out);
  unlinkSync(raw);

  report.push([line.id, await duration(out), line.text, 'generated']);
}

console.log('id   dur      frames  status      text');
for (const [id, dur, text, state] of report) {
  console.log(`${id}   ${dur.toFixed(2)}s   ${String(Math.ceil(dur * fps)).padStart(4)}   ${state.padEnd(10)}  ${text}`);
}
console.log(`\ntotal voice: ${report.reduce((a, [, d]) => a + d, 0).toFixed(2)}s`);
console.log('\nCopy the frames column into your timing module: those are the real durations.\n');
