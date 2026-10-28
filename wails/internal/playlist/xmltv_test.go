package playlist

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const guideDoc = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE tv SYSTEM "xmltv.dtd">
<tv generator-info-name="test">
  <channel id="la1.es"><display-name>La 1</display-name><display-name>TVE 1</display-name></channel>
  <channel id="unknown.es"><display-name>Nobody</display-name></channel>
  <programme start="20261028180000 +0100" stop="20261028190000 +0100" channel="la1.es">
    <title lang="es">Telediario</title><desc lang="es">  Noticias
      del día  </desc>
  </programme>
  <programme start="20261028160000 +0100" stop="20261028180000 +0100" channel="la1.es">
    <title>Antes</title>
  </programme>
  <programme start="20261028190000 +0100" channel="la1.es"><title>Sin fin</title></programme>
  <programme start="20261028200000 +0100" stop="20261028210000 +0100" channel="la1.es"><title>Cine</title></programme>
  <programme start="20261028180000 +0100" stop="20261028190000 +0100" channel="unknown.es"><title>Skipped</title></programme>
  <programme start="garbage" channel="la1.es"><title>Bad time</title></programme>
  <programme start="20261028230000 +0100" stop="20261029000000 +0100" channel="la1.es"><title></title></programme>
</tv>`

func unix(s string) int64 {
	t, err := time.Parse("2006-01-02 15:04 -0700", s)
	if err != nil {
		panic(err)
	}
	return t.Unix()
}

func resolveLa1(c GuideChannel) []string {
	if c.ID == "la1.es" {
		return []string{"src-1", "src-2"}
	}
	return nil
}

func TestParseXMLTV(t *testing.T) {
	from, to := unix("2026-10-28 17:00 +0100"), unix("2026-10-28 22:00 +0100")
	g, err := ParseXMLTV(strings.NewReader(guideDoc), from, to, resolveLa1)
	if err != nil {
		t.Fatal(err)
	}
	want := []Programme{
		{Start: unix("2026-10-28 16:00 +0100"), Stop: unix("2026-10-28 18:00 +0100"), Title: "Antes"},
		{Start: unix("2026-10-28 18:00 +0100"), Stop: unix("2026-10-28 19:00 +0100"), Title: "Telediario", Desc: "Noticias del día"},
		// No stop: it ends when the next one starts.
		{Start: unix("2026-10-28 19:00 +0100"), Stop: unix("2026-10-28 20:00 +0100"), Title: "Sin fin"},
		{Start: unix("2026-10-28 20:00 +0100"), Stop: unix("2026-10-28 21:00 +0100"), Title: "Cine"},
	}
	for _, id := range []string{"src-1", "src-2"} {
		got := g[id]
		if fmt.Sprint(got) != fmt.Sprint(want) {
			t.Errorf("%s:\n got %v\nwant %v", id, got, want)
		}
	}
	if len(g) != 2 {
		t.Errorf("unresolved channels kept: %v", g)
	}
}

func TestParseXMLTVWindow(t *testing.T) {
	from, to := unix("2026-10-28 18:30 +0100"), unix("2026-10-28 19:00 +0100")
	g, err := ParseXMLTV(strings.NewReader(guideDoc), from, to, resolveLa1)
	if err != nil {
		t.Fatal(err)
	}
	// Telediario overlaps the window; "Sin fin" starts at its end, so it is out.
	if got := g["src-1"]; len(got) != 1 || got[0].Title != "Telediario" {
		t.Errorf("got %v", got)
	}
}

func TestParseXMLTVGzipAndLatin1(t *testing.T) {
	doc := "<?xml version=\"1.0\" encoding=\"ISO-8859-1\"?><tv>" +
		"<channel id=\"a\"><display-name>A</display-name></channel>" +
		"<programme start=\"20261028180000\" stop=\"20261028190000\" channel=\"a\"><title>Espa\xf1a</title></programme></tv>"
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	_, _ = zw.Write([]byte(doc))
	_ = zw.Close()
	g, err := ParseXMLTV(&buf, 0, 1<<62, func(GuideChannel) []string { return []string{"x"} })
	if err != nil {
		t.Fatal(err)
	}
	p := g["x"]
	if len(p) != 1 || p[0].Title != "España" || p[0].Start != unix("2026-10-28 18:00 +0000") {
		t.Errorf("got %v", p)
	}
}

func TestParseXMLTVResolvesUndeclaredChannelsOnce(t *testing.T) {
	doc := `<tv><programme start="20261028180000" stop="20261028190000" channel="b"><title>X</title></programme>` +
		`<programme start="20261028190000" stop="20261028200000" channel="b"><title>Y</title></programme></tv>`
	calls := 0
	g, err := ParseXMLTV(strings.NewReader(doc), 0, 1<<62, func(c GuideChannel) []string {
		calls++
		if c.ID != "b" || len(c.Names) != 0 {
			t.Errorf("resolve got %+v", c)
		}
		return []string{"x"}
	})
	if err != nil || calls != 1 || len(g["x"]) != 2 {
		t.Errorf("err %v, calls %d, guide %v", err, calls, g)
	}
}

func TestParseXMLTVRejectsNonGuides(t *testing.T) {
	for _, doc := range []string{"", "not xml at all", `{"json":true}`, "<html><body>login</body></html>"} {
		if _, err := ParseXMLTV(strings.NewReader(doc), 0, 1<<62, resolveLa1); err == nil {
			t.Errorf("%q: want an error", doc)
		}
	}
}

func TestParseXMLTVKeepsTruncatedGuide(t *testing.T) {
	head, _, _ := strings.Cut(guideDoc, `<programme start="20261028200000`)
	doc := head + `<programme start="2026`
	g, err := ParseXMLTV(strings.NewReader(doc), 0, 1<<62, resolveLa1)
	if err != nil || len(g["src-1"]) != 3 {
		t.Errorf("err %v, got %v", err, g["src-1"])
	}
}

func TestParseXMLTVTruncatesLongText(t *testing.T) {
	long := strings.Repeat("é", 1000)
	doc := `<tv><programme start="20261028180000" stop="20261028190000" channel="a"><title>` + long +
		`</title><desc>` + long + `</desc></programme></tv>`
	g, _ := ParseXMLTV(strings.NewReader(doc), 0, 1<<62, func(GuideChannel) []string { return []string{"x"} })
	p := g["x"][0]
	if n := len([]rune(p.Title)); n > maxTitle {
		t.Errorf("title has %d runes", n)
	}
	if n := len([]rune(p.Desc)); n > maxDesc {
		t.Errorf("desc has %d runes", n)
	}
}

func TestParseXMLTVTime(t *testing.T) {
	cases := map[string]int64{
		"20261028200000 +0100": unix("2026-10-28 20:00 +0100"),
		"20261028200000+0100":  unix("2026-10-28 20:00 +0100"),
		"20261028200000 -0500": unix("2026-10-28 20:00 -0500"),
		"20261028200000":       unix("2026-10-28 20:00 +0000"),
		"202610282000 +0100":   unix("2026-10-28 20:00 +0100"),
	}
	for in, want := range cases {
		if got, ok := parseXMLTVTime(in); !ok || got != want {
			t.Errorf("%q: got %d %v, want %d", in, got, ok, want)
		}
	}
	for _, bad := range []string{"", "2026", "tomorrow at 8", "20261328200000"} {
		if _, ok := parseXMLTVTime(bad); ok {
			t.Errorf("%q: want failure", bad)
		}
	}
}

func TestFetchGuide(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, guideDoc)
	}))
	t.Cleanup(srv.Close)
	g, err := FetchGuide(context.Background(), srv.URL+"/epg.xml", true, 0, 1<<62, resolveLa1)
	if err != nil || len(g["src-1"]) != 4 {
		t.Fatalf("err %v, got %v", err, g)
	}
	// A guide URL from a provider may not reach the LAN (the test server is local).
	if _, err := FetchGuide(context.Background(), srv.URL+"/epg.xml", false, 0, 1<<62, resolveLa1); err == nil ||
		err.Error() != "download_failed: private_address_blocked" {
		t.Errorf("provider guide on the LAN: got %v", err)
	}
}

func TestSameHost(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"http://192.168.1.5:34400/lineup.m3u", "http://192.168.1.5:34400/epg.xml", true},
		{"http://Proxy.lan/a.m3u", "http://proxy.lan/epg.xml", true},
		{"http://192.168.1.5:34400/a.m3u", "http://192.168.1.1/epg.xml", false},
		{"http://192.168.1.5/a.m3u", "http://192.168.1.5:8080/epg.xml", false},
		{"not a url", "http://x/epg.xml", false},
	}
	for _, c := range cases {
		if got := SameHost(c.a, c.b); got != c.want {
			t.Errorf("SameHost(%q, %q) = %v", c.a, c.b, got)
		}
	}
}
