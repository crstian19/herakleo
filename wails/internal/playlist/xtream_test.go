package playlist

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// endless serves `[` followed by elem forever, counting what was read.
type endless struct {
	elem []byte
	read atomic.Int64
	open bool
}

func (e *endless) Read(p []byte) (int, error) {
	if !e.open {
		e.open = true
		p[0] = '['
		e.read.Add(1)
		return 1, nil
	}
	n := 0
	for n+len(e.elem) <= len(p) {
		n += copy(p[n:], e.elem)
	}
	e.read.Add(int64(n))
	return n, nil
}

func testBody(r io.Reader, limit int64) *body {
	return &body{rc: io.NopCloser(r), r: io.LimitReader(r, limit+1), limit: limit}
}

func TestDecodeListStopsAtMaxEntries(t *testing.T) {
	src := &endless{elem: []byte(`{},`)}
	n := 0
	if err := decodeList(testBody(src, maxAPIBytes), func(xtreamStream) { n++ }); err != nil {
		t.Fatal(err)
	}
	if n != MaxEntries {
		t.Fatalf("decoded %d elements, want %d", n, MaxEntries)
	}
	// Stopping means the body is left unread, not buffered up to the cap.
	if got := src.read.Load(); got > 4<<20 {
		t.Fatalf("read %d bytes for %d elements", got, n)
	}
}

func TestLiveChannelsTruncatesEndlessList(t *testing.T) {
	src := &endless{elem: []byte(`{"stream_id":7,"name":"x"},`)}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("action") == "get_live_categories" {
			_, _ = io.WriteString(w, `[]`)
			return
		}
		buf := make([]byte, 32<<10)
		for { // until the client hangs up
			n, _ := src.Read(buf)
			if _, err := w.Write(buf[:n]); err != nil {
				return
			}
		}
	}))
	defer srv.Close()

	got, err := Xtream{Server: srv.URL, Username: "u", Password: "p"}.LiveChannels(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != MaxEntries {
		t.Fatalf("got %d entries, want %d", len(got), MaxEntries)
	}
}

func TestXtreamErrorCodes(t *testing.T) {
	for body, want := range map[string]string{
		`{"user_info":{"auth":0}}`: "xtream_rejected",
		``:                         "xtream_rejected",
		`not json`:                 "xtream_rejected",
		`[1,2]`:                    "xtream_invalid_response",
		`[{"stream_id":1},`:        "xtream_invalid_response",
	} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = io.WriteString(w, body)
		}))
		_, err := Xtream{Server: srv.URL, Username: "u", Password: "p"}.LiveChannels(context.Background())
		srv.Close()
		if err == nil || err.Error() != want {
			t.Errorf("%q: got %v, want %s", body, err, want)
		}
	}
}

func TestDecodeListTooLarge(t *testing.T) {
	big := "[" + strings.Repeat(`{"name":"x"},`, 100) + "{}]"
	err := decodeList(testBody(strings.NewReader(big), 64), func(xtreamStream) {})
	if !errors.Is(err, errTooLarge) {
		t.Fatalf("got %v", err)
	}
	// Exactly at the limit is fine.
	small := `[{"name":"x"}]`
	if err := decodeList(testBody(strings.NewReader(small), int64(len(small))), func(xtreamStream) {}); err != nil {
		t.Fatalf("at limit: %v", err)
	}
}

func TestAccountInfo(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, `{"user_info":{"exp_date":"1700000000","status":" Active "}}`)
	}))
	defer srv.Close()
	acc, err := Xtream{Server: srv.URL, Username: "u", Password: "p"}.AccountInfo(context.Background())
	if err != nil || acc.ExpiresAt != 1700000000 || acc.Status != "Active" {
		t.Fatalf("got %+v, %v", acc, err)
	}
}
