// Helpers for the "new version" dialog (UpdateDialog.svelte).

const SKIP_KEY = "herakleo:update-skipped";

/**
 * Release text (Markdown from releaser-pleaser) as plain text: the dialog
 * never renders HTML from the network. Headings and bold lose their marks,
 * links keep their text, and the "Full changes" footer goes.
 */
export function releaseNotes(md: string): string {
  return md
    .replace(/\r/g, "")
    .split("\n")
    .filter((l) => !/^\s*\*\*full changes\*\*/i.test(l) && !/^\s*\[compare to previous version\]/i.test(l))
    .map((l) =>
      l
        .replace(/^#{1,6}\s+/, "")
        .replace(/^(\s*)[-*]\s+/, "$1• ")
        .replace(/\s*\(\[[0-9a-f]{7,40}\]\([^)]*\)\)/gi, "")
        .replace(/\[([^\]]*)\]\([^)]*\)/g, "$1")
        .replace(/\*\*([^*]+)\*\*/g, "$1")
        .replace(/`([^`]+)`/g, "$1"),
    )
    .join("\n")
    .replace(/\n{3,}/g, "\n\n")
    .trim();
}

/** The version the user chose to skip, or "". */
export function skippedVersion(): string {
  try {
    return localStorage.getItem(SKIP_KEY) ?? "";
  } catch {
    return "";
  }
}

export function skipVersion(version: string) {
  try {
    localStorage.setItem(SKIP_KEY, version);
  } catch {
    // Storage blocked: the dialog just shows again next time.
  }
}
