package main

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/crstian19/herakleo/wails/internal/library"
)

func pingServer(t *testing.T) (*atomic.Int32, string) {
	t.Helper()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hits.Add(1) }))
	t.Cleanup(srv.Close)
	return &hits, srv.URL
}

func waitHits(hits *atomic.Int32, n int32) {
	for i := 0; hits.Load() < n && i < 100; i++ {
		time.Sleep(20 * time.Millisecond)
	}
}

func TestTelemetryOnByDefault(t *testing.T) {
	t.Setenv("DO_NOT_TRACK", "")
	hits, url := pingServer(t)
	a := newApp(t)
	a.telemetryURL = url

	if v := a.GetSettings(); v.Telemetry != library.TelemetryOn {
		t.Fatalf("default: %q", v.Telemetry)
	}
	a.ping(false)
	if hits.Load() != 1 || a.lib.Settings().InstallID == "" {
		t.Fatalf("first launch: %d pings, %+v", hits.Load(), a.lib.Settings())
	}
	// At most one a day.
	a.ping(false)
	if hits.Load() != 1 {
		t.Errorf("second ping the same day: %d", hits.Load())
	}

	// Off in Settings: nothing is sent and the ID is gone.
	off := library.TelemetryOff
	if v, err := a.UpdateSettings(library.SettingsPatch{Telemetry: &off}); err != nil || v.Telemetry != off {
		t.Fatalf("%+v, %v", v, err)
	}
	a.ping(true)
	if hits.Load() != 1 || a.lib.Settings().InstallID != "" {
		t.Errorf("after turning it off: %d pings, %+v", hits.Load(), a.lib.Settings())
	}

	// Back on: counted at once, with a new ID.
	on := library.TelemetryOn
	if _, err := a.UpdateSettings(library.SettingsPatch{Telemetry: &on}); err != nil {
		t.Fatal(err)
	}
	waitHits(hits, 2)
	if hits.Load() != 2 {
		t.Errorf("after turning it back on: %d pings", hits.Load())
	}

	bad := "maybe"
	if _, err := a.UpdateSettings(library.SettingsPatch{Telemetry: &bad}); err == nil {
		t.Error("invalid telemetry value accepted")
	}
}

func TestDoNotTrack(t *testing.T) {
	hits, url := pingServer(t)
	a := newApp(t)
	a.telemetryURL = url
	for _, v := range []string{"1", "true", "yes"} {
		t.Setenv("DO_NOT_TRACK", v)
		a.ping(true)
		if got := a.GetSettings().Telemetry; got != "dnt" {
			t.Errorf("DO_NOT_TRACK=%s: settings say %q", v, got)
		}
	}
	if hits.Load() != 0 {
		t.Errorf("DO_NOT_TRACK ignored: %d pings", hits.Load())
	}
	for _, v := range []string{"0", "false", ""} {
		t.Setenv("DO_NOT_TRACK", v)
		if doNotTrack() {
			t.Errorf("DO_NOT_TRACK=%q must not opt out", v)
		}
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
