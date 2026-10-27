<script lang="ts">
  import CloseButton from "./CloseButton.svelte";
  import { untrack } from "svelte";
  import { api, errorMessage, type NewSource, type SourceInfo, type SourceKind } from "./api";
  import { t, type MessageKey } from "./i18n.svelte";

  // Without `editing` it adds a list; with it, it edits that list. When editing,
  // empty fields keep their current value so credentials never travel back to the UI.
  let {
    editing = null,
    onclose,
    onsaved,
  }: { editing?: SourceInfo | null; onclose: () => void; onsaved: (s: SourceInfo) => void } = $props();

  // Snapshot at open: later prop changes must not reset what the user is typing.
  const initial = untrack(() => editing);
  let kind = $state<SourceKind>(initial?.kind ?? "m3u_url");
  let name = $state(initial?.name ?? "");
  let url = $state("");
  let path = $state("");
  let server = $state("");
  let username = $state("");
  let password = $state("");
  let busy = $state(false);
  let error = $state("");

  const tabs: { kind: SourceKind; label: MessageKey }[] = [
    { kind: "m3u_url", label: "dialog.tabUrl" },
    { kind: "m3u_file", label: "dialog.tabFile" },
    { kind: "xtream", label: "dialog.tabXtream" },
  ];

  // The backend never sends the stored password to a new server (scheme or
  // host differ), so moving an Xtream list needs it typed again.
  const sameServer = (a: string, b: string) => {
    const norm = (v: string) => v.trim().replace(/\/+$/, "");
    try {
      const [x, y] = [new URL(norm(a)), new URL(norm(b))];
      return x.protocol === y.protocol && x.host === y.host;
    } catch {
      return norm(a) === norm(b);
    }
  };
  const needsPassword = $derived(
    initial !== null && kind === "xtream" && server.trim() !== "" && !sameServer(server, initial.location),
  );

  const valid = $derived(
    name.trim() !== "" &&
      !(needsPassword && password === "") &&
      (initial !== null ||
        (kind === "m3u_url"
          ? url.trim() !== ""
          : kind === "m3u_file"
            ? path !== ""
            : server.trim() !== "" && username.trim() !== "" && password !== "")),
  );

  // Placeholders show what is stored now, so "leave empty to keep" is obvious.
  const keep = (current: string, fallback: string) =>
    initial ? t("dialog.keep", { current: current || t("dialog.current") }) : fallback;

  async function pick() {
    try {
      const picked = await api.pickPlaylistFile(t("dialog.pickTitle"));
      if (picked) {
        path = picked;
        if (!name) name = picked.split("/").pop()?.replace(/\.m3u8?$/i, "") ?? "";
      }
    } catch (err) {
      error = errorMessage(err);
    }
  }

  async function submit(e: SubmitEvent) {
    e.preventDefault();
    if (!valid || busy) return;
    const req: NewSource =
      kind === "m3u_url"
        ? { kind, name: name.trim(), url: url.trim() }
        : kind === "m3u_file"
          ? { kind, name: name.trim(), path }
          : { kind, name: name.trim(), server: server.trim(), username: username.trim(), password };
    busy = true;
    error = "";
    try {
      onsaved(initial ? await api.updateSource(initial.id, req) : await api.addSource(req));
    } catch (err) {
      error = errorMessage(err);
    } finally {
      busy = false;
    }
  }
</script>

<svelte:window onkeydown={(e) => e.key === "Escape" && !busy && onclose()} />

<div class="fixed inset-0 z-50 grid place-items-center overflow-y-auto bg-black/75 p-4 sm:p-6">
  <form onsubmit={submit} class="relative w-full max-w-md rounded-3xl border border-line bg-[#11111a] p-5 shadow-2xl sm:p-6">
    <CloseButton {onclose} disabled={busy} />
    <h2 class="mb-1 pr-10 text-lg font-semibold">{initial ? t("dialog.editTitle", { name: initial.name }) : t("dialog.addTitle")}</h2>
    <p class="mb-5 text-sm text-muted">{t(initial ? "dialog.editHint" : "dialog.addHint")}</p>

    {#if !initial}
      <div class="mb-5 grid grid-cols-3 gap-1 rounded-xl bg-white/5 p-1">
        {#each tabs as tab (tab.kind)}
          <button type="button" class="chip justify-center text-xs" class:is-active={kind === tab.kind} onclick={() => (kind = tab.kind)}
            >{t(tab.label)}</button
          >
        {/each}
      </div>
    {/if}

    <div class="space-y-3">
      <input class="field" placeholder={t("dialog.name")} bind:value={name} />

      {#if kind === "m3u_url"}
        <input class="field" type="url" placeholder={keep(initial?.location ?? "", "https://provider.tv/list.m3u")} bind:value={url} />
      {:else if kind === "m3u_file"}
        <div class="flex gap-2">
          <input class="field truncate" placeholder={keep(initial?.location ?? "", t("dialog.noFile"))} readonly value={path} />
          <button type="button" class="btn-ghost shrink-0" onclick={pick}>{t("dialog.choose")}</button>
        </div>
      {:else}
        <input class="field" type="url" placeholder={keep(initial?.location ?? "", "http://server:8080")} bind:value={server} />
        <div class="grid grid-cols-1 gap-2 min-[400px]:grid-cols-2">
          <input class="field" placeholder={t(initial ? "dialog.userKeep" : "dialog.user")} autocomplete="off" bind:value={username} />
          <input
            class="field"
            type="password"
            placeholder={t(initial && !needsPassword ? "dialog.passwordKeep" : "dialog.password")}
            autocomplete="off"
            required={needsPassword}
            bind:value={password}
          />
        </div>
        {#if needsPassword}
          <p class="text-xs text-accent">{t("dialog.passwordForServer")}</p>
        {/if}
        {#if initial}
          <p class="text-xs text-muted">{t("dialog.credentialsWarning")}</p>
        {/if}
      {/if}
    </div>

    {#if error}
      <p class="mt-4 rounded-xl border border-red-500/30 bg-red-500/10 px-3 py-2 text-sm text-red-300">{error}</p>
    {/if}

    <div class="mt-6 flex justify-end gap-2">
      <button type="button" class="btn-ghost" onclick={onclose} disabled={busy}>{t("dialog.cancel")}</button>
      <button class="btn-accent" disabled={!valid || busy}>{t(busy ? "dialog.loading" : initial ? "dialog.save" : "dialog.add")}</button>
    </div>
  </form>
</div>
