import type { MessageKey } from "./i18n.svelte";

// Keyboard and TV-remote navigation helpers. Remotes usually send only the
// arrows, Enter and Backspace/BrowserBack, so everything reachable by
// shortcut is also reachable with those.

/** Typing in a field: single-key shortcuts must not fire. */
export function isTyping(el: EventTarget | null): boolean {
  if (!(el instanceof HTMLElement)) return false;
  if (el.isContentEditable || el instanceof HTMLTextAreaElement || el instanceof HTMLSelectElement) return true;
  return el instanceof HTMLInputElement && !["checkbox", "radio", "range", "button", "submit"].includes(el.type);
}

/**
 * Index a navigation key moves to from `from` in a grid of `total` items laid
 * out in `cols` columns, or null if the key is not a grid key. Indices are
 * positions in the whole loaded list, not in the DOM: the grid is virtualised
 * and only renders the rows near the viewport. Moves past the edges clamp;
 * `pageRows` sets how far PageUp/PageDown go.
 */
export function gridMove(from: number, key: string, cols: number, total: number, pageRows: number): number | null {
  const last = total - 1;
  const to = (() => {
    switch (key) {
      case "ArrowLeft":
        return from - 1;
      case "ArrowRight":
        return from + 1;
      case "ArrowUp":
        return from - cols;
      case "ArrowDown":
        // Into a partial last row: land on its last card instead of stopping.
        return from + cols > last && Math.floor(from / cols) < Math.floor(last / cols) ? last : from + cols;
      case "PageUp":
        return from - cols * pageRows;
      case "PageDown":
        return from + cols * pageRows;
      case "Home":
        return 0;
      case "End":
        return last;
      default:
        return null;
    }
  })();
  return to === null ? null : Math.min(Math.max(to, 0), last);
}

export interface Shortcut {
  keys: string[];
  label: MessageKey;
}

/** What the help overlay lists; keep in sync with App.svelte's onkey. */
export const shortcutGroups: { title: MessageKey; items: Shortcut[] }[] = [
  {
    title: "shortcuts.general",
    items: [
      { keys: ["/", "Ctrl F"], label: "shortcuts.search" },
      { keys: ["1"], label: "shortcuts.all" },
      { keys: ["2"], label: "shortcuts.favorites" },
      { keys: ["3"], label: "shortcuts.recent" },
      { keys: ["G"], label: "shortcuts.groups" },
      { keys: [","], label: "shortcuts.settings" },
      { keys: ["Esc", "⌫"], label: "shortcuts.back" },
      { keys: ["?"], label: "shortcuts.help" },
    ],
  },
  {
    title: "shortcuts.channels",
    items: [
      { keys: ["←", "↑", "→", "↓"], label: "shortcuts.move" },
      { keys: ["Home", "End"], label: "shortcuts.firstLast" },
      { keys: ["PgUp", "PgDn"], label: "shortcuts.page" },
      { keys: ["[", "]"], label: "shortcuts.pages" },
      { keys: ["Enter"], label: "shortcuts.play" },
      { keys: ["F"], label: "shortcuts.favorite" },
      { keys: ["H"], label: "shortcuts.hide" },
    ],
  },
  {
    title: "shortcuts.moving",
    items: [
      { keys: ["↓"], label: "shortcuts.searchToGrid" },
      { keys: ["←"], label: "shortcuts.gridToSidebar" },
      { keys: ["→"], label: "shortcuts.sidebarToGrid" },
      { keys: ["↓"], label: "shortcuts.groupFilterToList" },
    ],
  },
];
