package player

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"
)

const secretURL = "http://host:8080/live/user/s3cret/1.ts"

func testLaunch() launch {
	return launch{title: "Herakleo · a$b", list: "/run/user/1000/herakleo/play-1.m3u"}
}

func TestArgsPerPlayer(t *testing.T) {
	mpv := known[0].args(testLaunch(), Options{})
	want := []string{
		"--force-window=immediate", "--really-quiet", "--title=Herakleo · a$$b", "--ytdl=no",
		"--hwdec=auto-safe", "--playlist=/run/user/1000/herakleo/play-1.m3u",
	}
	if !slices.Equal(mpv, want) {
		t.Fatalf("mpv args %q", mpv)
	}
	if vlc := known[1].args(testLaunch(), Options{}); !slices.Equal(vlc, []string{"/run/user/1000/herakleo/play-1.m3u"}) {
		t.Fatalf("vlc args %q", vlc)
	}
	if !Known("vlc") || Known("sh") {
		t.Fatal("allowlist broken")
	}
}

func TestOptionsPerPlayer(t *testing.T) {
	o := Options{SoftwareDecoding: true, CacheSecs: 30, Volume: 70}
	mpv := known[0].args(testLaunch(), o)
	want := []string{"--hwdec=no", "--cache=yes", "--cache-secs=30", "--volume=70", "--playlist=/run/user/1000/herakleo/play-1.m3u"}
	if !slices.Equal(mpv[4:], want) {
		t.Fatalf("mpv args %q", mpv)
	}
	vlc := known[1].args(testLaunch(), o)
	if !slices.Equal(vlc, []string{"--avcodec-hw=none", "--network-caching=30000", "/run/user/1000/herakleo/play-1.m3u"}) {
		t.Fatalf("vlc args %q", vlc)
	}
	if got := known[2].args(testLaunch(), o); !slices.Equal(got, []string{"/run/user/1000/herakleo/play-1.m3u"}) {
		t.Fatalf("celluloid must ignore options, got %q", got)
	}
	for _, bad := range []Options{{CacheSecs: 7}, {Volume: 101}, {Volume: -1}} {
		if bad.Validate() == nil {
			t.Errorf("%+v should be rejected", bad)
		}
	}
	if o.Validate() != nil {
		t.Errorf("%+v should be valid", o)
	}
}

// The scoped flag must wrap only our playlist, never be global.
func TestUnsafeSchemeArgs(t *testing.T) {
	if !needsUnsafePlaylist("udp://@239.0.0.1:1234") || needsUnsafePlaylist(secretURL) || needsUnsafePlaylist("rtmp://h/a") {
		t.Fatal("needsUnsafePlaylist")
	}
	l := testLaunch()
	l.unsafeScheme = true
	mpv := known[0].args(l, Options{})
	if !slices.Equal(mpv[len(mpv)-4:], []string{"--{", "--playlist=" + l.list, "--load-unsafe-playlists", "--}"}) {
		t.Fatalf("mpv args %q", mpv)
	}
	if got := known[2].args(l, Options{}); !slices.Equal(got, []string{"--mpv-load-unsafe-playlists=yes", l.list}) {
		t.Fatalf("celluloid args %q", got)
	}
}

// What every player gets, built the same way Play builds it.
func TestArgvNeverHoldsStreamURL(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"Canal 1", secretURL, "  " + secretURL + " ", ""} {
		title := displayTitle(name, secretURL)
		list, err := writePlaylist(dir, title, secretURL)
		if err != nil {
			t.Fatal(err)
		}
		for _, p := range known {
			for _, o := range []Options{{}, {SoftwareDecoding: true, CacheSecs: 30, Volume: 70}} {
				for _, unsafe := range []bool{false, true} {
					for _, a := range p.args(launch{title: title, list: list, unsafeScheme: unsafe}, o) {
						if strings.Contains(a, "s3cret") || strings.Contains(a, "://") {
							t.Errorf("%s argv leaks the URL: %q", p.id, a)
						}
					}
				}
			}
		}
	}
}

func TestDisplayTitle(t *testing.T) {
	cases := map[string]string{
		"La 1":                  "Herakleo · La 1",
		secretURL:               "Herakleo · host",
		"rtmp://other/x":        "Herakleo · host",
		"":                      "Herakleo · host",
		"a\nhttp://evil/x\r\nb": "Herakleo · host",
		"two\nlines":            "Herakleo · two lines",
		"#EXTINF:-1,x\x7f":      "Herakleo · #EXTINF:-1,x",
	}
	for in, want := range cases {
		if got := displayTitle(in, secretURL); got != want {
			t.Errorf("displayTitle(%q) = %q, want %q", in, got, want)
		}
	}
	if got := displayTitle("", "not a url"); got != "Herakleo" {
		t.Errorf("no host: %q", got)
	}
}

func TestPlaylistFile(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	dir, err := playlistDir()
	if err != nil {
		t.Fatal(err)
	}
	// A dir left 0755 by something else is tightened, not trusted.
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if dir, err = playlistDir(); err != nil {
		t.Fatal(err)
	}
	if fi, _ := os.Stat(dir); fi.Mode().Perm() != 0o700 || filepath.Base(dir) != "herakleo" {
		t.Fatalf("dir %s mode %v", dir, fi.Mode())
	}
	list, err := writePlaylist(dir, "Herakleo · La 1", secretURL)
	if err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(list)
	if err != nil || fi.Mode().Perm() != 0o600 || filepath.Dir(list) != dir {
		t.Fatalf("file %s: %v %v", list, fi.Mode(), err)
	}
	b, _ := os.ReadFile(list)
	if string(b) != "#EXTM3U\n#EXTINF:-1,Herakleo · La 1\n"+secretURL+"\n" {
		t.Fatalf("content %q", b)
	}
	other, _ := writePlaylist(dir, "x", secretURL)
	if other == list {
		t.Fatal("names must not repeat")
	}
}

func TestPlaylistDirFallback(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", "")
	t.Setenv("TMPDIR", tmp)
	dir, err := playlistDir()
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(dir) != tmp || !strings.HasPrefix(filepath.Base(dir), "herakleo-") {
		t.Fatalf("fallback dir %s", dir)
	}
	// Someone else's planted symlink must be refused, not followed.
	if err := os.Remove(dir); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(t.TempDir(), dir); err != nil {
		t.Fatal(err)
	}
	if _, err := playlistDir(); err == nil {
		t.Fatal("symlinked dir accepted")
	}
}

func TestRemoveStale(t *testing.T) {
	dir := t.TempDir()
	old, _ := writePlaylist(dir, "x", secretURL)
	fresh, _ := writePlaylist(dir, "x", secretURL)
	unrelated := filepath.Join(dir, "notes.txt")
	_ = os.WriteFile(unrelated, nil, 0o600)
	past := time.Now().Add(-2 * staleAfter)
	for _, p := range []string{old, unrelated} {
		if err := os.Chtimes(p, past, past); err != nil {
			t.Fatal(err)
		}
	}
	removeStale(dir, time.Now().Add(-staleAfter))
	for p, want := range map[string]bool{old: false, fresh: true, unrelated: true} {
		if _, err := os.Stat(p); (err == nil) != want {
			t.Errorf("%s exists=%v, want %v", filepath.Base(p), err == nil, want)
		}
	}
}

// Play end to end against a stand-in "mpv" on PATH that records its argv and
// the playlist it was given, so no real player is needed.
func TestPlayRemovesPlaylistOnExit(t *testing.T) {
	if _, err := os.Stat("/bin/sh"); err != nil {
		t.Skip("needs /bin/sh")
	}
	bin, out := t.TempDir(), t.TempDir()
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > " + out + "/argv\nfor a; do case $a in --playlist=*) cat \"${a#--playlist=}\" > " + out + "/list;; esac; done\n"
	if err := os.WriteFile(filepath.Join(bin, "mpv"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	run := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", run)

	var m Player
	if err := m.Play("mpv", secretURL, secretURL, Options{}); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(run, "herakleo")
	deadline := time.Now().Add(5 * time.Second)
	for {
		entries, _ := os.ReadDir(dir)
		if len(entries) == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("playlist not removed after the player exited")
		}
		time.Sleep(10 * time.Millisecond)
	}
	argv, _ := os.ReadFile(filepath.Join(out, "argv"))
	list, _ := os.ReadFile(filepath.Join(out, "list"))
	if strings.Contains(string(argv), "s3cret") || !strings.Contains(string(argv), "--title=Herakleo · host\n") {
		t.Fatalf("argv %q", argv)
	}
	if !strings.Contains(string(list), "\n"+secretURL+"\n") {
		t.Fatalf("player read %q", list)
	}
}

func TestPlayersPerOS(t *testing.T) {
	players := Available()
	ids := make([]string, 0, len(players))
	for _, p := range players {
		ids = append(ids, p.ID)
	}
	want := map[string][]string{
		"linux":   {"mpv", "vlc", "celluloid", "haruna"},
		"darwin":  {"mpv", "vlc", "iina"},
		"windows": {"mpv", "vlc"},
	}[runtime.GOOS]
	if !slices.Equal(ids, want) {
		t.Fatalf("players on %s: %q, want %q", runtime.GOOS, ids, want)
	}
	if runtime.GOOS != "darwin" && Known("iina") {
		t.Fatal("iina offered outside macOS")
	}
}

func TestLookPathOr(t *testing.T) {
	t.Setenv("PATH", "")
	dir := t.TempDir()
	bin := filepath.Join(dir, "player")
	if err := os.WriteFile(bin, nil, 0o755); err != nil {
		t.Fatal(err)
	}
	if got, err := lookPathOr("no-such-player", []string{filepath.Join(dir, "missing"), dir, bin}); err != nil || got != bin {
		t.Fatalf("got %q, %v", got, err)
	}
	if _, err := lookPathOr("no-such-player", []string{dir}); err == nil {
		t.Fatal("a directory is not a player")
	}
}
