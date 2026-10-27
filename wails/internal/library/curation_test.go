package library

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// newTestLibrary opens a library with one M3U source of four channels in two groups.
func newTestLibrary(t *testing.T, dir string) (*Library, SourceInfo) {
	t.Helper()
	srv := fakeProvider(t)
	l, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	s, err := l.Add(context.Background(), NewSource{Kind: KindM3UURL, Name: "List", URL: srv.URL + "/big.m3u"})
	if err != nil {
		t.Fatal(err)
	}
	return l, s
}

func names(p Page) []string {
	out := make([]string, 0, len(p.Items))
	for _, c := range p.Items {
		out = append(out, c.Name)
	}
	return out
}

func TestSortHideAndGroups(t *testing.T) {
	l, _ := newTestLibrary(t, t.TempDir())

	if got := names(l.Channels(Query{Sort: SortAZ})); !equal(got, "antena 3", "BBC", "cnn", "Zeta") {
		t.Fatalf("az = %v", got)
	}
	if got := names(l.Channels(Query{Sort: SortZA, Limit: 2, Offset: 1})); !equal(got, "cnn", "BBC") {
		t.Fatalf("za page = %v", got)
	}

	all := l.Channels(Query{})
	bbc := all.Items[0]
	if err := l.SetChannelHidden(bbc.ID, true); err != nil {
		t.Fatal(err)
	}
	if err := l.SetGroupHidden("Sports", true); err != nil {
		t.Fatal(err)
	}
	if got := names(l.Channels(Query{})); !equal(got, "cnn") {
		t.Fatalf("visible = %v", got)
	}
	if got := names(l.Channels(Query{View: ViewHidden})); !equal(got, "BBC") {
		t.Fatalf("hidden view = %v", got)
	}
	if g := l.Groups(""); len(g) != 1 || g[0].Name != "News" || g[0].Count != 1 {
		t.Fatalf("groups = %+v", g)
	}
	if hg := l.HiddenGroups(); !equal(hg, "Sports") {
		t.Fatalf("hidden groups = %v", hg)
	}
}

func TestHistoryAndDisable(t *testing.T) {
	l, s := newTestLibrary(t, t.TempDir())
	all := l.Channels(Query{}).Items
	for _, c := range []Channel{all[0], all[1], all[0]} {
		if err := l.AddHistory(c.ID); err != nil {
			t.Fatal(err)
		}
	}
	if got := names(l.Channels(Query{View: ViewHistory})); !equal(got, all[0].Name, all[1].Name) {
		t.Fatalf("history = %v", got)
	}

	if _, err := l.SetEnabled(s.ID, false); err != nil {
		t.Fatal(err)
	}
	if p := l.Channels(Query{}); p.Total != 0 || len(l.Groups("")) != 0 {
		t.Fatalf("disabled source still listed: %+v", p)
	}
	if err := l.ClearHistory(); err != nil {
		t.Fatal(err)
	}
	if _, err := l.SetEnabled(s.ID, true); err != nil {
		t.Fatal(err)
	}
	if p := l.Channels(Query{View: ViewHistory}); p.Total != 0 {
		t.Fatalf("history not cleared: %+v", p)
	}
}

// Two Library values on one dir stand in for the Tauri and Wails apps.
func TestTwoAppsDoNotOverwriteEachOther(t *testing.T) {
	dir := t.TempDir()
	a, s := newTestLibrary(t, dir)
	b, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(10 * time.Millisecond) // distinct mtimes on coarse filesystems

	srv := fakeProvider(t)
	if _, err := b.Add(context.Background(), NewSource{Kind: KindM3UURL, Name: "Otra", URL: srv.URL + "/list.m3u"}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(10 * time.Millisecond)
	// a saves after b: without the mtime check this dropped b's list.
	if err := a.SetFavorite(a.Channels(Query{SourceID: s.ID}).Items[0].ID, true); err != nil {
		t.Fatal(err)
	}
	if n := len(b.Sources()); n != 2 {
		t.Fatalf("b sees %d sources", n)
	}
	if p := b.Channels(Query{View: ViewFavorites}); p.Total != 1 {
		t.Fatalf("b missed a's favourite: %+v", p)
	}
}

func TestFavoritesBackupSurvivesReAdd(t *testing.T) {
	dir := t.TempDir()
	l, s := newTestLibrary(t, dir)
	if err := l.SetFavorite(l.Channels(Query{Sort: SortAZ}).Items[0].ID, true); err != nil {
		t.Fatal(err)
	}
	backup := l.ExportFavorites()
	if len(backup.Favorites) != 1 || backup.Favorites[0].Channel != "antena 3" {
		t.Fatalf("backup = %+v", backup)
	}

	// Delete and re-add the list: new source ID, so favourites are gone...
	srv := fakeProvider(t)
	if err := l.Remove(s.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := l.Add(context.Background(), NewSource{Kind: KindM3UURL, Name: "LIST", URL: srv.URL + "/big.m3u"}); err != nil {
		t.Fatal(err)
	}
	// ...until the backup brings them back, matched by names.
	n, err := l.ImportFavorites(backup)
	if err != nil || n != 1 {
		t.Fatalf("import = %d, %v", n, err)
	}
	if _, err := l.ImportFavorites(FavoritesBackup{Format: "other"}); err == nil {
		t.Fatal("foreign file accepted")
	}
}

func TestOrphanChannelFilesAreRemoved(t *testing.T) {
	dir := t.TempDir()
	newTestLibrary(t, dir)
	orphan := filepath.Join(dir, "channels", "0123456789ab.json")
	if err := os.WriteFile(orphan, []byte("[]"), 0o600); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-2 * orphanGrace)
	_ = os.Chtimes(orphan, old, old)
	if _, err := Open(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(orphan); !os.IsNotExist(err) {
		t.Fatal("orphan channel file survived")
	}
}

func equal(got []string, want ...string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

// The A→Z order is cached; changing the channel lists must invalidate it.
// Same scenario as sorted_cache_follows_changes in tests.rs.
func TestSortedCacheFollowsChanges(t *testing.T) {
	l, s := newTestLibrary(t, t.TempDir())
	if got := l.Channels(Query{Sort: SortAZ}).Total; got != 4 {
		t.Fatalf("total = %d", got)
	}
	if err := l.Remove(s.ID); err != nil {
		t.Fatal(err)
	}
	if got := l.Channels(Query{Sort: SortAZ}); got.Total != 0 || len(got.Items) != 0 {
		t.Fatalf("stale sorted cache after remove: %+v", got)
	}
}
