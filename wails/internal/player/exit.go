package player

import (
	"bytes"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"sync"
)

// Exit tells the UI that a player closed on its own with an error, with what
// it printed, so the user can see (and copy) why a channel opened and closed.
// A player the user closes normally, or one we stop, is not reported.
type Exit struct {
	// Player is the player's display name ("mpv").
	Player string `json:"player"`
	// Title is the channel name.
	Title string `json:"title"`
	// Code is the exit code, -1 when the process did not exit normally.
	Code int `json:"code"`
	// Seconds the player ran.
	Seconds int `json:"seconds"`
	// Log is the end of the player's output. The stream URL holds the
	// Xtream password and players repeat it, so the URL's parts are scrubbed
	// and every URL is cut down to scheme://host (scrubLog).
	Log []string `json:"log"`
	// At is when it closed, unix seconds.
	At int64 `json:"at"`
}

const (
	logLines  = 30
	lineMax   = 300
	tailBytes = 32 << 10
)

// tailBuffer keeps the last tailBytes the player wrote to stdout/stderr.
type tailBuffer struct {
	mu      sync.Mutex
	buf     []byte
	trimmed bool
}

func (t *tailBuffer) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.buf = append(t.buf, p...)
	if over := len(t.buf) - tailBytes; over > 0 {
		t.buf = append(t.buf[:0], t.buf[over:]...)
		t.trimmed = true
	}
	return len(p), nil
}

// String is the kept output. After a trim, the first, cut line goes: it may
// start in the middle of a URL, where redactLine cannot recognise it.
func (t *tailBuffer) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	b := t.buf
	if t.trimmed {
		if i := bytes.IndexByte(b, '\n'); i >= 0 {
			b = b[i+1:]
		} else {
			b = nil
		}
	}
	return string(bytes.ToValidUTF8(b, []byte("?")))
}

var urlRe = regexp.MustCompile(`(?i)([a-z0-9+.-]*)://([^\s/?#"'<>]*)[^\s"'<>]*`)

// redactLine cuts every URL in a log line down to scheme://host: no user
// info, path or query (Xtream puts the user and password in the path).
func redactLine(line string) string {
	// Control characters go first: one inside a URL would end the match
	// early and, once removed, glue the rest of the path back on.
	line = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, line)
	line = urlRe.ReplaceAllStringFunc(line, func(u string) string {
		m := urlRe.FindStringSubmatch(u)
		host := m[2]
		if i := strings.LastIndexByte(host, '@'); i >= 0 {
			host = host[i+1:]
		}
		return strings.ToLower(m[1]) + "://" + host + "/…"
	})
	if len(line) > lineMax {
		line = strings.ToValidUTF8(line[:lineMax], "") + "…"
	}
	return line
}

// plainSegments are path segments Xtream and HLS URLs use that hold no
// credential; scrubbing them would mangle ordinary words in the log.
var plainSegments = []string{"live", "movie", "movies", "series", "play", "hls", "get.php", "index.m3u8"}

// secretsOf lists the parts of a stream URL that may hold credentials, as
// players may print them: the whole URL, the user info and query values (any
// length) and the path segments (Xtream puts user and password there), raw
// and decoded.
func secretsOf(streamURL string) []string {
	creds := []string{streamURL}
	var segs []string
	if u, err := url.Parse(streamURL); err == nil {
		if u.User != nil {
			creds = append(creds, u.User.Username())
			if p, ok := u.User.Password(); ok {
				creds = append(creds, p)
			}
		}
		for _, vs := range u.Query() {
			creds = append(creds, vs...)
		}
		creds = append(creds, u.RawQuery)
		for _, s := range strings.Split(u.EscapedPath(), "/") {
			if !slices.Contains(plainSegments, strings.ToLower(s)) {
				segs = append(segs, s)
			}
		}
	}
	var out []string
	add := func(parts []string, minLen int) {
		for _, p := range parts {
			for _, v := range []string{p, unescape(url.PathUnescape, p), unescape(url.QueryUnescape, p), url.PathEscape(p), url.QueryEscape(p)} {
				if len(v) >= minLen && !slices.Contains(out, v) {
					out = append(out, v)
				}
			}
		}
	}
	add(creds, 1)
	add(segs, 2) // a one-character segment ("1") would blank every digit
	// Longest first, so a whole URL goes before the parts inside it.
	slices.SortFunc(out, func(a, b string) int { return len(b) - len(a) })
	return out
}

// replaceWhole replaces s in text only where it stands as a whole token (not
// inside a longer word), so "live" in "deliver" survives.
func replaceWhole(text, s string) string {
	isWord := func(b byte) bool {
		return b == '_' || b >= '0' && b <= '9' || b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z'
	}
	var b strings.Builder
	for {
		i := strings.Index(text, s)
		if i < 0 {
			b.WriteString(text)
			return b.String()
		}
		end := i + len(s)
		whole := (i == 0 || !isWord(text[i-1]) || !isWord(s[0])) &&
			(end == len(text) || !isWord(text[end]) || !isWord(s[len(s)-1]))
		b.WriteString(text[:i])
		if whole {
			b.WriteString("…")
		} else {
			b.WriteString(s)
		}
		text = text[end:]
	}
}

func unescape(f func(string) (string, error), s string) string {
	if v, err := f(s); err == nil {
		return v
	}
	return s
}

// scrubLog removes the stream URL's parts from the player's output, then
// returns the last non-empty lines with every remaining URL cut down.
func scrubLog(out, streamURL string) []string {
	for _, s := range secretsOf(streamURL) {
		out = replaceWhole(out, s)
	}
	var lines []string
	for _, l := range strings.Split(strings.ReplaceAll(out, "\r", "\n"), "\n") {
		if l = strings.TrimSpace(redactLine(l)); l != "" {
			lines = append(lines, l)
		}
	}
	if len(lines) > logLines {
		lines = lines[len(lines)-logLines:]
	}
	return lines
}

// streamErrorRe matches the errors a refused or cut stream produces.
var streamErrorRe = regexp.MustCompile(`(?i)http error|failed to open|could not connect|connection (?:reset|refused|timed out)|failed to resolve|could not resolve|network is unreachable|no route to host|server returned [45]\d\d`)

// failed reports whether an exit is worth telling the user about: an error
// exit code, or mpv (which prints errors only) closing with 0 after a stream
// error, as it does when a live stream is cut mid-way. The GUI players print
// startup chatter every time and exit 0 when the user closes them, so for
// them only the exit code counts.
func failed(playerID string, code int, lines []string) bool {
	if code != 0 {
		return true
	}
	return playerID == "mpv" && streamErrorRe.MatchString(strings.Join(lines, "\n"))
}
