package library

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/crstian19/herakleo/wails/internal/playlist"
)

// The guide (EPG) of a source is fetched on demand, matched to its channels
// and kept in epg/<sourceID>.json: programmes by channel ID, only from a few
// hours ago to a day and a half ahead.
const (
	guideBehind = 3 * time.Hour
	guideAhead  = 36 * time.Hour
	// maxNowNext bounds one NowNext call; the UI asks for the visible cards.
	maxNowNext = 300
	// maxReminders keeps a hand-edited or runaway list in check.
	maxReminders = 500
)

// Programme is one guide entry; times are unix seconds.
type Programme = playlist.Programme

// NowNext is what a channel shows now and next; either may be missing.
type NowNext struct {
	Now  *Programme `json:"now,omitempty"`
	Next *Programme `json:"next,omitempty"`
}

// Reminder asks the UI to announce a programme when it starts.
type Reminder struct {
	ChannelID string `json:"channelId"`
	Channel   string `json:"channel"`
	Title     string `json:"title"`
	Start     int64  `json:"start"`
	Stop      int64  `json:"stop"`
}

// guideSource picks where a source's guide comes from: the URL the user set,
// the Xtream panel, or the list's own header. lan says whether the download
// may reach private addresses; a URL a provider chose may only when it is on
// the list's own host (a local proxy such as Threadfin serving both).
func guideSource(s *source) (rawURL string, lan bool, err error) {
	switch {
	case s.GuideURL != "":
		return s.GuideURL, true, nil
	case s.Kind == KindXtream:
		u, err := playlist.Xtream{Server: s.Server, Username: s.Username, Password: s.Password}.GuideURL()
		return u, true, err
	case s.ListGuideURL != "":
		return s.ListGuideURL, s.Kind == KindM3UURL && playlist.SameHost(s.ListGuideURL, s.URL), nil
	}
	return "", false, errors.New("guide_not_set")
}

// RefreshGuide downloads a source's guide and matches it to its channels,
// by tvg-id first and by name for channels without one.
func (l *Library) RefreshGuide(ctx context.Context, id string) (SourceInfo, error) {
	l.sync()
	l.mu.RLock()
	s := l.find(id)
	if s == nil {
		l.mu.RUnlock()
		return SourceInfo{}, errors.New("source_not_found")
	}
	snap := *s
	byEPG, byName := map[string][]string{}, map[string][]string{}
	for _, c := range l.channels[id] {
		if c.EPGID != "" {
			k := strings.ToLower(c.EPGID)
			byEPG[k] = append(byEPG[k], c.ID)
		} else if k := guideName(c.Name); k != "" {
			byName[k] = append(byName[k], c.ID)
		}
	}
	l.mu.RUnlock()

	rawURL, lan, err := guideSource(&snap)
	if err != nil {
		return SourceInfo{}, err
	}
	resolve := func(c playlist.GuideChannel) []string {
		if ids := byEPG[strings.ToLower(c.ID)]; len(ids) > 0 {
			return ids
		}
		var ids []string
		for _, n := range append([]string{c.ID}, c.Names...) {
			ids = append(ids, byName[guideName(n)]...)
		}
		slices.Sort(ids)
		return slices.Compact(ids)
	}
	now := time.Now()
	g, err := playlist.FetchGuide(ctx, rawURL, lan, now.Add(-guideBehind).Unix(), now.Add(guideAhead).Unix(), resolve)
	if err != nil {
		return SourceInfo{}, err
	}
	if len(g) == 0 {
		return SourceInfo{}, errors.New("guide_no_match")
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	l.syncLocked()
	s = l.find(id)
	if s == nil {
		return SourceInfo{}, errors.New("source_removed")
	}
	if err := l.storeGuide(id, g); err != nil {
		return SourceInfo{}, err
	}
	s.GuideUpdatedAt, s.GuideChannels = now.Unix(), len(g)
	if err := l.saveState(); err != nil {
		return SourceInfo{}, err
	}
	return s.info(), nil
}

// SetGuideURL sets the guide URL the user typed for a source; "" goes back
// to the panel's or the list's own guide.
func (l *Library) SetGuideURL(id, rawURL string) (SourceInfo, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL != "" {
		if _, err := playlist.ValidateHTTPURL(rawURL); err != nil {
			return SourceInfo{}, err
		}
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.syncLocked()
	s := l.find(id)
	if s == nil {
		return SourceInfo{}, errors.New("source_not_found")
	}
	s.GuideURL = rawURL
	if err := l.saveState(); err != nil {
		return SourceInfo{}, err
	}
	return s.info(), nil
}

// NowNext answers for up to maxNowNext channel IDs; channels without guide
// data are left out.
func (l *Library) NowNext(ids []string) map[string]NowNext {
	if len(ids) > maxNowNext {
		ids = ids[:maxNowNext]
	}
	now := time.Now().Unix()
	out := make(map[string]NowNext, len(ids))
	for _, id := range ids {
		ps := l.programmes(id)
		i := sort.Search(len(ps), func(i int) bool { return ps[i].Stop > now })
		var nn NowNext
		if i < len(ps) && ps[i].Start <= now {
			nn.Now = &ps[i]
			i++
		}
		if i < len(ps) {
			nn.Next = &ps[i]
		}
		if nn.Now != nil || nn.Next != nil {
			out[id] = nn
		}
	}
	return out
}

// ChannelGuide lists a channel's programmes from the one on air onwards.
func (l *Library) ChannelGuide(id string) []Programme {
	ps := l.programmes(id)
	now := time.Now().Unix()
	i := sort.Search(len(ps), func(i int) bool { return ps[i].Stop > now })
	return slices.Clone(ps[i:])
}

// programmes returns a channel's cached programmes, loading its source's
// guide file the first time (or when another process rewrote it).
func (l *Library) programmes(channelID string) []Programme {
	sourceID, _, ok := strings.Cut(channelID, "-")
	if !ok || !validID(sourceID) {
		return nil
	}
	path := l.guidePath(sourceID)
	mod := modTime(path)
	l.guideMu.RLock()
	g, cached := l.guides[sourceID]
	fresh := cached && mod.Equal(l.guideMod[sourceID])
	l.guideMu.RUnlock()
	if !fresh {
		g = nil
		if data, err := os.ReadFile(path); err == nil { //nolint:gosec // guidePath of a validated source ID
			_ = json.Unmarshal(data, &g)
		}
		l.guideMu.Lock()
		l.guides[sourceID], l.guideMod[sourceID] = g, mod
		l.guideMu.Unlock()
	}
	return g[channelID]
}

// storeGuide persists a source's guide. Caller holds mu.
func (l *Library) storeGuide(sourceID string, g playlist.Guide) error {
	data, err := json.Marshal(g)
	if err != nil {
		return err
	}
	path := l.guidePath(sourceID)
	if err := writeFileAtomic(path, data); err != nil {
		return err
	}
	l.guideMu.Lock()
	l.guides[sourceID], l.guideMod[sourceID] = g, modTime(path)
	l.guideMu.Unlock()
	return nil
}

// dropGuide forgets a removed source's guide. Caller holds mu.
func (l *Library) dropGuide(sourceID string) {
	l.guideMu.Lock()
	delete(l.guides, sourceID)
	delete(l.guideMod, sourceID)
	l.guideMu.Unlock()
	_ = os.Remove(l.guidePath(sourceID))
}

func (l *Library) guidePath(id string) string { return filepath.Join(l.dir, "epg", id+".json") }

// SetReminder adds (on) or removes the reminder for a channel's programme.
func (l *Library) SetReminder(r Reminder, on bool) ([]Reminder, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.syncLocked()
	l.reminders = slices.DeleteFunc(l.reminders, func(x Reminder) bool {
		return x.ChannelID == r.ChannelID && x.Start == r.Start
	})
	if on {
		if l.channel(r.ChannelID) == nil {
			return nil, errors.New("channel_not_found")
		}
		if r.Stop <= time.Now().Unix() {
			return nil, errors.New("programme_over")
		}
		if len(l.reminders) >= maxReminders {
			return nil, errors.New("too_many_reminders")
		}
		r.Title = strings.TrimSpace(r.Title)
		l.reminders = append(l.reminders, r)
		slices.SortFunc(l.reminders, func(a, b Reminder) int { return int(a.Start - b.Start) })
	}
	l.pruneReminders()
	return slices.Clone(l.reminders), l.saveState()
}

// Reminders lists upcoming and running reminders, soonest first.
func (l *Library) Reminders() []Reminder {
	l.sync()
	l.mu.RLock()
	defer l.mu.RUnlock()
	now := time.Now().Unix()
	out := make([]Reminder, 0, len(l.reminders))
	for _, r := range l.reminders {
		if r.Stop > now {
			out = append(out, r)
		}
	}
	return out
}

// pruneReminders drops reminders whose programme is over. Caller holds mu.
func (l *Library) pruneReminders() {
	now := time.Now().Unix()
	l.reminders = slices.DeleteFunc(l.reminders, func(r Reminder) bool { return r.Stop <= now })
}

// qualityWords are dropped when matching names: "La 1 HD" is "La 1" in the guide.
var qualityWords = map[string]bool{"hd": true, "fhd": true, "uhd": true, "sd": true, "4k": true, "hevc": true, "h265": true, "1080p": true, "720p": true}

// guideName is the key names are matched on: lower case, letters and digits
// only, quality tags dropped.
func guideName(s string) string {
	words := strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	words = slices.DeleteFunc(words, func(w string) bool { return qualityWords[w] })
	return strings.Join(words, "")
}
