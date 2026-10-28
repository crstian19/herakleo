package library

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// vodPanel is an Xtream panel with one live channel, one film and one series.
func vodPanel(t *testing.T, withVOD bool) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		action := r.URL.Query().Get("action")
		if !withVOD && strings.Contains(action, "vod") || !withVOD && strings.Contains(action, "series") {
			http.Error(w, "no", http.StatusNotFound)
			return
		}
		switch action {
		case "":
			_, _ = fmt.Fprint(w, `{"user_info":{}}`)
		case "get_live_categories", "get_vod_categories", "get_series_categories":
			_, _ = fmt.Fprint(w, `[{"category_id":"1","category_name":"Cine"}]`)
		case "get_live_streams":
			_, _ = fmt.Fprint(w, `[{"name":"La 1","stream_id":1,"category_id":"1"}]`)
		case "get_vod_streams":
			_, _ = fmt.Fprint(w, `[{"name":"Film","stream_id":5,"category_id":"1","container_extension":"mkv"}]`)
		case "get_series":
			_, _ = fmt.Fprint(w, `[{"name":"Show","series_id":77,"category_id":"1"}]`)
		case "get_series_info":
			_, _ = fmt.Fprint(w, `{"episodes":{"1":[{"id":"101","episode_num":1,"title":"Pilot","container_extension":"mp4"}]}}`)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func addPanel(t *testing.T, l *Library, srv *httptest.Server) SourceInfo {
	t.Helper()
	info, err := l.Add(context.Background(), NewSource{Kind: KindXtream, Name: "P", Server: srv.URL, Username: "u", Password: "p"})
	if err != nil {
		t.Fatal(err)
	}
	return info
}

func TestVODFromXtream(t *testing.T) {
	srv := vodPanel(t, true)
	l := openTemp(t)
	info := addPanel(t, l, srv)
	if info.ChannelCount != 3 || info.MovieCount != 1 || info.SeriesCount != 1 {
		t.Fatalf("counts %+v", info)
	}

	names := func(media string) string {
		items := l.Channels(Query{Media: media}).Items
		out := make([]string, 0, len(items))
		for _, c := range items {
			out = append(out, c.Name+"/"+c.Media)
		}
		return strings.Join(out, ",")
	}
	if got := names(MediaLive); got != "La 1/live" {
		t.Errorf("live: %s", got)
	}
	if got := names(MediaMovie); got != "Film/movie" {
		t.Errorf("movies: %s", got)
	}
	if got := names(MediaSeries); got != "Show/series" {
		t.Errorf("series: %s", got)
	}
	if got := names(MediaAll); got != "La 1/live,Film/movie,Show/series" {
		t.Errorf("all: %s", got)
	}
	if g := l.Groups("", MediaMovie); len(g) != 1 || g[0].Count != 1 {
		t.Errorf("movie groups %+v", g)
	}
	if g := l.Groups("", ""); len(g) != 1 || g[0].Count != 3 {
		t.Errorf("all groups %+v", g)
	}

	show := l.Channels(Query{Media: MediaSeries}).Items[0]
	if !show.Container {
		t.Errorf("a series is a container: %+v", show)
	}
	if _, _, err := l.StreamURL(show.ID); err == nil || err.Error() != "not_playable" {
		t.Errorf("playing a series: %v", err)
	}
	film := l.Channels(Query{Media: MediaMovie}).Items[0]
	if _, u, err := l.StreamURL(film.ID); err != nil || u != srv.URL+"/movie/u/p/5.mkv" {
		t.Errorf("film URL %q, %v", u, err)
	}

	if _, _, err := l.EpisodeURL(show.ID, "101"); err == nil {
		t.Error("episodes must be listed before they are played")
	}
	eps, err := l.Episodes(context.Background(), show.ID)
	if err != nil || len(eps) != 1 || eps[0].Title != "Pilot" {
		t.Fatalf("%+v, %v", eps, err)
	}
	name, u, err := l.EpisodeURL(show.ID, "101")
	if err != nil || name != "Show · S01E01 Pilot" || u != srv.URL+"/series/u/p/101.mp4" {
		t.Errorf("episode %q %q %v", name, u, err)
	}
	if _, _, err := l.EpisodeURL(show.ID, "999"); err == nil {
		t.Error("unknown episode accepted")
	}
	if _, err := l.Episodes(context.Background(), film.ID); err == nil || err.Error() != "not_a_series" {
		t.Errorf("episodes of a film: %v", err)
	}
	if _, err := l.Episodes(context.Background(), info.ID+"-0000000000000000"); err == nil {
		t.Error("unknown channel accepted")
	}

	// History and favourites keep films and series.
	if err := l.AddHistory(film.ID); err != nil {
		t.Fatal(err)
	}
	if p := l.Channels(Query{View: ViewHistory, Media: MediaLive}); p.Total != 1 {
		t.Errorf("history ignores the media filter: %+v", p)
	}
}

func TestPanelWithoutVODStillLoads(t *testing.T) {
	l := openTemp(t)
	info := addPanel(t, l, vodPanel(t, false))
	if info.ChannelCount != 1 || info.MovieCount != 0 || info.SeriesCount != 0 {
		t.Errorf("%+v", info)
	}
}
