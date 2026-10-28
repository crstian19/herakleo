package playlist

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"slices"
	"strconv"
	"strings"
)

// maxSeriesInfoBytes caps one series' episode list; long-running shows with
// plots for every episode stay well under a few MB.
const maxSeriesInfoBytes = 16 << 20

type xtreamVOD struct {
	Name       string          `json:"name"`
	StreamID   json.RawMessage `json:"stream_id"`
	Icon       string          `json:"stream_icon"`
	CategoryID json.RawMessage `json:"category_id"`
	Extension  string          `json:"container_extension"`
}

type xtreamSeries struct {
	Name       string          `json:"name"`
	SeriesID   json.RawMessage `json:"series_id"`
	Cover      string          `json:"cover"`
	CategoryID json.RawMessage `json:"category_id"`
}

// Movies lists the account's films.
func (x Xtream) Movies(ctx context.Context) ([]Entry, error) {
	base, err := x.base()
	if err != nil {
		return nil, err
	}
	groups, err := x.categories(ctx, base, "get_vod_categories")
	if err != nil {
		return nil, err
	}
	var entries []Entry
	err = xtreamList(ctx, x, base, "get_vod_streams", func(v xtreamVOD) {
		if id := rawID(v.StreamID); id != "" {
			entries = append(entries, Entry{
				Name:  strings.TrimSpace(v.Name),
				Group: groups[rawID(v.CategoryID)],
				Logo:  logo(v.Icon),
				URL:   x.mediaURL(base, "movie", id, v.Extension),
				Media: MediaMovie,
			})
		}
	})
	return entries, err
}

// Series lists the account's shows. They are not playable: SeriesInfo lists
// their episodes when the user opens one.
func (x Xtream) Series(ctx context.Context) ([]Entry, error) {
	base, err := x.base()
	if err != nil {
		return nil, err
	}
	groups, err := x.categories(ctx, base, "get_series_categories")
	if err != nil {
		return nil, err
	}
	var entries []Entry
	err = xtreamList(ctx, x, base, "get_series", func(s xtreamSeries) {
		if id := rawID(s.SeriesID); id != "" {
			entries = append(entries, Entry{
				Name:     strings.TrimSpace(s.Name),
				Group:    groups[rawID(s.CategoryID)],
				Logo:     logo(s.Cover),
				URL:      "xtream-series:" + id,
				Media:    MediaSeries,
				SeriesID: id,
			})
		}
	})
	return entries, err
}

// Episode is one playable episode of a series.
type Episode struct {
	ID     string `json:"id"`
	Season int    `json:"season"`
	Number int    `json:"number"`
	Title  string `json:"title"`
	Plot   string `json:"plot,omitempty"`
	URL    string `json:"-"`
}

type xtreamEpisode struct {
	ID        json.RawMessage `json:"id"`
	Number    json.RawMessage `json:"episode_num"`
	Season    json.RawMessage `json:"season"`
	Title     string          `json:"title"`
	Extension string          `json:"container_extension"`
	Info      json.RawMessage `json:"info"`
}

// SeriesInfo lists a series' episodes by season, then episode number.
func (x Xtream) SeriesInfo(ctx context.Context, seriesID string) ([]Episode, error) {
	base, err := x.base()
	if err != nil {
		return nil, err
	}
	q := url.Values{"username": {x.Username}, "password": {x.Password}, "action": {"get_series_info"}, "series_id": {seriesID}}
	b, err := open(ctx, base+"/player_api.php?"+q.Encode(), maxSeriesInfoBytes)
	if err != nil {
		return nil, err
	}
	defer func() { _ = b.Close() }()
	var resp struct {
		Episodes json.RawMessage `json:"episodes"`
	}
	if err := json.NewDecoder(b).Decode(&resp); err != nil {
		return nil, invalidResponse(b)
	}

	// Panels send {"1": [...], "2": [...]} by season, and some a plain list of lists.
	var seasons map[string][]xtreamEpisode
	if err := json.Unmarshal(resp.Episodes, &seasons); err != nil {
		var lists [][]xtreamEpisode
		if json.Unmarshal(resp.Episodes, &lists) != nil {
			return nil, errors.New("xtream_invalid_response")
		}
		seasons = map[string][]xtreamEpisode{}
		for i, l := range lists {
			seasons[strconv.Itoa(i+1)] = l
		}
	}

	var out []Episode
	for key, eps := range seasons {
		keySeason, _ := strconv.Atoi(strings.TrimSpace(key))
		for _, e := range eps {
			id := rawID(e.ID)
			if id == "" || len(out) >= MaxEntries {
				continue
			}
			season, err := strconv.Atoi(rawID(e.Season))
			if err != nil || season <= 0 {
				season = keySeason
			}
			num, _ := strconv.Atoi(rawID(e.Number))
			out = append(out, Episode{
				ID: id, Season: season, Number: num,
				Title: truncate(strings.TrimSpace(e.Title), maxTitle),
				Plot:  truncate(strings.TrimSpace(plot(e.Info)), maxDesc),
				URL:   x.mediaURL(base, "series", id, e.Extension),
			})
		}
	}
	slices.SortFunc(out, func(a, b Episode) int {
		if a.Season != b.Season {
			return a.Season - b.Season
		}
		return a.Number - b.Number
	})
	return out, nil
}

// plot reads info.plot; panels send info as an object, or as [] when empty.
func plot(info json.RawMessage) string {
	var v struct {
		Plot string `json:"plot"`
	}
	if len(info) > 0 && info[0] == '{' && json.Unmarshal(info, &v) == nil {
		return v.Plot
	}
	return ""
}

func (x Xtream) categories(ctx context.Context, base, action string) (map[string]string, error) {
	groups := map[string]string{}
	err := xtreamList(ctx, x, base, action, func(c xtreamCategory) {
		groups[rawID(c.ID)] = c.Name
	})
	return groups, err
}

// mediaURL builds /movie/… or /series/… stream URLs. The extension comes from
// the panel, so only plain alphanumeric ones are kept.
func (x Xtream) mediaURL(base, kind, id, ext string) string {
	ext = strings.TrimSpace(ext)
	if ext == "" || len(ext) > 5 || bytes.ContainsFunc([]byte(ext), func(r rune) bool {
		return (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9')
	}) {
		ext = "mp4"
	}
	user, pass := url.PathEscape(x.Username), url.PathEscape(x.Password)
	return base + "/" + kind + "/" + user + "/" + pass + "/" + url.PathEscape(id) + "." + ext
}
