package main

import (
	"embed"
	"log"
	"os"
	"path/filepath"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/crstian19/herakleo/wails/internal/library"
)

// Built by `bun run build:wails` in ../frontend.
//
//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/appicon.png
var icon []byte

// Wails v3 runs on GTK 4 + webkitgtk-6.0 by default (v2 was GTK 3): that WebKit
// scrolls at the monitor's refresh rate, while the GTK 3 one stays at 60 fps
// (docs/PERFORMANCE.md). Release builds need `-tags production`; without it,
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
		Assets:   application.AssetOptions{Handler: application.AssetFileServerFS(assets)},
		Linux: application.LinuxOptions{
			// Under GTK 4 the Wayland app_id (the window class Hyprland and
			// docks match .desktop files on) comes from the application ID,
			// not the program name. Keep it distinct from the Tauri build.
			ApplicationID: "me.crstian.herakleo-wails",
		},
	})

	app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:            "Herakleo · Wails",
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
