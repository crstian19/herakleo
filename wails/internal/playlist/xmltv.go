package playlist

import (
	"bufio"
	"compress/gzip"
	"context"
	"encoding/xml"
	"errors"
	"io"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	// MaxGuideBytes caps a guide download. Full-country XMLTV files run to
	// tens of MB, and most come gzipped.
	MaxGuideBytes = 256 << 20
	// maxGuideXML caps the XML after gunzip, so a small zip bomb cannot keep
	// the parser busy for ever.
	maxGuideXML = 2 << 30
	// MaxProgrammes bounds memory across the whole guide.
	MaxProgrammes = 400_000
	maxTitle      = 200
	maxDesc       = 400
)

// Programme is one guide entry; times are unix seconds.
type Programme struct {
	Start int64  `json:"start"`
	Stop  int64  `json:"stop"`
	Title string `json:"title"`
	Desc  string `json:"desc,omitempty"`
}

// GuideChannel is an XMLTV <channel>: its ID and display names.
type GuideChannel struct {
	ID    string
	Names []string
}

// Resolve maps a guide channel to the IDs of the playlist channels it
// describes; none means its programmes are skipped. It is called once per
// guide channel, before any of its programmes.
type Resolve func(GuideChannel) []string

// Guide is programmes by playlist channel ID, sorted by start.
type Guide map[string][]Programme

// FetchGuide downloads an XMLTV guide (plain or gzipped) and keeps the
// programmes of resolved channels that overlap [from, to). With lan=false the
// download may not reach private addresses: use it for guide URLs a provider
// chose (an M3U header) rather than the user.
func FetchGuide(ctx context.Context, rawURL string, lan bool, from, to int64, resolve Resolve) (Guide, error) {
	b, err := openWith(ctx, rawURL, MaxGuideBytes, lan)
	if err != nil {
		return nil, err
	}
	defer func() { _ = b.Close() }()
	g, err := ParseXMLTV(b, from, to, resolve)
	if err != nil {
		if rerr := b.readErr(); rerr != nil {
			return nil, rerr
		}
		return nil, err
	}
	return g, nil
}

// ParseXMLTV streams an XMLTV document, gunzipping it first if needed.
// Malformed programmes are skipped; only an unreadable document fails.
func ParseXMLTV(r io.Reader, from, to int64, resolve Resolve) (Guide, error) {
	br := bufio.NewReaderSize(r, 64*1024)
	var src io.Reader = br
	if magic, err := br.Peek(2); err == nil && magic[0] == 0x1f && magic[1] == 0x8b {
		zr, err := gzip.NewReader(br)
		if err != nil {
			return nil, errors.New("guide_invalid")
		}
		defer func() { _ = zr.Close() }()
		src = io.LimitReader(zr, maxGuideXML)
	}

	dec := xml.NewDecoder(src)
	dec.Strict = false
	dec.CharsetReader = charsetReader

	g := Guide{}
	resolved := map[string][]string{}
	lookup := func(id string, names []string) []string {
		ids, ok := resolved[id]
		if !ok {
			ids = resolve(GuideChannel{ID: id, Names: names})
			resolved[id] = ids
		}
		return ids
	}
	total, sawTV := 0, false
	for total < MaxProgrammes {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			if !sawTV {
				return nil, errors.New("guide_invalid")
			}
			break // a truncated tail still leaves a usable guide
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		switch se.Name.Local {
		case "tv":
			sawTV = true
		case "channel":
			var c struct {
				ID    string   `xml:"id,attr"`
				Names []string `xml:"display-name"`
			}
			if dec.DecodeElement(&c, &se) == nil && c.ID != "" {
				for i, n := range c.Names {
					c.Names[i] = strings.TrimSpace(n)
				}
				lookup(strings.TrimSpace(c.ID), c.Names)
			}
		case "programme":
			p, channel, ok := decodeProgramme(dec, &se)
			if !ok || p.Stop <= from || p.Start >= to {
				continue
			}
			for _, id := range lookup(channel, nil) {
				g[id] = append(g[id], p)
				total++
			}
		}
	}
	if !sawTV {
		return nil, errors.New("guide_invalid")
	}
	for id, ps := range g {
		g[id] = tidy(ps)
	}
	return g, nil
}

type xmlText struct {
	Text string `xml:",chardata"`
}

func decodeProgramme(dec *xml.Decoder, se *xml.StartElement) (Programme, string, bool) {
	var x struct {
		Channel string    `xml:"channel,attr"`
		Start   string    `xml:"start,attr"`
		Stop    string    `xml:"stop,attr"`
		Titles  []xmlText `xml:"title"`
		Descs   []xmlText `xml:"desc"`
	}
	if dec.DecodeElement(&x, se) != nil {
		return Programme{}, "", false
	}
	start, ok := parseXMLTVTime(x.Start)
	if !ok {
		return Programme{}, "", false
	}
	stop, _ := parseXMLTVTime(x.Stop) // missing stops are filled in by tidy
	p := Programme{Start: start, Stop: stop, Title: firstText(x.Titles, maxTitle), Desc: firstText(x.Descs, maxDesc)}
	if p.Title == "" {
		return Programme{}, "", false
	}
	if p.Stop == 0 {
		// Unknown end: assume it overlaps the window until tidy knows better.
		p.Stop = start + 1
	}
	return p, strings.TrimSpace(x.Channel), true
}

// tidy sorts a channel's programmes, drops duplicates (two guide channels can
// map to one playlist channel) and closes programmes without a stop at the
// next start.
func tidy(ps []Programme) []Programme {
	slices.SortFunc(ps, func(a, b Programme) int {
		switch {
		case a.Start < b.Start:
			return -1
		case a.Start > b.Start:
			return 1
		}
		return 0
	})
	out := ps[:0]
	for _, p := range ps {
		if n := len(out); n > 0 && out[n-1].Start == p.Start {
			continue
		}
		out = append(out, p)
	}
	for i := range out {
		if out[i].Stop == out[i].Start+1 && i+1 < len(out) {
			out[i].Stop = out[i+1].Start
		}
	}
	return slices.Clip(out)
}

// parseXMLTVTime reads "20261028200000 +0100"; the offset is optional (UTC)
// and some guides glue it on without the space or drop the seconds.
func parseXMLTVTime(s string) (int64, bool) {
	s = strings.ReplaceAll(strings.TrimSpace(s), " ", "")
	if len(s) < 12 {
		return 0, false
	}
	digits := len(s)
	if i := strings.IndexAny(s, "+-"); i >= 0 {
		digits = i
	}
	layout := "200601021504"
	if digits >= 14 {
		layout = "20060102150405"
	}
	stamp, zone := s[:len(layout)], s[digits:]
	if zone == "" {
		t, err := time.ParseInLocation(layout, stamp, time.UTC)
		return t.Unix(), err == nil
	}
	t, err := time.Parse(layout+"-0700", stamp+zone)
	return t.Unix(), err == nil
}

func firstText(ts []xmlText, limit int) string {
	for _, t := range ts {
		if s := strings.Join(strings.Fields(t.Text), " "); s != "" {
			return truncate(s, limit)
		}
	}
	return ""
}

func truncate(s string, limit int) string {
	if utf8.RuneCountInString(s) <= limit {
		return s
	}
	r := []rune(s)
	return strings.TrimSpace(string(r[:limit-1])) + "…"
}

// charsetReader handles the legacy encodings guides declare. Latin-1 and
// Windows-1252 map byte for byte onto the first 256 code points (close enough
// for titles); anything else is read as UTF-8.
func charsetReader(label string, r io.Reader) (io.Reader, error) {
	switch strings.ToLower(label) {
	case "iso-8859-1", "iso8859-1", "latin1", "latin-1", "windows-1252", "cp1252", "iso-8859-15":
		return &latin1Reader{r: bufio.NewReader(r)}, nil
	}
	return r, nil
}

type latin1Reader struct {
	r   *bufio.Reader
	buf []byte
}

func (l *latin1Reader) Read(p []byte) (int, error) {
	for len(l.buf) < len(p) {
		b, err := l.r.ReadByte()
		if err != nil {
			if len(l.buf) == 0 {
				return 0, err
			}
			break
		}
		l.buf = utf8.AppendRune(l.buf, rune(b))
	}
	n := copy(p, l.buf)
	l.buf = l.buf[n:]
	return n, nil
}
