<script lang="ts">
  import { accentFor, type Channel } from "./api";
  import { benchFlags, perfEvent } from "./bench";
  import { t } from "./i18n.svelte";

  let {
    channel,
    index,
    color = "",
    playing = false,
    onplay,
    onfavorite,
    onhide,
  }: {
    channel: Channel;
    /** Position in the whole loaded list (the grid is virtualised); keyboard navigation uses it. */
    index: number;
    /** Accent from the logo; falls back to a hue derived from the name. */
    color?: string;
    playing?: boolean;
    onplay: (c: Channel) => void;
    onfavorite: (c: Channel) => void;
    /** Hides the channel, or unhides it when it is already hidden. */
    onhide: (c: Channel) => void;
  } = $props();

  let logoFailed = $state(false);
  let card: HTMLElement;

  // Same spotlight trick as tesdash: feed the cursor position to the CSS gradients.
  // At most one update per frame: each write restyles and repaints the
  // card's gradients, and pointer events can arrive several times per frame.
  let pending: PointerEvent | null = null;
  function track(e: PointerEvent) {
    if (!pending) requestAnimationFrame(applyTrack);
    pending = e;
  }
  function applyTrack() {
    const e = pending;
    pending = null;
    if (!e) return;
    const r = card.getBoundingClientRect();
    card.style.setProperty("--mouse-x", `${((e.clientX - r.left) / r.width) * 100}%`);
    card.style.setProperty("--mouse-y", `${((e.clientY - r.top) / r.height) * 100}%`);
  }

  function onkey(e: KeyboardEvent) {
    // Only the card itself: Enter on its favourite/hide buttons must not play.
    if (e.target !== e.currentTarget) return;
    if (e.key === "Enter" || e.key === " ") {
      e.preventDefault();
      onplay(channel);
    }
  }
</script>

<div
  bind:this={card}
  class="card"
  data-index={index}
  class:is-playing={playing}
  style="--app-color: {color || accentFor(channel.name)}"
  role="button"
  tabindex="0"
  title={channel.group ? `${channel.name} · ${channel.group}` : channel.name}
  onpointermove={track}
  onclick={() => onplay(channel)}
  onkeydown={onkey}
>
  <span class="spotlight" aria-hidden="true"></span>
  <span class="border-glow" aria-hidden="true"></span>

  <button
    class="card-fav"
    class:is-fav={channel.favorite}
    aria-label={t(channel.favorite ? "card.favRemove" : "card.favAdd")}
    onclick={(e) => {
      e.stopPropagation();
      onfavorite(channel);
    }}
  >
    <svg width="15" height="15" viewBox="0 0 24 24" fill={channel.favorite ? "currentColor" : "none"} stroke="currentColor" stroke-width="2" stroke-linejoin="round">
      <path d="m12 3 2.7 5.6 6.1.9-4.4 4.3 1 6.1L12 17l-5.4 2.9 1-6.1-4.4-4.3 6.1-.9z" />
    </svg>
  </button>

  <button
    class="card-hide"
    class:is-hidden={channel.hidden}
    aria-label={t(channel.hidden ? "card.show" : "card.hide")}
    title={t(channel.hidden ? "card.show" : "card.hide")}
    onclick={(e) => {
      e.stopPropagation();
      onhide(channel);
    }}
  >
    <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round">
      {#if channel.hidden}
        <path d="M2 12c1-2.5 5-7 10-7s9 4.5 10 7c-1 2.5-5 7-10 7S3 14.5 2 12z" /><circle cx="12" cy="12" r="3" />
      {:else}
        <path
          d="M3 3l18 18M10.6 10.6a2 2 0 0 0 2.8 2.8M9.4 5.2A9.5 9.5 0 0 1 12 5c5 0 9 4.5 10 7-.4 1-1.2 2.3-2.4 3.5M6.2 6.2C4.2 7.6 2.7 9.6 2 12c1 2.5 5 7 10 7 1.7 0 3.2-.5 4.6-1.2"
        />
      {/if}
    </svg>
  </button>

  <div class="card-inner">
    {#if channel.logo && !logoFailed}
      <img
        class="card-logo"
        src={channel.logo}
        alt=""
        loading="lazy"
        decoding={benchFlags.syncDecode ? "sync" : "async"}
        referrerpolicy="no-referrer"
        onload={() => perfEvent("logo")}
        onerror={() => (logoFailed = true)}
      />
    {:else}
      <span class="card-fallback">{channel.name.trim().charAt(0).toUpperCase() || "?"}</span>
    {/if}
    <p class="card-name">{channel.name}</p>
  </div>
</div>
