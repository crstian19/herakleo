<script lang="ts">
  import CloseButton from "./CloseButton.svelte";
  import { t } from "./i18n.svelte";

  let { report, onclose }: { report: string; onclose: () => void } = $props();
  let copied = $state(false);
  let pre: HTMLElement;

  async function copy() {
    try {
      await navigator.clipboard.writeText(report);
      copied = true;
    } catch {
      // Some webviews block the async clipboard: select the text so Ctrl+C works.
      const range = document.createRange();
      range.selectNodeContents(pre);
      getSelection()?.removeAllRanges();
      getSelection()?.addRange(range);
    }
  }
</script>

<svelte:window onkeydown={(e) => e.key === "Escape" && onclose()} />

<div class="fixed inset-0 z-50 grid place-items-center overflow-y-auto bg-black/75 p-4 sm:p-6">
  <div class="relative w-full max-w-3xl rounded-3xl border border-line bg-[#11111a] p-5 shadow-2xl sm:p-6" role="dialog" aria-modal="true" aria-labelledby="diag-title">
    <CloseButton {onclose} />
    <h2 id="diag-title" class="mb-1 pr-10 text-lg font-semibold">{t("diag.title")}</h2>
    <p class="mb-4 text-sm text-muted">{t("diag.hint")}</p>
    <pre
      bind:this={pre}
      class="max-h-[50vh] overflow-auto rounded-xl border border-line bg-black/40 p-4 text-xs leading-relaxed whitespace-pre-wrap select-text">{report}</pre>
    <div class="mt-5 flex justify-end gap-2">
      <button class="btn-accent" onclick={copy}>{copied ? t("diag.copied") : t("diag.copy")}</button>
    </div>
  </div>
</div>
