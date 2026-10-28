import { fileURLToPath } from "node:url";
import { svelte } from "@sveltejs/vite-plugin-svelte";
import tailwindcss from "@tailwindcss/vite";
import { defineConfig, type Plugin } from "vite";

// The mode picks which backend adapter is bundled and where the build lands:
// "wails" is embedded by the Go app (../wails/frontend/dist), "mock" runs the
// UI in a browser against fake data. Any other mode (e.g. svelte-check
// loading this file) falls back to wails.
const modes = ["wails", "mock"];

// index.html carries the production CSP. Under `just dev-wails` the page is
// served by Wails (not from http://localhost:5173), so Vite's HMR websocket is
// not 'self': allow it while serving only, never in a build.
const devCsp = (): Plugin => ({
  name: "herakleo-dev-csp",
  apply: "serve",
  transformIndexHtml: (html) => html.replace("connect-src 'self'", "connect-src 'self' ws://localhost:*"),
});

export default defineConfig(({ mode: requested }) => {
  const mode = modes.includes(requested) ? requested : "wails";

  return {
    plugins: [svelte(), tailwindcss(), devCsp()],
    resolve: {
      alias: {
        $backend: fileURLToPath(new URL(`./src/lib/backend/${mode}.ts`, import.meta.url)),
      },
    },
    clearScreen: false,
    server: { port: 5173, strictPort: true },
    build: {
      outDir: mode === "wails" ? "../wails/frontend/dist" : "dist-mock",
      emptyOutDir: true,
      target: "es2022",
    },
  };
});
