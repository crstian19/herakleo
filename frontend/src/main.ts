import { mount } from "svelte";
import App from "./App.svelte";
import { api } from "./lib/api";
import { applyAccent, applyEffects } from "./lib/prefs.svelte";
import "@fontsource-variable/geist";
import "./app.css";

// Tells the three builds apart in browser tabs and window switchers.
document.title = `Herakleo · ${api.name[0]!.toUpperCase()}${api.name.slice(1)}`;

applyAccent();
applyEffects();

export default mount(App, { target: document.getElementById("app")! });
