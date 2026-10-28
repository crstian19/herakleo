package playlist

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseExtinfWithCommas(t *testing.T) {
	list := "#EXTM3U\n#EXTINF:-1 tvg-logo=\"http://l/a.png\" group-title=\"News, World\",BBC, One\nhttp://s/1\n"
	got, err := ParseM3U(strings.NewReader(list))
	if err != nil {
		t.Fatal(err)
	}
	want := []Entry{{Name: "BBC, One", Group: "News, World", Logo: "http://l/a.png", URL: "http://s/1"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestBareURLAndExtgrp(t *testing.T) {
	list := "#EXTINF:-1,A\r\n#EXTGRP:Sports\n#EXTVLCOPT:x=y\nhttp://s/a\nhttp://s/b\n"
	got, err := ParseM3U(strings.NewReader(list))
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Group != "Sports" || got[1].Name != "http://s/b" {
		t.Fatalf("unexpected %+v", got)
	}
}

func TestValidateHTTPURL(t *testing.T) {
	for _, bad := range []string{"file:///etc/passwd", "ftp://x/y", "http://", "list.m3u"} {
		if _, err := ValidateHTTPURL(bad); err == nil {
			t.Errorf("%q should be rejected", bad)
		}
	}
}

func TestDropsNonNetworkStreamsAndLogos(t *testing.T) {
	list := "#EXTINF:-1 tvg-logo=\"file:///etc/x\",Cam\nav://v4l2:/dev/video0\n#EXTINF:-1 tvg-logo=\"javascript:x\",Ok\nhttp://s/ok\n/etc/passwd\n"
	got, err := ParseM3U(strings.NewReader(list))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != "Ok" || got[0].Logo != "" {
		t.Fatalf("unexpected %+v", got)
	}
}

func TestBOMAndXtreamIDs(t *testing.T) {
	got, err := ParseM3U(strings.NewReader("\uFEFF#EXTM3U\n#EXTINF:-1,A\nhttp://s/a\n"))
	if err != nil || len(got) != 1 || got[0].Name != "A" {
		t.Fatalf("BOM: %+v, %v", got, err)
	}
	for raw, want := range map[string]string{`" 12 "`: "12", `42`: "42", `null`: "", `true`: "", `{}`: ""} {
		if got := rawID([]byte(raw)); got != want {
			t.Errorf("rawID(%s) = %q, want %q", raw, got, want)
		}
	}
}

func TestGuideIDsAndHeader(t *testing.T) {
	list := "#EXTM3U url-tvg=\"ftp://bad, https://epg.example/guide.xml.gz,https://other/x.xml\"\n" +
		"#EXTINF:-1 tvg-id=\"la1.es\" tvg-name=\"La 1\",La 1 HD\nhttp://s/la1\n" +
		"#EXTINF:-1,No id\nhttp://s/x\n"
	got, err := ParseM3UList(strings.NewReader(list))
	if err != nil {
		t.Fatal(err)
	}
	if got.GuideURL != "https://epg.example/guide.xml.gz" {
		t.Errorf("guide URL %q", got.GuideURL)
	}
	if len(got.Entries) != 2 || got.Entries[0].EPGID != "la1.es" || got.Entries[1].EPGID != "" {
		t.Errorf("entries %+v", got.Entries)
	}
	for header, want := range map[string]string{
		`#EXTM3U x-tvg-url="http://a/epg.xml"`: "http://a/epg.xml",
		`#EXTM3U url-tvg="file:///etc/passwd"`: "",
		`#EXTM3U`:                              "",
	} {
		l, _ := ParseM3UList(strings.NewReader(header + "\n#EXTINF:-1,A\nhttp://s/a\n"))
		if l.GuideURL != want {
			t.Errorf("%s: got %q, want %q", header, l.GuideURL, want)
		}
	}
}
