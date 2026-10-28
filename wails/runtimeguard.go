package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// The webview reaches Go through POST /wails/runtime, and Wails routes that
// endpoint to every runtime object: Browser.OpenURL (any scheme but a few),
// Clipboard, file dialogs, window control... The UI only needs to call App's
// methods by name (frontend/src/lib/backend/wails.ts), so runtimeGuard lets
// through only that and the cancellation of those calls. Anything else gets a
// 403: a webview bug (XSS in a channel name, say) then cannot open arbitrary
// URLs or read the clipboard, and OpenLink's allowlist stays the only way out.
//
// Wails' own plumbing doesn't use this endpoint on Linux: "runtime ready",
// window drag/resize and the non-client regions go through the WebKit message
// handler (window.webkit.messageHandlers.external), and Go→JS events through
// evaluateJavaScript. Object/method IDs are the ones in Wails v3.0.0
// (pkg/application/messageprocessor.go, @wailsio/runtime runtime.js).

const (
	runtimePath = "/wails/runtime"

	objectCall       = 0  // Call; method 0 = CallBinding
	objectCancelCall = 10 // CancelCall; method 0 = Cancel
	methodDefault    = 0

	// Only methods of the App service, called by name. Name takes precedence
	// over methodID in Wails, and both sides decode with encoding/json.
	allowedMethodPrefix = "main.App."

	// The largest legit request is BenchReport (capped at 1 MiB of text in
	// app.go, a bit more once JSON-escaped). Wails accepts up to 64 MiB.
	maxRuntimeBody = 8 << 20

	// The JS runtime splits bodies over 512 KiB into chunks it sends serially
	// (x-wails-chunk-* headers); the same limits as Wails' transport apply.
	chunkIDHeader    = "x-wails-chunk-id"
	chunkIndexHeader = "x-wails-chunk-index"
	chunkTotalHeader = "x-wails-chunk-total"
	maxChunkBody     = 1 << 20
	maxChunks        = 16
	chunkTTL         = 30 * time.Second
)

type runtimeGuard struct {
	next http.Handler

	mu     sync.Mutex
	chunks map[string]*pendingRequest
	now    func() time.Time
}

type pendingRequest struct {
	parts   map[int][]byte
	total   int
	size    int
	created time.Time
}

// guardRuntime is the application.AssetOptions.Middleware. Wails puts it in
// front of its own handlers, so /wails/runtime reaches the transport only
// through it.
func guardRuntime(next http.Handler) http.Handler {
	return &runtimeGuard{next: next, chunks: map[string]*pendingRequest{}, now: time.Now}
}

func (g *runtimeGuard) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != runtimePath {
		g.next.ServeHTTP(w, r)
		return
	}

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxRuntimeBody))
	if err != nil {
		http.Error(w, "runtime request too large", http.StatusRequestEntityTooLarge)
		return
	}

	// Chunks are reassembled here and handed to Wails as one plain request,
	// so the transport never assembles a body this guard hasn't checked.
	if id := r.Header.Get(chunkIDHeader); id != "" {
		var done bool
		body, done, err = g.addChunk(id, r.Header, body)
		if err != nil {
			deny(w, r, err.Error())
			return
		}
		if !done {
			w.WriteHeader(http.StatusOK) // what Wails answers for a partial upload
			return
		}
		r.Header.Del(chunkIDHeader)
		r.Header.Del(chunkIndexHeader)
		r.Header.Del(chunkTotalHeader)
	}

	if why := checkRuntimeCall(r, body); why != "" {
		deny(w, r, why)
		return
	}

	r.Body = io.NopCloser(bytes.NewReader(body))
	r.ContentLength = int64(len(body))
	g.next.ServeHTTP(w, r)
}

func deny(w http.ResponseWriter, r *http.Request, why string) {
	// %q keeps a crafted path on one log line.
	log.Printf("runtime guard: blocked %q %q: %s", r.Method, r.URL.Path, why) //nolint:gosec
	http.Error(w, "forbidden", http.StatusForbidden)
}

// checkRuntimeCall returns why the call is not allowed, or "". It reads the
// request the way Wails' HTTPTransport.processBody does: a JSON body
// {object, method, args}, or, when the body is empty (WebKitGTK may send POST
// data that way), the object/method/args query parameters.
func checkRuntimeCall(r *http.Request, body []byte) string {
	var req struct {
		Object *int            `json:"object"`
		Method *int            `json:"method"`
		Args   json.RawMessage `json:"args"`
	}
	if len(body) > 0 {
		if err := json.Unmarshal(body, &req); err != nil {
			return "invalid JSON"
		}
	} else {
		q := r.URL.Query()
		if n, err := strconv.Atoi(q.Get("object")); err == nil {
			req.Object = &n
		}
		if n, err := strconv.Atoi(q.Get("method")); err == nil {
			req.Method = &n
		}
		if a := q.Get("args"); a != "" {
			var args json.RawMessage
			if json.Unmarshal([]byte(a), &args) == nil {
				req.Args = args
			}
		}
	}
	if req.Object == nil || req.Method == nil {
		return "missing object or method"
	}

	switch {
	case *req.Object == objectCancelCall && *req.Method == methodDefault:
		return ""
	case *req.Object == objectCall && *req.Method == methodDefault:
		var opts struct {
			MethodName string `json:"methodName"`
		}
		if len(req.Args) == 0 || json.Unmarshal(req.Args, &opts) != nil {
			return "invalid call arguments"
		}
		if !strings.HasPrefix(opts.MethodName, allowedMethodPrefix) {
			return "method " + strconv.Quote(opts.MethodName) + " not allowed"
		}
		return ""
	default:
		return "object " + strconv.Itoa(*req.Object) + " method " + strconv.Itoa(*req.Method) + " not allowed"
	}
}

// addChunk stores one piece of a chunked request and returns the whole body
// once every piece has arrived.
func (g *runtimeGuard) addChunk(id string, h http.Header, part []byte) ([]byte, bool, error) {
	total, err := strconv.Atoi(h.Get(chunkTotalHeader))
	if err != nil || total <= 0 || total > maxChunks {
		return nil, false, errors.New("invalid chunk total")
	}
	index, err := strconv.Atoi(h.Get(chunkIndexHeader))
	if err != nil || index < 0 || index >= total {
		return nil, false, errors.New("invalid chunk index")
	}
	if len(part) > maxChunkBody {
		return nil, false, errors.New("chunk too large")
	}

	g.mu.Lock()
	defer g.mu.Unlock()

	now := g.now()
	for k, p := range g.chunks {
		if now.Sub(p.created) > chunkTTL {
			delete(g.chunks, k)
		}
	}

	p := g.chunks[id]
	if p == nil {
		p = &pendingRequest{parts: map[int][]byte{}, total: total, created: now}
		g.chunks[id] = p
	}
	if p.total != total {
		delete(g.chunks, id)
		return nil, false, errors.New("inconsistent chunk total")
	}
	p.size += len(part) - len(p.parts[index])
	p.parts[index] = part
	if p.size > maxRuntimeBody {
		delete(g.chunks, id)
		return nil, false, errors.New("assembled body too large")
	}
	if len(p.parts) < p.total {
		return nil, false, nil
	}

	delete(g.chunks, id)
	body := make([]byte, 0, p.size)
	for i := range p.total {
		body = append(body, p.parts[i]...)
	}
	return body, true, nil
}
