<script lang="ts">
  import { api } from "./api";
  import { celebrate } from "./celebrate";
  import CloseButton from "./CloseButton.svelte";
  import { t } from "./i18n.svelte";

  // Opened once by App.svelte (see STAR_AFTER in prompts.ts). Starring needs
  // the user's own GitHub session, so the button only opens the repo page in
  // the system browser; the user presses Star there.
  let { onclose }: { onclose: () => void } = $props();
  let panel: HTMLElement;
  $effect(() => panel.focus());

  // The browser may open behind the app (compositors can refuse focus), so say where it went.
  let opened = $state(false);
  const open = () =>
    api
      .openLink("github")
      .then(() => (opened = true))
      .catch(() => {});
</script>

<svelte:window onkeydown={(e) => e.key === "Escape" && onclose()} />

<div class="fixed inset-0 z-50 grid place-items-center overflow-y-auto bg-black/75 p-4 sm:p-6">
  <button class="absolute inset-0 cursor-default" aria-label={t("support.later")} onclick={onclose}></button>
  <div
    bind:this={panel}
    class="relative w-full max-w-md rounded-3xl border border-line bg-[#11111a] p-6 text-center shadow-2xl outline-none sm:p-8"
    role="dialog"
    aria-modal="true"
    aria-labelledby="star-title"
    tabindex="-1"
  >
    <CloseButton {onclose} />
    <span class="mx-auto mb-4 grid size-16 place-items-center rounded-full bg-[#facc15]/15" aria-hidden="true">
      <svg class="size-9 text-[#facc15]" viewBox="0 0 24 24" fill="currentColor"
        ><path
          d="M12 2.5l2.94 5.96 6.56.95-4.75 4.63 1.12 6.54L12 17.5l-5.87 3.08 1.12-6.54L2.5 9.41l6.56-.95z"
        /></svg
      >
    </span>
    <h2 id="star-title" class="mb-2 text-xl font-semibold">{t("star.title")}</h2>
    <p class="mb-6 text-sm text-muted">{t("star.desc")}</p>

    <!-- Same card and confetti as the donate buttons in SupportDialog.svelte. -->
    <button
      class="flex w-full items-center justify-center gap-3 rounded-2xl border border-line bg-surface px-3 py-3 transition-colors hover:border-line-hi hover:bg-white/5"
      onclick={(e) => {
        celebrate(e.currentTarget);
        open();
      }}
    >
      <span class="grid size-9 shrink-0 place-items-center rounded-full bg-[#facc15]/15">
        <svg class="size-5 text-[#facc15]" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true"
          ><path d="M12 2.5l2.94 5.96 6.56.95-4.75 4.63 1.12 6.54L12 17.5l-5.87 3.08 1.12-6.54L2.5 9.41l6.56-.95z" /></svg
        >
      </span>
      <span class="text-sm font-medium">{t("star.button")}</span>
    </button>

    <p class="mt-4 min-h-5 text-sm text-accent transition-opacity" class:opacity-0={!opened} role="status">
      {opened ? t("star.opened") : ""}
    </p>

    <div class="mt-2 flex justify-center">
      <button class="btn-ghost px-6" onclick={onclose}>{opened ? t("shortcuts.close") : t("support.later")}</button>
    </div>
  </div>
</div>
