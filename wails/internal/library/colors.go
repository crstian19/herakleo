package library

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"github.com/crstian19/herakleo/wails/internal/logocolor"
)

const (
	maxColorBatch = 64
	// Parallel logo downloads per batch; the webview fetches the same logos,
	// so keep it modest.
	colorWorkers = 8
)

// LogoColors returns "#rrggbb" (or "" when unusable) for each logo URL.
// Results are cached in memory and in colors.json, shared with the Tauri
// build. Failures are cached in memory only, so a flaky host is retried
// on the next launch.
func (l *Library) LogoColors(ctx context.Context, urls []string) map[string]string {
	if len(urls) > maxColorBatch {
		urls = urls[:maxColorBatch]
	}
	out := make(map[string]string, len(urls))
	var todo []string
	l.colorMu.Lock()
	l.loadColorsLocked()
	for _, u := range urls {
		if c, ok := l.colors[u]; ok {
			out[u] = c
		} else if _, seen := out[u]; !seen {
			out[u] = ""
			todo = append(todo, u)
		}
	}
	l.colorMu.Unlock()
	if len(todo) == 0 {
		return out
	}

	var (
		wg   sync.WaitGroup
		mu   sync.Mutex
		jobs = make(chan string)
	)
	for range min(colorWorkers, len(todo)) {
		wg.Go(func() {
			for u := range jobs {
				c, err := logocolor.FromURL(ctx, u)
				if err != nil {
					c = ""
				}
				mu.Lock()
				out[u] = c
				mu.Unlock()
			}
		})
	}
	for _, u := range todo {
		jobs <- u
	}
	close(jobs)
	wg.Wait()

	l.colorMu.Lock()
	defer l.colorMu.Unlock()
	changed := false
	for _, u := range todo {
		l.colors[u] = out[u]
		if out[u] != "" {
			changed = true
		}
	}
	if changed {
		_ = l.saveColorsLocked() // a lost cache write only costs a re-download
	}
	return out
}

func (l *Library) colorsPath() string { return filepath.Join(l.dir, "colors.json") }

// loadColorsLocked reads colors.json once. Caller holds colorMu.
func (l *Library) loadColorsLocked() {
	if l.colors != nil {
		return
	}
	l.colors = map[string]string{}
	if data, err := os.ReadFile(l.colorsPath()); err == nil {
		_ = json.Unmarshal(data, &l.colors)
	}
}

// saveColorsLocked persists only successful colours. Caller holds colorMu.
func (l *Library) saveColorsLocked() error {
	ok := make(map[string]string, len(l.colors))
	for u, c := range l.colors {
		if c != "" {
			ok[u] = c
		}
	}
	data, err := json.Marshal(ok)
	if err != nil {
		return err
	}
	return writeFileAtomic(l.colorsPath(), data)
}
