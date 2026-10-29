package telemetry

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"
)

func TestBodyCarriesOnlyTheDocumentedFields(t *testing.T) {
	b, err := Body(Event{Version: "0.3.0", InstallID: "abc", Language: "es-ES", Flatpak: true})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	p := got["payload"].(map[string]any)
	want := map[string]any{
		"website": WebsiteID, "hostname": "herakleo.crstian.me", "url": "/app", "title": "Herakleo",
		"language": "es-ES", "name": "launch", "id": "abc",
		"data": map[string]any{"version": "0.3.0", "os": runtime.GOOS, "arch": runtime.GOARCH, "flatpak": "true"},
	}
	if got["type"] != "event" || len(p) != len(want) {
		t.Fatalf("unexpected shape: %s", b)
	}
	for k, v := range want {
		if gotV, _ := json.Marshal(p[k]); string(gotV) != mustJSON(v) {
			t.Errorf("%s = %s, want %s", k, gotV, mustJSON(v))
		}
	}
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func TestSend(t *testing.T) {
	var body []byte
	var ua, ctype string
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		body, _ = io.ReadAll(r.Body)
		ua, ctype = r.Header.Get("User-Agent"), r.Header.Get("Content-Type")
	}))
	t.Cleanup(srv.Close)
	if err := Send(context.Background(), srv.URL, Event{Version: "0.3.0", InstallID: "abc"}); err != nil {
		t.Fatal(err)
	}
	if ctype != "application/json" || !strings.Contains(string(body), `"id":"abc"`) {
		t.Errorf("request %q %s", ctype, body)
	}
	// Umami drops requests with bot-like user agents, like Go's default one.
	if !strings.HasPrefix(ua, "Mozilla/5.0 (") || !strings.HasSuffix(ua, "Herakleo/0.3.0") {
		t.Errorf("user agent %q", ua)
	}
}

func TestSendReportsServerErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "nope", http.StatusBadRequest)
	}))
	t.Cleanup(srv.Close)
	if err := Send(context.Background(), srv.URL, Event{}); err == nil {
		t.Error("want an error")
	}
}

func TestNewInstallID(t *testing.T) {
	a, b := NewInstallID(), NewInstallID()
	if len(a) != 32 || a == b {
		t.Errorf("%q %q", a, b)
	}
}
