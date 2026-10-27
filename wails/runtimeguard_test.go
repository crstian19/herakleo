package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
)

// recorder stands in for Wails' transport: it records what got through.
type recorder struct {
	called bool
	body   string
	header http.Header
}

func (rec *recorder) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	rec.called = true
	b, _ := io.ReadAll(r.Body)
	rec.body = string(b)
	rec.header = r.Header.Clone()
	w.WriteHeader(http.StatusOK)
}

func post(t *testing.T, h http.Handler, target, body string, header map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, target, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range header {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

func TestRuntimeGuard(t *testing.T) {
	cases := []struct {
		name   string
		target string
		body   string
		allow  bool
	}{
		{"app call by name", "/wails/runtime", `{"object":0,"method":0,"args":{"call-id":"a","methodName":"main.App.ListSources","args":[]}}`, true},
		{"cancel call", "/wails/runtime", `{"object":10,"method":0,"args":{"call-id":"a"}}`, true},
		{"app call via query", "/wails/runtime?object=0&method=0&args=" + url.QueryEscape(`{"call-id":"a","methodName":"main.App.Play","args":["x"]}`), "", true},
		{"other path untouched", "/assets/index.js", `whatever`, true},

		{"Browser.OpenURL", "/wails/runtime", `{"object":9,"method":0,"args":{"url":"smb://evil/share"}}`, false},
		{"Clipboard.Text", "/wails/runtime", `{"object":1,"method":1}`, false},
		{"Clipboard.SetText", "/wails/runtime", `{"object":1,"method":0,"args":{"text":"x"}}`, false},
		{"Dialog open file", "/wails/runtime", `{"object":5,"method":4,"args":{}}`, false},
		{"Window close", "/wails/runtime", `{"object":6,"method":2}`, false},
		{"Application quit", "/wails/runtime", `{"object":2,"method":2}`, false},
		{"Events emit", "/wails/runtime", `{"object":3,"method":0,"args":{"name":"x"}}`, false},
		{"System env", "/wails/runtime", `{"object":8,"method":1}`, false},
		{"ContextMenu open", "/wails/runtime", `{"object":4,"method":0,"args":{}}`, false},
		{"Browser via query", "/wails/runtime?object=9&method=0&args=" + url.QueryEscape(`{"url":"https://x"}`), "", false},
		{"call by method ID", "/wails/runtime", `{"object":0,"method":0,"args":{"call-id":"a","methodID":12345}}`, false},
		{"call to another service", "/wails/runtime", `{"object":0,"method":0,"args":{"call-id":"a","methodName":"main.Other.Do"}}`, false},
		{"call with no args", "/wails/runtime", `{"object":0,"method":0}`, false},
		{"Call, unknown method", "/wails/runtime", `{"object":0,"method":1,"args":{"methodName":"main.App.Play"}}`, false},
		{"missing object", "/wails/runtime", `{"method":0}`, false},
		{"bad JSON", "/wails/runtime", `{"object":0,`, false},
		{"empty", "/wails/runtime", ``, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := &recorder{}
			w := post(t, guardRuntime(rec), tc.target, tc.body, nil)
			if tc.allow {
				if !rec.called || w.Code != http.StatusOK {
					t.Fatalf("want pass-through, got called=%v code=%d", rec.called, w.Code)
				}
				if rec.body != tc.body {
					t.Fatalf("body not restored: got %q, want %q", rec.body, tc.body)
				}
				return
			}
			if rec.called {
				t.Fatal("blocked call reached the transport")
			}
			if w.Code != http.StatusForbidden {
				t.Fatalf("want 403, got %d", w.Code)
			}
		})
	}
}

func chunks(body string, size int) []string {
	var out []string
	for len(body) > size {
		out = append(out, body[:size])
		body = body[size:]
	}
	return append(out, body)
}

func sendChunked(t *testing.T, h http.Handler, id, body string) (*httptest.ResponseRecorder, int) {
	t.Helper()
	parts := chunks(body, 16)
	var w *httptest.ResponseRecorder
	for i, p := range parts {
		w = post(t, h, "/wails/runtime", p, map[string]string{
			chunkIDHeader:    id,
			chunkIndexHeader: strconv.Itoa(i),
			chunkTotalHeader: strconv.Itoa(len(parts)),
		})
		if i < len(parts)-1 && w.Code != http.StatusOK {
			t.Fatalf("chunk %d: code %d", i, w.Code)
		}
	}
	return w, len(parts)
}

func TestRuntimeGuardChunked(t *testing.T) {
	allowed := `{"object":0,"method":0,"args":{"call-id":"a","methodName":"main.App.BenchReport","args":["` + strings.Repeat("x", 40) + `"]}}`
	rec := &recorder{}
	h := guardRuntime(rec)
	w, n := sendChunked(t, h, "ok", allowed)
	if n < 3 {
		t.Fatalf("test body should span several chunks, got %d", n)
	}
	if !rec.called || w.Code != http.StatusOK {
		t.Fatalf("want pass-through, got called=%v code=%d", rec.called, w.Code)
	}
	if rec.body != allowed {
		t.Fatalf("reassembled body = %q", rec.body)
	}
	if rec.header.Get(chunkIDHeader) != "" {
		t.Fatal("chunk headers should be stripped before Wails sees the request")
	}

	blocked := `{"object":9,"method":0,"args":{"url":"smb://evil/share/` + strings.Repeat("x", 40) + `"}}`
	rec = &recorder{}
	h = guardRuntime(rec)
	w, _ = sendChunked(t, h, "bad", blocked)
	if rec.called {
		t.Fatal("chunked Browser call reached the transport")
	}
	if w.Code != http.StatusForbidden {
		t.Fatalf("want 403, got %d", w.Code)
	}

	w = post(t, h, "/wails/runtime", "x", map[string]string{chunkIDHeader: "z", chunkIndexHeader: "0", chunkTotalHeader: "100000"})
	if w.Code != http.StatusForbidden || rec.called {
		t.Fatalf("absurd chunk total: code %d called %v", w.Code, rec.called)
	}
}
