package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/crstian19/herakleo/wails/internal/library"
	"github.com/crstian19/herakleo/wails/internal/player"
)

func newApp(t *testing.T) *App {
	t.Helper()
	lib, err := library.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return &App{lib: lib}
}

func TestDisplayDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	sep := string(filepath.Separator)

	cases := []struct{ name, in, want string }{
		{"inside home", home + sep + ".config" + sep + "herakleo", "~" + sep + ".config" + sep + "herakleo"},
		{"outside home", sep + "var" + sep + "lib" + sep + "herakleo", sep + "var" + sep + "lib" + sep + "herakleo"},
		{"sibling with home as prefix", home + "-other" + sep + "x", home + "-other" + sep + "x"},
		{"home itself", home, home},
		{"empty", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			want := tc.want
			if runtime.GOOS == "windows" {
				want = tc.in
			}
			if got := displayDir(tc.in); got != want {
				t.Fatalf("displayDir(%q) = %q, want %q", tc.in, got, want)
			}
		})
	}
}

func TestPlayerOptions(t *testing.T) {
	got := playerOptions(library.Settings{Player: "vlc", RefreshOnStart: true, SoftwareDecoding: true, CacheSecs: 30, Volume: 80})
	want := player.Options{SoftwareDecoding: true, CacheSecs: 30, Volume: 80}
	if got != want {
		t.Fatalf("playerOptions = %+v, want %+v", got, want)
	}
	if got := playerOptions(library.Settings{}); got != (player.Options{}) {
		t.Fatalf("zero settings gave %+v", got)
	}
}

func TestDiagnosticEnv(t *testing.T) {
	cases := map[string]bool{
		"WEBKIT_DISABLE_COMPOSITING_MODE": true,
		"GSK_RENDERER":                    true,
		"GDK_BACKEND":                     true,
		"MESA_GL_VERSION_OVERRIDE":        true,
		"__GL_THREADED_OPTIMIZATIONS":     true,
		"LIBGL_ALWAYS_SOFTWARE":           true,
		"HOME":                            false,
		"PATH":                            false,
		"AWS_SECRET_ACCESS_KEY":           false,
		"GITHUB_TOKEN":                    false,
		"MY_WEBKIT_VAR":                   false, // prefix match only
		"webkit_lowercase":                false,
		"":                                false,
	}
	for k, want := range cases {
		if got := diagnosticEnv(k); got != want {
			t.Errorf("diagnosticEnv(%q) = %v, want %v", k, got, want)
		}
	}
}

func TestDiagnostics(t *testing.T) {
	const secret = "hunter2-super-secret"
	t.Setenv("HERAKLEO_FAKE_SECRET", secret)
	t.Setenv("XTREAM_PASSWORD", secret)
	t.Setenv("MY_WEBKIT_TOKEN", secret)
	t.Setenv("XDG_SESSION_TYPE", "wayland")
	t.Setenv("XDG_CURRENT_DESKTOP", "Hyprland")
	t.Setenv("GSK_RENDERER", "ngl")
	t.Setenv("WEBKIT_DISABLE_DMABUF_RENDERER", "1")

	got := (&App{}).Diagnostics()

	for k, want := range map[string]string{
		"session":                            "wayland",
		"desktop":                            "Hyprland",
		"env.GSK_RENDERER":                   "ngl",
		"env.WEBKIT_DISABLE_DMABUF_RENDERER": "1",
	} {
		if got[k] != want {
			t.Errorf("Diagnostics()[%q] = %q, want %q", k, got[k], want)
		}
	}
	if !strings.Contains(got["webview"], "webkitgtk-6.0") {
		t.Errorf("webview = %q", got["webview"])
	}
	if _, ok := got["os"]; !ok {
		t.Error("os key missing")
	}
	for k, v := range got {
		if strings.Contains(v, secret) || strings.Contains(k, "SECRET") || strings.Contains(k, "PASSWORD") {
			t.Errorf("Diagnostics leaked %q=%q", k, v)
		}
		if strings.HasPrefix(k, "env.") && !diagnosticEnv(strings.TrimPrefix(k, "env.")) {
			t.Errorf("non-rendering variable reported: %q", k)
		}
	}
}

func TestParseOSRelease(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"quoted", "NAME=\"Arch Linux\"\nPRETTY_NAME=\"Arch Linux\"\nID=arch\n", "Arch Linux"},
		{"unquoted", "ID=x\nPRETTY_NAME=Plain 1.0\n", "Plain 1.0"},
		{"last line, no newline", "ID=x\nPRETTY_NAME=\"Last\"", "Last"},
		{"missing", "NAME=\"Arch\"\nID=arch\n", ""},
		{"not a prefix match", "XPRETTY_NAME=\"no\"\n", ""},
		{"empty", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := parseOSRelease(tc.in); got != tc.want {
				t.Fatalf("parseOSRelease = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestBenchMode(t *testing.T) {
	a := &App{}
	t.Setenv("HERAKLEO_BENCH_MODE", "wheel:fast")
	t.Setenv(benchEnv, "")
	if got := a.BenchMode(); got != "" {
		t.Fatalf("without %s, BenchMode = %q, want empty", benchEnv, got)
	}
	t.Setenv(benchEnv, filepath.Join(t.TempDir(), "out.json"))
	if got := a.BenchMode(); got != "wheel:fast" {
		t.Fatalf("BenchMode = %q, want wheel:fast", got)
	}
	t.Setenv("HERAKLEO_BENCH_MODE", "")
	if got := a.BenchMode(); got != "full" {
		t.Fatalf("default BenchMode = %q, want full", got)
	}
}

func TestBenchReportDisabled(t *testing.T) {
	t.Setenv(benchEnv, "")
	quit := stubQuit(t)
	err := (&App{}).BenchReport("{}")
	if err == nil || err.Error() != "bench_disabled" {
		t.Fatalf("err = %v, want bench_disabled", err)
	}
	assertNoQuit(t, quit)
}

func TestBenchReportTooLarge(t *testing.T) {
	out := filepath.Join(t.TempDir(), "out.json")
	t.Setenv(benchEnv, out)
	quit := stubQuit(t)
	err := (&App{}).BenchReport(strings.Repeat("x", 1<<20+1))
	if err == nil || err.Error() != "bench_report_too_large" {
		t.Fatalf("err = %v, want bench_report_too_large", err)
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatalf("oversized report was written (stat err %v)", err)
	}
	assertNoQuit(t, quit)
}

func TestBenchReportWriteError(t *testing.T) {
	t.Setenv(benchEnv, filepath.Join(t.TempDir(), "missing-dir", "out.json"))
	quit := stubQuit(t)
	if err := (&App{}).BenchReport("{}"); err == nil {
		t.Fatal("want an error when the file cannot be written")
	}
	assertNoQuit(t, quit)
}

func TestBenchReportWritesPrivateFileAndQuits(t *testing.T) {
	out := filepath.Join(t.TempDir(), "out.json")
	t.Setenv(benchEnv, out)
	quit := stubQuit(t)

	// Exactly the cap is accepted.
	report := strings.Repeat("y", 1<<20)
	if err := (&App{}).BenchReport(report); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != report {
		t.Fatalf("file has %d bytes, want %d", len(b), len(report))
	}
	if runtime.GOOS != "windows" {
		fi, err := os.Stat(out)
		if err != nil {
			t.Fatal(err)
		}
		if perm := fi.Mode().Perm(); perm != 0o600 {
			t.Fatalf("mode = %o, want 600", perm)
		}
	}
	<-quit // the app quits after the call returns
}

// stubQuit replaces quitApp so no test needs a Wails application; the channel
// receives one value per call.
func stubQuit(t *testing.T) chan struct{} {
	t.Helper()
	ch := make(chan struct{}, 4)
	orig := quitApp
	quitApp = func() { ch <- struct{}{} }
	t.Cleanup(func() { quitApp = orig })
	return ch
}

func assertNoQuit(t *testing.T, ch chan struct{}) {
	t.Helper()
	select {
	case <-ch:
		t.Fatal("app quit although the report was refused")
	default:
	}
}

func TestProjectLinksHTTPSOnly(t *testing.T) {
	if len(projectLinks) == 0 {
		t.Fatal("no project links")
	}
	for id, u := range projectLinks {
		if !strings.HasPrefix(u, "https://") {
			t.Errorf("link %q = %q: not https", id, u)
		}
		if strings.ContainsAny(u, " \t\r\n") {
			t.Errorf("link %q = %q holds whitespace", id, u)
		}
	}
}

func TestOpenLinkRejectsUnknown(t *testing.T) {
	a := &App{}
	for _, id := range []string{
		"", "unknown", "GitHub", " github", "github ",
		"https://github.com/crstian19/herakleo", "file:///etc/passwd", "smb://evil/share",
	} {
		err := a.OpenLink(id)
		if err == nil || !strings.HasPrefix(err.Error(), "unknown_link") {
			t.Errorf("OpenLink(%q) = %v, want unknown_link", id, err)
		}
	}
}

func TestGetSettingsDefaults(t *testing.T) {
	a := newApp(t)
	got := a.GetSettings()
	if got.Player != player.DefaultID {
		t.Errorf("Player = %q, want %q", got.Player, player.DefaultID)
	}
	if got.Version != Version {
		t.Errorf("Version = %q, want %q", got.Version, Version)
	}
	if !slices.Equal(got.CacheChoices, player.CacheChoices) {
		t.Errorf("CacheChoices = %v", got.CacheChoices)
	}
	if len(got.Players) == 0 {
		t.Error("no players listed")
	}
	if got.DataDir != displayDir(a.lib.Dir()) {
		t.Errorf("DataDir = %q", got.DataDir)
	}
}

func TestPlayerIDFallsBack(t *testing.T) {
	a := newApp(t)
	bad := "rm -rf"
	// A hand-edited library.json can hold anything: bypass UpdateSettings.
	if _, err := a.lib.UpdateSettings(library.SettingsPatch{Player: &bad}); err != nil {
		t.Fatal(err)
	}
	if got := a.playerID(); got != player.DefaultID {
		t.Fatalf("playerID = %q, want default", got)
	}
	if got := a.GetSettings().Player; got != player.DefaultID {
		t.Fatalf("GetSettings().Player = %q, want default", got)
	}
}

func TestUpdateSettings(t *testing.T) {
	ptr := func(s string) *string { return &s }
	num := func(n int) *int { return &n }
	yes := true

	t.Run("applies a valid patch", func(t *testing.T) {
		a := newApp(t)
		got, err := a.UpdateSettings(library.SettingsPatch{
			Player: ptr(player.DefaultID), RefreshOnStart: &yes, SoftwareDecoding: &yes, CacheSecs: num(30), Volume: num(55),
		})
		if err != nil {
			t.Fatal(err)
		}
		if !got.RefreshOnStart || !got.SoftwareDecoding || got.CacheSecs != 30 || got.Volume != 55 {
			t.Fatalf("view = %+v", got)
		}
		if s := a.lib.Settings(); s.CacheSecs != 30 || s.Volume != 55 {
			t.Fatalf("stored = %+v", s)
		}
	})

	t.Run("a partial patch keeps the rest", func(t *testing.T) {
		a := newApp(t)
		if _, err := a.UpdateSettings(library.SettingsPatch{Volume: num(40)}); err != nil {
			t.Fatal(err)
		}
		got, err := a.UpdateSettings(library.SettingsPatch{CacheSecs: num(10)})
		if err != nil {
			t.Fatal(err)
		}
		if got.Volume != 40 || got.CacheSecs != 10 {
			t.Fatalf("view = %+v", got)
		}
	})

	rejects := []struct {
		name  string
		patch library.SettingsPatch
		want  string
	}{
		{"unknown player", library.SettingsPatch{Player: ptr("bash")}, "unknown_player"},
		{"empty player", library.SettingsPatch{Player: ptr("")}, "unknown_player"},
		{"player with arguments", library.SettingsPatch{Player: ptr("mpv --script=x")}, "unknown_player"},
		{"cache not offered", library.SettingsPatch{CacheSecs: num(7)}, "invalid_option"},
		{"negative cache", library.SettingsPatch{CacheSecs: num(-1)}, "invalid_option"},
		{"volume too high", library.SettingsPatch{Volume: num(101)}, "invalid_option"},
		{"negative volume", library.SettingsPatch{Volume: num(-1)}, "invalid_option"},
	}
	for _, tc := range rejects {
		t.Run("rejects "+tc.name, func(t *testing.T) {
			a := newApp(t)
			before := a.lib.Settings()
			_, err := a.UpdateSettings(tc.patch)
			if err == nil || !strings.HasPrefix(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %s", err, tc.want)
			}
			if after := a.lib.Settings(); after != before {
				t.Fatalf("rejected patch changed settings: %+v -> %+v", before, after)
			}
		})
	}

	t.Run("an invalid patch with a valid player stores nothing", func(t *testing.T) {
		a := newApp(t)
		_, err := a.UpdateSettings(library.SettingsPatch{Player: ptr(player.DefaultID), Volume: num(500)})
		if err == nil {
			t.Fatal("want error")
		}
		if a.lib.Settings().Volume != 0 {
			t.Fatal("volume stored")
		}
	})
}

func TestPlayUnknownChannel(t *testing.T) {
	a := newApp(t)
	for _, id := range []string{"", "nope", "src:1"} {
		err := a.Play(id)
		if err == nil || err.Error() != "channel_not_found" {
			t.Errorf("Play(%q) = %v, want channel_not_found", id, err)
		}
	}
}

func TestEmptyLibraryPassThroughs(t *testing.T) {
	a := newApp(t)
	if got := a.ListSources(); len(got) != 0 {
		t.Errorf("ListSources = %v", got)
	}
	if got := a.ListHiddenGroups(); len(got) != 0 {
		t.Errorf("ListHiddenGroups = %v", got)
	}
	if got := a.ListGroups("missing"); len(got) != 0 {
		t.Errorf("ListGroups = %v", got)
	}
	if got := a.ListChannels(library.Query{}); len(got.Items) != 0 {
		t.Errorf("ListChannels = %+v", got)
	}
	if err := a.RemoveSource("missing"); err == nil {
		t.Error("RemoveSource(missing) should fail")
	}
	if _, err := a.SetSourceEnabled("missing", true); err == nil {
		t.Error("SetSourceEnabled(missing) should fail")
	}
	if err := a.ClearHistory(); err != nil {
		t.Errorf("ClearHistory = %v", err)
	}
	if err := a.ServiceShutdown(); err != nil {
		t.Errorf("ServiceShutdown = %v", err)
	}
}

func TestSettingsViewJSONKeys(t *testing.T) {
	// The frontend reads these exact keys (lib/types.ts).
	b, err := json.Marshal(newApp(t).GetSettings())
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"player", "players", "refreshOnStart", "softwareDecoding", "cacheSecs", "volume", "cacheChoices", "version", "dataDir"} {
		if _, ok := m[k]; !ok {
			t.Errorf("key %q missing", k)
		}
	}
}
