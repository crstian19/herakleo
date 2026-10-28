package library

import (
	"cmp"
	"slices"
	"strings"
)

// Channel is a channel as the UI sees it (no stream URL).
type Channel struct {
	ID       string `json:"id"`
	SourceID string `json:"sourceId"`
	Name     string `json:"name"`
	Group    string `json:"group"`
	Logo     string `json:"logo"`
	Favorite bool   `json:"favorite"`
	Hidden   bool   `json:"hidden"`
}

// Group is a channel group with its size.
type Group struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// Views and sorts the UI can ask for; unknown values behave like the default.
const (
	ViewAll       = ""
	ViewFavorites = "favorites"
	ViewHistory   = "history" // newest first; ignores Sort
	ViewHidden    = "hidden"  // hidden channels only, to unhide them

	SortProvider = "" // playlist order
	SortAZ       = "az"
	SortZA       = "za"
)

// Query filters channels; empty fields mean "any".
type Query struct {
	SourceID string `json:"sourceId"`
	Group    string `json:"group"`
	Search   string `json:"search"`
	View     string `json:"view"`
	Sort     string `json:"sort"`
	Offset   int    `json:"offset"`
	Limit    int    `json:"limit"`
}

// Page is one slice of a query result plus the total match count.
type Page struct {
	Items []Channel `json:"items"`
	Total int       `json:"total"`
}

const maxLimit = 500

// Channels runs a query. It is a linear scan: even 100k channels filter in a
// few milliseconds, which beats keeping an index in sync. Disabled sources
// are always skipped; hidden channels and groups only show in ViewHidden.
func (l *Library) Channels(q Query) Page {
	l.sync()
	l.mu.RLock()
	defer l.mu.RUnlock()

	limit := q.Limit
	if limit <= 0 || limit > maxLimit {
		limit = maxLimit
	}
	offset := max(q.Offset, 0)
	needle := strings.ToLower(strings.TrimSpace(q.Search))

	match := func(s *source, c *channel) bool {
		if s.Disabled || (q.SourceID != "" && s.ID != q.SourceID) {
			return false
		}
		hidden := l.hiddenChannels[c.ID] || l.hiddenGroups[c.Group]
		switch q.View {
		case ViewHidden:
			if !l.hiddenChannels[c.ID] {
				return false
			}
		case ViewFavorites:
			if hidden || !l.favorites[c.ID] {
				return false
			}
		default:
			if hidden {
				return false
			}
		}
		if q.Group != "" && c.Group != q.Group {
			return false
		}
		return needle == "" || strings.Contains(c.lower, needle)
	}
	view := func(s *source, c *channel) Channel {
		return Channel{
			ID: c.ID, SourceID: s.ID, Name: c.Name, Group: c.Group, Logo: c.Logo,
			Favorite: l.favorites[c.ID], Hidden: l.hiddenChannels[c.ID],
		}
	}

	var matches []Channel
	if q.View == ViewHistory {
		for _, h := range l.history {
			c := l.channel(h.ID)
			s := l.find(sourceOf(h.ID))
			if c != nil && s != nil && match(s, c) {
				matches = append(matches, view(s, c))
			}
		}
	} else {
		// Provider order walks the playlists; A→Z/Z→A walk the cached sorted
		// order. Either way nothing is sorted per query and only the page's
		// channels are copied out.
		page := Page{Items: []Channel{}}
		add := func(s *source, c *channel) {
			if !match(s, c) {
				return
			}
			if page.Total >= offset && len(page.Items) < limit {
				page.Items = append(page.Items, view(s, c))
			}
			page.Total++
		}
		if q.Sort == SortAZ || q.Sort == SortZA {
			refs := l.sortedRefs()
			if q.Sort == SortAZ {
				for _, r := range refs {
					add(r.src, r.ch)
				}
			} else {
				for i := len(refs) - 1; i >= 0; i-- {
					add(refs[i].src, refs[i].ch)
				}
			}
		} else {
			for _, s := range l.sources {
				for i := range l.channels[s.ID] {
					add(s, &l.channels[s.ID][i])
				}
			}
		}
		return page
	}

	page := Page{Items: []Channel{}, Total: len(matches)}
	if offset < len(matches) {
		page.Items = matches[offset:min(offset+limit, len(matches))]
	}
	return page
}

type chanRef struct {
	src *source
	ch  *channel
}

// sortCache is every channel in A→Z order, rebuilt only when chanGen moves.
// Sorting 65k names took ~140 ms and used to run on every page of the
// infinite scroll; now it runs once per change of the channel lists.
type sortCache struct {
	gen  uint64
	ok   bool
	refs []chanRef
}

// sortedRefs returns the cached A→Z order. Caller holds mu (read is enough):
// the pointers stay valid until l.channels changes, which bumps chanGen.
func (l *Library) sortedRefs() []chanRef {
	l.sortMu.Lock()
	defer l.sortMu.Unlock()
	if l.sorted.ok && l.sorted.gen == l.chanGen {
		return l.sorted.refs
	}
	var refs []chanRef
	for _, s := range l.sources {
		for i := range l.channels[s.ID] {
			refs = append(refs, chanRef{s, &l.channels[s.ID][i]})
		}
	}
	// Case-insensitive, ties by exact name then ID: a total order, so pages
	// never shuffle. Z→A is this order reversed.
	slices.SortFunc(refs, func(a, b chanRef) int {
		return cmp.Or(
			strings.Compare(a.ch.lower, b.ch.lower),
			strings.Compare(a.ch.Name, b.ch.Name),
			strings.Compare(a.ch.ID, b.ch.ID),
		)
	})
	l.sorted = sortCache{gen: l.chanGen, ok: true, refs: refs}
	return refs
}

func sourceOf(channelID string) string {
	id, _, _ := strings.Cut(channelID, "-")
	return id
}

// Groups lists visible groups in first-seen order, for one source or all of
// them, counting only visible channels.
func (l *Library) Groups(sourceID string) []Group {
	l.sync()
	l.mu.RLock()
	defer l.mu.RUnlock()

	index := map[string]int{}
	groups := []Group{}
	for _, s := range l.sources {
		if s.Disabled || (sourceID != "" && s.ID != sourceID) {
			continue
		}
		for _, c := range l.channels[s.ID] {
			if l.hiddenGroups[c.Group] || l.hiddenChannels[c.ID] {
				continue
			}
			i, ok := index[c.Group]
			if !ok {
				i = len(groups)
				index[c.Group] = i
				groups = append(groups, Group{Name: c.Group})
			}
			groups[i].Count++
		}
	}
	return groups
}
