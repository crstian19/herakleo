package main

import (
	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/crstian19/herakleo/wails/internal/player"
)

// playerExitEvent carries a player.Exit to the UI when a player closes on
// its own with an error, so the user sees (and can copy) what it printed.
// Go→JS events reach the webview through evaluateJavaScript; a payload over
// 8 KiB is fetched by the page from /wails/eventpayload/<id> instead, which
// runtimeGuard lets through (runtimeguard_test.go pins that).
const playerExitEvent = "player:exited"

// emitEvent is a variable so tests run without a Wails application.
var emitEvent = func(name string, data any) {
	if app := application.Get(); app != nil {
		app.Event.Emit(name, data)
	}
}

func (a *App) playerExited(e player.Exit) { emitEvent(playerExitEvent, e) }
