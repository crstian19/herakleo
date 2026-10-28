package library

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func xmltvTime(t time.Time) string { return t.UTC().Format("20060102150405 +0000") }

// guideServer serves an M3U whose header points at its own XMLTV guide, with
// programmes around now: one on air and one next per guide channel.
func guideServer(t *testing.T, header string) (*httptest.Server, time.Time) {
	t.Helper()
	now := time.Now().Truncate(time.Minute)
	mux := http.NewServeMux()
	var srv *httptest.Server
	mux.HandleFunc("/list.m3u", func(w http.ResponseWriter, _ *http.Request) {
		h := strings.ReplaceAll(header, "SELF", srv.URL)
		_, _ = fmt.Fprint(w, h+"\n"+
			"#EXTINF:-1 tvg-id=\"La1.es\",La 1 HD\nhttp://s/la1\n"+
			"#EXTINF:-1,Cuatro FHD\nhttp://s/cuatro\n"+
			"#EXTINF:-1 tvg-id=\"none\",Sin guía\nhttp://s/none\n")
	})
	guide := func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprintf(w, `<tv>
<channel id="la1.es"><display-name>La 1</display-name></channel>
<channel id="cuatro"><display-name>Cuatro</display-name></channel>
<programme start="%[1]s" stop="%[2]s" channel="la1.es"><title>Telediario</title></programme>
<programme start="%[2]s" stop="%[3]s" channel="la1.es"><title>Cine</title></programme>
<programme start="%[1]s" stop="%[2]s" channel="cuatro"><title>Noticias Cuatro</title></programme>
<programme start="%[4]s" stop="%[1]s" channel="cuatro"><title>Ayer</title></programme>
</tv>`, xmltvTime(now.Add(-30*time.Minute)), xmltvTime(now.Add(30*time.Minute)),
			xmltvTime(now.Add(90*time.Minute)), xmltvTime(now.Add(-48*time.Hour)))
	}
	mux.HandleFunc("/epg.xml", guide)
	mux.HandleFunc("/xmltv.php", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("password") != "secret" {
			http.Error(w, "no", http.StatusForbidden)
			return
		}
		guide(w, r)
	})
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, now
}

func addList(t *testing.T, l *Library, srv *httptest.Server) SourceInfo {
	t.Helper()
	info, err := l.Add(context.Background(), NewSource{Kind: KindM3UURL, Name: "TV", URL: srv.URL + "/list.m3u"})
	if err != nil {
		t.Fatal(err)
	}
	return info
}

func TestGuideFromListHeader(t *testing.T) {
	srv, now := guideServer(t, `#EXTM3U url-tvg="SELF/epg.xml"`)
	l := openTemp(t)
	info := addList(t, l, srv)
	if !info.HasGuide || info.GuideURL != "" {
		t.Fatalf("source info %+v", info)
	}

	info, err := l.RefreshGuide(context.Background(), info.ID)
	if err != nil {
		t.Fatal(err)
	}
	// La 1 by tvg-id (case-insensitive), Cuatro by name ("Cuatro FHD" = "Cuatro").
	if info.GuideChannels != 2 || info.GuideUpdatedAt < now.Unix()-60 {
		t.Errorf("after refresh: %+v", info)
	}

	la1, cuatro, none := ChannelID(info.ID, "http://s/la1"), ChannelID(info.ID, "http://s/cuatro"), ChannelID(info.ID, "http://s/none")
	nn := l.NowNext([]string{la1, cuatro, none, "bogus", "../../etc-x"})
	if got := nn[la1]; got.Now == nil || got.Now.Title != "Telediario" || got.Next == nil || got.Next.Title != "Cine" {
		t.Errorf("La 1: %+v", got)
	}
	if got := nn[cuatro]; got.Now == nil || got.Now.Title != "Noticias Cuatro" || got.Next != nil {
		t.Errorf("Cuatro: %+v", got)
	}
	if _, ok := nn[none]; ok || len(nn) != 2 {
		t.Errorf("channels without a guide must be left out: %v", nn)
	}

	if g := l.ChannelGuide(la1); len(g) != 2 || g[0].Title != "Telediario" {
		t.Errorf("ChannelGuide: %+v", g)
	}

	// A second Library on the same folder (the other app) reads the cached file.
	l2, err := Open(l.Dir())
	if err != nil {
		t.Fatal(err)
	}
	if g := l2.ChannelGuide(cuatro); len(g) != 1 {
		t.Errorf("reopened guide: %+v", g)
	}
}

func TestGuideFromHeaderStaysOffTheLAN(t *testing.T) {
	other, _ := guideServer(t, "#EXTM3U")
	// The list's header names a guide on another private host.
	srv, _ := guideServer(t, `#EXTM3U url-tvg="`+other.URL+`/epg.xml"`)
	l := openTemp(t)
	info := addList(t, l, srv)
	if _, err := l.RefreshGuide(context.Background(), info.ID); err == nil ||
		err.Error() != "download_failed: private_address_blocked" {
		t.Errorf("got %v", err)
	}
	// The same URL typed by the user is trusted.
	if _, err := l.SetGuideURL(info.ID, other.URL+"/epg.xml"); err != nil {
		t.Fatal(err)
	}
	if info, err := l.RefreshGuide(context.Background(), info.ID); err != nil || info.GuideChannels != 2 {
		t.Errorf("user guide URL: %+v, %v", info, err)
	}
}

func TestSetGuideURL(t *testing.T) {
	srv, _ := guideServer(t, "#EXTM3U")
	l := openTemp(t)
	info := addList(t, l, srv)
	if info.HasGuide {
		t.Errorf("no header, no guide: %+v", info)
	}
	if _, err := l.RefreshGuide(context.Background(), info.ID); err == nil || err.Error() != "guide_not_set" {
		t.Errorf("got %v", err)
	}
	for _, bad := range []string{"file:///etc/passwd", "ftp://x/epg", "not a url"} {
		if _, err := l.SetGuideURL(info.ID, bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
	got, err := l.SetGuideURL(info.ID, " "+srv.URL+"/epg.xml?token=abc ")
	if err != nil || !got.HasGuide || got.GuideURL != srv.URL+"/epg.xml" {
		t.Errorf("set: %+v, %v (the query must not reach the UI)", got, err)
	}
	if got, _ := l.SetGuideURL(info.ID, ""); got.HasGuide {
		t.Errorf("cleared: %+v", got)
	}
	if _, err := l.SetGuideURL("aaaaaaaaaaaa", srv.URL); err == nil {
		t.Error("unknown source accepted")
	}
}

func TestXtreamGuide(t *testing.T) {
	srv, _ := guideServer(t, "#EXTM3U")
	mux := http.NewServeMux()
	mux.HandleFunc("/player_api.php", func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("action") {
		case "get_live_categories":
			_, _ = fmt.Fprint(w, `[]`)
		case "get_live_streams":
			_, _ = fmt.Fprint(w, `[{"name":"La 1","stream_id":1,"epg_channel_id":"la1.es"}]`)
		default:
			_, _ = fmt.Fprint(w, `{"user_info":{}}`)
		}
	})
	mux.Handle("/xmltv.php", http.RedirectHandler(srv.URL+"/xmltv.php?password=secret", http.StatusFound))
	panel := httptest.NewServer(mux)
	t.Cleanup(panel.Close)

	l := openTemp(t)
	info, err := l.Add(context.Background(), NewSource{Kind: KindXtream, Name: "P", Server: panel.URL, Username: "u", Password: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	if !info.HasGuide {
		t.Errorf("Xtream sources always have a guide: %+v", info)
	}
	if info, err = l.RefreshGuide(context.Background(), info.ID); err != nil || info.GuideChannels != 1 {
		t.Errorf("%+v, %v", info, err)
	}
}

func TestReminders(t *testing.T) {
	srv, now := guideServer(t, `#EXTM3U url-tvg="SELF/epg.xml"`)
	l := openTemp(t)
	info := addList(t, l, srv)
	la1 := ChannelID(info.ID, "http://s/la1")
	r := Reminder{ChannelID: la1, Channel: "La 1", Title: " Cine ", Start: now.Add(30 * time.Minute).Unix(), Stop: now.Add(90 * time.Minute).Unix()}

	list, err := l.SetReminder(r, true)
	if err != nil || len(list) != 1 || list[0].Title != "Cine" {
		t.Fatalf("%+v, %v", list, err)
	}
	// Setting it twice keeps one.
	if list, _ = l.SetReminder(r, true); len(list) != 1 {
		t.Errorf("duplicate: %+v", list)
	}
	if got := l.Reminders(); len(got) != 1 {
		t.Errorf("Reminders: %+v", got)
	}

	over := r
	over.Start, over.Stop = now.Add(-2*time.Hour).Unix(), now.Add(-time.Hour).Unix()
	if _, err := l.SetReminder(over, true); err == nil || err.Error() != "programme_over" {
		t.Errorf("past programme: %v", err)
	}
	ghost := r
	ghost.ChannelID = info.ID + "-0000000000000000"
	if _, err := l.SetReminder(ghost, true); err == nil || err.Error() != "channel_not_found" {
		t.Errorf("unknown channel: %v", err)
	}

	if list, _ = l.SetReminder(r, false); len(list) != 0 {
		t.Errorf("removed: %+v", list)
	}
	_, _ = l.SetReminder(r, true)

	// Removing the source drops its reminders and its guide file.
	if _, err := l.RefreshGuide(context.Background(), info.ID); err != nil {
		t.Fatal(err)
	}
	if err := l.Remove(info.ID); err != nil {
		t.Fatal(err)
	}
	if got := l.Reminders(); len(got) != 0 {
		t.Errorf("after remove: %+v", got)
	}
	if _, err := os.Stat(l.guidePath(info.ID)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("guide file left behind: %v", err)
	}
}

func TestGuideName(t *testing.T) {
	cases := map[string]string{
		"La 1 HD":         "la1",
		"LA-1":            "la1",
		"Cuatro FHD":      "cuatro",
		"Antena 3 (4K)":   "antena3",
		"Telecinco 1080p": "telecinco",
		"HD":              "",
		"Ñ TV":            "ñtv",
	}
	for in, want := range cases {
		if got := guideName(in); got != want {
			t.Errorf("guideName(%q) = %q, want %q", in, got, want)
		}
	}
}

func openTemp(t *testing.T) *Library {
	t.Helper()
	l, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return l
}
