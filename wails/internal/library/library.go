// Package library keeps the user's sources, their cached channels, favourites,
// hidden items, history and shared settings, persisted as JSON under the config
// dir. Two running copies of the app may share it (see sync.go).
package library

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/crstian19/herakleo/wails/internal/playlist"
)

// Kind is how a source is loaded.
type Kind string

// Source kinds, as stored in library.json and sent by the UI.
const (
	KindM3UURL  Kind = "m3u_url"
	KindM3UFile Kind = "m3u_file"
	KindXtream  Kind = "xtream"
)

// source is the persisted form; it holds credentials and never leaves the backend.
type source struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Kind         Kind   `json:"kind"`
	URL          string `json:"url,omitempty"`
	Path         string `json:"path,omitempty"`
	Server       string `json:"server,omitempty"`
	Username     string `json:"username,omitempty"`
	Password     string `json:"password,omitempty"`
	ChannelCount int    `json:"channelCount"`
	UpdatedAt    int64  `json:"updatedAt"`
	// Disabled rather than Enabled so lists saved before this field existed stay on.
	Disabled bool `json:"disabled,omitempty"`
	// Xtream account info from the last refresh; zero/empty when unknown.
	ExpiresAt     int64  `json:"expiresAt,omitempty"`
	AccountStatus string `json:"accountStatus,omitempty"`
	// GuideURL is the XMLTV URL the user typed; ListGuideURL the one in the
	// M3U header. Xtream guides are built from the credentials when fetched.
	GuideURL       string `json:"guideUrl,omitempty"`
	ListGuideURL   string `json:"listGuideUrl,omitempty"`
	GuideUpdatedAt int64  `json:"guideUpdatedAt,omitempty"`
	GuideChannels  int    `json:"guideChannels,omitempty"`
}

// channel is the persisted form; URL stays in the backend since Xtream URLs
// embed the password.
type channel struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Group string `json:"group"`
	Logo  string `json:"logo"`
	URL   string `json:"url"`
	EPGID string `json:"epgId,omitempty"`
	// lower is Name in lower case, computed once at load (withKeys) so search
	// and sort never re-lowercase 65k names per query. Not persisted.
	lower string
}

// withKeys fills the derived fields of freshly loaded channels.
func withKeys(chs []channel) []channel {
	for i := range chs {
		chs[i].lower = strings.ToLower(chs[i].Name)
	}
	return chs
}

// HistoryEntry is one played channel, newest first in the history list.
type HistoryEntry struct {
	ID string `json:"id"`
	At int64  `json:"at"`
}

type state struct {
	Sources        []*source      `json:"sources"`
	Favorites      []string       `json:"favorites"`
	Settings       Settings       `json:"settings"`
	HiddenChannels []string       `json:"hiddenChannels"`
	HiddenGroups   []string       `json:"hiddenGroups"`
	History        []HistoryEntry `json:"history"`
	Reminders      []Reminder     `json:"reminders,omitempty"`
}

// Settings are app preferences that both builds share, unlike the purely
// visual ones the frontend keeps in localStorage.
type Settings struct {
	// Player is a player ID from the player package; empty means the default.
	Player         string `json:"player"`
	RefreshOnStart bool   `json:"refreshOnStart"`
	// Playback options; validated by the player package before they get here.
	SoftwareDecoding bool `json:"softwareDecoding"`
	CacheSecs        int  `json:"cacheSecs"`
	Volume           int  `json:"volume"`
}

// Library is safe for concurrent use; Wails calls bound methods from many goroutines.
type Library struct {
	dir string

	mu             sync.RWMutex
	sources        []*source
	favorites      map[string]bool
	hiddenChannels map[string]bool
	hiddenGroups   map[string]bool
	history        []HistoryEntry
	reminders      []Reminder
	settings       Settings
	channels       map[string][]channel // by source ID
	// chanGen changes whenever l.channels does; it invalidates sorted (query.go).
	chanGen uint64
	sortMu  sync.Mutex
	sorted  sortCache
	// Modification times of what we last read or wrote, to notice the other
	// build changing the files underneath us (sync.go).
	stateMod time.Time
	chanMod  map[string]time.Time

	// Guides by source (guide.go), loaded lazily from epg/<id>.json.
	guideMu  sync.RWMutex
	guides   map[string]playlist.Guide
	guideMod map[string]time.Time

	// Logo accent cache (colors.go), separate lock: filled by slow downloads.
	colorMu sync.Mutex
	colors  map[string]string

	// Files the user chose in the native dialog this session (AllowFile). Only
	// these may become m3u_file sources: otherwise a compromised webview could
	// name any readable file and get its lines back as channel names.
	pickMu sync.Mutex
	picked map[string]bool
}

// Dir is the folder the library is stored in.
func (l *Library) Dir() string { return l.dir }

// Open loads the library from dir, creating it on first run.
func Open(dir string) (*Library, error) {
	// 0700 because library.json holds passwords; MkdirAll leaves existing
	// dirs alone, so tighten them too (an older build may have created them).
	// gosec: dir is the user's config folder, and a directory needs the x bit.
	for _, d := range []string{dir, filepath.Join(dir, "channels"), filepath.Join(dir, "epg")} {
		if err := os.MkdirAll(d, 0o700); err != nil { //nolint:gosec
			return nil, err
		}
		if err := os.Chmod(d, 0o700); err != nil { //nolint:gosec
			return nil, err
		}
	}
	l := &Library{
		dir: dir, channels: map[string][]channel{}, chanMod: map[string]time.Time{}, picked: map[string]bool{},
		guides: map[string]playlist.Guide{}, guideMod: map[string]time.Time{},
	}
	if err := l.reloadLocked(); err != nil {
		return nil, err
	}
	l.removeOrphans()
	return l, nil
}

// NewSource is what the UI sends to add a source.
type NewSource struct {
	Kind     Kind   `json:"kind"`
	Name     string `json:"name"`
	URL      string `json:"url"`
	Path     string `json:"path"`
	Server   string `json:"server"`
	Username string `json:"username"`
	Password string `json:"password"`
	// GuideURL is an optional XMLTV URL; Add only (SetGuideURL changes it).
	GuideURL string `json:"guideUrl"`
}

// SourceInfo is a source as the UI sees it.
type SourceInfo struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Kind          Kind   `json:"kind"`
	Location      string `json:"location"`
	ChannelCount  int    `json:"channelCount"`
	UpdatedAt     int64  `json:"updatedAt"`
	Enabled       bool   `json:"enabled"`
	ExpiresAt     int64  `json:"expiresAt"`
	AccountStatus string `json:"accountStatus"`
	// GuideURL is the guide URL the user set, without credentials.
	GuideURL string `json:"guideUrl"`
	// HasGuide: a guide can be fetched (user URL, Xtream panel or list header).
	HasGuide       bool  `json:"hasGuide"`
	GuideUpdatedAt int64 `json:"guideUpdatedAt"`
	// GuideChannels is how many channels the last guide matched.
	GuideChannels int `json:"guideChannels"`
}

// Add downloads a new source and stores it. Nothing is saved if loading fails.
func (l *Library) Add(ctx context.Context, n NewSource) (SourceInfo, error) {
	s := &source{
		Name:     strings.TrimSpace(n.Name),
		Kind:     n.Kind,
		URL:      strings.TrimSpace(n.URL),
		Path:     n.Path,
		Server:   strings.TrimSpace(n.Server),
		Username: strings.TrimSpace(n.Username),
		Password: n.Password,
		GuideURL: strings.TrimSpace(n.GuideURL),
	}
	if s.Name == "" {
		return SourceInfo{}, errors.New("name_required")
	}
	if s.Kind == KindM3UFile && !l.wasPicked(s.Path) {
		return SourceInfo{}, errors.New("file_not_picked")
	}
	if s.GuideURL != "" {
		if _, err := playlist.ValidateHTTPURL(s.GuideURL); err != nil {
			return SourceInfo{}, err
		}
	}
	id, err := newID()
	if err != nil {
		return SourceInfo{}, err
	}
	s.ID = id

	loaded, err := load(ctx, s)
	if err != nil {
		return SourceInfo{}, err
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	l.syncLocked()
	if err := l.storeChannels(s, loaded); err != nil {
		return SourceInfo{}, err
	}
	l.sources = append(l.sources, s)
	if err := l.saveState(); err != nil {
		l.sources = l.sources[:len(l.sources)-1]
		delete(l.channels, s.ID)
		l.chanGen++
		_ = os.Remove(l.channelsPath(s.ID))
		return SourceInfo{}, err
	}
	return s.info(), nil
}

// Refresh re-downloads a source. Favourites survive because channel IDs are
// derived from the stream URL.
func (l *Library) Refresh(ctx context.Context, id string) (SourceInfo, error) {
	snapshot, ok := l.snapshot(id)
	if !ok {
		return SourceInfo{}, errors.New("source_not_found")
	}

	loaded, err := load(ctx, &snapshot)
	if err != nil {
		return SourceInfo{}, err
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	l.syncLocked()
	s := l.find(id)
	if s == nil {
		return SourceInfo{}, errors.New("source_removed")
	}
	if err := l.storeChannels(s, loaded); err != nil {
		return SourceInfo{}, err
	}
	if err := l.saveState(); err != nil {
		return SourceInfo{}, err
	}
	return s.info(), nil
}

// Update edits a source. Empty fields keep their current value, so the UI
// never needs to read credentials back. Changing where or how the list is
// loaded re-downloads it first and saves nothing if that fails; a rename alone
// is instant. New stream URLs mean new channel IDs, so favourites of an
// Xtream source do not survive a credential change.
//
// Keeping stored fields must never send them somewhere new: an Xtream server
// change needs the password in the same request, and a new file path must
// come from the native dialog.
func (l *Library) Update(ctx context.Context, id string, n NewSource) (SourceInfo, error) {
	next, ok := l.snapshot(id)
	if !ok {
		return SourceInfo{}, errors.New("source_not_found")
	}
	if next.Kind == KindXtream && n.Password == "" {
		if server := strings.TrimSpace(n.Server); server != "" && !sameServer(server, next.Server) {
			return SourceInfo{}, errors.New("credentials_required")
		}
	}
	if n.Path != "" && n.Path != next.Path && !l.wasPicked(n.Path) {
		return SourceInfo{}, errors.New("file_not_picked")
	}

	reload := false
	set := func(dst *string, v string) {
		if v != "" && v != *dst {
			*dst, reload = v, true
		}
	}
	if name := strings.TrimSpace(n.Name); name != "" {
		next.Name = name
	}
	set(&next.URL, strings.TrimSpace(n.URL))
	set(&next.Path, n.Path)
	set(&next.Server, strings.TrimSpace(n.Server))
	set(&next.Username, strings.TrimSpace(n.Username))
	set(&next.Password, n.Password)

	var loaded loadResult
	if reload {
		var err error
		if loaded, err = load(ctx, &next); err != nil {
			return SourceInfo{}, err
		}
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	l.syncLocked()
	s := l.find(id)
	if s == nil {
		return SourceInfo{}, errors.New("source_removed")
	}
	// Keep fields another call may have changed meanwhile.
	next.Disabled = s.Disabled
	if reload {
		if err := l.storeChannels(&next, loaded); err != nil {
			return SourceInfo{}, err
		}
	}
	*s = next
	if err := l.saveState(); err != nil {
		return SourceInfo{}, err
	}
	return s.info(), nil
}

// AllowFile records a path the user chose in the native file dialog, so Add
// and Update accept it as an m3u_file source.
func (l *Library) AllowFile(path string) {
	if path == "" {
		return
	}
	l.pickMu.Lock()
	defer l.pickMu.Unlock()
	l.picked[path] = true
}

func (l *Library) wasPicked(path string) bool {
	l.pickMu.Lock()
	defer l.pickMu.Unlock()
	return l.picked[path]
}

// sameServer reports whether two Xtream server addresses reach the same
// scheme and host, ignoring case, spaces and a trailing slash. Anything
// unparsable only matches itself.
func sameServer(a, b string) bool {
	a, b = playlist.Xtream{Server: a}.Location(), playlist.Xtream{Server: b}.Location()
	ua, errA := url.Parse(a)
	ub, errB := url.Parse(b)
	if errA != nil || errB != nil || ua.Host == "" || ub.Host == "" {
		return a == b
	}
	return strings.EqualFold(ua.Scheme, ub.Scheme) && strings.EqualFold(ua.Host, ub.Host)
}

// SetEnabled turns a source on or off without deleting it. Disabled sources
// keep their cache but are left out of channels, groups and refresh-all.
func (l *Library) SetEnabled(id string, enabled bool) (SourceInfo, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.syncLocked()
	s := l.find(id)
	if s == nil {
		return SourceInfo{}, errors.New("source_not_found")
	}
	s.Disabled = !enabled
	if err := l.saveState(); err != nil {
		return SourceInfo{}, err
	}
	return s.info(), nil
}

// Remove deletes a source and everything that points at its channels.
func (l *Library) Remove(id string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.syncLocked()
	idx := slices.IndexFunc(l.sources, func(s *source) bool { return s.ID == id })
	if idx < 0 {
		return errors.New("source_not_found")
	}
	l.sources = slices.Delete(l.sources, idx, idx+1)
	prefix := id + "-"
	for _, set := range []map[string]bool{l.favorites, l.hiddenChannels} {
		for k := range set {
			if strings.HasPrefix(k, prefix) {
				delete(set, k)
			}
		}
	}
	l.history = slices.DeleteFunc(l.history, func(h HistoryEntry) bool { return strings.HasPrefix(h.ID, prefix) })
	l.reminders = slices.DeleteFunc(l.reminders, func(r Reminder) bool { return strings.HasPrefix(r.ChannelID, prefix) })
	l.dropGuide(id)
	delete(l.channels, id)
	delete(l.chanMod, id)
	l.chanGen++
	if err := os.Remove(l.channelsPath(id)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return l.saveState()
}

// Sources lists every source in insertion order, enabled or not.
func (l *Library) Sources() []SourceInfo {
	l.sync()
	l.mu.RLock()
	defer l.mu.RUnlock()
	out := make([]SourceInfo, 0, len(l.sources))
	for _, s := range l.sources {
		out = append(out, s.info())
	}
	return out
}

// SetFavorite marks or unmarks a channel.
func (l *Library) SetFavorite(channelID string, fav bool) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.syncLocked()
	setFlag(l.favorites, channelID, fav)
	return l.saveState()
}

// Settings returns the shared preferences.
func (l *Library) Settings() Settings {
	l.sync()
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.settings
}

// SettingsPatch changes only the fields that are set.
type SettingsPatch struct {
	Player           *string `json:"player,omitempty"`
	RefreshOnStart   *bool   `json:"refreshOnStart,omitempty"`
	SoftwareDecoding *bool   `json:"softwareDecoding,omitempty"`
	CacheSecs        *int    `json:"cacheSecs,omitempty"`
	Volume           *int    `json:"volume,omitempty"`
}

// UpdateSettings applies a patch; the caller validates the player ID and options.
func (l *Library) UpdateSettings(p SettingsPatch) (Settings, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.syncLocked()
	if p.Player != nil {
		l.settings.Player = *p.Player
	}
	if p.RefreshOnStart != nil {
		l.settings.RefreshOnStart = *p.RefreshOnStart
	}
	if p.SoftwareDecoding != nil {
		l.settings.SoftwareDecoding = *p.SoftwareDecoding
	}
	if p.CacheSecs != nil {
		l.settings.CacheSecs = *p.CacheSecs
	}
	if p.Volume != nil {
		l.settings.Volume = *p.Volume
	}
	return l.settings, l.saveState()
}

// StreamURL resolves a channel ID to its stream, for the player.
func (l *Library) StreamURL(channelID string) (name, streamURL string, err error) {
	l.sync()
	l.mu.RLock()
	defer l.mu.RUnlock()
	if c := l.channel(channelID); c != nil {
		return c.Name, c.URL, nil
	}
	return "", "", errors.New("channel_not_found")
}

// loadResult is a downloaded source plus Xtream account info, if any.
type loadResult struct {
	entries []playlist.Entry
	account playlist.Account
	// guideURL is the M3U header's guide, if any.
	guideURL string
}

func load(ctx context.Context, s *source) (loadResult, error) {
	switch s.Kind {
	case KindM3UURL:
		l, err := playlist.M3UFromURL(ctx, s.URL)
		return loadResult{entries: l.Entries, guideURL: l.GuideURL}, err
	case KindM3UFile:
		l, err := playlist.M3UFromFile(s.Path)
		return loadResult{entries: l.Entries, guideURL: l.GuideURL}, err
	case KindXtream:
		if s.Username == "" || s.Password == "" {
			return loadResult{}, errors.New("credentials_required")
		}
		x := playlist.Xtream{Server: s.Server, Username: s.Username, Password: s.Password}
		e, err := x.LiveChannels(ctx)
		if err != nil {
			return loadResult{}, err
		}
		// Expiry is nice to have: a panel that hides it must not block loading.
		acc, _ := x.AccountInfo(ctx)
		return loadResult{entries: e, account: acc}, nil
	default:
		return loadResult{}, fmt.Errorf("unknown_kind: %s", s.Kind)
	}
}

// storeChannels assigns IDs, persists the cache and updates s. Caller holds mu.
func (l *Library) storeChannels(s *source, r loadResult) error {
	if len(r.entries) == 0 {
		return errors.New("playlist_empty")
	}
	seen := make(map[string]bool, len(r.entries))
	chs := make([]channel, 0, len(r.entries))
	for _, e := range r.entries {
		id := ChannelID(s.ID, e.URL)
		if seen[id] {
			continue
		}
		seen[id] = true
		chs = append(chs, channel{ID: id, Name: e.Name, Group: e.Group, Logo: e.Logo, URL: e.URL, EPGID: e.EPGID, lower: strings.ToLower(e.Name)})
	}

	data, err := json.Marshal(chs)
	if err != nil {
		return err
	}
	path := l.channelsPath(s.ID)
	if err := writeFileAtomic(path, data); err != nil {
		return err
	}
	l.channels[s.ID] = chs
	l.chanMod[s.ID] = modTime(path)
	l.chanGen++
	s.ChannelCount = len(chs)
	s.UpdatedAt = time.Now().Unix()
	s.ExpiresAt, s.AccountStatus = r.account.ExpiresAt, r.account.Status
	s.ListGuideURL = r.guideURL
	return nil
}

// saveState persists everything but the channel caches. Caller holds mu.
func (l *Library) saveState() error {
	st := state{
		Sources:        l.sources,
		Favorites:      sortedKeys(l.favorites),
		Settings:       l.settings,
		HiddenChannels: sortedKeys(l.hiddenChannels),
		HiddenGroups:   sortedKeys(l.hiddenGroups),
		History:        append([]HistoryEntry{}, l.history...),
		Reminders:      l.reminders,
	}
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	if err := writeFileAtomic(l.statePath(), data); err != nil {
		return err
	}
	l.stateMod = modTime(l.statePath())
	return nil
}

// snapshot copies a source so network I/O can run without holding the lock.
func (l *Library) snapshot(id string) (source, bool) {
	l.sync()
	l.mu.RLock()
	defer l.mu.RUnlock()
	if s := l.find(id); s != nil {
		return *s, true
	}
	return source{}, false
}

func (l *Library) find(id string) *source {
	for _, s := range l.sources {
		if s.ID == id {
			return s
		}
	}
	return nil
}

func (l *Library) channel(channelID string) *channel {
	sourceID, _, _ := strings.Cut(channelID, "-")
	chs := l.channels[sourceID]
	for i := range chs {
		if chs[i].ID == channelID {
			return &chs[i]
		}
	}
	return nil
}

func (l *Library) statePath() string { return filepath.Join(l.dir, "library.json") }

func (l *Library) channelsPath(id string) string {
	return filepath.Join(l.dir, "channels", id+".json")
}

func (s *source) info() SourceInfo {
	loc := redactURL(s.URL)
	switch s.Kind {
	case KindM3UFile:
		loc = s.Path
	case KindXtream:
		loc = playlist.Xtream{Server: s.Server}.Location()
	}
	return SourceInfo{
		ID: s.ID, Name: s.Name, Kind: s.Kind, Location: loc, ChannelCount: s.ChannelCount,
		UpdatedAt: s.UpdatedAt, Enabled: !s.Disabled, ExpiresAt: s.ExpiresAt, AccountStatus: s.AccountStatus,
		GuideURL: redactURL(s.GuideURL), HasGuide: s.GuideURL != "" || s.ListGuideURL != "" || s.Kind == KindXtream,
		GuideUpdatedAt: s.GuideUpdatedAt, GuideChannels: s.GuideChannels,
	}
}

// ChannelID is "<sourceID>-<fnv64a(url)>". Favourites are stored by ID, so
// changing this would orphan every saved one (library_test.go pins it).
func ChannelID(sourceID, streamURL string) string {
	h := fnv.New64a()
	_, _ = h.Write([]byte(streamURL))
	return fmt.Sprintf("%s-%016x", sourceID, h.Sum64())
}

// redactURL drops credentials and the query: provider M3U links usually look
// like get.php?username=X&password=Y, and the UI only needs to recognise the host.
func redactURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	u.User, u.RawQuery, u.ForceQuery, u.Fragment = nil, "", false, ""
	return u.String()
}

func validID(id string) bool {
	if len(id) != 12 {
		return false
	}
	for _, r := range id {
		if !strings.ContainsRune("0123456789abcdef", r) {
			return false
		}
	}
	return true
}

func newID() (string, error) {
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func setFlag(set map[string]bool, key string, on bool) {
	if on {
		set[key] = true
	} else {
		delete(set, key)
	}
}

// sortedKeys gives stable file contents across saves and across builds.
func sortedKeys(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

func toSet(keys []string) map[string]bool {
	out := make(map[string]bool, len(keys))
	for _, k := range keys {
		out[k] = true
	}
	return out
}

// writeFileAtomic writes via a temp file + rename so a crash never leaves a
// half-written library. Files are 0600: library.json holds Xtream passwords.
func writeFileAtomic(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
