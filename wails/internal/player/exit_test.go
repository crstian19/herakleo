package player

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRedactLine(t *testing.T) {
	tests := []struct{ in, want string }{
		{"Failed to open http://user:pw@prov.example:8080/live/myuser/s3cret/123.ts.", "Failed to open http://prov.example:8080/…"},
		{"[curl] HTTP error 403", "[curl] HTTP error 403"},
		{"redirect to HTTPS://cdn.example/x?token=abc and udp://239.0.0.1:1234", "redirect to https://cdn.example/… and udp://239.0.0.1:1234/…"},
		{"bell\x07 and esc\x1b[31m", "bell and esc[31m"},
		{"x-herakleo-custom-proxy-scheme://u:p@proxy.example/live/u/pw/1.ts", "x-herakleo-custom-proxy-scheme://proxy.example/…"},
	}
	for _, tt := range tests {
		if got := redactLine(tt.in); got != tt.want {
			t.Errorf("redactLine(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
	if got := redactLine(strings.Repeat("é", 400)); len(got) > lineMax+len("…") {
		t.Errorf("long line kept %d bytes", len(got))
	}
}

// Whatever surrounds it, a URL's user info, path and query never survive.
func FuzzRedactLineDropsSecrets(f *testing.F) {
	f.Add("Failed to open ", "http", "u:p@", "host:80", "/live/USER/PASS/1.ts", "?token=SECRET", ".")
	f.Add("", "rtmp", "", "h", "/app/PASS", "", "")
	f.Add("x", "HTTPS", "a@b@", "h", "/PASS", "#frag", " tail")
	f.Fuzz(func(t *testing.T, pre, scheme, userinfo, host, path, query, post string) {
		const secret = "Zq9SECRETq9Z"
		if strings.ContainsAny(scheme, " \t\n\r\"'<>/?#") || strings.ContainsAny(host+userinfo, " \t\n\r\"'<>/?#") ||
			strings.ContainsAny(pre, "/") || strings.Contains(pre+post+host+scheme, secret) {
			t.Skip()
		}
		path = "/" + secret + strings.Map(func(r rune) rune {
			if strings.ContainsRune(" \t\n\r\"'<>", r) {
				return -1
			}
			return r
		}, path)
		line := pre + " " + scheme + "://" + userinfo + host + path + query + " " + post
		if strings.Contains(redactLine(line), secret) {
			t.Fatalf("secret survived in %q → %q", line, redactLine(line))
		}
	})
}

// The stream URL's parts go even when a quote or space (raw M3U URLs are not
// escaped) stops redactLine's URL match early.
func TestScrubLogRemovesTheStreamURLParts(t *testing.T) {
	tests := []struct {
		name, url, out string
	}{
		{"xtream path", "http://h:8080/live/myuser/s3cretpw/1.ts", "Failed to open http://h:8080/live/myuser/s3cretpw/1.ts."},
		{"quote in the path", "http://h/u/pa'ss3cret/1.ts", "Failed to open http://h/u/pa'ss3cret/1.ts."},
		{"quote in the user info", "http://myuser:pa'ss3cret@h/x", "Failed to open http://myuser:pa'ss3cret@h/x."},
		{"space in the path", "http://h/u/pa ss3cret/1.ts", "Failed to open http://h/u/pa ss3cret/1.ts."},
		{"decoded by the player", "http://h/live/myuser/pa%20ss3cret/1.ts", "cannot open h/live/myuser/pa ss3cret/1.ts"},
		{"query credentials", "http://h/get.php?" + query("myuser", "s3cretpw"), "GET http://h/get.php?" + query("myuser", "s3cretpw") + " failed"},
		{"path printed without the scheme", "http://h/live/myuser/s3cretpw/1.ts", "segment /live/myuser/s3cretpw/1.ts failed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := strings.Join(scrubLog(tt.out, tt.url), "\n")
			for _, secret := range []string{"s3cret", "myuser"} {
				if strings.Contains(got, secret) {
					t.Fatalf("%q survived: %q", secret, got)
				}
			}
		})
	}
}

func TestTailBufferDropsTheCutLine(t *testing.T) {
	var b tailBuffer
	_, _ = b.Write([]byte("Failed to open http://h/live/u/s3cret/1.ts\n"))
	_, _ = b.Write([]byte(strings.Repeat("x", tailBytes-10) + "\nlast line\n"))
	if s := b.String(); strings.Contains(s, "s3cret") || strings.Contains(s, "xxx") || !strings.HasPrefix(s, "last line") {
		t.Fatalf("kept %q…", s[:min(40, len(s))])
	}
}

func TestFailed(t *testing.T) {
	tests := []struct {
		name   string
		player string
		code   int
		lines  []string
		want   bool
	}{
		{"error exit", "mpv", 2, nil, true},
		{"killed", "vlc", -1, nil, true},
		{"mpv stream cut mid-way, exit 0", "mpv", 0, []string{"[ffmpeg] http: Connection reset by peer"}, true},
		{"mpv closed after decoder noise", "mpv", 0, []string{"[ffmpeg/video] h264: error while decoding MB 3 4"}, false},
		{"mpv closed by the user", "mpv", 0, nil, false},
		{"VLC closed by the user after its startup chatter", "vlc", 0, []string{"main libvlc: Running vlc with the default interface"}, false},
		{"VLC exits 0 even after an HTTP error", "vlc", 0, []string{"http stream error: HTTP error 403"}, false},
	}
	for _, tt := range tests {
		if got := failed(tt.player, tt.code, tt.lines); got != tt.want {
			t.Errorf("%s: failed = %v", tt.name, got)
		}
	}
}

func TestScrubLogShortCredentialsAndPlainWords(t *testing.T) {
	got := strings.Join(scrubLog("segment /live/tv/42/123.ts failed\nhls: deliver live stream\nGET h/get.php?"+query("ab", "xy"), "http://h:8080/live/tv/42/123.ts?"+query("ab", "xy")), "\n")
	for _, secret := range []string{"/tv/", "/42/", "=ab", "=xy"} {
		if strings.Contains(got, secret) {
			t.Fatalf("%q survived: %q", secret, got)
		}
	}
	if !strings.Contains(got, "deliver live stream") {
		t.Fatalf("ordinary words mangled: %q", got)
	}
}

// query builds an Xtream-style query string from fake values; built, not
// written out, so secret scanners don't take the fixtures for credentials.
func query(user, pass string) string {
	return url.Values{"username": {user}, "password": {pass}}.Encode()
}

func TestReplaceWhole(t *testing.T) {
	for _, tt := range []struct{ text, s, want string }{
		{"a/pw/b", "pw", "a/…/b"},
		{"pwd pw", "pw", "pwd …"},
		{"pw/pw", "pw", "…/…"},
		{"x'p ss'y", "'p ss'", "x…y"},
	} {
		if got := replaceWhole(tt.text, tt.s); got != tt.want {
			t.Errorf("replaceWhole(%q, %q) = %q, want %q", tt.text, tt.s, got, tt.want)
		}
	}
}

func TestTailBufferKeepsTheEnd(t *testing.T) {
	var b tailBuffer
	_, _ = b.Write([]byte("first\n"))
	_, _ = b.Write([]byte(strings.Repeat("a", tailBytes) + "\nEND"))
	if s := b.String(); s != "END" {
		t.Fatalf("kept %d bytes ending %q", len(s), s[max(0, len(s)-3):])
	}
}

// fakeMPV puts a stand-in mpv on PATH that runs script.
func fakeMPV(t *testing.T, script string) {
	t.Helper()
	if _, err := os.Stat("/bin/sh"); err != nil {
		t.Skip("needs /bin/sh")
	}
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "mpv"), []byte("#!/bin/sh\n"+script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
}

func watchExits(m *Player) chan Exit {
	ch := make(chan Exit, 4)
	m.SetOnExit(func(e Exit) { ch <- e })
	return ch
}

func TestPlayReportsRefusedStream(t *testing.T) {
	fakeMPV(t, `echo "[curl] HTTP error 403" >&2
echo "Failed to open `+secretURL+`." >&2
exit 2
`)
	var m Player
	exits := watchExits(&m)
	if err := m.Play("mpv", "La 1", secretURL, Options{}); err != nil {
		t.Fatal(err)
	}
	select {
	case e := <-exits:
		if e.Code != 2 || e.Player != "mpv" || e.Title != "La 1" || len(e.Log) != 2 || e.Log[0] != "[curl] HTTP error 403" {
			t.Fatalf("exit %+v", e)
		}
		if strings.Contains(strings.Join(e.Log, "\n"), "s3cret") || strings.Contains(strings.Join(e.Log, "\n"), "/live/") {
			t.Fatalf("the password reached the log: %q", e.Log)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no exit reported")
	}
}

// A background child that keeps the output pipe open must not delay the
// exit report (or the playlist removal) for long.
func TestPlayDoesNotWaitForChildrenHoldingThePipe(t *testing.T) {
	fakeMPV(t, "echo '[curl] HTTP error 403' >&2\n(sleep 30 &)\nexit 2\n")
	var m Player
	exits := watchExits(&m)
	if err := m.Play("mpv", "x", secretURL, Options{}); err != nil {
		t.Fatal(err)
	}
	select {
	case e := <-exits:
		if e.Code != 2 {
			t.Fatalf("exit %+v", e)
		}
	case <-time.After(6 * time.Second):
		t.Fatal("waited for the child holding the pipe")
	}
}

func TestPlayIgnoresNormalAndStoppedExits(t *testing.T) {
	t.Run("closed by the user", func(t *testing.T) {
		fakeMPV(t, "exit 0\n")
		var m Player
		exits := watchExits(&m)
		if err := m.Play("mpv", "x", secretURL, Options{}); err != nil {
			t.Fatal(err)
		}
		select {
		case e := <-exits:
			t.Fatalf("reported %+v", e)
		case <-time.After(500 * time.Millisecond):
		}
	})
	t.Run("stopped or replaced by us", func(t *testing.T) {
		fakeMPV(t, "echo 'some error' >&2\nexec sleep 30\n")
		var m Player
		exits := watchExits(&m)
		if err := m.Play("mpv", "x", secretURL, Options{}); err != nil {
			t.Fatal(err)
		}
		time.Sleep(100 * time.Millisecond)
		if err := m.Play("mpv", "y", secretURL, Options{}); err != nil { // replaces the first
			t.Fatal(err)
		}
		time.Sleep(100 * time.Millisecond)
		m.Stop()
		select {
		case e := <-exits:
			t.Fatalf("reported %+v", e)
		case <-time.After(500 * time.Millisecond):
		}
	})
}
