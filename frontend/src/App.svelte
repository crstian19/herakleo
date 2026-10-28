<script lang="ts">
  import { onMount, tick, untrack } from "svelte";
  import { benchFlags, idleFps, perfEvent, runBench, scrollBench, scrollVariantsBench, wheelFrames } from "./lib/bench";
  import {
    api,
    errorMessage,
    type AppSettings,
    type Channel,
    type ChannelSort,
    type ChannelView,
    type Group,
    type SettingsPatch,
    type SourceInfo,
  } from "./lib/api";
  import ChannelCard from "./lib/ChannelCard.svelte";
  import { num, t } from "./lib/i18n.svelte";
  import { prefs, savePrefs, setEffects } from "./lib/prefs.svelte";
  import { watchSlowScroll } from "./lib/slowscroll";
  import Settings from "./lib/Settings.svelte";
  import { gridMove, isTyping } from "./lib/shortcuts";
  import ShortcutsHelp from "./lib/ShortcutsHelp.svelte";
  import SupportDialog from "./lib/SupportDialog.svelte";
  import MadeWith from "./lib/MadeWith.svelte";
  import DiagnosticsDialog from "./lib/DiagnosticsDialog.svelte";
  import { buildReport } from "./lib/diagnostics";
  import SourceDialog from "./lib/SourceDialog.svelte";

  const PAGE = 120; // cards fetched per load in scroll mode
  const PAGE_SIZE = 60; // cards per page in pagination mode (the default)

  let sources = $state<SourceInfo[]>([]);
  let groups = $state<Group[]>([]);
  let hiddenGroups = $state<string[]>([]);
  let channels = $state<Channel[]>([]);
  let total = $state(0);

  // Pagination (default) or infinite scroll, from Settings; page is 0-based.
  let page = $state(0);
  const paged = $derived(prefs.browse === "pages");
  const pageCount = $derived(Math.max(1, Math.ceil(total / PAGE_SIZE)));

  let sourceId = $state("");
  let group = $state("");
  let listView = $state<ChannelView>("");
  let searchInput = $state("");
  let search = $state("");
  // Sidebar group filter. Matches like the channel search in the backend:
  // trimmed, case-insensitive substring. Names are lower-cased once per load,
  // not per keystroke; hundreds of groups need no debounce.
  let groupQuery = $state("");
  const groupKeys = $derived(groups.map((g) => g.name.toLowerCase()));
  const shownGroups = $derived.by(() => {
    const needle = groupQuery.trim().toLowerCase();
    return needle ? groups.filter((_, i) => groupKeys[i]!.includes(needle)) : groups;
  });

  let loading = $state(false);
  let refreshing = $state("");
  let playingId = $state("");
  let view = $state<"channels" | "settings">("channels");
  let appSettings = $state<AppSettings | null>(null);
  const playerName = $derived(appSettings?.players.find((p) => p.id === appSettings?.player)?.name ?? t("toast.thePlayer"));
  // null: closed; { editing: null }: adding; { editing: s }: editing s.
  let dialog = $state<{ editing: SourceInfo | null } | null>(null);
  // Logo URL -> accent colour from the backend ("" when it has none).
  let logoColors = $state<Record<string, string>>({});
  const colorRequested = new Set<string>();
  // Below the lg breakpoint the sidebar is an off-canvas drawer.
  let navOpen = $state(false);
  let toast = $state<{ text: string; error: boolean } | null>(null);

  let sentinel = $state<HTMLElement>();
  let gridWrap = $state<HTMLElement>();
  let gridEl = $state<HTMLElement>();
  // Virtualised grid: only the rows near the viewport are in the DOM. With
  // infinite scroll the page used to keep every card ever loaded, and past
  // ~1,500 cards each frame took 600+ ms (docs/PERFORMANCE.md).
  let win = $state({ start: 0, end: 0, padTop: 0, padBottom: 0, cols: 1, rowH: 160 });
  const visible = $derived(channels.slice(win.start, win.end));
  const WINDOW_MARGIN = 800; // px rendered above and below the viewport
  let scroller = $state<HTMLElement>();
  let sidebar = $state<HTMLElement>();
  let searchEl = $state<HTMLInputElement>();
  let groupSearchEl = $state<HTMLInputElement>();
  let helpOpen = $state(false);
  // The "open project, support it" popup: never on first launch. It shows on
  // the SUPPORT_AFTER-th channel played (the app has proved useful by then),
  // after that only by chance (SUPPORT_ODDS per play, about once every 200
  // plays), and from the sidebar any time. Counters are per build (localStorage).
  const SUPPORT_SEEN = "herakleo:support-seen";
  const PLAYS = "herakleo:plays";
  const SUPPORT_AFTER = 10;
  const SUPPORT_ODDS = 1 / 200;
  let supportOpen = $state(false);
  function countPlay() {
    try {
      const plays = Number(localStorage.getItem(PLAYS) ?? 0) + 1;
      localStorage.setItem(PLAYS, String(plays));
      if (plays < SUPPORT_AFTER) return;
      if (!localStorage.getItem(SUPPORT_SEEN) || Math.random() < SUPPORT_ODDS) supportOpen = true;
    } catch {
      // No storage: never ask.
    }
  }
  function closeSupport() {
    supportOpen = false;
    try {
      localStorage.setItem(SUPPORT_SEEN, "1");
    } catch {
      // Without storage it can show again next launch.
    }
  }
  // Settings → Performance: "measuring" while the user scrolls, then the report.
  let perf = $state<"measuring" | { report: string } | null>(null);
  // True while the grid scrolls and for 150 ms after: cards then ignore the
  // pointer (app.css .is-scrolling) so hover effects don't churn.
  let scrolling = $state(false);
  let scrollIdle: ReturnType<typeof setTimeout> | undefined;
  // Only while the effects are on and the user never picked them by hand;
  // the performance meter is left to measure undisturbed.
  const slowScroll = watchSlowScroll(() => {
    if (!prefs.effects || prefs.effectsChosen || perf === "measuring") return;
    setEffects(false, false);
    notify(t("toast.effectsAutoOff"), false, 8000);
  });
  function markScrolling() {
    scheduleWindow();
    if (prefs.effects && !prefs.effectsChosen && perf !== "measuring") slowScroll();
    if (!scrolling) perfEvent("hover-off");
    scrolling = true;
    clearTimeout(scrollIdle);
    scrollIdle = setTimeout(() => {
      scrolling = false;
      perfEvent("hover-on");
    }, 150);
  }
  // Bumped on every new query so late responses from older ones are dropped.
  let generation = 0;
  let groupsGeneration = 0;
  const pending = new Set<string>();

  // Each build shows its own mascot (Ferris for Tauri, the gopher for Wails).
  const logoSrc = api.name === "mock" ? "/logo.svg" : `/logo-${api.name}.png`;
  const enabledSources = $derived(sources.filter((s) => s.enabled));
  const viewTitle = (v: ChannelView) =>
    t(v === "favorites" ? "view.favorites" : v === "history" ? "view.history" : v === "hidden" ? "view.hidden" : "view.all");
  const heading = $derived(
    view === "settings"
      ? t("view.settings")
      : listView
        ? viewTitle(listView)
        : group || (sources.find((s) => s.id === sourceId)?.name ?? viewTitle("")),
  );

  function notify(text: string, error = false, ms = error ? 6000 : 2500) {
    toast = { text, error };
    const current = toast;
    setTimeout(() => toast === current && (toast = null), ms);
  }

  async function loadSources() {
    try {
      sources = await api.listSources();
    } catch (err) {
      notify(errorMessage(err), true);
    }
  }

  async function loadGroups() {
    const gen = ++groupsGeneration;
    try {
      const [result, hidden] = await Promise.all([api.listGroups(sourceId), api.listHiddenGroups()]);
      if (gen === groupsGeneration) [groups, hiddenGroups] = [result, hidden];
    } catch (err) {
      notify(errorMessage(err), true);
    }
  }

  async function loadChannels(reset: boolean) {
    const gen = reset ? ++generation : generation;
    // Pagination loads one page at a time (goToPage); only scroll mode appends.
    if (!reset && (paged || loading || channels.length >= total)) return;
    loading = true;
    try {
      const result = await api.listChannels({
        sourceId,
        group,
        search,
        view: listView,
        sort: prefs.sort,
        offset: paged ? page * PAGE_SIZE : reset ? 0 : channels.length,
        limit: paged ? PAGE_SIZE : PAGE,
      });
      if (gen !== generation) return;
      channels = reset ? result.items : [...channels, ...result.items];
      perfEvent(reset ? "page-reset" : "page");
      total = result.total;
      requestColors(result.items);
      if (reset) scroller?.scrollTo({ top: 0 });
      // The observer only fires on visibility changes; on tall screens the
      // sentinel can still be in view after a page lands, so keep filling.
      queueMicrotask(() => {
        if (sentinel && scroller && sentinel.getBoundingClientRect().top < scroller.getBoundingClientRect().bottom + 600) {
          loadChannels(false);
        }
      });
    } catch (err) {
      notify(errorMessage(err), true);
    } finally {
      if (gen === generation) loading = false;
    }
  }

  /** Recomputes which rows to render from the scroll position and grid geometry. */
  function updateWindow() {
    if (!scroller || !gridWrap || !gridEl) return;
    const style = getComputedStyle(gridEl);
    const tracks = style.gridTemplateColumns.split(" ").filter((t) => t.endsWith("px"));
    const cols = Math.max(1, tracks.length);
    // Cards are square (aspect-ratio: 1), so a row is one column wide plus the gap.
    const rowH = (parseFloat(tracks[0] ?? "160") || 160) + (parseFloat(style.rowGap) || 0);
    const top =
      scroller.scrollTop - (gridWrap.getBoundingClientRect().top - scroller.getBoundingClientRect().top + scroller.scrollTop);
    const rows = Math.ceil(channels.length / cols);
    const first = Math.max(0, Math.floor((top - WINDOW_MARGIN) / rowH));
    const last = Math.min(rows, Math.ceil((top + scroller.clientHeight + WINDOW_MARGIN) / rowH));
    const next = {
      start: first * cols,
      end: Math.min(channels.length, last * cols),
      padTop: first * rowH,
      padBottom: Math.max(0, rows - last) * rowH,
      cols,
      rowH,
    };
    if (Object.entries(next).some(([k, v]) => win[k as keyof typeof win] !== v)) win = next;
  }

  let windowFrame = 0;
  function scheduleWindow() {
    if (!windowFrame) windowFrame = requestAnimationFrame(() => ((windowFrame = 0), updateWindow()));
  }

  // Also when the grid itself (re)appears: channels can arrive before the
  // sources, while the empty state is still on screen and gridEl is unset.
  $effect(() => {
    void [channels.length, view, gridEl, gridWrap];
    tick().then(updateWindow);
  });

  // Reloads what the sidebar and grid show after anything that changes them.
  function reloadLists() {
    loadGroups();
    loadChannels(true);
  }

  // Asks the backend for logo colours in small batches so cards light up
  // progressively; each URL is requested once per session (the backend also
  // caches on disk).
  async function requestColors(items: Channel[]) {
    if (!prefs.logoColors) return;
    const urls = [...new Set(items.map((c) => c.logo))].filter((u) => u && !colorRequested.has(u));
    urls.forEach((u) => colorRequested.add(u));
    for (let i = 0; i < urls.length; i += 24) {
      try {
        const got = await api.logoColors(urls.slice(i, i + 24));
        logoColors = { ...logoColors, ...got };
        perfEvent("colors");
      } catch {
        // Colours are cosmetic; cards keep their name-based hue.
      }
    }
  }

  // Debounce typing so a 30k-channel list is not filtered on every keystroke.
  $effect(() => {
    const value = searchInput;
    const t = setTimeout(() => (search = value.trim()), 180);
    return () => clearTimeout(t);
  });

  $effect(() => {
    void sourceId;
    loadGroups();
  });

  // Any change of what is listed (or of the browse mode) starts again at page 1.
  // untrack: loadChannels reads `page`, which must not become a dependency.
  $effect(() => {
    void [sourceId, group, search, listView, prefs.sort, prefs.browse];
    untrack(() => {
      page = 0;
      loadChannels(true);
    });
  });

  function goToPage(n: number) {
    const next = Math.min(Math.max(n, 0), pageCount - 1);
    if (next === page) return false;
    page = next;
    loadChannels(true);
    return true;
  }

  async function loadSettings() {
    try {
      appSettings = await api.getSettings();
    } catch (err) {
      notify(errorMessage(err), true);
    }
  }

  async function changeSettings(patch: SettingsPatch, message: () => string) {
    try {
      appSettings = await api.updateSettings(patch);
      notify(message());
    } catch (err) {
      notify(errorMessage(err), true);
      loadSettings();
    }
  }

  // `HERAKLEO_BENCH_OUT=<file> herakleo-…`: wait for the first page of
  // channels to be on screen, time the API from inside the webview, write the
  // report and quit. Used by `just bench`.
  async function benchUnattended() {
    prefs.browse = "scroll"; // not saved: benchmarks measure infinite scroll
    await waitLoaded();
    while (channels.length === 0 && sources.length > 0) await new Promise((r) => setTimeout(r, 20));
    await tick();
    const firstPageAt = Date.now(); // wall clock, so the script can measure from process start
    const result = await runBench(api);
    const fps = await idleFps();
    const scroll = scroller ? await scrollBench(scroller) : null;
    const variants = scroller ? await scrollVariantsBench(scroller) : null;
    await api.benchReport(JSON.stringify({ ...result, firstPageAt, fps, scroll, variants }));
  }

  // scripts/profile-scroll.sh drives real wheel events with ydotool while the
  // cursor sits on the grid (so :hover churns like for a person scrolling);
  // here we only record frame times while scrolling is happening.
  async function wheelSession(variant: string) {
    prefs.browse = "scroll"; // not saved: the preload below needs infinite scroll
    // The mode switch reloads the list asynchronously: wait until it has landed,
    // or the preload below can see total = 0 and skip (a run then scrolls ~5k px).
    await waitLoaded();
    while (loading || total === 0) await new Promise((r) => setTimeout(r, 20));
    benchFlags.syncDecode = variant.startsWith("sync-decode");
    if (variant === "effects-off") document.documentElement.dataset.effects = "off";
    while (channels.length === 0 && sources.length > 0) await new Promise((r) => setTimeout(r, 20));
    // Preload enough cards that every run scrolls the same distance: otherwise
    // a run that outpaces infinite scroll hits the bottom, scrolls less, paints
    // less and looks faster than it is.
    while (channels.length < Math.min(1500, total)) {
      const before = channels.length;
      await loadChannels(false);
      if (channels.length === before) await new Promise((r) => setTimeout(r, 50));
    }
    await tick();
    const result = scroller ? { variant, ...(await wheelFrames(scroller, variant)) } : null;
    await api.benchReport(JSON.stringify({ mode: "wheel", ...result }));
  }

  onMount(() => {
    (async () => {
      await Promise.all([loadSources(), loadSettings()]);
      const bench = await api.benchMode();
      if (bench === "full") return benchUnattended();
      if (bench.startsWith("wheel")) return wheelSession(bench.split(":")[1] ?? "baseline");
      if (appSettings?.refreshOnStart) refreshAll();
    })();
    const io = new IntersectionObserver((entries) => entries[0]?.isIntersecting && loadChannels(false), {
      root: scroller,
      rootMargin: "600px",
    });
    if (sentinel) io.observe(sentinel);
    // Columns change with the window width: recompute the rendered rows.
    const ro = new ResizeObserver(scheduleWindow);
    if (scroller) ro.observe(scroller);
    return () => {
      io.disconnect();
      ro.disconnect();
    };
  });

  function show(next: { sourceId?: string; group?: string; listView?: ChannelView }) {
    // Settings and the grid share the scroller: don't land mid-list at whatever
    // offset Settings was scrolled to.
    if (view !== "channels") scroller?.scrollTo({ top: 0 });
    view = "channels";
    sourceId = next.sourceId ?? "";
    group = next.group ?? "";
    listView = next.listView ?? "";
    navOpen = false;
  }

  function selectGroup(name: string) {
    view = "channels";
    group = name;
    navOpen = false;
  }

  async function play(c: Channel) {
    try {
      await api.play(c.id);
      playingId = c.id;
      notify(t("toast.playing", { name: c.name, player: playerName }));
      countPlay();
      // The backend records history on play; keep the Recent view current.
      if (listView === "history") loadChannels(true);
    } catch (err) {
      notify(errorMessage(err), true);
    }
  }

  // Runs one toggle per channel at a time and drops the card if it no longer
  // belongs in the list the click happened in.
  async function toggle(c: Channel, call: () => Promise<void>, apply: () => void, leaves: boolean) {
    if (pending.has(c.id)) return;
    pending.add(c.id);
    const gen = generation;
    try {
      await call();
      apply();
      if (leaves && gen === generation) {
        channels = channels.filter((x) => x.id !== c.id);
        total -= 1;
      }
    } catch (err) {
      notify(errorMessage(err), true);
    } finally {
      pending.delete(c.id);
    }
  }

  function toggleFavorite(c: Channel) {
    const next = !c.favorite;
    toggle(c, () => api.setFavorite(c.id, next), () => (c.favorite = next), listView === "favorites" && !next);
  }

  function toggleHidden(c: Channel) {
    const next = !c.hidden;
    toggle(
      c,
      () => api.setChannelHidden(c.id, next),
      () => {
        c.hidden = next;
        notify(t(next ? "toast.channelHidden" : "toast.channelShown", { name: c.name }));
        loadGroups();
      },
      // Hiding removes it from every normal view; unhiding removes it from the hidden view.
      true,
    );
  }

  async function setGroupHidden(name: string, hidden: boolean) {
    try {
      await api.setGroupHidden(name, hidden);
      if (hidden && group === name) group = "";
      notify(t(hidden ? "toast.groupHidden" : "toast.groupShown", { name: name || t("toast.unnamed") }));
      reloadLists();
    } catch (err) {
      notify(errorMessage(err), true);
    }
  }

  async function refresh(s: SourceInfo, quiet = false) {
    if (refreshing) return;
    refreshing = s.id;
    try {
      const updated = await api.refreshSource(s.id);
      sources = sources.map((x) => (x.id === updated.id ? updated : x));
      if (!quiet) notify(t("toast.listCount", { name: updated.name, n: updated.channelCount }));
      reloadLists();
    } catch (err) {
      notify(`${s.name}: ${errorMessage(err)}`, true);
    } finally {
      refreshing = "";
    }
  }

  // One list at a time: providers often limit concurrent connections.
  async function refreshAll() {
    const todo = sources.filter((s) => s.enabled);
    for (const s of todo) await refresh(s, true);
    if (todo.length) notify(todo.length === 1 ? t("toast.refreshedOne") : t("toast.refreshedMany", { n: todo.length }));
  }

  async function setEnabled(s: SourceInfo, enabled: boolean) {
    try {
      const updated = await api.setSourceEnabled(s.id, enabled);
      sources = sources.map((x) => (x.id === updated.id ? updated : x));
      if (!enabled && sourceId === s.id) sourceId = "";
      reloadLists();
    } catch (err) {
      notify(errorMessage(err), true);
    }
  }

  async function remove(s: SourceInfo) {
    if (!confirm(t("confirm.removeList", { name: s.name }))) return;
    try {
      await api.removeSource(s.id);
      sources = sources.filter((x) => x.id !== s.id);
      if (sourceId === s.id) sourceId = "";
      reloadLists();
    } catch (err) {
      notify(errorMessage(err), true);
    }
  }

  function saved(s: SourceInfo) {
    const isNew = !sources.some((x) => x.id === s.id);
    dialog = null;
    sources = isNew ? [...sources, s] : sources.map((x) => (x.id === s.id ? s : x));
    notify(t("toast.listCount", { name: s.name, n: s.channelCount }));
    if (isNew) show({ sourceId: s.id });
    else reloadLists();
  }

  async function clearHistory() {
    if (!confirm(t("confirm.clearHistory"))) return;
    try {
      await api.clearHistory();
      notify(t("toast.historyCleared"));
      if (listView === "history") loadChannels(true);
    } catch (err) {
      notify(errorMessage(err), true);
    }
  }

  async function exportFavorites() {
    try {
      const path = await api.exportFavorites(t("settings.exportTitle"));
      if (path) notify(t("toast.favoritesSaved", { path }));
    } catch (err) {
      notify(errorMessage(err), true);
    }
  }

  async function importFavorites() {
    try {
      const n = await api.importFavorites(t("settings.importTitle"));
      if (n === null) return;
      notify(n ? t("toast.favoritesRestored", { n }) : t("toast.favoritesNone"), n === 0);
      if (n) loadChannels(true);
    } catch (err) {
      notify(errorMessage(err), true);
    }
  }

  function setSort(sort: ChannelSort) {
    prefs.sort = sort;
    savePrefs();
  }

  // Measures real wheel scrolling on whatever the user's machine is: go to the
  // grid, wait for them to scroll, then show a copyable report.
  async function measureScroll() {
    show({});
    await tick();
    while (loading) await new Promise((r) => setTimeout(r, 50));
    if (!scroller) return;
    scroller.scrollTo({ top: 0 });
    perf = "measuring";
    const fps = await idleFps(1500);
    const scroll = await wheelFrames(scroller, "baseline", 30000);
    perf = { report: await buildReport(api, fps, scroll, total) };
  }

  // ── Keyboard and TV remote ──
  // Single keys only fire outside text fields; everything is also reachable
  // with arrows + Enter + back, which is all most remotes send.

  async function focusSearch() {
    if (view !== "channels") view = "channels";
    await tick();
    searchEl?.focus();
    searchEl?.select();
  }

  async function waitLoaded() {
    await tick();
    while (loading) await new Promise((r) => setTimeout(r, 20));
    await tick();
  }

  /** Focuses the card at a list position, scrolling its row into view first (it may not be rendered yet). */
  function focusCard(index: number) {
    if (!scroller || !gridWrap || channels.length === 0) return false;
    const i = Math.min(Math.max(index, 0), channels.length - 1);
    const rowTop =
      gridWrap.getBoundingClientRect().top - scroller.getBoundingClientRect().top + scroller.scrollTop +
      Math.floor(i / win.cols) * win.rowH;
    const cardH = win.rowH - (parseFloat(getComputedStyle(gridEl!).rowGap) || 0);
    if (rowTop < scroller.scrollTop) scroller.scrollTop = rowTop;
    else if (rowTop + cardH > scroller.scrollTop + scroller.clientHeight) scroller.scrollTop = rowTop + cardH - scroller.clientHeight;
    updateWindow();
    tick().then(() => {
      const el = gridEl?.querySelector<HTMLElement>(`.card[data-index="${i}"]`);
      el?.focus({ preventScroll: true });
      el?.scrollIntoView({ block: "nearest" });
    });
    return true;
  }

  /** The sidebar entries arrows can walk, in order (only visible ones). The group filter is one of them. */
  const sidebarItems = () =>
    [...(sidebar?.querySelectorAll<HTMLElement>("button.chip, input[data-group-filter]") ?? [])].filter((b) => b.offsetParent);

  /** Focuses the first sidebar element matching one of the selectors, tried in order. */
  async function focusSidebar(...selectors: string[]) {
    if (window.matchMedia("(max-width: 1023px)").matches) navOpen = true;
    await tick();
    const found = (selectors.length ? selectors : ["button.chip.is-active"])
      .map((s) => sidebar?.querySelector<HTMLElement>(s))
      .find(Boolean);
    (found ?? sidebarItems()[0])?.focus();
  }

  // The active group may be filtered out: then the first match, then the filter itself.
  const focusGroups = () =>
    focusSidebar('[data-nav="groups"] button.chip.is-active', '[data-nav="groups"] button.chip', "input[data-group-filter]");

  /** Arrows and Enter inside the group filter; Esc goes through back(). */
  function onGroupFilterKey(e: KeyboardEvent): boolean {
    if (e.key === "ArrowDown") {
      sidebar?.querySelector<HTMLElement>('[data-nav="groups"] button.chip')?.focus();
      return true;
    }
    if (e.key === "ArrowUp") {
      const items = sidebarItems();
      items[items.indexOf(groupSearchEl!) - 1]?.focus();
      return true;
    }
    // Enter picks the first match, so a remote needs no extra key presses.
    if (e.key === "Enter" && groupQuery.trim() && shownGroups[0]) {
      selectGroup(shownGroups[0].name);
      return true;
    }
    return false;
  }

  /** One step back, from the most local state to the widest. */
  function back(target: EventTarget | null) {
    // Before closing the drawer: the filter lives inside it.
    if (target === groupSearchEl && groupQuery) groupQuery = "";
    else if (target === groupSearchEl) focusGroups();
    else if (navOpen) navOpen = false;
    else if (target === searchEl && searchInput) searchInput = "";
    else if (target === searchEl) searchEl?.blur();
    else if (view === "settings") view = "channels";
    else if (group) group = "";
    else if (searchInput) searchInput = "";
    else if (listView || sourceId) show({});
    else return false;
    return true;
  }

  function onkey(e: KeyboardEvent) {
    // The list dialog, the support popup and the help own their keys while open.
    if (dialog || supportOpen) return;
    if (helpOpen) {
      if (e.key === "Escape" || e.key === "?") {
        e.preventDefault();
        helpOpen = false;
      }
      return;
    }
    const target = e.target;
    const typing = isTyping(target);

    if ((e.ctrlKey || e.metaKey) && !e.altKey && e.key.toLowerCase() === "f") {
      e.preventDefault();
      focusSearch();
      return;
    }
    if (e.key === "Escape") {
      if (back(target)) e.preventDefault();
      return;
    }
    if (typing) {
      if (target === searchEl && e.key === "ArrowDown" && focusCard(0)) e.preventDefault();
      if (target === groupSearchEl && onGroupFilterKey(e)) e.preventDefault();
      return;
    }
    if (e.ctrlKey || e.metaKey || e.altKey) return;

    const el = target instanceof HTMLElement ? target : null;
    const card = el?.closest<HTMLElement>(".channel-grid > .card");
    if (card && el === card && onCardKey(e, card)) return e.preventDefault();
    if (el && sidebar?.contains(el) && onSidebarKey(e, el)) return e.preventDefault();

    const handled = (() => {
      switch (e.key) {
        case "/":
          return void focusSearch();
        case "?":
          return void (helpOpen = true);
        case "1":
          return show({});
        case "2":
          return show({ listView: "favorites" });
        case "3":
          return show({ listView: "history" });
        case "g":
        case "G":
          return void focusGroups();
        case ",":
          view = "settings";
          return;
        case "]":
          return paged && view === "channels" && goToPage(page + 1) ? undefined : false;
        case "[":
          return paged && view === "channels" && goToPage(page - 1) ? undefined : false;
        case "Backspace":
        case "BrowserBack":
          return back(target) ? undefined : false;
        case "ArrowDown":
        case "ArrowRight":
          // Nothing focused yet: start on the grid.
          return !card && view === "channels" && focusCard(0) ? undefined : false;
        default:
          return false;
      }
    })();
    if (handled !== false) e.preventDefault();
  }

  function onCardKey(e: KeyboardEvent, card: HTMLElement): boolean {
    const index = Number(card.dataset.index);
    const c = channels[index];
    if (!Number.isInteger(index) || !c) return false;
    const cols = win.cols;
    if (e.key === "f" || e.key === "F") return (toggleFavorite(c), true);
    if (e.key === "h" || e.key === "H") return (toggleHidden(c), true);
    if (e.key === "ArrowUp" && index < cols) return (focusSearch(), true);
    if (e.key === "ArrowLeft" && index % cols === 0) return (focusSidebar(), true);
    const rows = Math.max(1, Math.floor((scroller?.clientHeight ?? 0) / win.rowH));
    const to = gridMove(index, e.key, cols, channels.length, rows);
    if (to === null) return false;
    // Pagination: going down past the last row turns the page (handy on a TV remote).
    if (paged && to === index && (e.key === "ArrowDown" || e.key === "PageDown") && page < pageCount - 1) {
      goToPage(page + 1);
      waitLoaded().then(() => focusCard(0));
      return true;
    }
    focusCard(to);
    // Walking near the end of what is loaded pulls in the next page.
    if (to >= channels.length - cols * 2) loadChannels(false);
    return true;
  }

  function onSidebarKey(e: KeyboardEvent, el: HTMLElement): boolean {
    const items = sidebarItems();
    const i = items.indexOf(el);
    if (i < 0) return false;
    if (e.key === "ArrowDown" || e.key === "ArrowUp") {
      items[Math.min(Math.max(i + (e.key === "ArrowDown" ? 1 : -1), 0), items.length - 1)]?.focus();
      return true;
    }
    if (e.key === "ArrowRight" && view === "channels") {
      navOpen = false;
      return focusCard(0);
    }
    return false;
  }
</script>

<div class="ambient" aria-hidden="true"></div>

<svelte:window onkeydown={onkey} />

{#snippet navItem(label: string, active: boolean, onclick: () => void, count?: number)}
  <button class="chip" class:is-active={active} {onclick}>
    <span class="flex-1 truncate">{label}</span>
    {#if count !== undefined}<span class="text-xs text-muted tabular-nums">{num(count)}</span>{/if}
  </button>
{/snippet}

<div class="relative z-10 flex h-full">
  {#if navOpen}
    <button class="fixed inset-0 z-30 bg-black/70 lg:hidden" aria-label={t("nav.closeMenu")} onclick={() => (navOpen = false)}></button>
  {/if}

  <!-- Sidebar: static on lg+, drawer below -->
  <!-- bind: arrow keys walk every chip in here, footer (settings) included -->
  <aside
    bind:this={sidebar}
    class="fixed inset-y-0 left-0 z-40 flex w-72 max-w-[85vw] shrink-0 flex-col border-r border-line bg-[#0c0c12]
      transition-transform duration-200 ease-out lg:static lg:z-auto lg:max-w-none lg:translate-x-0 lg:bg-[#0c0c12]/70
      {navOpen ? 'translate-x-0 shadow-2xl' : '-translate-x-full'}"
  >
    <!-- 128 px is exactly half the 256 px pixel-art mascot, so it stays crisp.
         Backend and channel count already show in the header. -->
    <div class="flex flex-col items-center gap-2 px-5 pt-6 pb-5">
      <div class="fx-glow logo-glow"><img src={logoSrc} alt="" class="relative size-32 [image-rendering:pixelated]" /></div>
      <h1 class="text-2xl leading-none font-bold tracking-tight">Herakleo</h1>
    </div>

    <nav class="flex-1 space-y-6 overflow-y-auto px-3 pb-6">
      <section class="space-y-0.5">
        {@render navItem(t("nav.all"), view === "channels" && !listView && !sourceId, () => show({}))}
        {@render navItem(t("nav.favorites"), view === "channels" && listView === "favorites", () => show({ listView: "favorites" }))}
        {@render navItem(t("nav.recent"), view === "channels" && listView === "history", () => show({ listView: "history" }))}
      </section>

      <section>
        <div class="mb-2 flex items-center justify-between pr-1">
          <h2 class="section-title">{t("nav.lists")}</h2>
          <button class="btn-ghost px-2 py-1 text-xs" onclick={() => (dialog = { editing: null })}>{t("nav.add")}</button>
        </div>
        <div class="space-y-0.5">
          {#each enabledSources as s (s.id)}
            {@render navItem(s.name, view === "channels" && !listView && sourceId === s.id, () => show({ sourceId: s.id }), s.channelCount)}
          {:else}
            <p class="px-3 py-1 text-xs text-muted">{t("nav.noActiveLists")}</p>
          {/each}
        </div>
      </section>

      {#if groups.length > 0 && view === "channels" && (listView === "" || listView === "favorites")}
        <section data-nav="groups">
          <h2 class="section-title mb-2">{t("nav.groups")}</h2>
          <!-- Same look as the channel search in the header -->
          <div class="relative mb-2">
            <svg class="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"
              ><circle cx="11" cy="11" r="7" /><path d="m20 20-3.5-3.5" /></svg
            >
            <input
              bind:this={groupSearchEl}
              data-group-filter
              class="field pl-9"
              type="search"
              placeholder={t("nav.filterGroups")}
              aria-label={t("nav.filterGroups")}
              bind:value={groupQuery}
            />
          </div>
          <div class="space-y-0.5">
            <!-- "All" is not a match: hidden while filtering, so ↓ lands on the first result -->
            {#if !groupQuery.trim()}{@render navItem(t("nav.all"), !group, () => selectGroup(""))}{/if}
            {#each shownGroups as g (g.name)}
              <div class="group/g relative">
                <button class="chip pr-9 lg:pr-3 lg:group-hover/g:pr-9" class:is-active={group === g.name} onclick={() => selectGroup(g.name)}>
                  <span class="flex-1 truncate">{g.name || t("nav.noGroupName")}</span>
                  <span class="text-xs text-muted tabular-nums">{g.count}</span>
                </button>
                <!-- Always visible on small screens, where there is no hover -->
                <button
                  class="absolute top-1/2 right-1.5 grid size-6 -translate-y-1/2 place-items-center rounded-md text-muted hover:bg-white/10 hover:text-text lg:hidden lg:group-hover/g:grid"
                  title={t("nav.hideGroup")}
                  aria-label={t("nav.hideGroupNamed", { name: g.name || t("toast.unnamed") })}
                  onclick={() => setGroupHidden(g.name, true)}
                >
                  <svg class="size-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"
                    ><path d="M3 3l18 18M10.6 10.6a2 2 0 0 0 2.8 2.8M9.4 5.2A9.5 9.5 0 0 1 12 5c5 0 9 4.5 10 7-.4 1-1.2 2.3-2.4 3.5M6.2 6.2C4.2 7.6 2.7 9.6 2 12c1 2.5 5 7 10 7 1.7 0 3.2-.5 4.6-1.2" /></svg
                  >
                </button>
              </div>
            {:else}
              <p class="px-3 py-1 text-xs text-muted">{t("nav.noGroupsMatch")}</p>
            {/each}
          </div>
        </section>
      {/if}
    </nav>

    <div class="border-t border-line p-3">
      <button
        class="chip"
        class:is-active={view === "settings"}
        onclick={() => {
          view = "settings";
          navOpen = false;
        }}
      >
        <svg class="size-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"
          ><circle cx="12" cy="12" r="3" /><path
            d="M19.4 15a1.7 1.7 0 0 0 .3 1.8l.1.1a2 2 0 1 1-2.8 2.8l-.1-.1a1.7 1.7 0 0 0-1.8-.3 1.7 1.7 0 0 0-1 1.5V21a2 2 0 1 1-4 0v-.1a1.7 1.7 0 0 0-1.1-1.5 1.7 1.7 0 0 0-1.8.3l-.1.1a2 2 0 1 1-2.8-2.8l.1-.1a1.7 1.7 0 0 0 .3-1.8 1.7 1.7 0 0 0-1.5-1H3a2 2 0 1 1 0-4h.1a1.7 1.7 0 0 0 1.5-1.1 1.7 1.7 0 0 0-.3-1.8l-.1-.1a2 2 0 1 1 2.8-2.8l.1.1a1.7 1.7 0 0 0 1.8.3H9a1.7 1.7 0 0 0 1-1.5V3a2 2 0 1 1 4 0v.1a1.7 1.7 0 0 0 1 1.5 1.7 1.7 0 0 0 1.8-.3l.1-.1a2 2 0 1 1 2.8 2.8l-.1.1a1.7 1.7 0 0 0-.3 1.8V9a1.7 1.7 0 0 0 1.5 1H21a2 2 0 1 1 0 4h-.1a1.7 1.7 0 0 0-1.5 1z"
          /></svg
        >
        <span class="flex-1">{t("nav.settings")}</span>
      </button>
      <button class="chip mt-0.5 hidden sm:flex" onclick={() => (helpOpen = true)}>
        <span class="flex-1">{t("nav.shortcuts")}</span>
        <kbd class="kbd">?</kbd>
      </button>
      <button class="chip mt-0.5" onclick={() => (supportOpen = true)}>
        <span class="support-heart" aria-hidden="true"
          ><span class="support-ping"></span><svg class="relative size-4 text-accent" viewBox="0 0 24 24" fill="currentColor"
            ><path
              d="M20.84 4.61a5.5 5.5 0 0 0-7.78 0L12 5.67l-1.06-1.06a5.5 5.5 0 0 0-7.78 7.78l1.06 1.06L12 21.23l7.78-7.78 1.06-1.06a5.5 5.5 0 0 0 0-7.78z"
            /></svg
          ></span
        >
        <span class="flex-1">{t("nav.support")}</span>
      </button>
      <div class="mt-3 text-center"><MadeWith /></div>
    </div>
  </aside>

  <!-- Main -->
  <main class="flex min-w-0 flex-1 flex-col">
    <header class="flex flex-wrap items-center gap-3 px-4 pt-4 pb-3 sm:gap-4 sm:px-6 lg:px-8 lg:pt-6 lg:pb-4">
      <button
        class="grid size-10 shrink-0 place-items-center rounded-xl border border-line bg-white/5 text-muted hover:text-text lg:hidden"
        aria-label={t("nav.openMenu")}
        onclick={() => (navOpen = true)}
      >
        <svg class="size-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"
          ><path d="M4 7h16M4 12h16M4 17h16" /></svg
        >
      </button>
      <div class="min-w-0 flex-1">
        <h2 class="truncate text-xl font-semibold tracking-tight sm:text-2xl">{heading}</h2>
        <p class="text-sm text-muted tabular-nums">
          {view === "settings" ? t("header.settingsSubtitle") : t("header.channels", { n: total })}
        </p>
      </div>
      {#if view === "channels"}
        <!-- Full-width second row on phones, inline from sm up -->
        <div class="flex w-full gap-2 sm:w-auto">
          <div class="relative flex-1 sm:w-64 sm:flex-none md:w-72">
            <svg class="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"
              ><circle cx="11" cy="11" r="7" /><path d="m20 20-3.5-3.5" /></svg
            >
            <input bind:this={searchEl} class="field pl-9 pr-9" type="search" placeholder={t("search.placeholder")} bind:value={searchInput} />
            {#if !searchInput}<kbd class="kbd pointer-events-none absolute top-1/2 right-2.5 hidden -translate-y-1/2 sm:inline-grid">/</kbd>{/if}
          </div>
          <!-- The icon says "this is the order" whatever option is shown. -->
          <div class="relative shrink-0">
            <svg
              class="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              stroke-width="2"
              stroke-linecap="round"
              stroke-linejoin="round"
              aria-hidden="true"><path d="M7 4v16M3 16l4 4 4-4M17 20V4M13 8l4-4 4 4" /></svg
            >
            <select
              class="field w-auto pl-9 disabled:opacity-40"
              aria-label={t("sort.label")}
              title={listView === "history" ? t("sort.historyHint") : t("sort.label")}
              disabled={listView === "history"}
              value={prefs.sort}
              onchange={(e) => setSort(e.currentTarget.value as ChannelSort)}
            >
              <option value="">{t("sort.provider")}</option>
              <option value="az">{t("sort.az")}</option>
              <option value="za">{t("sort.za")}</option>
            </select>
          </div>
        </div>
      {/if}
    </header>

    <div
      bind:this={scroller}
      class="flex-1 overflow-y-auto px-4 pb-10 sm:px-6 lg:px-8"
      class:is-scrolling={scrolling}
      onscroll={markScrolling}
    >
      {#if view === "settings"}
        <Settings
          {sources}
          settings={appSettings}
          {hiddenGroups}
          {refreshing}
          onadd={() => (dialog = { editing: null })}
          onedit={(s) => (dialog = { editing: s })}
          onrefresh={(s) => refresh(s)}
          onrefreshall={refreshAll}
          onenable={setEnabled}
          onremove={remove}
          onsettings={changeSettings}
          onunhidegroup={(name) => setGroupHidden(name, false)}
          onshowhidden={() => show({ listView: "hidden" })}
          onclearhistory={clearHistory}
          onexport={exportFavorites}
          onimport={importFavorites}
          onperf={measureScroll}
        />
      {:else if sources.length === 0}
        <div class="grid h-full place-items-center">
          <div class="max-w-sm text-center">
            <div class="fx-glow logo-glow mx-auto mb-6 w-fit"><img src={logoSrc} alt="" class="relative size-40 [image-rendering:pixelated]" /></div>
            <h3 class="mb-2 text-xl font-semibold">{t("empty.noListsTitle")}</h3>
            <p class="mb-6 text-sm text-muted">{t("empty.noListsBody")}</p>
            <button class="btn-accent" onclick={() => (dialog = { editing: null })}>{t("empty.addList")}</button>
          </div>
        </div>
      {:else if channels.length === 0 && !loading}
        <p class="pt-20 text-center text-muted">
          {t(
            listView === "history"
              ? "empty.history"
              : listView === "hidden"
                ? "empty.hidden"
                : listView === "favorites"
                  ? "empty.favorites"
                  : "empty.noResults",
          )}
        </p>
      {:else}
        <div bind:this={gridWrap} class="pt-2">
          <div style="height: {win.padTop}px"></div>
          <div bind:this={gridEl} class="channel-grid">
            {#each visible as c, i (c.id)}
              <ChannelCard
                channel={c}
                index={win.start + i}
              color={prefs.logoColors ? logoColors[c.logo] : ""}
              playing={c.id === playingId}
              onplay={play}
              onfavorite={toggleFavorite}
                onhide={toggleHidden}
              />
            {/each}
          </div>
          <div style="height: {win.padBottom}px"></div>
        </div>
      {/if}
      {#if paged && view === "channels" && pageCount > 1}
        <nav class="flex items-center justify-center gap-2 pt-6" aria-label={t("pager.label")}>
          <button class="btn-ghost px-3 py-1.5" disabled={page === 0} onclick={() => goToPage(0)} aria-label={t("pager.first")}>«</button>
          <button class="btn-ghost px-3 py-1.5" disabled={page === 0} onclick={() => goToPage(page - 1)} aria-label={t("pager.prev")}>‹</button>
          <span class="px-2 text-sm text-muted tabular-nums">{t("pager.page", { page: num(page + 1), pages: num(pageCount) })}</span>
          <button class="btn-ghost px-3 py-1.5" disabled={page >= pageCount - 1} onclick={() => goToPage(page + 1)} aria-label={t("pager.next")}>›</button>
          <button class="btn-ghost px-3 py-1.5" disabled={page >= pageCount - 1} onclick={() => goToPage(pageCount - 1)} aria-label={t("pager.last")}>»</button>
        </nav>
      {/if}
      <div bind:this={sentinel} class="h-px" class:hidden={view === "settings"}></div>
    </div>
  </main>
</div>

{#if perf === "measuring"}
  <div
    class="fixed top-4 left-1/2 z-50 w-max max-w-[calc(100%-2rem)] -translate-x-1/2 rounded-2xl border border-accent/40 bg-[#111118] px-4 py-2.5 text-center text-sm shadow-2xl"
    role="status"
  >
    {t("diag.measuring")}
  </div>
{:else if perf}
  <DiagnosticsDialog report={perf.report} onclose={() => (perf = null)} />
{/if}

{#if supportOpen}
  <SupportDialog onclose={closeSupport} />
{/if}

{#if helpOpen}
  <ShortcutsHelp onclose={() => (helpOpen = false)} />
{/if}

{#if dialog}
  <SourceDialog editing={dialog.editing} onclose={() => (dialog = null)} onsaved={saved} />
{/if}

{#if toast}
  <div
    class="fixed bottom-6 left-1/2 z-50 w-max max-w-[calc(100%-2rem)] -translate-x-1/2 rounded-2xl border px-4 py-2.5 text-center text-sm shadow-2xl
      {toast.error ? 'border-red-500/30 bg-[#2a0f12] text-red-200' : 'border-accent/30 bg-[#111118] text-text'}"
    role="status"
  >
    {toast.text}
  </div>
{/if}
