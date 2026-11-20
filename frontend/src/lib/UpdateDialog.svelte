<script lang="ts">
  import { api, errorMessage, type UpdateInfo } from "./api";
  import CloseButton from "./CloseButton.svelte";
  import { t, type MessageKey } from "./i18n.svelte";
  import { releaseNotes, skipVersion } from "./update";

  // Shown by App.svelte when CheckUpdate finds a newer release. What it offers
  // depends on who owns the install (UpdateInfo.mode): the app itself
  // (download and restart), a package manager (its command) or nobody (the
  // download page).
  let { info, onclose }: { info: UpdateInfo; onclose: () => void } = $props();
  let panel: HTMLElement;
  let cmd = $state<HTMLElement>();
  let busy = $state(false);
  let error = $state("");
  let copied = $state(false);
  $effect(() => panel.focus());

  const notes = $derived(releaseNotes(info.notes));
  // aur, scoop, brew (command); flatpak, deb, rpm (download); "" for the rest.
  const managerHint = $derived(info.manager ? t(`update.via.${info.manager}` as MessageKey) : t("update.downloadHint"));

  async function install() {
    busy = true;
    error = "";
    try {
      // On success the app quits and the new version starts.
      await api.installUpdate();
    } catch (err) {
      error = errorMessage(err);
      busy = false;
    }
  }

  async function copy() {
    try {
      await navigator.clipboard.writeText(info.command);
      copied = true;
    } catch {
      // Some webviews block the async clipboard: select the text so Ctrl+C works.
      if (!cmd) return;
      const range = document.createRange();
      range.selectNodeContents(cmd);
      getSelection()?.removeAllRanges();
      getSelection()?.addRange(range);
    }
  }

  function skip() {
    skipVersion(info.latest);
    onclose();
  }

  const close = () => !busy && onclose();
</script>

<svelte:window onkeydown={(e) => e.key === "Escape" && close()} />

<div class="fixed inset-0 z-50 grid place-items-center overflow-y-auto bg-black/75 p-4 sm:p-6">
  <button class="absolute inset-0 cursor-default" aria-label={t("update.later")} onclick={close}></button>
  <div
    bind:this={panel}
    class="relative w-full max-w-lg rounded-3xl border border-line bg-[#11111a] p-6 shadow-2xl outline-none sm:p-8"
    role="dialog"
    aria-modal="true"
    aria-labelledby="update-title"
    tabindex="-1"
  >
    {#if !busy}<CloseButton onclose={close} />{/if}
    <p class="text-xs font-semibold tracking-wider text-accent uppercase">{t("update.kicker")}</p>
    <h2 id="update-title" class="mt-1 mb-1 pr-10 text-xl font-semibold">{t("update.title", { version: info.latest })}</h2>
    <p class="mb-4 text-sm text-muted">{t("update.current", { version: info.current })}</p>

    {#if notes}
      <pre
        class="mb-5 max-h-[40vh] overflow-auto rounded-xl border border-line bg-black/40 p-4 font-sans text-sm leading-relaxed whitespace-pre-wrap select-text">{notes}</pre>
    {/if}

    {#if info.mode === "install"}
      <p class="mb-4 text-sm text-muted">{t("update.installHint")}</p>
    {:else if info.mode === "command"}
      <p class="mb-2 text-sm text-muted">{managerHint}</p>
      <div class="mb-4 flex items-center gap-2">
        <code bind:this={cmd} class="flex-1 rounded-xl border border-line bg-black/40 px-3 py-2 font-mono text-sm select-all">{info.command}</code>
        <button class="btn-ghost px-3 py-2 text-sm" onclick={copy}>{copied ? t("diag.copied") : t("update.copy")}</button>
      </div>
    {:else}
      <p class="mb-4 text-sm text-muted">{managerHint}</p>
    {/if}

    {#if error}
      <p class="mb-4 rounded-xl border border-red-500/30 bg-[#2a0f12] px-3 py-2 text-sm text-red-200" role="alert">{error}</p>
    {/if}

    <div class="flex flex-wrap items-center justify-end gap-2">
      <button class="btn-ghost mr-auto px-3 py-1.5 text-sm text-muted" disabled={busy} onclick={skip}>{t("update.skip")}</button>
      <button class="btn-ghost px-4 py-1.5 text-sm" disabled={busy} onclick={close}>{t("update.later")}</button>
      {#if info.mode === "install"}
        <button class="btn-accent px-4 py-1.5 text-sm" disabled={busy} onclick={install}>
          {busy ? t("update.installing") : t("update.install")}
        </button>
      {:else if info.mode === "download"}
        <button class="btn-accent px-4 py-1.5 text-sm" onclick={() => api.openLink("release").then(onclose, () => {})}>
          {t("update.download")}
        </button>
      {/if}
    </div>
  </div>
</div>
