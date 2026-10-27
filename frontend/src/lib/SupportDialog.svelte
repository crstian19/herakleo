<script lang="ts">
  import confetti from "canvas-confetti";
  import { siGithubsponsors, siKofi, siPaypal } from "simple-icons";
  // The light build: SVG renderer only and no expression eval, so it works
  // under Tauri's CSP. heart.json is the same animation tesdash uses.
  import lottie from "lottie-web/build/player/lottie_light";
  import { api } from "./api";
  import CloseButton from "./CloseButton.svelte";
  import { t } from "./i18n.svelte";
  import heartAnimation from "./lottie/heart.json";
  import MadeWith from "./MadeWith.svelte";
  import { prefs } from "./prefs.svelte";

  // Opened by App.svelte after enough plays (see SUPPORT_AFTER) and from
  // "Support the project" in the sidebar. Links open in the system browser through the backend, by ID.
  let { onclose }: { onclose: () => void } = $props();
  let panel: HTMLElement;
  let heartBox: HTMLElement;
  // Intro like tesdash's bolt, but a play button: it is traced, fills, flashes
  // and bursts into the heart (CSS in app.css, .support-hero).
  let playing = $state(false);
  $effect(() => panel.focus());

  // The heart bursts in when the popup opens; with effects off it is drawn without the animation.
  $effect(() => {
    const anim = lottie.loadAnimation({
      container: heartBox,
      renderer: "svg",
      loop: false,
      autoplay: false,
      animationData: heartAnimation,
    });
    if (!prefs.effects) {
      anim.addEventListener("DOMLoaded", () => anim.goToAndStop(anim.totalFrames - 1, true));
      return () => anim.destroy();
    }
    playing = true;
    // The heart bursts in right as the play button flashes.
    const start = setTimeout(() => anim.play(), 800);
    return () => {
      clearTimeout(start);
      anim.destroy();
    };
  });

  // Same confetti as tesdash's support cards, from the button that was clicked.
  function celebrate(el: HTMLElement) {
    if (!prefs.effects) return;
    const r = el.getBoundingClientRect();
    const origin = { x: (r.left + r.width / 2) / innerWidth, y: (r.top + r.height / 2) / innerHeight };
    const colors = ["#26ccff", "#a25afd", "#ff5e7e", "#88ff5a", "#fcff42", "#ffa62d", "#ff36ff"];
    confetti({ particleCount: 90, spread: 72, startVelocity: 42, origin, colors, zIndex: 200, disableForReducedMotion: true });
    confetti({ particleCount: 45, spread: 110, startVelocity: 28, decay: 0.92, scalar: 0.9, origin, colors, zIndex: 200, disableForReducedMotion: true });
  }

  // Same options and order as https://blog.crstian.me/support/. Real brand
  // marks from simple-icons (CC0), in their official colours.
  // PayPal's official navy (#002991) vanishes on the dark panel, so it uses
  // the brand's light blue instead.
  const donations = [
    { id: "sponsors", icon: siGithubsponsors, color: siGithubsponsors.hex },
    { id: "kofi", icon: siKofi, color: siKofi.hex },
    { id: "paypal", icon: siPaypal, color: "009CDE" },
  ] as const;

  // The browser may open behind the app (compositors can refuse focus), so say where it went.
  let opened = $state(false);
  let openedTimer: ReturnType<typeof setTimeout> | undefined;
  const open = (id: Parameters<typeof api.openLink>[0]) =>
    api
      .openLink(id)
      .then(() => {
        opened = true;
        clearTimeout(openedTimer);
        openedTimer = setTimeout(() => (opened = false), 4000);
      })
      .catch(() => {});
</script>

<svelte:window onkeydown={(e) => e.key === "Escape" && onclose()} />

<div class="fixed inset-0 z-50 grid place-items-center overflow-y-auto bg-black/75 p-4 sm:p-6">
  <button class="absolute inset-0 cursor-default" aria-label={t("support.later")} onclick={onclose}></button>
  <div
    bind:this={panel}
    class="relative w-full max-w-lg rounded-3xl border border-line bg-[#11111a] p-6 text-center shadow-2xl outline-none sm:p-8"
    role="dialog"
    aria-modal="true"
    aria-labelledby="support-title"
    tabindex="-1"
  >
    <CloseButton {onclose} />
    <div class="support-hero" class:is-playing={playing} class:is-static={!prefs.effects} aria-hidden="true">
      <svg class="support-play" viewBox="0 0 24 24">
        <path class="support-play-fill" d="M8 5.5v13l10.5-6.5z" />
        <path class="support-play-trace" d="M8 5.5v13l10.5-6.5z" pathLength="100" />
      </svg>
      <span class="support-flash"></span>
      <div bind:this={heartBox} class="support-heart-anim"></div>
    </div>
    <h2 id="support-title" class="mb-2 text-xl font-semibold">{t("support.title")}</h2>
    <p class="mb-6 text-sm text-muted">{t("support.desc")}</p>

    <button class="btn-ghost mb-6 flex w-full items-center justify-center gap-2 py-3" onclick={() => open("github")}>
      <svg class="size-5" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true"
        ><path
          d="M12 2C6.477 2 2 6.477 2 12c0 4.418 2.865 8.166 6.839 9.489.5.092.682-.217.682-.482 0-.237-.009-.868-.013-1.703-2.782.604-3.369-1.341-3.369-1.341-.454-1.154-1.11-1.462-1.11-1.462-.908-.62.069-.608.069-.608 1.003.07 1.531 1.03 1.531 1.03.892 1.529 2.341 1.087 2.91.832.092-.647.35-1.088.636-1.338-2.22-.253-4.555-1.11-4.555-4.943 0-1.091.39-1.984 1.029-2.683-.103-.253-.446-1.27.098-2.647 0 0 .84-.269 2.75 1.025A9.564 9.564 0 0 1 12 6.844a9.59 9.59 0 0 1 2.504.337c1.909-1.294 2.747-1.025 2.747-1.025.546 1.377.202 2.394.1 2.647.64.699 1.028 1.592 1.028 2.683 0 3.842-2.339 4.687-4.566 4.935.359.309.678.919.678 1.852 0 1.336-.012 2.415-.012 2.741 0 .267.18.578.688.48C19.138 20.163 22 16.418 22 12c0-5.523-4.477-10-10-10z"
        /></svg
      >
      <span class="font-medium text-text">{t("support.github")}</span>
    </button>

    <p class="section-title mb-3 text-left">{t("support.donate")}</p>
    <div class="grid gap-2 sm:grid-cols-3">
      {#each donations as d (d.id)}
        <button
          class="flex items-center gap-3 rounded-2xl border border-line bg-surface px-3 py-3 text-left transition-colors hover:border-line-hi hover:bg-white/5 sm:flex-col sm:text-center"
          onclick={(e) => {
            celebrate(e.currentTarget);
            open(d.id);
          }}
        >
          <span class="grid size-9 shrink-0 place-items-center rounded-full" style="background: #{d.color}26">
            <svg class="size-5" viewBox="0 0 24 24" fill="#{d.color}" aria-hidden="true"><path d={d.icon.path} /></svg>
          </span>
          <span class="text-sm font-medium">{d.icon.title}</span>
        </button>
      {/each}
    </div>

    <p class="mt-4 h-5 text-sm text-accent transition-opacity" class:opacity-0={!opened} role="status">
      {opened ? t("support.opened") : ""}
    </p>

    <div class="mt-2 flex flex-col items-center gap-3">
      <button class="btn-ghost px-6" onclick={onclose}>{t("support.later")}</button>
      <MadeWith />
    </div>
  </div>
</div>
