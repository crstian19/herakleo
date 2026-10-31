// Herakleo is a desktop IPTV player: this is its Go backend on Wails v3, which
// serves the Svelte UI in ../frontend and opens channels in an external player.
package main

import (
	"embed"
	"log"
	"os"
	"path/filepath"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/crstian19/herakleo/wails/internal/library"
)

// Version is the release version; `just bump` sets it and the release
// workflow checks it against the tag.
const Version = "0.4.0"

// Built by `bun run build:wails` in ../frontend.
//
//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/appicon.png
var icon []byte

// Wails v3 runs on GTK 4 + webkitgtk-6.0 by default (v2 was GTK 3): that WebKit
// scrolls at the monitor's refresh rate, while the GTK 3 one stays at 60 fps.
// Release builds need `-tags production`; without it,
// FRONTEND_DEVSERVER_URL points the window at the Vite dev server.
func main() {
	cfg, err := os.UserConfigDir()
	if err != nil {
		log.Fatal(err)
	}
	lib, err := library.Open(filepath.Join(cfg, "herakleo"))
	if err != nil {
		log.Fatal(err)
	}

	app := application.New(application.Options{
		Name:     "Herakleo",
		Icon:     icon,
		Services: []application.Service{application.NewService(&App{lib: lib})},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
			// The webview may only call App's methods (runtimeguard.go), not
			// the rest of the Wails runtime (OpenURL, clipboard, dialogs...).
			Middleware: guardRuntime,
		},
		Linux: application.LinuxOptions{
			// Under GTK 4 the Wayland app_id (the window class Hyprland and
			// docks match .desktop files on) comes from the application ID,
			// not the program name. It is also the Flatpak ID and the
			// .desktop file name (packaging/linux).
			ApplicationID: "me.crstian.Herakleo",
		},
		Mac: application.MacOptions{ApplicationShouldTerminateAfterLastWindowClosed: true},
	})

	app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:            "Herakleo",
		Width:            1400,
		Height:           900,
		MinWidth:         420,
		MinHeight:        500,
		BackgroundColour: application.NewRGB(10, 10, 15),
		URL:              "/",
		Linux:            application.LinuxWindow{Icon: icon},
	})

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
