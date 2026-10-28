// Package player opens streams in an external media player. The webview
// cannot decode MPEG-TS, and desktop players handle every codec IPTV
// providers throw at them.
package player

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/crstian19/herakleo/wails/internal/playlist"
)

// DefaultID is used until the user picks another player.
const DefaultID = "mpv"

// Options are the playback tweaks the UI may set. They are a closed set of
// typed values, never free-form arguments, for the same reason as the
// player allowlist. Zero values mean "leave the player's default".
type Options struct {
	SoftwareDecoding bool
	// Network cache in seconds; one of CacheChoices.
	CacheSecs int
	// Start volume 1-100.
	Volume int
}

// Option names, as reported in Info.Options and used by the UI.
const (
	OptSoftwareDecoding = "softwareDecoding"
	OptCacheSecs        = "cacheSecs"
	OptVolume           = "volume"
)

// CacheChoices are the cache sizes offered in settings (0 = player default).
var CacheChoices = []int{0, 10, 30, 60, 120}

// Validate rejects values outside the offered choices.
func (o Options) Validate() error {
	if !slices.Contains(CacheChoices, o.CacheSecs) {
		return fmt.Errorf("invalid_option: cacheSecs %d", o.CacheSecs)
	}
	if o.Volume < 0 || o.Volume > 100 {
		return fmt.Errorf("invalid_option: volume %d", o.Volume)
	}
	return nil
}

// known is a fixed allowlist on purpose: letting the UI send an arbitrary
// command would turn any webview bug into code execution. SMPlayer is left
// out: it starts its own mpv with the stream URL in argv.
// Celluloid, Haruna and IINA get only the playlist: they have no stable
// CLI flags for these options, and passing mpv options through them is
// exactly the kind of string building the allowlist exists to avoid.
//
// No player ever gets the stream URL in argv: Xtream URLs carry the password
// in their path and /proc/<pid>/cmdline is world-readable. They get a 0600
// playlist file instead (see playlist.go).
var known = []struct {
	id, name, bin string
	// GOOS values the player exists on; empty means everywhere.
	goos    []string
	options []string
	args    func(l launch, o Options) []string
}{
	{"mpv", "mpv", "mpv", nil, []string{OptSoftwareDecoding, OptCacheSecs, OptVolume}, func(l launch, o Options) []string {
		args := []string{
			"--force-window=immediate", "--really-quiet",
			// "$$" stops mpv expanding ${path} in a hostile channel name into
			// the window title, which would show the password-bearing URL.
			"--title=" + strings.ReplaceAll(l.title, "$", "$$"),
			// IPTV never needs yt-dlp, and on a failed open the hook would
			// hand the provider URL to yt-dlp's generic extractor.
			"--ytdl=no",
		}
		if o.SoftwareDecoding {
			args = append(args, "--hwdec=no")
		} else {
			args = append(args, "--hwdec=auto-safe")
		}
		if o.CacheSecs > 0 {
			args = append(args, "--cache=yes", "--cache-secs="+strconv.Itoa(o.CacheSecs))
		}
		if o.Volume > 0 {
			args = append(args, "--volume="+strconv.Itoa(o.Volume))
		}
		if l.unsafeScheme {
			// mpv refuses udp:// read from a playlist. The flag is scoped to
			// our own one-entry file, whose URL already passed IsStreamURL.
			return append(args, "--{", "--playlist="+l.list, "--load-unsafe-playlists", "--}")
		}
		return append(args, "--playlist="+l.list)
	}},
	// The #EXTINF line names the item: VLC's --meta-title would only label
	// the playlist node, not the stream it expands to.
	{"vlc", "VLC", "vlc", nil, []string{OptSoftwareDecoding, OptCacheSecs}, func(l launch, o Options) []string {
		var args []string
		if o.SoftwareDecoding {
			args = append(args, "--avcodec-hw=none")
		}
		if o.CacheSecs > 0 {
			args = append(args, "--network-caching="+strconv.Itoa(o.CacheSecs*1000))
		}
		return append(args, l.list)
	}},
	{"celluloid", "Celluloid", "celluloid", []string{"linux"}, nil, func(l launch, _ Options) []string {
		// Celluloid is libmpv, so it has mpv's udp-from-playlist refusal; a
		// fixed flag, not a pass-through of anything the UI sends.
		if l.unsafeScheme {
			return []string{"--mpv-load-unsafe-playlists=yes", l.list}
		}
		return []string{l.list}
	}},
	{"haruna", "Haruna", "haruna", []string{"linux"}, nil, func(l launch, _ Options) []string { return []string{l.list} }},
	{"iina", "IINA", "iina", []string{"darwin"}, nil, func(l launch, _ Options) []string {
		// IINA is libmpv too and forwards --mpv-* options to it.
		if l.unsafeScheme {
			return []string{"--mpv-load-unsafe-playlists=yes", l.list}
		}
		return []string{l.list}
	}},
}

// onThisOS reports whether a known player is offered on this platform.
func onThisOS(goos []string) bool {
	return len(goos) == 0 || slices.Contains(goos, runtime.GOOS)
}

// launch is what a player is started with: never the stream URL itself.
type launch struct {
	// Window/item title, already passed through displayTitle.
	title string
	// Absolute path of the playlist file holding the URL.
	list string
	// The URL uses a scheme mpv only opens from a playlist when told to.
	unsafeScheme bool
}

// Info describes a supported player for the settings screen.
type Info struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Installed bool   `json:"installed"`
	// Which Options this player honours; the UI greys out the rest.
	Options []string `json:"options"`
}

// Available lists every supported player and whether it is on PATH.
func Available() []Info {
	out := make([]Info, 0, len(known))
	for _, p := range known {
		if !onThisOS(p.goos) {
			continue
		}
		_, err := findPlayer(p.id, p.bin)
		out = append(out, Info{ID: p.id, Name: p.name, Installed: err == nil, Options: append([]string{}, p.options...)})
	}
	return out
}

// Known reports whether id is a supported player.
func Known(id string) bool {
	for _, p := range known {
		if p.id == id && onThisOS(p.goos) {
			return true
		}
	}
	return false
}

// lookPathOr finds bin on PATH, then at the given absolute paths. Apps
// started from a desktop shortcut on macOS or Windows rarely have the
// player's install directory on PATH.
func lookPathOr(bin string, paths []string) (string, error) {
	if p, err := exec.LookPath(bin); err == nil {
		return p, nil
	}
	for _, p := range paths {
		if fi, err := os.Stat(p); err == nil && fi.Mode().IsRegular() {
			return p, nil
		}
	}
	return "", exec.ErrNotFound
}

// Player keeps a single instance: starting a channel replaces the previous one.
type Player struct {
	mu  sync.Mutex
	cmd *exec.Cmd
	// Playlist file of the running player.
	list string
}

// Play opens the stream in the chosen player, closing whatever was playing before.
func (m *Player) Play(playerID, title, streamURL string, o Options) error {
	// Checked here too because caches written before the parser filtered
	// schemes may hold anything. It also rejects control characters, so the
	// URL stays on its own line in the playlist file.
	if !playlist.IsStreamURL(streamURL) {
		return errors.New("stream_url_forbidden")
	}
	idx := -1
	for i, p := range known {
		if p.id == playerID && onThisOS(p.goos) {
			idx = i
		}
	}
	if idx < 0 {
		return fmt.Errorf("unknown_player: %s", playerID)
	}
	p := known[idx]
	bin, err := findPlayer(p.id, p.bin)
	if err != nil {
		return fmt.Errorf("player_missing: %s", p.name)
	}

	title = displayTitle(title, streamURL)
	dir, err := playlistDir()
	if err != nil {
		return fmt.Errorf("player_launch_failed: %w", err)
	}
	removeStale(dir, time.Now().Add(-staleAfter))
	list, err := writePlaylist(dir, title, streamURL)
	if err != nil {
		return fmt.Errorf("player_launch_failed: %w", err)
	}
	l := launch{title: title, list: list, unsafeScheme: needsUnsafePlaylist(streamURL)}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopLocked()

	cmd := playerCommand(bin, p.args(l, o))
	if err := cmd.Start(); err != nil {
		_ = os.Remove(list)
		return err
	}
	m.cmd, m.list = cmd, list
	// Reap the process whenever it exits, whether the user closed it or we
	// did, and drop its playlist: the file holds the password.
	go func() {
		_ = cmd.Wait()
		_ = os.Remove(list)
	}()
	return nil
}

// Stop closes the player if it is running.
func (m *Player) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopLocked()
}

func (m *Player) stopLocked() {
	if m.cmd != nil && m.cmd.Process != nil {
		stopProcess(m.cmd.Process)
	}
	// Removed here as well because on app exit the Wait goroutine may not
	// get to run.
	if m.list != "" {
		_ = os.Remove(m.list)
	}
	m.cmd, m.list = nil, ""
}
