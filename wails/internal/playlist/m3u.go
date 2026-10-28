// Package playlist turns M3U files and Xtream Codes accounts into channels.
package playlist

import (
	"bufio"
	"errors"
	"io"
	"net/url"
	"strings"
)

const (
	// MaxEntries bounds memory for hostile lists; real ones stay well below.
	MaxEntries = 500_000
	// maxLine skips absurd lines instead of failing the whole import. It is
	// generous because some #EXTINF lines carry base64 logos.
	maxLine = 1 << 20
)

// streamSchemes is what mpv may open. Anything else (file paths, edl://,
// av://v4l2:..., lavf://) would let a playlist make mpv read local files or devices.
var streamSchemes = map[string]bool{
	"http": true, "https": true, "rtmp": true, "rtmps": true, "rtsp": true, "udp": true, "rtp": true,
}

// IsStreamURL reports whether a stream URL is safe to hand to mpv.
func IsStreamURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && streamSchemes[strings.ToLower(u.Scheme)] && u.Host != ""
}

// isLogoURL keeps logos to web images; Wails has no CSP to fall back on.
func isLogoURL(raw string) bool {
	lower := strings.ToLower(raw)
	return strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") ||
		strings.HasPrefix(lower, "data:image/")
}

// Entry is one playable item from a playlist, before it is assigned an ID.
type Entry struct {
	Name  string
	Group string
	Logo  string
	URL   string
	// EPGID is the channel's ID in the XMLTV guide (tvg-id, epg_channel_id).
	EPGID string
}

// List is a parsed playlist plus the guide URL its header names, if any.
type List struct {
	Entries []Entry
	// GuideURL comes from `#EXTM3U url-tvg="…"` (or x-tvg-url); the first
	// http(s) URL when the header lists several.
	GuideURL string
}

// ParseM3U reads an extended M3U playlist. It is lenient on purpose: provider
// lists are rarely well-formed, so unknown directives are skipped and a URL
// without a preceding #EXTINF still becomes an entry named after the URL.
func ParseM3U(r io.Reader) ([]Entry, error) {
	l, err := ParseM3UList(r)
	return l.Entries, err
}

// ParseM3UList is ParseM3U plus the header's guide URL.
func ParseM3UList(r io.Reader) (List, error) {
	br := bufio.NewReaderSize(r, 64*1024)

	var (
		entries []Entry
		guide   string
		pending Entry
		hasInfo bool
	)
	for len(entries) < MaxEntries {
		raw, err := br.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return List{}, err
		}
		if raw == "" && err != nil {
			break
		}
		// A BOM is not whitespace to TrimSpace; Windows editors add one.
		line := strings.TrimSpace(strings.TrimPrefix(raw, "\uFEFF"))
		switch {
		case line == "" || len(line) > maxLine:
		case strings.HasPrefix(line, "#EXTM3U"):
			if guide == "" {
				guide = headerGuide(line)
			}
		case strings.HasPrefix(line, "#EXTINF:"):
			pending = parseExtinf(line[len("#EXTINF:"):])
			hasInfo = true
		case strings.HasPrefix(line, "#EXTGRP:"):
			if pending.Group == "" {
				pending.Group = strings.TrimSpace(line[len("#EXTGRP:"):])
			}
		case strings.HasPrefix(line, "#"):
		default:
			e := pending
			e.URL = line
			if !hasInfo || e.Name == "" {
				e.Name = line
			}
			if !isLogoURL(e.Logo) {
				e.Logo = ""
			}
			if IsStreamURL(e.URL) {
				entries = append(entries, e)
			}
			pending, hasInfo = Entry{}, false
		}
	}
	// err above is io.EOF here: the end of the list, not a failure.
	return List{Entries: entries, GuideURL: guide}, nil //nolint:nilerr
}

// headerGuide picks the guide URL from an #EXTM3U line. Providers put one or
// several comma-separated URLs in url-tvg or x-tvg-url.
func headerGuide(line string) string {
	attrs := attributes(line)
	for _, key := range []string{"url-tvg", "x-tvg-url", "tvg-url"} {
		for _, u := range strings.Split(attrs[key], ",") {
			if u = strings.TrimSpace(u); u != "" {
				if _, err := ValidateHTTPURL(u); err == nil {
					return u
				}
			}
		}
	}
	return ""
}

// parseExtinf parses `-1 tvg-logo="x" group-title="y",Display Name`. The name
// starts after the first comma outside quotes, since titles often contain commas.
func parseExtinf(s string) Entry {
	var (
		e       Entry
		inQuote bool
		split   = -1
	)
	for i, r := range s {
		if r == '"' {
			inQuote = !inQuote
		} else if r == ',' && !inQuote {
			split = i
			break
		}
	}
	attrs := s
	if split >= 0 {
		attrs = s[:split]
		e.Name = strings.TrimSpace(s[split+1:])
	}

	for key, val := range attributes(attrs) {
		switch strings.ToLower(key) {
		case "group-title":
			e.Group = val
		case "tvg-logo":
			e.Logo = val
		case "tvg-id":
			e.EPGID = val
		case "tvg-name":
			if e.Name == "" {
				e.Name = val
			}
		}
	}
	return e
}

// attributes extracts key="value" pairs, ignoring the leading duration.
func attributes(s string) map[string]string {
	out := map[string]string{}
	for {
		eq := strings.Index(s, `="`)
		if eq < 0 {
			return out
		}
		key := s[:eq]
		if sp := strings.LastIndexAny(key, " \t"); sp >= 0 {
			key = key[sp+1:]
		}
		rest := s[eq+2:]
		end := strings.IndexByte(rest, '"')
		if end < 0 {
			return out
		}
		out[key] = strings.TrimSpace(rest[:end])
		s = rest[end+1:]
	}
}
