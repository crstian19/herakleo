<script lang="ts">
  import { api } from "./api";
  import { t } from "./i18n.svelte";

  // Asked once, on the first launch: the app sends nothing until the user says yes.
  let { onanswer }: { onanswer: (on: boolean) => void } = $props();
</script>

<div
  class="fixed right-6 bottom-6 z-40 w-96 max-w-[calc(100%-3rem)] rounded-2xl border border-line bg-[#111118] p-4 shadow-2xl"
  role="dialog"
  aria-labelledby="telemetry-title"
>
  <p id="telemetry-title" class="font-medium">{t("telemetry.title")}</p>
  <p class="mt-1 text-sm text-muted">
    {t("telemetry.body")}
    <button class="text-accent hover:underline" onclick={() => api.openLink("telemetry")}>{t("telemetry.what")}</button>
  </p>
  <div class="mt-3 flex justify-end gap-2">
    <button class="btn-ghost px-3 py-1.5 text-sm" onclick={() => onanswer(false)}>{t("telemetry.no")}</button>
    <button class="btn-accent px-3 py-1.5 text-sm" onclick={() => onanswer(true)}>{t("telemetry.yes")}</button>
  </div>
</div>
