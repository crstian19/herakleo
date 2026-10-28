package player

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Playlist files left behind by a crash are swept once they are this old.
// Players read the file only at start, so a long session is never affected.
const staleAfter = 24 * time.Hour

const (
	listPrefix = "play-"
	listSuffix = ".m3u"
)

// playlistDir returns a private directory for playlist files, creating it
// if needed. XDG_RUNTIME_DIR is already per-user, 0700 and tmpfs; the shared
// temp dir fallback gets a per-uid name and an owner check, so another user
// cannot pre-create it and read the files. Windows has no uid (-1) and a
// per-user %TEMP%.
func playlistDir() (string, error) {
	base, name := runtimeDir(), "herakleo"
	if !filepath.IsAbs(base) {
		base = os.TempDir()
		if uid := os.Getuid(); uid >= 0 {
			name += "-" + strconv.Itoa(uid)
		}
	}
	dir := filepath.Join(base, name)
	if err := os.Mkdir(dir, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
		return "", err
	}
	// Lstat: a symlink planted in the shared temp dir must not be followed.
	fi, err := os.Lstat(dir)
	if err != nil {
		return "", err
	}
	if !fi.IsDir() || !ownedByMe(fi) {
		return "", fmt.Errorf("%s is not a directory owned by this user", dir)
	}
	if fi.Mode().Perm() != 0o700 {
		if err := os.Chmod(dir, 0o700); err != nil { //nolint:gosec // a directory needs the x bit
			return "", err
		}
	}
	return dir, nil
}

// writePlaylist stores the stream in a one-entry M3U only this user can
// read. CreateTemp makes the file 0600 with an unpredictable name.
func writePlaylist(dir, title, streamURL string) (string, error) {
	f, err := os.CreateTemp(dir, listPrefix+"*"+listSuffix)
	if err != nil {
		return "", err
	}
	// The URL cannot break out of its line: IsStreamURL already rejected it
	// if it held control characters (url.Parse does), and the title went
	// through displayTitle.
	_, err = fmt.Fprintf(f, "#EXTM3U\n#EXTINF:-1,%s\n%s\n", title, streamURL)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		_ = os.Remove(f.Name())
		return "", err
	}
	return f.Name(), nil
}

// removeStale is best-effort: a leftover file only matters if it is never
// removed, and a failed sweep must not stop playback.
func removeStale(dir string, before time.Time) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		name := e.Name()
		if !e.Type().IsRegular() || !strings.HasPrefix(name, listPrefix) || !strings.HasSuffix(name, listSuffix) {
			continue
		}
		if fi, err := e.Info(); err == nil && fi.ModTime().Before(before) {
			_ = os.Remove(filepath.Join(dir, name))
		}
	}
}

// displayTitle is the title shown by the player. A channel without an
// #EXTINF name is named after its URL, which would put the password in the
// window title (and, for mpv, in argv), so such names fall back to the host.
func displayTitle(name, streamURL string) string {
	// Control characters would let a name add lines to the playlist file.
	name = strings.TrimSpace(strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return ' '
		}
		return r
	}, name))
	if name == "" || name == streamURL || strings.Contains(name, "://") {
		name = ""
		if u, err := url.Parse(streamURL); err == nil {
			name = u.Hostname()
		}
	}
	if name == "" {
		return "Herakleo"
	}
	return "Herakleo · " + name
}

// needsUnsafePlaylist reports schemes mpv treats as unsafe when they come
// from a playlist (checked against mpv 0.41: only udp of those we allow).
func needsUnsafePlaylist(streamURL string) bool {
	u, err := url.Parse(streamURL)
	return err == nil && strings.EqualFold(u.Scheme, "udp")
}
