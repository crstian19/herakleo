package library

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"slices"
	"strings"
	"time"
)

// maxHistory keeps the Recent view short and library.json small.
const maxHistory = 50

// SetChannelHidden hides or unhides one channel everywhere but the hidden view.
func (l *Library) SetChannelHidden(channelID string, hidden bool) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.syncLocked()
	setFlag(l.hiddenChannels, channelID, hidden)
	return l.saveState()
}

// SetGroupHidden hides or unhides a group by name, across all sources: lists
// from the same provider repeat group names, and that is what users expect.
func (l *Library) SetGroupHidden(name string, hidden bool) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.syncLocked()
	setFlag(l.hiddenGroups, name, hidden)
	return l.saveState()
}

// HiddenGroups lists hidden group names, sorted.
func (l *Library) HiddenGroups() []string {
	l.sync()
	l.mu.RLock()
	defer l.mu.RUnlock()
	return sortedKeys(l.hiddenGroups)
}

// AddHistory records a played channel at the top of the history.
func (l *Library) AddHistory(channelID string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.syncLocked()
	l.history = slices.DeleteFunc(l.history, func(h HistoryEntry) bool { return h.ID == channelID })
	l.history = slices.Insert(l.history, 0, HistoryEntry{ID: channelID, At: time.Now().Unix()})
	if len(l.history) > maxHistory {
		l.history = l.history[:maxHistory]
	}
	return l.saveState()
}

// ClearHistory empties the Recent view.
func (l *Library) ClearHistory() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.syncLocked()
	l.history = nil
	return l.saveState()
}

// FavoriteRef identifies a favourite by names instead of IDs, so a backup
// still matches after a list is deleted and added again (new source ID) or
// moved to another machine. Stream URLs are left out: they carry passwords.
type FavoriteRef struct {
	Source  string `json:"source"`
	Channel string `json:"channel"`
	Group   string `json:"group"`
}

// FavoritesBackup is the file format of an exported favourites list.
type FavoritesBackup struct {
	Format    string        `json:"format"`
	Version   int           `json:"version"`
	Favorites []FavoriteRef `json:"favorites"`
}

const backupFormat = "herakleo-favorites"

// ExportFavorites returns every favourite whose channel still exists.
func (l *Library) ExportFavorites() FavoritesBackup {
	l.sync()
	l.mu.RLock()
	defer l.mu.RUnlock()
	out := FavoritesBackup{Format: backupFormat, Version: 1, Favorites: []FavoriteRef{}}
	for _, s := range l.sources {
		for _, c := range l.channels[s.ID] {
			if l.favorites[c.ID] {
				out.Favorites = append(out.Favorites, FavoriteRef{Source: s.Name, Channel: c.Name, Group: c.Group})
			}
		}
	}
	return out
}

// ImportFavorites marks the channels a backup names and returns how many it
// found. A list is matched by name (case-insensitive); inside it a channel
// by name and group, else by name alone.
func (l *Library) ImportFavorites(b FavoritesBackup) (int, error) {
	if b.Format != backupFormat {
		return 0, errors.New("backup_invalid")
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.syncLocked()

	type key struct{ source, channel, group string }
	byFull := map[key]string{}
	byName := map[key]string{}
	for _, s := range l.sources {
		src := strings.ToLower(s.Name)
		for _, c := range l.channels[s.ID] {
			full := key{src, c.Name, c.Group}
			if _, ok := byFull[full]; !ok {
				byFull[full] = c.ID
			}
			short := key{src, c.Name, ""}
			if _, ok := byName[short]; !ok {
				byName[short] = c.ID
			}
		}
	}

	found := 0
	for _, f := range b.Favorites {
		src := strings.ToLower(f.Source)
		id, ok := byFull[key{src, f.Channel, f.Group}]
		if !ok {
			id, ok = byName[key{src, f.Channel, ""}]
		}
		if ok {
			l.favorites[id] = true
			found++
		}
	}
	if found == 0 {
		return 0, nil
	}
	return found, l.saveState()
}

// maxBackupBytes bounds an imported file; real backups are a few KB.
const maxBackupBytes = 8 << 20

// WriteBackup saves an exported favourites list as indented JSON.
func WriteBackup(path string, b FavoritesBackup) error {
	data, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600) //nolint:gosec // path comes from a native save dialog
}

// ReadBackup loads a favourites list chosen by the user.
func ReadBackup(path string) (FavoritesBackup, error) {
	f, err := os.Open(path) //nolint:gosec // path comes from a native open dialog
	if err != nil {
		return FavoritesBackup{}, err
	}
	defer func() { _ = f.Close() }()
	var b FavoritesBackup
	if err := json.NewDecoder(io.LimitReader(f, maxBackupBytes)).Decode(&b); err != nil {
		return FavoritesBackup{}, errors.New("backup_invalid")
	}
	return b, nil
}
