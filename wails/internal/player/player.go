// Package player opens streams in an external media player. The webview
// cannot decode MPEG-TS, and desktop players handle every codec IPTV
// providers throw at them.
package player

import (
	"errors"
	"fmt"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"sync"

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
// command would turn any webview bug into code execution. Mirrors player.rs.
// Celluloid, Haruna and SMPlayer get only the URL: they have no stable CLI
// flags for these options, and passing mpv options through them is exactly
// the kind of string building the allowlist exists to avoid.
var known = []struct {
	id, name, bin string
	options       []string
	args          func(title, url string, o Options) []string
}{
	{"mpv", "mpv", "mpv", []string{OptSoftwareDecoding, OptCacheSecs, OptVolume}, func(title, url string, o Options) []string {
		args := []string{"--force-window=immediate", "--really-quiet",
			// "$$" stops mpv expanding ${path} in a hostile channel name into
			// the window title, which would show the password-bearing URL.
			"--title=Herakleo · " + strings.ReplaceAll(title, "$", "$$")}
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
		return append(args, "--", url)
	}},
	{"vlc", "VLC", "vlc", []string{OptSoftwareDecoding, OptCacheSecs}, func(title, url string, o Options) []string {
		args := []string{"--meta-title=Herakleo · " + title}
		if o.SoftwareDecoding {
			args = append(args, "--avcodec-hw=none")
		}
		if o.CacheSecs > 0 {
			args = append(args, "--network-caching="+strconv.Itoa(o.CacheSecs*1000))
		}
		return append(args, url)
	}},
	{"celluloid", "Celluloid", "celluloid", nil, func(_, url string, _ Options) []string { return []string{url} }},
	{"haruna", "Haruna", "haruna", nil, func(_, url string, _ Options) []string { return []string{url} }},
	{"smplayer", "SMPlayer", "smplayer", nil, func(_, url string, _ Options) []string { return []string{url} }},
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
		_, err := exec.LookPath(p.bin)
		out = append(out, Info{ID: p.id, Name: p.name, Installed: err == nil, Options: append([]string{}, p.options...)})
	}
	return out
}

// Known reports whether id is a supported player.
func Known(id string) bool {
	for _, p := range known {
		if p.id == id {
			return true
		}
	}
	return false
}

// Player keeps a single instance: starting a channel replaces the previous one.
type Player struct {
	mu  sync.Mutex
	cmd *exec.Cmd
}

// Play opens the stream in the chosen player, closing whatever was playing before.
func (m *Player) Play(playerID, title, streamURL string, o Options) error {
	// Checked here too because caches written before the parser filtered
	// schemes may hold anything. It also guarantees the URL starts with a
	// scheme, so it can never be parsed as a flag by players without "--".
	if !playlist.IsStreamURL(streamURL) {
		return errors.New("stream_url_forbidden")
	}
	idx := -1
	for i, p := range known {
		if p.id == playerID {
			idx = i
		}
	}
	if idx < 0 {
		return fmt.Errorf("unknown_player: %s", playerID)
	}
	p := known[idx]
	bin, err := exec.LookPath(p.bin)
	if err != nil {
		return fmt.Errorf("player_missing: %s", p.name)
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopLocked()

	cmd := exec.Command(bin, p.args(title, streamURL, o)...)
	if err := cmd.Start(); err != nil {
		return err
	}
	m.cmd = cmd
	// Reap the process whenever it exits, whether the user closed it or we did.
	go func() { _ = cmd.Wait() }()
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
		_ = m.cmd.Process.Kill()
	}
	m.cmd = nil
}
