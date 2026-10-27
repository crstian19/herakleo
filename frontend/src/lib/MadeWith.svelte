<script lang="ts">
  import { api } from "./api";

  // Same signature in every language; each build names its own framework,
  // without a version so it doesn't go stale on the next upgrade.
  const framework = api.name === "wails" ? "Wails" : api.name === "tauri" ? "Tauri" : "Svelte";
</script>

<p class="text-xs text-muted">Made with ❤️ from 🇪🇸 with {framework}</p>
