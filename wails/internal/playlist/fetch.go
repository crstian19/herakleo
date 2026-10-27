package playlist

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"syscall"
	"time"
)

const (
	// MaxPlaylistBytes caps downloads; real lists top out around tens of MB, so
	// anything far beyond that is a misconfigured URL or a hostile server.
	MaxPlaylistBytes = 256 << 20
	// maxAPIBytes caps an Xtream API answer. The biggest panels list some
	// 50k live streams at ~0.5 KB of JSON each, about 25 MB. Decoding is
	// streamed, so this bounds time and bandwidth rather than memory.
	maxAPIBytes = 64 << 20
)

var clients = NewClients(2 * time.Minute)

var errTooLarge = errors.New("playlist_too_large")

// ValidateHTTPURL rejects anything that is not an absolute http(s) URL, so a
// user-supplied source cannot read local files or reach odd schemes.
func ValidateHTTPURL(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, errors.New("url_not_http")
	}
	return u, nil
}

// body is a response body that fails with errTooLarge past its limit and
// remembers why reading stopped, so decode errors can be told from transport
// errors.
type body struct {
	rc       io.ReadCloser
	r        io.Reader // rc limited to limit+1 bytes: one more decides "over"
	n, limit int64
	err      error
}

func (b *body) Read(p []byte) (int, error) {
	if b.err != nil {
		return 0, b.err
	}
	n, err := b.r.Read(p)
	b.n += int64(n)
	if b.n > b.limit {
		n -= int(b.n - b.limit)
		err = errTooLarge
	}
	if err != nil {
		b.err = err
	}
	return n, err
}

func (b *body) Close() error { return b.rc.Close() }

// readErr turns the reason reading stopped into an error code, or nil when
// the body itself was fine and the content is to blame.
func (b *body) readErr() error {
	switch {
	case b.err == nil || errors.Is(b.err, io.EOF):
		return nil
	case errors.Is(b.err, errTooLarge):
		return errTooLarge
	default:
		return fmt.Errorf("download_interrupted: %w", b.err)
	}
}

// open starts a GET and returns the body capped at limit bytes. Errors never
// echo the URL because Xtream URLs carry the account password.
func open(ctx context.Context, rawURL string, limit int64) (*body, error) {
	u, err := ValidateHTTPURL(rawURL)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, http.NoBody)
	if err != nil {
		return nil, errors.New("url_not_http")
	}
	// Some providers block Go's default UA; present as a media player.
	req.Header.Set("User-Agent", "VLC/3.0.21 LibVLC/3.0.21")

	resp, err := clients.For(ctx, u, true).Do(req)
	if err != nil {
		var uerr *url.Error
		if errors.As(err, &uerr) {
			err = uerr.Err
		}
		if errors.Is(err, errPrivateAddress) {
			err = errPrivateAddress // drop the dialled address from the message
		}
		return nil, fmt.Errorf("download_failed: %w", err)
	}
	if resp.StatusCode != http.StatusOK { // Rust checks the same: 200 only
		_ = resp.Body.Close()
		return nil, fmt.Errorf("server_status: %s", resp.Status)
	}
	return &body{rc: resp.Body, r: io.LimitReader(resp.Body, limit+1), limit: limit}, nil
}

// M3UFromURL downloads and parses a remote playlist as it streams in.
func M3UFromURL(ctx context.Context, rawURL string) ([]Entry, error) {
	b, err := open(ctx, rawURL, MaxPlaylistBytes)
	if err != nil {
		return nil, err
	}
	defer func() { _ = b.Close() }()
	entries, err := ParseM3U(b)
	if err != nil {
		if rerr := b.readErr(); rerr != nil {
			return nil, rerr
		}
		return nil, err
	}
	return entries, nil
}

// M3UFromFile parses a playlist from disk.
func M3UFromFile(path string) ([]Entry, error) {
	// O_NONBLOCK so a FIFO does not block the open until a writer shows up;
	// the fstat below then rejects it. Regular files ignore the flag.
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0) //nolint:gosec // the user picked this file in a native dialog
	if err != nil {
		return nil, fmt.Errorf("file_open_failed: %w", err)
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("not_regular_file")
	}
	if info.Size() > MaxPlaylistBytes {
		return nil, errors.New("playlist_too_large")
	}
	return ParseM3U(io.LimitReader(f, MaxPlaylistBytes))
}
