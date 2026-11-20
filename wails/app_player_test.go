package main

import (
	"testing"

	"github.com/crstian19/herakleo/wails/internal/player"
)

func TestPlayerExitReachesTheUI(t *testing.T) {
	var got []any
	prev := emitEvent
	emitEvent = func(name string, data any) {
		if name != playerExitEvent {
			t.Errorf("event %q", name)
		}
		got = append(got, data)
	}
	t.Cleanup(func() { emitEvent = prev })

	a := newApp(t)
	e := player.Exit{Player: "mpv", Title: "La 1", Code: 2, Log: []string{"[curl] HTTP error 403"}}
	a.playerExited(e)
	if len(got) != 1 || got[0].(player.Exit).Title != "La 1" {
		t.Fatalf("emitted %v", got)
	}
}
