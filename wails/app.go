package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/crstian19/herakleo/wails/internal/library"
	"github.com/crstian19/herakleo/wails/internal/player"
)

// App is a Wails service: every exported method is callable from the
// frontend as "main.App.<Method>" (lib/backend/wails.ts) and must match the
// Backend interface in lib/types.ts. Methods that download take the call's
// context first; Wails fills it in and cancels it if the frontend cancels the
// call.
type App struct {
	lib    *library.Library
	player player.Player
}

// ServiceShutdown runs when the app quits: never leave a player behind.
func (a *App) ServiceShutdown() error { //nolint:unparam // Wails' service interface
	a.player.Stop()
	return nil
}

func (a *App) ListSources() []library.SourceInfo { return a.lib.Sources() }

func (a *App) AddSource(ctx context.Context, req library.NewSource) (library.SourceInfo, error) {
	return a.lib.Add(ctx, req)
}

func (a *App) RefreshSource(ctx context.Context, id string) (library.SourceInfo, error) {
	return a.lib.Refresh(ctx, id)
}

func (a *App) UpdateSource(ctx context.Context, id string, req library.NewSource) (library.SourceInfo, error) {
	return a.lib.Update(ctx, id, req)
}

func (a *App) RemoveSource(id string) error { return a.lib.Remove(id) }

func (a *App) ListGroups(sourceID string) []library.Group { return a.lib.Groups(sourceID) }

func (a *App) ListChannels(req library.Query) library.Page { return a.lib.Channels(req) }

func (a *App) SetFavorite(channelID string, favorite bool) error {
	return a.lib.SetFavorite(channelID, favorite)
}

func (a *App) SetSourceEnabled(id string, enabled bool) (library.SourceInfo, error) {
	return a.lib.SetEnabled(id, enabled)
}

func (a *App) SetChannelHidden(channelID string, hidden bool) error {
	return a.lib.SetChannelHidden(channelID, hidden)
}

func (a *App) SetGroupHidden(name string, hidden bool) error {
	return a.lib.SetGroupHidden(name, hidden)
}

func (a *App) ListHiddenGroups() []string { return a.lib.HiddenGroups() }

func (a *App) ClearHistory() error { return a.lib.ClearHistory() }

// Play opens the channel and records it in the history; a failed history
// write must not stop playback that already started.
func (a *App) Play(channelID string) error {
	name, url, err := a.lib.StreamURL(channelID)
	if err != nil {
		return err
	}
	opts := playerOptions(a.lib.Settings())
	if opts.Validate() != nil {
		opts = player.Options{} // a hand-edited library.json must not reach the player
	}
	if err := a.player.Play(a.playerID(), name, url, opts); err != nil {
		return err
	}
	_ = a.lib.AddHistory(channelID)
	return nil
}

// SettingsView is the shared settings plus what this machine can play with.
type SettingsView struct {
	Player           string        `json:"player"`
	Players          []player.Info `json:"players"`
	RefreshOnStart   bool          `json:"refreshOnStart"`
	SoftwareDecoding bool          `json:"softwareDecoding"`
	CacheSecs        int           `json:"cacheSecs"`
	Volume           int           `json:"volume"`
	CacheChoices     []int         `json:"cacheChoices"`
	Version          string        `json:"version"`
	// Where library.json lives, with the home folder shown as ~ outside Windows.
	DataDir string `json:"dataDir"`
}

func (a *App) GetSettings() SettingsView {
	s := a.lib.Settings()
	return SettingsView{
		Player: a.playerID(), Players: player.Available(), RefreshOnStart: s.RefreshOnStart,
		SoftwareDecoding: s.SoftwareDecoding, CacheSecs: s.CacheSecs, Volume: s.Volume,
		CacheChoices: player.CacheChoices, DataDir: displayDir(a.lib.Dir()),
		Version: Version,
	}
}

// UpdateSettings applies a partial change; the player must be on the
// allowlist and the options within the offered choices.
func (a *App) UpdateSettings(patch library.SettingsPatch) (SettingsView, error) {
	if patch.Player != nil && !player.Known(*patch.Player) {
		return SettingsView{}, fmt.Errorf("unknown_player: %s", *patch.Player)
	}
	next := playerOptions(a.lib.Settings())
	if patch.CacheSecs != nil {
		next.CacheSecs = *patch.CacheSecs
	}
	if patch.Volume != nil {
		next.Volume = *patch.Volume
	}
	if err := next.Validate(); err != nil {
		return SettingsView{}, err
	}
	if _, err := a.lib.UpdateSettings(patch); err != nil {
		return SettingsView{}, err
	}
	return a.GetSettings(), nil
}

// ExportFavorites asks where to save and returns the path, or "" if cancelled.
func (a *App) ExportFavorites(title string) (string, error) {
	path, err := application.Get().Dialog.SaveFile().
		SetMessage(title).
		SetFilename("herakleo-favorites.json").
		AddFilter("JSON (*.json)", "*.json").
		PromptForSingleSelection()
	if err != nil || path == "" {
		return "", err
	}
	return path, library.WriteBackup(path, a.lib.ExportFavorites())
}

// ImportFavorites asks for a backup and returns how many favourites matched,
// or -1 if the dialog was cancelled.
func (a *App) ImportFavorites(title string) (int, error) {
	path, err := application.Get().Dialog.OpenFile().
		SetTitle(title).
		AddFilter("JSON (*.json)", "*.json").
		PromptForSingleSelection()
	if err != nil || path == "" {
		return -1, err
	}
	b, err := library.ReadBackup(path)
	if err != nil {
		return 0, err
	}
	return a.lib.ImportFavorites(b)
}

func (a *App) playerID() string {
	if id := a.lib.Settings().Player; player.Known(id) {
		return id
	}
	return player.DefaultID
}

func (a *App) LogoColors(ctx context.Context, urls []string) map[string]string {
	return a.lib.LogoColors(ctx, urls)
}

// PickPlaylistFile returns "" when the dialog is cancelled. Dialog titles come
// from the UI so they follow its language. Only paths returned here can be
// saved as m3u_file sources (library.AllowFile).
func (a *App) PickPlaylistFile(title string) (string, error) {
	path, err := application.Get().Dialog.OpenFile().
		SetTitle(title).
		AddFilter("M3U (*.m3u, *.m3u8)", "*.m3u;*.m3u8").
		PromptForSingleSelection()
	if err != nil {
		return "", err
	}
	a.lib.AllowFile(path)
	return path, nil
}

func playerOptions(s library.Settings) player.Options {
	return player.Options{SoftwareDecoding: s.SoftwareDecoding, CacheSecs: s.CacheSecs, Volume: s.Volume}
}

// benchEnv names the file an unattended benchmark run writes to (just bench).
// The path comes from the environment, never from the UI.
const benchEnv = "HERAKLEO_BENCH_OUT"

// BenchMode is "" outside a benchmark run, otherwise HERAKLEO_BENCH_MODE
// ("full" by default; "wheel[:variant]" for scripts/profile-scroll.sh).
// Same contract as bench_mode in lib.rs.
func (a *App) BenchMode() string {
	if os.Getenv(benchEnv) == "" {
		return ""
	}
	if m := os.Getenv("HERAKLEO_BENCH_MODE"); m != "" {
		return m
	}
	return "full"
}

// BenchReport writes the frontend's measurements and quits. Same contract as
// bench_report in lib.rs.
func (a *App) BenchReport(report string) error {
	path := os.Getenv(benchEnv)
	if path == "" {
		return errors.New("bench_disabled")
	}
	if len(report) > 1<<20 {
		return errors.New("bench_report_too_large")
	}
	// The path comes from the environment of the bench script, not the UI.
	if err := os.WriteFile(path, []byte(report), 0o600); err != nil { //nolint:gosec
		return err
	}
	// Quit from outside this call, so the frontend still gets its answer.
	go application.Get().Quit()
	return nil
}

// Diagnostics describes the machine for the in-app performance report
// (Settings → Performance), so a tester can send it without a terminal.
// Only display/renderer environment variables are included: never secrets.
// Same contract as diagnostics in lib.rs.
func (a *App) Diagnostics() map[string]string {
	out := map[string]string{
		"webview": "webkitgtk-6.0 (GTK 4, Wails v3)",
		"session": os.Getenv("XDG_SESSION_TYPE"),
		"desktop": os.Getenv("XDG_CURRENT_DESKTOP"),
		"os":      osName(),
	}
	for _, kv := range os.Environ() {
		k, v, _ := strings.Cut(kv, "=")
		if diagnosticEnv(k) {
			out["env."+k] = v
		}
	}
	return out
}

// diagnosticEnv lists the variables that change how GTK/WebKit render.
func diagnosticEnv(k string) bool {
	for _, p := range []string{"WEBKIT_", "GSK_", "GDK_", "MESA_", "__GL_", "LIBGL_"} {
		if strings.HasPrefix(k, p) {
			return true
		}
	}
	return false
}

func osName() string {
	data, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		if v, ok := strings.CutPrefix(line, "PRETTY_NAME="); ok {
			return strings.Trim(v, `"`)
		}
	}
	return ""
}

// projectLinks are the only URLs the UI may open in the system browser: it
// sends an ID, never a URL, so a webview bug cannot open arbitrary links.
// Donation links match https://blog.crstian.me/support/.
var projectLinks = map[string]string{
	"github":   "https://github.com/crstian19/herakleo",
	"sponsors": "https://github.com/sponsors/crstian19",
	"kofi":     "https://ko-fi.com/crstian",
	"paypal":   "https://paypal.me/CrstianG",
}

// OpenLink opens one of projectLinks in the default browser.
func (a *App) OpenLink(id string) error {
	u, ok := projectLinks[id]
	if !ok {
		return fmt.Errorf("unknown_link: %s", id)
	}
	return openURL(u)
}

func displayDir(dir string) string {
	home, err := os.UserHomeDir()
	if runtime.GOOS == "windows" || err != nil || !strings.HasPrefix(dir, home+string(filepath.Separator)) {
		return dir
	}
	return "~" + dir[len(home):]
}
