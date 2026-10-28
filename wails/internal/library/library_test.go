package library

import (
	"testing"

	"github.com/crstian19/herakleo/wails/internal/playlist"
)

// Pinned vectors: changing the ID scheme would orphan every saved favourite.
func TestChannelIDVectors(t *testing.T) {
	cases := map[string]string{
		"":                                 "abc-cbf29ce484222325",
		"http://example.com/live/u/p/1.ts": "abc-7a8bab84a10cac93",
	}
	for url, want := range cases {
		if got := ChannelID("abc", url); got != want {
			t.Errorf("ChannelID(%q) = %q, want %q", url, got, want)
		}
	}
}

func TestQueryPaginationAndFavorites(t *testing.T) {
	l, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	s := &source{ID: "s1", Name: "test", Kind: KindM3UFile}
	l.sources = append(l.sources, s)
	if err := l.storeChannels(s, loadResult{entries: entries("News", "BBC", "CNN", "BBC")}); err != nil {
		t.Fatal(err)
	}

	if got := l.Channels(Query{Search: "bbc"}); got.Total != 1 {
		t.Fatalf("duplicate URL should be dropped, total = %d", got.Total)
	}
	page := l.Channels(Query{Offset: 1, Limit: 1})
	if page.Total != 2 || len(page.Items) != 1 || page.Items[0].Name != "CNN" {
		t.Fatalf("unexpected page %+v", page)
	}
	if err := l.SetFavorite(page.Items[0].ID, true); err != nil {
		t.Fatal(err)
	}
	if fav := l.Channels(Query{View: ViewFavorites}); fav.Total != 1 || !fav.Items[0].Favorite {
		t.Fatalf("unexpected favourites %+v", fav)
	}
}

func entries(group string, names ...string) []playlist.Entry {
	out := make([]playlist.Entry, 0, len(names))
	for _, n := range names {
		out = append(out, playlist.Entry{Name: n, Group: group, URL: "http://x/" + n})
	}
	return out
}
