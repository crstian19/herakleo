<script lang="ts">
  import { type AppSettings, type PlaybackOptions, type SettingsPatch, type SourceInfo } from "./api";
  import { ago, currentLocale, date, localeNames, setLanguage, systemLocale, t, type Locale, type MessageKey } from "./i18n.svelte";
  import { accents, prefs, savePrefs, setAccent, setEffects } from "./prefs.svelte";

  const browseModes = [
    { id: "pages", label: "settings.browsePages", hint: "settings.browsePagesHint" },
    { id: "scroll", label: "settings.browseScroll", hint: "settings.browseScrollHint" },
  ] as const;

  let {
    sources,
    settings,
    hiddenGroups,
    refreshing,
    onadd,
    onedit,
    onrefresh,
    onrefreshall,
    onenable,
    onremove,
    onsettings,
    onunhidegroup,
    onshowhidden,
    onclearhistory,
    onexport,
    onimport,
    onperf,
  }: {
    sources: SourceInfo[];
    settings: AppSettings | null;
    hiddenGroups: string[];
    refreshing: string;
    onadd: () => void;
    onedit: (s: SourceInfo) => void;
    onrefresh: (s: SourceInfo) => void;
    onrefreshall: () => void;
    onenable: (s: SourceInfo, enabled: boolean) => void;
    onremove: (s: SourceInfo) => void;
    onsettings: (patch: SettingsPatch, message: () => string) => void;
    onunhidegroup: (name: string) => void;
    onshowhidden: () => void;
    onclearhistory: () => void;
    onexport: () => void;
    onimport: () => void;
    /** Starts the wheel-scroll measurement (App.svelte runs it on the channel grid). */
    onperf: () => void;
  } = $props();

  const kindKey: Record<SourceInfo["kind"], MessageKey> = {
    m3u_url: "settings.kindM3uUrl",
    m3u_file: "settings.kindM3uFile",
    xtream: "settings.kindXtream",
  };
  const DAY = 86400;

  // Xtream subscription line: red once expired or the panel says so, amber in the last week.
  function account(s: SourceInfo): { text: string; tone: "ok" | "warn" | "bad" } | null {
    if (s.kind !== "xtream" || (!s.expiresAt && !s.accountStatus)) return null;
    const status = s.accountStatus && s.accountStatus.toLowerCase() !== "active" ? s.accountStatus : "";
    if (!s.expiresAt) return { text: status || t("account.noExpiry"), tone: status ? "bad" : "ok" };
    const left = s.expiresAt - Date.now() / 1000;
    if (left <= 0) return { text: t("account.expired", { date: date(s.expiresAt) }), tone: "bad" };
    const tone = status ? "bad" : left < 7 * DAY ? "warn" : "ok";
    return { text: `${status ? status + " · " : ""}${t("account.expires", { date: date(s.expiresAt) })}`, tone };
  }

  const toneClass = {
    ok: "border-emerald-500/30 text-emerald-300",
    warn: "border-amber-500/40 text-amber-300",
    bad: "border-red-500/40 text-red-300",
  } as const;

  // Swatch colours mirror the --color-accent values in app.css.
  const swatch: Record<(typeof accents)[number], string> = {
    gold: "#e0b04a",
    nile: "#2dd4bf",
    lapis: "#7aa2ff",
    crimson: "#ff5a64",
    amethyst: "#c08cff",
    emerald: "#5ee08a",
  };
  const locales = Object.keys(localeNames) as Locale[];
  const languagePref = $derived(prefs.language === "auto" ? "auto" : currentLocale());
</script>

<div class="mx-auto max-w-3xl space-y-10 pt-2 pb-6">
  <section>
    <div class="mb-4 flex flex-wrap items-center justify-between gap-3">
      <h3 class="section-title">{t("settings.lists")}</h3>
      <div class="flex gap-2">
        <button class="btn-ghost px-3 py-1.5 text-sm" disabled={refreshing !== "" || !sources.some((s) => s.enabled)} onclick={onrefreshall}>
          {t("settings.refreshAll")}
        </button>
        <button class="btn-accent px-3 py-1.5 text-sm" onclick={onadd}>{t("settings.addList")}</button>
      </div>
    </div>

    {#if sources.length === 0}
      <p class="rounded-2xl border border-dashed border-line px-4 py-8 text-center text-sm text-muted">{t("settings.noLists")}</p>
    {:else}
      <ul class="space-y-2">
        {#each sources as s (s.id)}
          {@const acc = account(s)}
          <li class="flex flex-col gap-3 rounded-2xl border border-line bg-surface p-4 sm:flex-row sm:items-center">
            <label class="flex shrink-0 cursor-pointer items-center" title={t(s.enabled ? "settings.disable" : "settings.enable")}>
              <input
                type="checkbox"
                class="size-4 accent-accent"
                checked={s.enabled}
                aria-label={t(s.enabled ? "settings.disableNamed" : "settings.enableNamed", { name: s.name })}
                onchange={(e) => onenable(s, e.currentTarget.checked)}
              />
            </label>
            <div class="min-w-0 flex-1" class:opacity-75={!s.enabled}>
              <div class="flex flex-wrap items-center gap-2">
                <p class="truncate font-semibold">{s.name}</p>
                <span class="shrink-0 rounded-full border border-line px-2 py-0.5 text-[0.65rem] tracking-wider text-muted uppercase"
                  >{t(kindKey[s.kind])}</span
                >
                {#if !s.enabled}
                  <span class="shrink-0 rounded-full border border-line px-2 py-0.5 text-[0.65rem] tracking-wider text-muted uppercase"
                    >{t("settings.disabled")}</span
                  >
                {/if}
                {#if acc}
                  <span class="shrink-0 rounded-full border px-2 py-0.5 text-[0.7rem] {toneClass[acc.tone]}">{acc.text}</span>
                {/if}
              </div>
              <p class="mt-0.5 truncate text-sm text-muted" title={s.location}>{s.location || "—"}</p>
              <p class="mt-1 text-xs text-muted tabular-nums">
                {t("settings.listStats", { n: s.channelCount, when: s.updatedAt ? ago(s.updatedAt) : t("settings.never") })}
              </p>
            </div>
            <div class="flex shrink-0 gap-2">
              <button class="btn-ghost px-3 py-1.5 text-sm" disabled={refreshing !== ""} onclick={() => onrefresh(s)}>
                <span class="inline-block" class:animate-spin={refreshing === s.id}>↻</span>
                {t("settings.refresh")}
              </button>
              <button class="btn-ghost px-3 py-1.5 text-sm" onclick={() => onedit(s)}>{t("settings.edit")}</button>
              <button
                class="btn-ghost px-3 py-1.5 text-sm hover:!bg-red-500/15 hover:!text-red-300"
                aria-label={t("settings.removeNamed", { name: s.name })}
                onclick={() => onremove(s)}>{t("settings.remove")}</button
              >
            </div>
          </li>
        {/each}
      </ul>
    {/if}

    <label class="mt-3 flex cursor-pointer items-center gap-3 px-1 text-sm">
      <input
        type="checkbox"
        class="size-4 accent-accent"
        checked={settings?.refreshOnStart ?? false}
        disabled={!settings}
        onchange={(e) => {
          const on = e.currentTarget.checked;
          onsettings({ refreshOnStart: on }, () => t(on ? "toast.refreshOnStartOn" : "toast.refreshOnStartOff"));
        }}
      />
      {t("settings.refreshOnStart")}
    </label>
  </section>

  <section>
    <h3 class="section-title mb-1">{t("settings.player")}</h3>
    <p class="mb-4 text-sm text-muted">{t("settings.playerHint")}</p>
    {#if settings}
      <div class="grid gap-2 sm:grid-cols-2">
        {#each settings.players as p (p.id)}
          <label
            class="flex items-center gap-3 rounded-2xl border p-4 transition-colors
              {settings.player === p.id ? 'border-accent/75 bg-accent/10' : 'border-line bg-surface'}
              {p.installed ? 'cursor-pointer hover:border-line-hi' : 'cursor-not-allowed opacity-60'}"
          >
            <input
              type="radio"
              name="player"
              class="size-4 accent-accent"
              value={p.id}
              checked={settings.player === p.id}
              disabled={!p.installed}
              onchange={() => onsettings({ player: p.id }, () => t("toast.playerSet", { player: p.name }))}
            />
            <span class="flex-1 font-medium">{p.name}</span>
            <span class="text-xs text-muted">{t(p.installed ? "settings.installed" : "settings.notInstalled")}</span>
          </label>
        {/each}
      </div>

      {@const current = settings.players.find((p) => p.id === settings.player)}
      {@const supports = (o: keyof PlaybackOptions) => current?.options.includes(o) ?? false}
      {@const unsupported = t("settings.optionUnsupported", { player: current?.name ?? "" })}
      <div class="mt-4 space-y-4 rounded-2xl border border-line bg-surface p-4">
        <div>
          <p class="font-medium">{t("settings.playback")}</p>
          <p class="text-sm text-muted">{t("settings.playbackHint")}</p>
        </div>

        <label class="flex items-start gap-3" class:opacity-50={!supports("softwareDecoding")}>
          <input
            type="checkbox"
            class="mt-1 size-4 accent-accent"
            checked={settings.softwareDecoding}
            disabled={!supports("softwareDecoding")}
            onchange={(e) => onsettings({ softwareDecoding: e.currentTarget.checked }, () => t("toast.settingsSaved"))}
          />
          <span>
            <span class="block">{t("settings.softwareDecoding")}</span>
            <span class="block text-sm text-muted">
              {supports("softwareDecoding") ? t("settings.softwareDecodingHint") : unsupported}
            </span>
          </span>
        </label>

        <div class="grid gap-4 sm:grid-cols-2">
          <label class="block" class:opacity-50={!supports("cacheSecs")}>
            <span class="mb-1 block text-sm">{t("settings.cache")}</span>
            <select
              class="field"
              value={settings.cacheSecs}
              disabled={!supports("cacheSecs")}
              onchange={(e) => onsettings({ cacheSecs: Number(e.currentTarget.value) }, () => t("toast.settingsSaved"))}
            >
              {#each settings.cacheChoices as n (n)}
                <option value={n}>{n === 0 ? t("settings.cacheDefault") : t("settings.cacheSecs", { n })}</option>
              {/each}
            </select>
            <span class="mt-1 block text-xs text-muted">{supports("cacheSecs") ? t("settings.cacheHint") : unsupported}</span>
          </label>

          <label class="block" class:opacity-50={!supports("volume")}>
            <span class="mb-1 flex justify-between text-sm">
              <span>{t("settings.volume")}</span>
              <span class="text-muted tabular-nums">{settings.volume === 0 ? t("settings.volumeDefault") : `${settings.volume} %`}</span>
            </span>
            <!-- Saved on release ("change"), not on every step of the drag. -->
            <input
              type="range"
              min="0"
              max="100"
              step="5"
              class="w-full accent-accent"
              value={settings.volume}
              disabled={!supports("volume")}
              onchange={(e) => onsettings({ volume: Number(e.currentTarget.value) }, () => t("toast.settingsSaved"))}
            />
            {#if !supports("volume")}<span class="mt-1 block text-xs text-muted">{unsupported}</span>{/if}
          </label>
        </div>
      </div>
    {:else}
      <p class="text-sm text-muted">{t("settings.loading")}</p>
    {/if}
  </section>

  <section>
    <h3 class="section-title mb-4">{t("settings.appearance")}</h3>
    <div class="space-y-3">
      <div class="rounded-2xl border border-line bg-surface p-4">
        <p class="mb-3 font-medium">{t("settings.accent")}</p>
        <div class="flex flex-wrap gap-2" role="radiogroup" aria-label={t("settings.accent")}>
          {#each accents as a (a)}
            <button
              role="radio"
              aria-checked={prefs.accent === a}
              class="flex items-center gap-2 rounded-full border px-3 py-1.5 text-sm transition-colors
                {prefs.accent === a ? 'border-accent/60 bg-accent/10 text-text' : 'border-line text-muted hover:border-line-hi hover:text-text'}"
              onclick={() => setAccent(a)}
            >
              <span class="size-3.5 rounded-full" style="background: {swatch[a]}"></span>
              {t(`accent.${a}` as MessageKey)}
            </button>
          {/each}
        </div>
      </div>

      <label class="flex flex-col gap-2 rounded-2xl border border-line bg-surface p-4 sm:flex-row sm:items-center sm:justify-between">
        <span class="font-medium">{t("settings.language")}</span>
        <select
          class="field w-auto"
          value={languagePref}
          onchange={(e) => setLanguage(e.currentTarget.value as "auto" | Locale)}
        >
          <option value="auto">{t("settings.languageAuto", { name: localeNames[systemLocale()] })}</option>
          {#each locales as l (l)}
            <option value={l}>{localeNames[l]}</option>
          {/each}
        </select>
      </label>

      <label class="flex cursor-pointer items-start gap-4 rounded-2xl border border-line bg-surface p-4">
        <input type="checkbox" class="mt-1 size-4 accent-accent" bind:checked={prefs.logoColors} onchange={savePrefs} />
        <span>
          <span class="block font-medium">{t("settings.logoColors")}</span>
          <span class="block text-sm text-muted">{t("settings.logoColorsHint")}</span>
        </span>
      </label>

      <div class="rounded-2xl border border-line bg-surface p-4">
        <p class="mb-3 font-medium">{t("settings.browse")}</p>
        <div class="grid gap-2 sm:grid-cols-2" role="radiogroup" aria-label={t("settings.browse")}>
          {#each browseModes as m (m.id)}
            <label
              class="flex cursor-pointer items-start gap-3 rounded-xl border p-3 transition-colors
                {prefs.browse === m.id ? 'border-accent/60 bg-accent/10' : 'border-line hover:border-line-hi'}"
            >
              <input
                type="radio"
                name="browse"
                class="mt-1 size-4 accent-accent"
                checked={prefs.browse === m.id}
                onchange={() => {
                  prefs.browse = m.id;
                  savePrefs();
                }}
              />
              <span>
                <span class="block font-medium">{t(m.label)}</span>
                <span class="block text-sm text-muted">{t(m.hint)}</span>
              </span>
            </label>
          {/each}
        </div>
        {#if prefs.browse === "scroll"}
          <p class="mt-3 rounded-xl border border-amber-500/30 bg-amber-500/10 px-3 py-2 text-sm text-amber-200" role="note">
            ⚠ {t("settings.browseScrollWarning")}
          </p>
        {/if}
      </div>

      <label class="flex cursor-pointer items-start gap-4 rounded-2xl border border-line bg-surface p-4">
        <input
          type="checkbox"
          class="mt-1 size-4 accent-accent"
          checked={prefs.effects}
          onchange={(e) => setEffects(e.currentTarget.checked)}
        />
        <span>
          <span class="block font-medium">{t("settings.effects")}</span>
          <span class="block text-sm text-muted">{t("settings.effectsHint")}</span>
        </span>
      </label>
    </div>
  </section>

  <section>
    <h3 class="section-title mb-1">{t("settings.hidden")}</h3>
    <p class="mb-4 text-sm text-muted">{t("settings.hiddenHint")}</p>
    <div class="space-y-3 rounded-2xl border border-line bg-surface p-4">
      {#if hiddenGroups.length}
        <div class="flex flex-wrap gap-2">
          {#each hiddenGroups as g (g)}
            <button
              class="rounded-full border border-line px-3 py-1 text-sm text-muted hover:border-accent/50 hover:text-text"
              title={t("settings.showAgain")}
              onclick={() => onunhidegroup(g)}>{g || t("nav.noGroupName")} ✕</button
            >
          {/each}
        </div>
      {:else}
        <p class="text-sm text-muted">{t("settings.noHiddenGroups")}</p>
      {/if}
      <button class="btn-ghost px-3 py-1.5 text-sm" onclick={onshowhidden}>{t("settings.showHiddenChannels")}</button>
    </div>
  </section>

  <section>
    <h3 class="section-title mb-1">{t("settings.data")}</h3>
    <p class="mb-4 text-sm text-muted">{t("settings.dataHint")}</p>
    <div class="flex flex-wrap gap-2">
      <button class="btn-ghost px-3 py-1.5 text-sm" onclick={onexport}>{t("settings.export")}</button>
      <button class="btn-ghost px-3 py-1.5 text-sm" onclick={onimport}>{t("settings.import")}</button>
      <button class="btn-ghost px-3 py-1.5 text-sm hover:!bg-red-500/15 hover:!text-red-300" onclick={onclearhistory}
        >{t("settings.clearHistory")}</button
      >
    </div>
  </section>

  <section>
    <h3 class="section-title mb-1">{t("settings.perf")}</h3>
    <p class="mb-4 text-sm text-muted">{t("settings.perfHint")}</p>
    <button class="btn-ghost" onclick={onperf}>{t("settings.perfRun")}</button>
  </section>

  <section>
    <h3 class="section-title mb-4">{t("settings.about")}</h3>
    <dl class="grid grid-cols-[auto_1fr] gap-x-6 gap-y-2 rounded-2xl border border-line bg-surface p-4 text-sm">
      <dt class="text-muted">{t("settings.dataDir")}</dt>
      <dd class="font-mono text-xs break-all">{settings?.dataDir ?? ""}</dd>
      <dt class="text-muted">{t("settings.playerRow")}</dt>
      <dd>{t("settings.playerExternal", { player: settings?.players.find((p) => p.id === settings?.player)?.name ?? "—" })}</dd>
    </dl>
  </section>
</div>
