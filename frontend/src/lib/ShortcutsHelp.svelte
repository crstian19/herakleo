<script lang="ts">
  import CloseButton from "./CloseButton.svelte";
  import { t } from "./i18n.svelte";
  import { shortcutGroups } from "./shortcuts";

  let { onclose }: { onclose: () => void } = $props();
  let panel: HTMLElement;

  // Focus moves into the dialog so Esc/Tab work and screen readers announce it.
  $effect(() => panel.focus());
</script>

<div class="fixed inset-0 z-50 grid place-items-center overflow-y-auto bg-black/75 p-4 sm:p-6">
  <button class="absolute inset-0 cursor-default" aria-label={t("shortcuts.close")} onclick={onclose}></button>
  <div
    bind:this={panel}
    class="relative w-full max-w-2xl rounded-3xl border border-line bg-[#11111a] p-5 shadow-2xl outline-none sm:p-6"
    role="dialog"
    aria-modal="true"
    aria-labelledby="shortcuts-title"
    tabindex="-1"
  >
    <CloseButton {onclose} />
    <div class="mb-5 pr-10">
      <h2 id="shortcuts-title" class="text-lg font-semibold">{t("shortcuts.title")}</h2>
      <p class="text-sm text-muted">{t("shortcuts.hint")}</p>
    </div>

    <div class="grid gap-6 sm:grid-cols-2">
      {#each shortcutGroups as g (g.title)}
        <section class={g.title === "shortcuts.general" ? "sm:row-span-2" : ""}>
          <h3 class="section-title mb-3">{t(g.title)}</h3>
          <dl class="space-y-2">
            {#each g.items as s (s.label)}
              <div class="flex items-center justify-between gap-3 text-sm">
                <dt class="text-muted">{t(s.label)}</dt>
                <dd class="flex shrink-0 gap-1">
                  {#each s.keys as k (k)}<kbd class="kbd">{k}</kbd>{/each}
                </dd>
              </div>
            {/each}
          </dl>
        </section>
      {/each}
    </div>
  </div>
</div>
