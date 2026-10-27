package playlist

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"
)

// MaxPlaylistBytes caps downloads; real lists top out around tens of MB, so
// anything far beyond that is a misconfigured URL or a hostile server.
const MaxPlaylistBytes = 256 << 20

var client = &http.Client{Timeout: 2 * time.Minute}

// ValidateHTTPURL rejects anything that is not an absolute http(s) URL, so a
// user-supplied source cannot read local files or reach odd schemes.
func ValidateHTTPURL(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, errors.New("url_not_http")
	}
	return u, nil
}

// get downloads a URL into memory, bounded by MaxPlaylistBytes. Errors never
// echo the URL because Xtream URLs carry the account password.
func get(ctx context.Context, rawURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, http.NoBody)
	if err != nil {
		return nil, errors.New("url_not_http")
	}
	// Some providers block Go's default UA; present as a media player.
	req.Header.Set("User-Agent", "VLC/3.0.21 LibVLC/3.0.21")

	resp, err := client.Do(req)
	if err != nil {
		var uerr *url.Error
		if errors.As(err, &uerr) {
			err = uerr.Err
		}
		return nil, fmt.Errorf("download_failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK { // Rust checks the same: 200 only
		return nil, fmt.Errorf("server_status: %s", resp.Status)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, MaxPlaylistBytes+1))
	if err != nil {
		return nil, fmt.Errorf("download_interrupted: %w", err)
	}
	if len(body) > MaxPlaylistBytes {
		return nil, errors.New("playlist_too_large")
	}
	return body, nil
}

// M3UFromURL downloads and parses a remote playlist.
func M3UFromURL(ctx context.Context, rawURL string) ([]Entry, error) {
	if _, err := ValidateHTTPURL(rawURL); err != nil {
		return nil, err
	}
	body, err := get(ctx, rawURL)
	if err != nil {
		return nil, err
	}
	return ParseM3U(bytes.NewReader(body))
}

// M3UFromFile parses a playlist from disk.
func M3UFromFile(path string) ([]Entry, error) {
	f, err := os.Open(path) //nolint:gosec // the user picked this file in a native dialog
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
