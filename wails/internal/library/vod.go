package library

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/crstian19/herakleo/wails/internal/playlist"
)

// Episodes lists an Xtream series' episodes. They are asked from the panel
// every time (series grow) and kept in memory so PlayEpisode can find their
// stream URLs, which never reach the UI.
func (l *Library) Episodes(ctx context.Context, channelID string) ([]playlist.Episode, error) {
	l.sync()
	l.mu.RLock()
	c := l.channel(channelID)
	s := l.find(sourceOf(channelID))
	var seriesID string
	var x playlist.Xtream
	if c != nil && s != nil {
		seriesID = c.SeriesID
		x = playlist.Xtream{Server: s.Server, Username: s.Username, Password: s.Password}
	}
	l.mu.RUnlock()
	switch {
	case c == nil || s == nil:
		return nil, errors.New("channel_not_found")
	case seriesID == "":
		return nil, errors.New("not_a_series")
	}

	eps, err := x.SeriesInfo(ctx, seriesID)
	if err != nil {
		return nil, err
	}
	l.epMu.Lock()
	// Only the series opened last needs its URLs.
	l.episodes = map[string][]playlist.Episode{channelID: eps}
	l.epMu.Unlock()
	return eps, nil
}

// EpisodeURL resolves an episode listed by the last Episodes call to a
// player title and its stream.
func (l *Library) EpisodeURL(channelID, episodeID string) (name, streamURL string, err error) {
	l.epMu.Lock()
	eps := l.episodes[channelID]
	l.epMu.Unlock()
	i := slices.IndexFunc(eps, func(e playlist.Episode) bool { return e.ID == episodeID })
	if i < 0 {
		return "", "", errors.New("episode_not_found")
	}
	l.mu.RLock()
	series := ""
	if c := l.channel(channelID); c != nil {
		series = c.Name
	}
	l.mu.RUnlock()
	e := eps[i]
	return fmt.Sprintf("%s · S%02dE%02d %s", series, e.Season, e.Number, e.Title), e.URL, nil
}
