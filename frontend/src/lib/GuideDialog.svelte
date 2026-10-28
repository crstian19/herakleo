<script lang="ts">
  import CloseButton from "./CloseButton.svelte";
  import { api, errorMessage, type Channel, type Programme, type Reminder } from "./api";
  import { clockTime, dayLabel, t } from "./i18n.svelte";

  // A channel's programmes from the one on air onwards, by day. Future ones
  // get a bell that sets a reminder; the app announces it when it starts.
  let {
    channel,
    reminders,
    onclose,
    onplay,
    onremind,
  }: {
    channel: Channel;
    reminders: Reminder[];
    onclose: () => void;
    onplay: (c: Channel) => void;
    onremind: (p: Programme, on: boolean) => void;
  } = $props();

  let panel: HTMLElement;
  let programmes = $state<Programme[] | null>(null);
  let error = $state("");
  const nowSecs = Math.floor(Date.now() / 1000);

  $effect(() => panel.focus());
  $effect(() => {
    api
      .channelGuide(channel.id)
      .then((p) => (programmes = p))
      .catch((err) => (error = errorMessage(err)));
  });

  const days = $derived.by(() => {
    const out: { label: string; items: Programme[] }[] = [];
    for (const p of programmes ?? []) {
      const label = dayLabel(Math.max(p.start, nowSecs));
      if (out.at(-1)?.label !== label) out.push({ label, items: [] });
      out.at(-1)!.items.push(p);
    }
    return out;
  });

  const reminded = (p: Programme) => reminders.some((r) => r.channelId === channel.id && r.start === p.start);
  const onAir = (p: Programme) => p.start <= nowSecs && p.stop > nowSecs;
</script>

<svelte:window onkeydown={(e) => e.key === "Escape" && (e.preventDefault(), e.stopPropagation(), onclose())} />

<div class="fixed inset-0 z-50 grid place-items-center overflow-y-auto bg-black/75 p-4 sm:p-6">
  <button class="absolute inset-0 cursor-default" aria-label={t("shortcuts.close")} onclick={onclose}></button>
  <div
    bind:this={panel}
    class="relative flex max-h-[85vh] w-full max-w-xl flex-col rounded-3xl border border-line bg-[#11111a] shadow-2xl outline-none"
    role="dialog"
    aria-modal="true"
    aria-labelledby="guide-title"
    tabindex="-1"
  >
    <CloseButton {onclose} />
    <div class="flex items-center gap-3 border-b border-line p-5 pr-16 sm:p-6 sm:pr-16">
      <div class="min-w-0 flex-1">
        <h2 id="guide-title" class="truncate text-lg font-semibold">{channel.name}</h2>
        <p class="truncate text-sm text-muted">{t("card.guide")}{channel.group ? ` · ${channel.group}` : ""}</p>
      </div>
      <button class="btn-accent shrink-0" onclick={() => onplay(channel)}>{t("guide.watch")}</button>
    </div>

    <div class="overflow-y-auto p-5 sm:p-6">
      {#if error}
        <p class="rounded-xl border border-red-500/30 bg-red-500/10 px-3 py-2 text-sm text-red-300">{error}</p>
      {:else if programmes === null}
        <p class="text-sm text-muted">{t("guide.loading")}</p>
      {:else if programmes.length === 0}
        <p class="text-sm text-muted">{t("guide.empty")}</p>
      {:else}
        {#each days as d (d.label)}
          <h3 class="section-title mt-5 mb-2 first:mt-0">{d.label}</h3>
          <ul class="space-y-1.5">
            {#each d.items as p (p.start)}
              {@const live = onAir(p)}
              {@const on = reminded(p)}
              <li class="flex gap-3 rounded-xl px-3 py-2.5 {live ? 'bg-accent/10 ring-1 ring-accent/40' : 'bg-white/[0.03]'}">
                <span class="w-16 shrink-0 pt-0.5 text-sm whitespace-nowrap text-muted tabular-nums">{clockTime(p.start)}</span>
                <div class="min-w-0 flex-1">
                  <p class="font-medium">
                    {#if live}<span class="mr-1.5 text-xs font-semibold tracking-wider text-accent uppercase">{t("guide.now")}</span>{/if}{p.title}
                  </p>
                  {#if p.desc}<p class="mt-0.5 line-clamp-2 text-sm text-muted">{p.desc}</p>{/if}
                  {#if live}
                    <div class="mt-2 h-1 overflow-hidden rounded bg-white/10">
                      <div class="h-full origin-left bg-accent" style="transform: scaleX({(nowSecs - p.start) / (p.stop - p.start)})"></div>
                    </div>
                  {/if}
                </div>
                {#if !live}
                  <button
                    class="grid size-8 shrink-0 place-items-center rounded-full transition-colors {on ? 'text-accent' : 'text-muted hover:bg-white/10 hover:text-text'}"
                    aria-pressed={on}
                    aria-label={t(on ? "guide.reminderOn" : "guide.remind")}
                    title={t(on ? "guide.reminderOn" : "guide.remind")}
                    onclick={() => onremind(p, !on)}
                  >
                    <svg class="size-4" viewBox="0 0 24 24" fill={on ? "currentColor" : "none"} stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"
                      ><path d="M6 8a6 6 0 0 1 12 0c0 7 3 9 3 9H3s3-2 3-9M10.3 21a1.94 1.94 0 0 0 3.4 0" /></svg
                    >
                  </button>
                {/if}
              </li>
            {/each}
          </ul>
        {/each}
      {/if}
    </div>
  </div>
</div>
