package main

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sync"
	"time"

	"github.com/crstian19/herakleo/wails/internal/update"
)

// UpdateInfo tells the UI about the newest release and what this install can
// do about it (update.Mode).
type UpdateInfo struct {
	Current   string `json:"current"`
	Latest    string `json:"latest"`
	Available bool   `json:"available"`
	// Notes is the release text (Markdown), shown as plain text.
	Notes   string `json:"notes"`
	Mode    string `json:"mode"`
	Manager string `json:"manager"`
	Command string `json:"command"`
}

// updates holds what CheckUpdate found, so InstallUpdate takes no input from
// the UI: it can only install the release the backend itself looked up.
type updates struct {
	mu      sync.Mutex
	once    sync.Once
	install update.Install
	release *update.Release
	busy    bool
	restart update.Restart

	// Tests point these at a local server and key.
	apiURL, base string
	key          []byte
	detect       func() update.Install
}

func (u *updates) target() update.Install {
	u.once.Do(func() {
		switch {
		case u.detect != nil:
			u.install = u.detect()
		case !productionBuild:
			u.install = update.Install{Mode: update.ModeDownload}
		default:
			u.install = update.Detect(update.SystemEnv(runtime.GOOS, runtime.GOARCH))
		}
	})
	return u.install
}

// CheckUpdate asks GitHub for the newest release. The UI calls it on start
// when the check is on (Settings), and from Settings → About on demand.
func (a *App) CheckUpdate(ctx context.Context) (UpdateInfo, error) {
	u := &a.updates
	api := u.apiURL
	if api == "" {
		api = update.LatestURL
	}
	rel, err := update.Latest(ctx, update.NewClient(20*time.Second), api)
	if err != nil {
		return UpdateInfo{}, fmt.Errorf("update_check_failed: %w", err)
	}
	in := u.target()
	info := UpdateInfo{
		Current: Version, Latest: rel.Version, Notes: rel.Notes,
		Mode: string(in.Mode), Manager: in.Manager, Command: in.Command,
		Available: update.Newer(rel.Version, Version),
	}
	if !rel.HasAsset(update.SumsName) {
		// releaser-pleaser publishes the release before the packages are
		// attached; SHA256SUMS goes up with them.
		info.Available = false
	}
	if in.Mode == update.ModeInstall {
		switch {
		case !rel.HasAsset(in.Asset(rel.Version)):
			// Not built yet (the .dmg lands after the rest): ask again later.
			info.Available = false
		case !rel.HasAsset(update.SigName):
			// An unsigned release is never installed, only offered as a download.
			info.Mode = string(update.ModeDownload)
		}
	}
	u.mu.Lock()
	u.release = nil
	if info.Available && info.Mode == string(update.ModeInstall) {
		u.release = &rel
	}
	u.mu.Unlock()
	return info, nil
}

// InstallUpdate installs the release found by the last CheckUpdate, then
// quits; main starts the new version once the app has closed.
func (a *App) InstallUpdate(ctx context.Context) error {
	u := &a.updates
	u.mu.Lock()
	rel := u.release
	if rel == nil {
		u.mu.Unlock()
		return errors.New("update_none")
	}
	if u.busy {
		u.mu.Unlock()
		return errors.New("update_busy")
	}
	u.busy = true
	u.mu.Unlock()
	defer func() {
		u.mu.Lock()
		u.busy = false
		u.mu.Unlock()
	}()

	up := &update.Updater{
		Client: update.NewClient(10 * time.Minute), Base: u.base, Key: u.key, Target: u.target(),
	}
	if up.Base == "" {
		up.Base = update.DownloadBase
	}
	if up.Key == nil {
		up.Key = update.PublicKey
	}
	restart, err := up.Apply(ctx, *rel)
	if err != nil {
		return err
	}
	u.mu.Lock()
	u.restart, u.release = restart, nil
	u.mu.Unlock()
	go quitApp()
	return nil
}

// cleanupUpdate removes what the last update left next to the app.
func (a *App) cleanupUpdate() {
	if in := a.updates.target(); in.Mode == update.ModeInstall {
		update.Cleanup(in.Target)
	}
}

// restartAfterUpdate starts the new version, if InstallUpdate ran; main
// calls it from PostShutdown. It runs the restart once.
func (a *App) restartAfterUpdate() error {
	a.updates.mu.Lock()
	r := a.updates.restart
	a.updates.restart = nil
	a.updates.mu.Unlock()
	if r == nil {
		return nil
	}
	return r()
}
