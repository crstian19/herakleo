package main

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/crstian19/herakleo/wails/internal/library"
)

func TestTelemetryPing(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hits.Add(1) }))
	t.Cleanup(srv.Close)
	a := newApp(t)
	a.telemetryURL = srv.URL

	// Not asked yet, or said no: nothing leaves the machine.
	a.ping(true)
	off := library.TelemetryOff
	if _, err := a.UpdateSettings(library.SettingsPatch{Telemetry: &off}); err != nil {
		t.Fatal(err)
	}
	a.ping(true)
	if hits.Load() != 0 {
		t.Fatalf("sent %d pings without consent", hits.Load())
	}

	on := library.TelemetryOn
	if v, err := a.UpdateSettings(library.SettingsPatch{Telemetry: &on}); err != nil || v.Telemetry != on {
		t.Fatalf("%+v, %v", v, err)
	}
	// Saying yes pings at once (in the background).
	for i := 0; hits.Load() == 0 && i < 100; i++ {
		time.Sleep(20 * time.Millisecond)
	}
	if hits.Load() != 1 || a.lib.Settings().LastPing == 0 {
		t.Fatalf("after opting in: %d pings, %+v", hits.Load(), a.lib.Settings())
	}
	// At most one a day.
	a.ping(false)
	if hits.Load() != 1 {
		t.Errorf("second ping the same day: %d", hits.Load())
	}

	bad := "maybe"
	if _, err := a.UpdateSettings(library.SettingsPatch{Telemetry: &bad}); err == nil {
		t.Error("invalid telemetry value accepted")
	}
}

func TestSystemLanguage(t *testing.T) {
	t.Setenv("LC_ALL", "")
	t.Setenv("LC_MESSAGES", "")
	t.Setenv("LANG", "es_ES.UTF-8")
	if got := systemLanguage(); got != "es-ES" {
		t.Errorf("got %q", got)
	}
	t.Setenv("LANG", "C")
	if got := systemLanguage(); got != "" {
		t.Errorf("C locale: %q", got)
	}
}
