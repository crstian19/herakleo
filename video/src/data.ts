// Fictional channels only: no real brands, logos or networks.

export type DemoChannel = { name: string; group: string; hue: number; mark: string };

const names = [
  ["Nile News 24", "News"],
  ["Luxor Live", "Entertainment"],
  ["Papyrus Kids", "Kids"],
  ["Sphinx Cinema", "Movies"],
  ["Oasis Music", "Music"],
  ["Delta Docs", "Documentaries"],
  ["Karnak HD", "Entertainment"],
  ["Memphis One", "General"],
  ["Pharaoh Sports", "Sports"],
  ["Giza Gaming", "Gaming"],
  ["Anubis Crime", "Series"],
  ["Scarab Science", "Documentaries"],
  ["Obelisk Weather", "News"],
  ["Lotus Lifestyle", "Lifestyle"],
  ["Cairo Comedy", "Entertainment"],
  ["Nile News Español", "News"],
  ["Horizon Travel", "Lifestyle"],
  ["Sunboat Classics", "Movies"],
  ["Ibis Learning", "Kids"],
  ["Desert Drive", "Motor"],
  ["Falcon Racing", "Motor"],
  ["Amun Arts", "Culture"],
  ["Golden Reel", "Movies"],
  ["Nubia Nature", "Documentaries"],
  ["Ra Radio TV", "Music"],
  ["Abydos Action", "Movies"],
  ["Thoth Tech", "Documentaries"],
  ["Bastet Pets", "Lifestyle"],
  ["Nile News Business", "News"],
  ["Sahara Stories", "Series"],
  ["Hathor Hits", "Music"],
  ["Pyramid Play", "Kids"],
  ["Edfu Eats", "Lifestyle"],
  ["Aswan Arena", "Sports"],
  ["Djed Drama", "Series"],
  ["Ankh Anime", "Kids"],
] as const;

const marks = ["◆", "▲", "●", "■", "✦", "⬢", "◗", "✶"];

export const CHANNELS: DemoChannel[] = names.map(([name, group], i) => ({
  name,
  group,
  hue: (i * 47 + 20) % 360,
  mark: marks[i % marks.length]!,
}));

/** A long list for the scrolling shots: the same names, numbered like a real provider list. */
export const MANY: DemoChannel[] = Array.from({ length: 36 * 8 }, (_, i) => {
  const c = CHANNELS[i % CHANNELS.length]!;
  const n = Math.floor(i / CHANNELS.length);
  return { ...c, name: n === 0 ? c.name : `${c.name} ${["HD", "FHD", "+1", "2", "Plus", "4K", "Max"][n - 1]}` };
});

export const GROUPS = ["News", "Sports", "Movies", "Series", "Kids", "Music", "Documentaries", "Lifestyle"];
