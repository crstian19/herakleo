package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestRuntimeGuardMoreRefusals(t *testing.T) {
	const call = `{"call-id":"a","methodName":"main.App.Play","args":["x"]}`
	cases := []struct {
		name, target, body string
		allow              bool
	}{
		{"name with the prefix in the middle", "/wails/runtime", `{"object":0,"method":0,"args":{"methodName":"x.main.App.Play"}}`, false},
		{"prefix without the dot", "/wails/runtime", `{"object":0,"method":0,"args":{"methodName":"main.Apple.Play"}}`, false},
		{"lowercase prefix", "/wails/runtime", `{"object":0,"method":0,"args":{"methodName":"main.app.Play"}}`, false},
		{"leading space", "/wails/runtime", `{"object":0,"method":0,"args":{"methodName":" main.App.Play"}}`, false},
		{"empty method name", "/wails/runtime", `{"object":0,"method":0,"args":{"methodName":""}}`, false},
		{"method name of the wrong type", "/wails/runtime", `{"object":0,"method":0,"args":{"methodName":7}}`, false},
		{"args is not an object", "/wails/runtime", `{"object":0,"method":0,"args":"main.App.Play"}`, false},
		{"args null", "/wails/runtime", `{"object":0,"method":0,"args":null}`, false},
		{"object of the wrong type", "/wails/runtime", `{"object":"0","method":0,"args":` + call + `}`, false},
		{"missing method", "/wails/runtime", `{"object":0}`, false},
		{"CancelCall other method", "/wails/runtime", `{"object":10,"method":1}`, false},
		{"negative object", "/wails/runtime", `{"object":-1,"method":0}`, false},
		{"huge object", "/wails/runtime", `{"object":99999999999999999999,"method":0}`, false},
		{"query with a non-numeric object", "/wails/runtime?object=x&method=0&args=" + url.QueryEscape(call), "", false},
		{"query with a missing method", "/wails/runtime?object=0&args=" + url.QueryEscape(call), "", false},
		{"query with invalid args JSON", "/wails/runtime?object=0&method=0&args=%7Bnope", "", false},
		{"query object 9 with an allowed method name", "/wails/runtime?object=9&method=0&args=" + url.QueryEscape(`{"methodName":"main.App.Play"}`), "", false},
		{"query cancel", "/wails/runtime?object=10&method=0", "", true},
		{"similar path", "/wails/runtime/", `{"object":9,"method":0}`, true}, // not the guarded endpoint
		{"case differs", "/Wails/runtime", `{"object":9,"method":0}`, true},  // Wails' mux is case sensitive too
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := &recorder{}
			w := post(t, guardRuntime(rec), tc.target, tc.body, nil)
			if tc.allow != rec.called {
				t.Fatalf("called = %v, want %v (code %d)", rec.called, tc.allow, w.Code)
			}
			if !tc.allow && w.Code != http.StatusForbidden {
				t.Fatalf("want 403, got %d", w.Code)
			}
		})
	}
}

// Every runtime object and method other than Call/CancelCall must get a 403,
// whatever the HTTP method carrying it.
func TestRuntimeGuardAnyHTTPMethod(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete, http.MethodPatch, http.MethodPost} {
		for object := 0; object <= 20; object++ {
			for m := 0; m <= 6; m++ {
				if (object == 0 || object == 10) && m == 0 {
					continue
				}
				rec := &recorder{}
				body := `{"object":` + strconv.Itoa(object) + `,"method":` + strconv.Itoa(m) + `,"args":{"methodName":"main.App.Play"}}`
				req := httptest.NewRequest(method, "/wails/runtime", strings.NewReader(body))
				w := httptest.NewRecorder()
				guardRuntime(rec).ServeHTTP(w, req)
				if rec.called || w.Code != http.StatusForbidden {
					t.Fatalf("%s object %d method %d: called=%v code=%d", method, object, m, rec.called, w.Code)
				}
			}
		}
	}
}

func TestRuntimeGuardGETWithQuery(t *testing.T) {
	rec := &recorder{}
	h := guardRuntime(rec)
	target := "/wails/runtime?object=0&method=0&args=" + url.QueryEscape(`{"methodName":"main.App.ListSources"}`)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, target, nil))
	if !rec.called || w.Code != http.StatusOK {
		t.Fatalf("called=%v code=%d", rec.called, w.Code)
	}
	if rec.body != "" {
		t.Fatalf("body = %q, want empty", rec.body)
	}
}

func TestRuntimeGuardBodySize(t *testing.T) {
	const prefix = `{"object":0,"method":0,"args":{"methodName":"main.App.BenchReport","args":["`
	const suffix = `"]}}`
	pad := func(n int) string { return prefix + strings.Repeat("x", n-len(prefix)-len(suffix)) + suffix }

	rec := &recorder{}
	w := post(t, guardRuntime(rec), "/wails/runtime", pad(maxRuntimeBody), nil)
	if !rec.called || w.Code != http.StatusOK {
		t.Fatalf("body at the cap: called=%v code=%d", rec.called, w.Code)
	}
	if len(rec.body) != maxRuntimeBody {
		t.Fatalf("body length %d", len(rec.body))
	}

	rec = &recorder{}
	w = post(t, guardRuntime(rec), "/wails/runtime", pad(maxRuntimeBody+1), nil)
	if rec.called || w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("body over the cap: called=%v code=%d", rec.called, w.Code)
	}

	// Other paths are passed through untouched, so the cap does not apply.
	rec = &recorder{}
	post(t, guardRuntime(rec), "/assets/big", strings.Repeat("x", maxRuntimeBody+1), nil)
	if !rec.called || len(rec.body) != maxRuntimeBody+1 {
		t.Fatalf("other path: called=%v len=%d", rec.called, len(rec.body))
	}
}

func chunkHeaders(id string, index, total int) map[string]string {
	return map[string]string{chunkIDHeader: id, chunkIndexHeader: strconv.Itoa(index), chunkTotalHeader: strconv.Itoa(total)}
}

func TestRuntimeGuardChunkRefusals(t *testing.T) {
	cases := []struct {
		name   string
		header map[string]string
		body   string
	}{
		{"total not a number", map[string]string{chunkIDHeader: "a", chunkIndexHeader: "0", chunkTotalHeader: "many"}, "x"},
		{"total missing", map[string]string{chunkIDHeader: "a", chunkIndexHeader: "0"}, "x"},
		{"total zero", chunkHeaders("a", 0, 0), "x"},
		{"total negative", chunkHeaders("a", 0, -1), "x"},
		{"total above the cap", chunkHeaders("a", 0, maxChunks+1), "x"},
		{"index not a number", map[string]string{chunkIDHeader: "a", chunkIndexHeader: "first", chunkTotalHeader: "2"}, "x"},
		{"index missing", map[string]string{chunkIDHeader: "a", chunkTotalHeader: "2"}, "x"},
		{"index negative", chunkHeaders("a", -1, 2), "x"},
		{"index equal to total", chunkHeaders("a", 2, 2), "x"},
		{"index above total", chunkHeaders("a", 9, 2), "x"},
		{"chunk over the limit", chunkHeaders("a", 0, 2), strings.Repeat("x", maxChunkBody+1)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := &recorder{}
			w := post(t, guardRuntime(rec), "/wails/runtime", tc.body, tc.header)
			if rec.called || w.Code != http.StatusForbidden {
				t.Fatalf("called=%v code=%d, want 403", rec.called, w.Code)
			}
		})
	}

	t.Run("chunk at the limit with the most chunks", func(t *testing.T) {
		rec := &recorder{}
		w := post(t, guardRuntime(rec), "/wails/runtime", strings.Repeat("x", maxChunkBody), chunkHeaders("a", 0, maxChunks))
		if w.Code != http.StatusOK || rec.called {
			t.Fatalf("partial upload: code=%d called=%v", w.Code, rec.called)
		}
	})
}

func TestRuntimeGuardChunkTotalChanges(t *testing.T) {
	rec := &recorder{}
	h := guardRuntime(rec)
	if w := post(t, h, "/wails/runtime", `{"object":`, chunkHeaders("a", 0, 2)); w.Code != http.StatusOK {
		t.Fatalf("first chunk: %d", w.Code)
	}
	// Same ID, different total: the upload is dropped and refused.
	if w := post(t, h, "/wails/runtime", `0,"method":0}`, chunkHeaders("a", 1, 3)); w.Code != http.StatusForbidden {
		t.Fatalf("inconsistent total: %d", w.Code)
	}
	// The pending upload is gone, so a late chunk starts a new one.
	if w := post(t, h, "/wails/runtime", `0,"method":0}`, chunkHeaders("a", 1, 2)); w.Code != http.StatusOK || rec.called {
		t.Fatalf("restart: code=%d called=%v", w.Code, rec.called)
	}
}

func TestRuntimeGuardChunksOutOfOrderAndResent(t *testing.T) {
	const body = `{"object":0,"method":0,"args":{"methodName":"main.App.ListSources"}}`
	cut := len(body) / 2
	rec := &recorder{}
	h := guardRuntime(rec)
	// The last piece first, then a resend of it: the body is reassembled in
	// index order, and a resent piece replaces the first copy.
	if w := post(t, h, "/wails/runtime", "stale", chunkHeaders("o", 1, 2)); w.Code != http.StatusOK || rec.called {
		t.Fatalf("chunk 1: code=%d called=%v", w.Code, rec.called)
	}
	// A resend of piece 1 must replace it, not double-count it.
	if w := post(t, h, "/wails/runtime", body[cut:], chunkHeaders("o", 1, 2)); w.Code != http.StatusOK || rec.called {
		t.Fatalf("resent chunk 1: code=%d called=%v", w.Code, rec.called)
	}
	w := post(t, h, "/wails/runtime", body[:cut], chunkHeaders("o", 0, 2))
	if w.Code != http.StatusOK || !rec.called || rec.body != body {
		t.Fatalf("code=%d called=%v body=%q", w.Code, rec.called, rec.body)
	}
}

func TestRuntimeGuardChunksTooLarge(t *testing.T) {
	rec := &recorder{}
	h := guardRuntime(rec)
	part := strings.Repeat("x", maxChunkBody)
	var last *httptest.ResponseRecorder
	// maxChunks pieces of maxChunkBody (16 MiB) overshoot maxRuntimeBody (8 MiB).
	for i := range maxChunks {
		last = post(t, h, "/wails/runtime", part, chunkHeaders("big", i, maxChunks))
		if last.Code != http.StatusOK {
			break
		}
	}
	if last.Code != http.StatusForbidden || rec.called {
		t.Fatalf("assembled body over the cap: code=%d called=%v", last.Code, rec.called)
	}
	if n := len(h.(*runtimeGuard).chunks); n != 0 {
		t.Fatalf("%d pending uploads left after the refusal", n)
	}
}

func TestRuntimeGuardChunkExpiry(t *testing.T) {
	g := guardRuntime(&recorder{}).(*runtimeGuard)
	now := time.Now()
	g.now = func() time.Time { return now }

	post(t, g, "/wails/runtime", "a", chunkHeaders("old", 0, 2))
	now = now.Add(chunkTTL / 2)
	post(t, g, "/wails/runtime", "b", chunkHeaders("young", 0, 2))
	if len(g.chunks) != 2 {
		t.Fatalf("pending = %d, want 2 before the TTL", len(g.chunks))
	}
	now = now.Add(chunkTTL/2 + time.Second)
	post(t, g, "/wails/runtime", "c", chunkHeaders("new", 0, 2))
	if _, ok := g.chunks["old"]; ok {
		t.Fatal("expired upload not dropped")
	}
	if len(g.chunks) != 2 {
		t.Fatalf("pending = %d, want young and new", len(g.chunks))
	}
}

func TestRuntimeGuardChunkedUploadsAreIndependent(t *testing.T) {
	rec := &recorder{}
	h := guardRuntime(rec)
	ok := `{"object":0,"method":0,"args":{"methodName":"main.App.A"}}`
	bad := `{"object":9,"method":0,"args":{"url":"smb://x/yyyyyyyyyyy"}}`
	// Interleave a refused and an allowed upload.
	post(t, h, "/wails/runtime", ok[:20], chunkHeaders("ok", 0, 2))
	post(t, h, "/wails/runtime", bad[:20], chunkHeaders("bad", 0, 2))
	if w := post(t, h, "/wails/runtime", bad[20:], chunkHeaders("bad", 1, 2)); w.Code != http.StatusForbidden {
		t.Fatalf("bad upload: %d", w.Code)
	}
	if w := post(t, h, "/wails/runtime", ok[20:], chunkHeaders("ok", 1, 2)); w.Code != http.StatusOK || rec.body != ok {
		t.Fatalf("ok upload: code=%d body=%q", w.Code, rec.body)
	}
}

func TestRuntimeGuardChunkedInvalidBody(t *testing.T) {
	rec := &recorder{}
	h := guardRuntime(rec)
	post(t, h, "/wails/runtime", `{"obj`, chunkHeaders("j", 0, 2))
	w := post(t, h, "/wails/runtime", `cts}`, chunkHeaders("j", 1, 2))
	if w.Code != http.StatusForbidden || rec.called {
		t.Fatalf("code=%d called=%v", w.Code, rec.called)
	}
}
