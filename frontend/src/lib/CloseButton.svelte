<script lang="ts">
  import { t } from "./i18n.svelte";

  // The one close control every dialog uses: an X in the top-right corner.
  // The dialog panel must be `relative`.
  let { onclose, disabled = false }: { onclose: () => void; disabled?: boolean } = $props();
</script>

<button
  type="button"
  class="absolute top-4 right-4 grid size-9 place-items-center rounded-full text-muted transition-colors hover:bg-white/10 hover:text-text disabled:opacity-40"
  aria-label={t("shortcuts.close")}
  title={t("shortcuts.close")}
  {disabled}
  onclick={onclose}
>
  <svg class="size-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" aria-hidden="true"
    ><path d="M6 6l12 12M18 6 6 18" /></svg
  >
</button>
