<script lang="ts">
  import CloseButton from "./CloseButton.svelte";
  import { api, errorMessage, type Channel, type Episode } from "./api";
  import { t } from "./i18n.svelte";

  // A series' seasons and episodes, asked from the panel when it opens.
  let {
    series,
    onclose,
    onplay,
  }: {
    series: Channel;
    onclose: () => void;
    onplay: (series: Channel, episode: Episode) => void;
  } = $props();

  let panel: HTMLElement;
  let episodes = $state<Episode[] | null>(null);
  let error = $state("");
  let season = $state(0);

  $effect(() => panel.focus());
  $effect(() => {
    api
      .seriesEpisodes(series.id)
      .then((eps) => {
        episodes = eps;
        season = eps[0]?.season ?? 0;
      })
      .catch((err) => (error = errorMessage(err)));
  });

  const seasons = $derived([...new Set((episodes ?? []).map((e) => e.season))]);
  const shown = $derived((episodes ?? []).filter((e) => e.season === season));
</script>

<svelte:window onkeydown={(e) => e.key === "Escape" && (e.preventDefault(), e.stopPropagation(), onclose())} />

<div class="fixed inset-0 z-50 grid place-items-center overflow-y-auto bg-black/75 p-4 sm:p-6">
  <button class="absolute inset-0 cursor-default" aria-label={t("shortcuts.close")} onclick={onclose}></button>
  <div
    bind:this={panel}
    class="relative flex max-h-[85vh] w-full max-w-xl flex-col rounded-3xl border border-line bg-[#11111a] shadow-2xl outline-none"
    role="dialog"
    aria-modal="true"
    aria-labelledby="series-title"
    tabindex="-1"
  >
    <CloseButton {onclose} />
    <div class="flex items-center gap-4 border-b border-line p-5 pr-16 sm:p-6 sm:pr-16">
      {#if series.logo}
        <img src={series.logo} alt="" class="h-16 w-12 shrink-0 rounded-lg object-cover" referrerpolicy="no-referrer" />
      {/if}
      <div class="min-w-0 flex-1">
        <h2 id="series-title" class="truncate text-lg font-semibold">{series.name}</h2>
        <p class="truncate text-sm text-muted">
          {episodes ? t("series.count", { seasons: seasons.length, episodes: episodes.length }) : series.group}
        </p>
      </div>
    </div>

    {#if seasons.length > 1}
      <div class="flex gap-1 overflow-x-auto border-b border-line px-5 py-2 sm:px-6" role="tablist">
        {#each seasons as n (n)}
          <button class="chip w-auto shrink-0 justify-center text-xs whitespace-nowrap" role="tab" aria-selected={season === n} class:is-active={season === n} onclick={() => (season = n)}
            >{t("series.season", { n })}</button
          >
        {/each}
      </div>
    {/if}

    <div class="overflow-y-auto p-5 sm:p-6">
      {#if error}
        <p class="rounded-xl border border-red-500/30 bg-red-500/10 px-3 py-2 text-sm text-red-300">{error}</p>
      {:else if episodes === null}
        <p class="text-sm text-muted">{t("series.loading")}</p>
      {:else if episodes.length === 0}
        <p class="text-sm text-muted">{t("series.empty")}</p>
      {:else}
        <ul class="space-y-1.5">
          {#each shown as e (e.id)}
            <li>
              <button class="flex w-full gap-3 rounded-xl bg-white/[0.03] px-3 py-2.5 text-left hover:bg-white/[0.07]" onclick={() => onplay(series, e)}>
                <span class="w-8 shrink-0 pt-0.5 text-sm text-muted tabular-nums">{e.number || "·"}</span>
                <span class="min-w-0 flex-1">
                  <span class="block font-medium">{e.title || t("series.episode", { n: e.number })}</span>
                  {#if e.plot}<span class="mt-0.5 line-clamp-2 block text-sm text-muted">{e.plot}</span>{/if}
                </span>
                <svg class="size-4 shrink-0 self-center text-accent" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true"><path d="M8 5v14l11-7z" /></svg>
              </button>
            </li>
          {/each}
        </ul>
      {/if}
    </div>
  </div>
</div>
