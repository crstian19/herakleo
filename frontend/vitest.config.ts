import { fileURLToPath } from "node:url";
import { svelte } from "@sveltejs/vite-plugin-svelte";
import { defineConfig } from "vitest/config";

// Tests run in plain node (browser globals are stubbed per test) and never
// touch the Wails runtime: $backend always points at the mock adapter. The
// Svelte plugin is only here to compile runes in *.svelte.ts files.
export default defineConfig({
  plugins: [svelte()],
  resolve: {
    alias: {
      $backend: fileURLToPath(new URL("./src/lib/backend/mock.ts", import.meta.url)),
    },
  },
  test: {
    environment: "node",
    include: ["src/**/*.test.ts"],
  },
});
