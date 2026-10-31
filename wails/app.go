package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/crstian19/herakleo/wails/internal/library"
	"github.com/crstian19/herakleo/wails/internal/player"
	"github.com/crstian19/herakleo/wails/internal/playlist"
	"github.com/crstian19/herakleo/wails/internal/telemetry"
)

// App is a Wails service: every exported method is callable from the
// frontend as "main.App.<Method>" (lib/backend/wails.ts) and must match the
// Backend interface in lib/types.ts. Methods that download take the call's
// context first; Wails fills it in and cancels it if the frontend cancels the
// call.
type App struct {
	lib    *library.Library
	player player.Player
	// telemetryURL is where pings go; telemetry.Endpoint unless a test sets it.
	telemetryURL string
}

// ServiceStartup sends the daily ping (unless turned off) a few seconds after
// start, so it never competes with loading the window.
func (a *App) ServiceStartup(context.Context, application.ServiceOptions) error { //nolint:unparam // Wails' service interface
	go func() {
		time.Sleep(5 * time.Second)
		a.ping(false)
	}()
	return nil
}

// ping sends the usage event when telemetry is on and the last one is older
// than telemetry.Interval (or always, with force). It is on by default; off
// in Settings or with DO_NOT_TRACK=1 it sends nothing.
// Failures are silent: a stats server being down must never bother the user.
func (a *App) ping(force bool) {
	s := a.lib.Settings()
	if !s.TelemetryEnabled() || doNotTrack() || (!force && time.Since(time.Unix(s.LastPing, 0)) < telemetry.Interval) {
		return
	}
	id, err := a.lib.TelemetryInstall(telemetry.NewInstallID)
	if err != nil || id == "" {
		return
	}
	endpoint := a.telemetryURL
	if endpoint == "" {
		endpoint = telemetry.Endpoint
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	e := telemetry.Event{Version: Version, InstallID: id, Language: systemLanguage(), Flatpak: inFlatpak()}
	// No ping has gone out with this ID yet: a new install (or statistics
	// turned back on, which also makes a new ID).
	if s.LastPing == 0 {
		e.Name = telemetry.Install
		if telemetry.Send(ctx, endpoint, e) != nil {
			return
		}
	}
	e.Name = telemetry.Launch
	if telemetry.Send(ctx, endpoint, e) == nil {
		_ = a.lib.SetLastPing(time.Now().Unix())
	}
}

// systemLanguage is the desktop locale as a BCP 47 tag ("es-ES"), or "".
func systemLanguage() string {
	for _, k := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if v := os.Getenv(k); v != "" && v != "C" && v != "POSIX" {
			v, _, _ = strings.Cut(v, ".")
			return strings.ReplaceAll(v, "_", "-")
		}
	}
	return ""
}

func telemetryView(s library.Settings) string {
	switch {
	case doNotTrack():
		return "dnt"
	case s.TelemetryEnabled():
		return library.TelemetryOn
	}
	return library.TelemetryOff
}

func doNotTrack() bool {
	v := os.Getenv("DO_NOT_TRACK")
	return v != "" && v != "0" && !strings.EqualFold(v, "false")
}

func inFlatpak() bool {
	_, err := os.Stat("/.flatpak-info")
	return err == nil
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

func (a *App) ListGroups(sourceID, media string) []library.Group {
	return a.lib.Groups(sourceID, media)
}

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

func (a *App) RefreshGuide(ctx context.Context, id string) (library.SourceInfo, error) {
	return a.lib.RefreshGuide(ctx, id)
}

func (a *App) SetGuideURL(id, url string) (library.SourceInfo, error) {
	return a.lib.SetGuideURL(id, url)
}

func (a *App) NowNext(channelIDs []string) map[string]library.NowNext {
	return a.lib.NowNext(channelIDs)
}

func (a *App) ChannelGuide(channelID string) []library.Programme {
	return a.lib.ChannelGuide(channelID)
}

func (a *App) SetReminder(r library.Reminder, on bool) ([]library.Reminder, error) {
	return a.lib.SetReminder(r, on)
}

func (a *App) ListReminders() []library.Reminder { return a.lib.Reminders() }

// SeriesEpisodes lists an Xtream series' episodes, asking the panel each time.
func (a *App) SeriesEpisodes(ctx context.Context, channelID string) ([]playlist.Episode, error) {
	return a.lib.Episodes(ctx, channelID)
}

// PlayEpisode plays an episode listed by the last SeriesEpisodes call for the
// series, and records the series in the history.
func (a *App) PlayEpisode(channelID, episodeID string) error {
	name, url, err := a.lib.EpisodeURL(channelID, episodeID)
	if err != nil {
		return err
	}
	if err := a.play(name, url); err != nil {
		return err
	}
	_ = a.lib.AddHistory(channelID)
	return nil
}

// Play opens the channel and records it in the history; a failed history
// write must not stop playback that already started.
func (a *App) Play(channelID string) error {
	name, url, err := a.lib.StreamURL(channelID)
	if err != nil {
		return err
	}
	if err := a.play(name, url); err != nil {
		return err
	}
	_ = a.lib.AddHistory(channelID)
	return nil
}

func (a *App) play(name, url string) error {
	opts := playerOptions(a.lib.Settings())
	if opts.Validate() != nil {
		opts = player.Options{} // a hand-edited library.json must not reach the player
	}
	return a.player.Play(a.playerID(), name, url, opts)
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
	// Telemetry is "on" or "off"; "dnt" when DO_NOT_TRACK turns it off.
	Telemetry string `json:"telemetry"`
	// Where library.json lives, with the home folder shown as ~ outside Windows.
	DataDir string `json:"dataDir"`
}

func (a *App) GetSettings() SettingsView {
	s := a.lib.Settings()
	return SettingsView{
		Player: a.playerID(), Players: player.Available(), RefreshOnStart: s.RefreshOnStart,
		SoftwareDecoding: s.SoftwareDecoding, CacheSecs: s.CacheSecs, Volume: s.Volume,
		CacheChoices: player.CacheChoices, DataDir: displayDir(a.lib.Dir()),
		Version: Version, Telemetry: telemetryView(s),
	}
}

// UpdateSettings applies a partial change; the player must be on the
// allowlist and the options within the offered choices.
func (a *App) UpdateSettings(patch library.SettingsPatch) (SettingsView, error) {
	if patch.Player != nil && !player.Known(*patch.Player) {
		return SettingsView{}, fmt.Errorf("unknown_player: %s", *patch.Player)
	}
	if t := patch.Telemetry; t != nil && *t != library.TelemetryOn && *t != library.TelemetryOff {
		return SettingsView{}, fmt.Errorf("invalid_option: telemetry %q", *t)
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
	if patch.Telemetry != nil && *patch.Telemetry == library.TelemetryOn {
		go a.ping(true) // turned back on: count it now, with its new install ID
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

// quitApp is a variable so tests can run BenchReport without a Wails app.
var quitApp = func() { application.Get().Quit() }

// BenchMode is "" outside a benchmark run, otherwise HERAKLEO_BENCH_MODE
// ("full" by default; "wheel[:variant]" for scripts/profile-scroll.sh).
func (a *App) BenchMode() string {
	if os.Getenv(benchEnv) == "" {
		return ""
	}
	if m := os.Getenv("HERAKLEO_BENCH_MODE"); m != "" {
		return m
	}
	return "full"
}

// BenchReport writes the frontend's measurements and quits.
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
	go quitApp()
	return nil
}

// Diagnostics describes the machine for the in-app performance report
// (Settings → Performance), so a tester can send it without a terminal.
// Only display/renderer environment variables are included: never secrets.
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
	return parseOSRelease(string(data))
}

// parseOSRelease returns PRETTY_NAME from the contents of an os-release file.
func parseOSRelease(data string) string {
	for _, line := range strings.Split(data, "\n") {
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
	// What the usage statistics send.
	"telemetry": "https://github.com/crstian19/herakleo#telemetry",
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
