package library

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func fakeProvider(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/list.m3u", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, "#EXTM3U\n#EXTINF:-1 group-title=\"News\",BBC\nhttp://s/bbc\n#EXTINF:-1,CNN\nhttp://s/cnn\n")
	})
	mux.HandleFunc("/big.m3u", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, "#EXTM3U\n#EXTINF:-1 group-title=\"News\",BBC\nhttp://s/bbc\n"+
			"#EXTINF:-1 group-title=\"Sports\",Zeta\nhttp://s/zeta\n"+
			"#EXTINF:-1 group-title=\"News\",cnn\nhttp://s/cnn\n"+
			"#EXTINF:-1 group-title=\"Sports\",antena 3\nhttp://s/a3\n")
	})
	mux.HandleFunc("/player_api.php", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("password") != "secret" {
			_, _ = fmt.Fprint(w, `{"user_info":{"auth":0}}`)
			return
		}
		switch r.URL.Query().Get("action") {
		case "":
			_, _ = fmt.Fprint(w, `{"user_info":{"auth":1,"status":"Active","exp_date":"1893456000"}}`)
		case "get_live_categories":
			_, _ = fmt.Fprint(w, `[{"category_id":"7","category_name":"Sports"}]`)
		case "get_live_streams":
			_, _ = fmt.Fprint(w, `[{"name":"ESPN","stream_id":42,"category_id":"7"},{"name":"no id","stream_id":null}]`)
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestAddM3UAndXtream(t *testing.T) {
	srv := fakeProvider(t)
	dir := t.TempDir()
	l, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	m3u, err := l.Add(ctx, NewSource{Kind: KindM3UURL, Name: "list", URL: srv.URL + "/list.m3u"})
	if err != nil || m3u.ChannelCount != 2 {
		t.Fatalf("m3u: %+v, %v", m3u, err)
	}

	if _, err := l.Add(ctx, NewSource{Kind: KindXtream, Name: "x", Server: srv.URL, Username: "u", Password: "bad"}); err == nil {
		t.Fatal("bad credentials should fail")
	}
	x, err := l.Add(ctx, NewSource{Kind: KindXtream, Name: "x", Server: srv.URL + "/", Username: "u", Password: "secret"})
	if err != nil || x.ChannelCount != 1 {
		t.Fatalf("xtream: %+v, %v", x, err)
	}
	if x.ExpiresAt != 1893456000 || x.AccountStatus != "Active" {
		t.Fatalf("account info: %+v", x)
	}
	if strings.Contains(x.Location, "secret") {
		t.Fatalf("location leaks the password: %q", x.Location)
	}

	ch := l.Channels(Query{SourceID: x.ID}).Items[0]
	if ch.Group != "Sports" {
		t.Fatalf("group = %q", ch.Group)
	}
	_, stream, err := l.StreamURL(ch.ID)
	if err != nil || stream != srv.URL+"/live/u/secret/42.ts" {
		t.Fatalf("stream = %q, %v", stream, err)
	}

	// Reopening from disk gives the same library.
	again, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := again.Channels(Query{}).Total; got != 3 {
		t.Fatalf("reopened total = %d", got)
	}
	if err := again.Remove(m3u.ID); err != nil {
		t.Fatal(err)
	}
	if got := again.Channels(Query{}).Total; got != 1 {
		t.Fatalf("after remove total = %d", got)
	}
}

func TestUpdateRenameAndReload(t *testing.T) {
	srv := fakeProvider(t)
	l, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	x, err := l.Add(ctx, NewSource{Kind: KindXtream, Name: "x", Server: srv.URL, Username: "u", Password: "secret"})
	if err != nil {
		t.Fatal(err)
	}

	// Rename only: no download, credentials kept.
	got, err := l.Update(ctx, x.ID, NewSource{Name: "renamed"})
	if err != nil || got.Name != "renamed" || got.ChannelCount != 1 {
		t.Fatalf("rename: %+v, %v", got, err)
	}
	// Bad new password: rejected and nothing changes.
	if _, err := l.Update(ctx, x.ID, NewSource{Password: "bad"}); err == nil {
		t.Fatal("bad password should fail")
	}
	ch := l.Channels(Query{}).Items[0]
	if _, stream, _ := l.StreamURL(ch.ID); !strings.Contains(stream, "/secret/") {
		t.Fatalf("failed update must keep the old password, stream = %q", stream)
	}
}
