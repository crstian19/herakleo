package playlist

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func vodPanel(t *testing.T, seriesInfo string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("action") {
		case "get_vod_categories":
			_, _ = io.WriteString(w, `[{"category_id":"1","category_name":"Cine"}]`)
		case "get_vod_streams":
			_, _ = io.WriteString(w, `[{"name":" Film ","stream_id":5,"category_id":1,"stream_icon":"http://i/f.jpg","container_extension":"mkv"},`+
				`{"name":"Odd ext","stream_id":"6","container_extension":"../x"},{"name":"No id","stream_id":null}]`)
		case "get_series_categories":
			_, _ = io.WriteString(w, `[{"category_id":"2","category_name":"Drama"}]`)
		case "get_series":
			_, _ = io.WriteString(w, `[{"name":"Show","series_id":77,"category_id":"2","cover":"javascript:alert(1)"}]`)
		case "get_series_info":
			if r.URL.Query().Get("series_id") != "77" {
				http.Error(w, "no", http.StatusNotFound)
				return
			}
			_, _ = io.WriteString(w, seriesInfo)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestMovies(t *testing.T) {
	srv := vodPanel(t, "")
	x := Xtream{Server: srv.URL, Username: "u", Password: "p/w"}
	got, err := x.Movies(context.Background())
	if err != nil || len(got) != 2 {
		t.Fatalf("%+v, %v", got, err)
	}
	f := got[0]
	if f.Name != "Film" || f.Group != "Cine" || f.Media != MediaMovie || f.Logo != "http://i/f.jpg" ||
		f.URL != srv.URL+"/movie/u/p%2Fw/5.mkv" {
		t.Errorf("film %+v", f)
	}
	// A panel-supplied extension cannot add path segments.
	if got[1].URL != srv.URL+"/movie/u/p%2Fw/6.mp4" {
		t.Errorf("odd extension: %q", got[1].URL)
	}
}

func TestSeries(t *testing.T) {
	srv := vodPanel(t, `{"info":{},"episodes":{
		"2":[{"id":"201","episode_num":"1","title":"S2E1","container_extension":"mp4","info":[]}],
		"1":[{"id":102,"episode_num":2,"title":"S1E2","container_extension":"mkv","info":{"plot":"Second."}},
		     {"id":"101","episode_num":"1","title":" S1E1 ","container_extension":"mp4","info":{"plot":"First."}},
		     {"id":null,"title":"broken"}]}}`)
	x := Xtream{Server: srv.URL, Username: "u", Password: "p"}
	shows, err := x.Series(context.Background())
	if err != nil || len(shows) != 1 {
		t.Fatalf("%+v, %v", shows, err)
	}
	s := shows[0]
	if s.SeriesID != "77" || s.Media != MediaSeries || s.Group != "Drama" || s.Logo != "" || s.URL != "xtream-series:77" {
		t.Errorf("series %+v", s)
	}

	eps, err := x.SeriesInfo(context.Background(), "77")
	if err != nil {
		t.Fatal(err)
	}
	titles := make([]string, 0, len(eps))
	for _, e := range eps {
		titles = append(titles, e.Title)
	}
	if strings.Join(titles, ",") != "S1E1,S1E2,S2E1" {
		t.Errorf("order: %v", titles)
	}
	if eps[0].Plot != "First." || eps[0].Season != 1 || eps[0].Number != 1 || eps[0].URL != srv.URL+"/series/u/p/101.mp4" {
		t.Errorf("first episode %+v", eps[0])
	}
	if eps[2].Season != 2 || eps[2].Plot != "" {
		t.Errorf("season from the key, empty info: %+v", eps[2])
	}
}

func TestSeriesInfoListOfLists(t *testing.T) {
	srv := vodPanel(t, `{"episodes":[[{"id":"1","episode_num":1,"title":"A"}],[{"id":"2","episode_num":1,"title":"B"}]]}`)
	eps, err := (Xtream{Server: srv.URL, Username: "u", Password: "p"}).SeriesInfo(context.Background(), "77")
	if err != nil || len(eps) != 2 || eps[1].Season != 2 {
		t.Errorf("%+v, %v", eps, err)
	}
	if _, err := (Xtream{Server: srv.URL, Username: "u", Password: "p"}).SeriesInfo(context.Background(), "78"); err == nil {
		t.Error("unknown series: want an error")
	}
}

func TestSeriesInfoRejectsGarbage(t *testing.T) {
	srv := vodPanel(t, `{"episodes":"nope"}`)
	if _, err := (Xtream{Server: srv.URL, Username: "u", Password: "p"}).SeriesInfo(context.Background(), "77"); err == nil {
		t.Error("want an error")
	}
}

func TestMediaOf(t *testing.T) {
	cases := map[string]string{
		"http://p/live/u/p/1.ts":            MediaLive,
		"http://p/u/p/1":                    MediaLive,
		"http://p/stream.m3u8":              MediaLive,
		"http://p/movie/u/p/5.mkv":          MediaMovie,
		"http://p/films/Big.Buck.Bunny.MP4": MediaMovie,
		"http://p/series/u/p/101.mp4":       MediaSeries,
		"udp://239.0.0.1:1234":              MediaLive,
	}
	for in, want := range cases {
		if got := mediaOf(in); got != want {
			t.Errorf("mediaOf(%q) = %q, want %q", in, got, want)
		}
	}
	got, _ := ParseM3U(strings.NewReader("#EXTM3U\n#EXTINF:-1,Film\nhttp://p/movie/u/p/5.mkv\n#EXTINF:-1,TV\nhttp://p/live/1.ts\n"))
	if got[0].Media != MediaMovie || got[1].Media != MediaLive {
		t.Errorf("parsed %+v", got)
	}
}
