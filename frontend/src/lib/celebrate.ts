import confetti from "canvas-confetti";
import { prefs } from "./prefs.svelte";

// Same confetti as tesdash's support cards, from the button that was clicked.
// Used by the donate and star buttons; nothing with effects off.
export function celebrate(el: HTMLElement) {
  if (!prefs.effects) return;
  const r = el.getBoundingClientRect();
  const origin = { x: (r.left + r.width / 2) / innerWidth, y: (r.top + r.height / 2) / innerHeight };
  const colors = ["#26ccff", "#a25afd", "#ff5e7e", "#88ff5a", "#fcff42", "#ffa62d", "#ff36ff"];
  confetti({ particleCount: 90, spread: 72, startVelocity: 42, origin, colors, zIndex: 200, disableForReducedMotion: true });
  confetti({ particleCount: 45, spread: 110, startVelocity: 28, decay: 0.92, scalar: 0.9, origin, colors, zIndex: 200, disableForReducedMotion: true });
}
