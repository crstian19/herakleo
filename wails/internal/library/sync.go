package library

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Both builds can run at once against the same files. Each keeps the library
// in memory, so without this the last one to save would silently drop what
// the other added. Before every read or write we compare library.json's
// modification time with the one we last saw and reload if it moved. It is
// last-writer-wins per operation, not a merge, which is enough for one person
// clicking in two windows.

// sync reloads from disk if another process changed library.json.
func (l *Library) sync() {
	l.mu.RLock()
	stale := !modTime(l.statePath()).Equal(l.stateMod)
	l.mu.RUnlock()
	if stale {
		l.mu.Lock()
		l.syncLocked()
		l.mu.Unlock()
	}
}

// syncLocked is sync for callers that already hold the write lock. A file
// that fails to parse mid-write by the other app is ignored until next time.
func (l *Library) syncLocked() {
	if !modTime(l.statePath()).Equal(l.stateMod) {
		_ = l.reloadLocked()
	}
}

// reloadLocked reads library.json and any channel cache that changed.
func (l *Library) reloadLocked() error {
	var st state
	switch data, err := os.ReadFile(l.statePath()); {
	case errors.Is(err, os.ErrNotExist):
	case err != nil:
		return err
	default:
		if err := json.Unmarshal(data, &st); err != nil {
			return fmt.Errorf("library_corrupt: %w", err)
		}
	}
	for _, s := range st.Sources {
		// IDs become file names; refuse anything a hand-edited file could use to escape channels/.
		if !validID(s.ID) {
			return fmt.Errorf("library_corrupt: invalid source id %q", s.ID)
		}
	}

	channels := make(map[string][]channel, len(st.Sources))
	chanMod := make(map[string]time.Time, len(st.Sources))
	for _, s := range st.Sources {
		path := l.channelsPath(s.ID)
		mod := modTime(path)
		if cached, ok := l.channels[s.ID]; ok && mod.Equal(l.chanMod[s.ID]) {
			channels[s.ID], chanMod[s.ID] = cached, mod
			continue
		}
		var chs []channel
		data, err := os.ReadFile(path) //nolint:gosec // channelsPath of a validated source ID
		if err == nil {
			err = json.Unmarshal(data, &chs)
		}
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("channels_corrupt: %q: %w", s.Name, err)
		}
		channels[s.ID], chanMod[s.ID] = withKeys(chs), mod
	}

	// Never persist "sources": null; older builds reject it.
	l.sources = append([]*source{}, st.Sources...)
	l.favorites = toSet(st.Favorites)
	l.hiddenChannels = toSet(st.HiddenChannels)
	l.hiddenGroups = toSet(st.HiddenGroups)
	l.history = append([]HistoryEntry{}, st.History...)
	l.reminders = append([]Reminder{}, st.Reminders...)
	l.settings = st.Settings
	l.channels, l.chanMod = channels, chanMod
	l.chanGen++
	l.stateMod = modTime(l.statePath())
	return nil
}

// orphanGrace leaves alone files the other app may be writing right now.
const orphanGrace = time.Minute

// removeOrphans deletes channel caches no source points at (left behind when
// both apps used to overwrite each other) and stale temp files.
func (l *Library) removeOrphans() {
	known := make(map[string]bool, len(l.sources))
	for _, s := range l.sources {
		known[s.ID+".json"] = true
	}
	cutoff := time.Now().Add(-orphanGrace)
	for _, dir := range []string{l.dir, filepath.Join(l.dir, "channels"), filepath.Join(l.dir, "epg")} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		inChannels := dir != l.dir
		for _, e := range entries {
			name := e.Name()
			orphan := strings.HasPrefix(name, ".tmp-") ||
				(inChannels && strings.HasSuffix(name, ".json") && !known[name])
			if !orphan || e.IsDir() {
				continue
			}
			if info, err := e.Info(); err == nil && info.ModTime().Before(cutoff) {
				_ = os.Remove(filepath.Join(dir, name))
			}
		}
	}
}

func modTime(path string) time.Time {
	info, err := os.Stat(path)
	if err != nil {
		return time.Time{}
	}
	return info.ModTime()
}
