package library

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestUpdateServerNeedsPassword(t *testing.T) {
	srv := fakeProvider(t)
	var hits atomic.Int32
	other := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hits.Add(1) }))
	t.Cleanup(other.Close)

	l, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	x, err := l.Add(ctx, NewSource{Kind: KindXtream, Name: "x", Server: srv.URL, Username: "u", Password: "secret"})
	if err != nil {
		t.Fatal(err)
	}

	// The stored password must not be replayed against another host.
	for _, n := range []NewSource{{Server: other.URL}, {Server: other.URL, Username: "u"}} {
		if _, err := l.Update(ctx, x.ID, n); err == nil || err.Error() != "credentials_required" {
			t.Fatalf("update %+v: err = %v", n, err)
		}
	}
	if hits.Load() != 0 {
		t.Fatalf("new server got %d requests", hits.Load())
	}
	if got := l.Sources()[0].Location; got != srv.URL {
		t.Fatalf("rejected update changed the server to %q", got)
	}

	// Same host written differently is not a change.
	if _, err := l.Update(ctx, x.ID, NewSource{Server: " " + strings.ToUpper(srv.URL) + "/ "}); err != nil {
		t.Fatalf("same server: %v", err)
	}

	// With the password in the same request the move is allowed (here back to
	// the original host under another spelling, which the fake accepts).
	moved := strings.Replace(srv.URL, "127.0.0.1", "localhost", 1)
	if _, err := l.Update(ctx, x.ID, NewSource{Server: moved, Password: "secret"}); err != nil {
		t.Fatalf("server change with password: %v", err)
	}
	if got := l.Sources()[0].Location; got != moved {
		t.Fatalf("location = %q, want %q", got, moved)
	}
}

func TestSameServer(t *testing.T) {
	for _, c := range []struct {
		a, b string
		same bool
	}{
		{"http://h:8080", "http://h:8080/", true},
		{"http://H:8080", " http://h:8080 ", true},
		{"http://h:8080", "http://h:8080/sub", true},
		{"http://h:8080", "https://h:8080", false},
		{"http://h:8080", "http://h:8081", false},
		{"http://h", "http://evil", false},
		{"not a url", "not a url", true},
		{"not a url", "http://h", false},
	} {
		if got := sameServer(c.a, c.b); got != c.same {
			t.Errorf("sameServer(%q, %q) = %v", c.a, c.b, got)
		}
	}
}

func writeM3U(t *testing.T, dir, name string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte("#EXTM3U\n#EXTINF:-1,BBC\nhttp://s/bbc\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestFileSourceMustBePicked(t *testing.T) {
	l, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	files := t.TempDir()
	first, second, other := writeM3U(t, files, "a.m3u"), writeM3U(t, files, "b.m3u"), writeM3U(t, files, "c.m3u")

	if _, err := l.Add(ctx, NewSource{Kind: KindM3UFile, Name: "a", Path: first}); err == nil || err.Error() != "file_not_picked" {
		t.Fatalf("unpicked add: %v", err)
	}
	if len(l.Sources()) != 0 {
		t.Fatal("rejected add stored a source")
	}

	l.AllowFile(first)
	s, err := l.Add(ctx, NewSource{Kind: KindM3UFile, Name: "a", Path: first})
	if err != nil || s.ChannelCount != 1 {
		t.Fatalf("picked add: %+v, %v", s, err)
	}

	// Editing without re-picking: empty path or the stored one both keep it.
	for _, p := range []string{"", first} {
		if got, err := l.Update(ctx, s.ID, NewSource{Name: "renamed", Path: p}); err != nil || got.Location != first {
			t.Fatalf("update with path %q: %+v, %v", p, got, err)
		}
	}

	if _, err := l.Update(ctx, s.ID, NewSource{Path: other}); err == nil || err.Error() != "file_not_picked" {
		t.Fatalf("unpicked update: %v", err)
	}
	if got := l.Sources()[0].Location; got != first {
		t.Fatalf("rejected update changed the path to %q", got)
	}

	l.AllowFile(second)
	if got, err := l.Update(ctx, s.ID, NewSource{Path: second}); err != nil || got.Location != second {
		t.Fatalf("picked update: %+v, %v", got, err)
	}
}
