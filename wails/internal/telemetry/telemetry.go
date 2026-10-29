// Package telemetry sends the anonymous usage ping (on by default, off in
// Settings): one "launch"
// event a day to the author's Umami, with the app version, OS, architecture,
// UI language and a random install ID. Never lists, channels or URLs. What it
// sends is documented in the README ("Telemetry"); keep both in step.
package telemetry

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"
	"time"
)

const (
	// Endpoint is Umami's event API.
	Endpoint = "https://umami.crstian.me/api/send"
	// WebsiteID is the Herakleo "website" in that Umami.
	WebsiteID = "169eb399-ef54-43e9-91e9-8f4559d09583"
	// Interval is the least time between two pings.
	Interval = 20 * time.Hour
	hostname = "herakleo.crstian.me"
)

// Event is what one ping says.
type Event struct {
	Version   string
	InstallID string
	Language  string
	// Flatpak is reported apart from the OS: it changes how players run.
	Flatpak bool
}

type payload struct {
	Type    string `json:"type"`
	Payload struct {
		Website  string            `json:"website"`
		Hostname string            `json:"hostname"`
		URL      string            `json:"url"`
		Title    string            `json:"title"`
		Language string            `json:"language"`
		Name     string            `json:"name"`
		ID       string            `json:"id"`
		Data     map[string]string `json:"data"`
	} `json:"payload"`
}

// Body is the JSON sent for e; exported for the tests and for anyone
// checking what leaves the machine.
func Body(e Event) ([]byte, error) {
	var p payload
	p.Type = "event"
	p.Payload.Website = WebsiteID
	p.Payload.Hostname = hostname
	p.Payload.URL = "/app"
	p.Payload.Title = "Herakleo"
	p.Payload.Language = e.Language
	p.Payload.Name = "launch"
	// Umami counts one visitor per ID: one per install, not per IP.
	p.Payload.ID = e.InstallID
	p.Payload.Data = map[string]string{
		"version": e.Version,
		"os":      runtime.GOOS,
		"arch":    runtime.GOARCH,
		"flatpak": fmt.Sprint(e.Flatpak),
	}
	return json.Marshal(p)
}

// userAgent lets Umami fill its OS column; it rejects requests that look
// like bots, and Go's default user agent does.
func userAgent(version string) string {
	platform := map[string]string{
		"linux":   "X11; Linux x86_64",
		"windows": "Windows NT 10.0; Win64; x64",
		"darwin":  "Macintosh; Intel Mac OS X 10_15_7",
	}[runtime.GOOS]
	if platform == "" {
		platform = runtime.GOOS
	}
	return fmt.Sprintf("Mozilla/5.0 (%s) Herakleo/%s", platform, version)
}

var client = &http.Client{Timeout: 15 * time.Second}

// Send posts one event to endpoint (Endpoint outside tests).
func Send(ctx context.Context, endpoint string, e Event) error {
	body, err := Body(e)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", userAgent(e.Version))
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("telemetry: %s", resp.Status)
	}
	return nil
}

// NewInstallID is a random ID with nothing derived from the machine.
func NewInstallID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
