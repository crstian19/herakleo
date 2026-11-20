// Package update finds new Herakleo releases on GitHub and, for installs it
// owns (the Linux tarball, the Windows installer or zip, the macOS .dmg),
// downloads, verifies and swaps in the new version. Installs that a package
// manager owns (AUR, Scoop, Homebrew, deb, rpm, Flatpak) are only told how to
// update, so the app never fights the manager's own records.
package update

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	// LatestURL is the GitHub API for the newest published, non-prerelease release.
	LatestURL = "https://api.github.com/repos/crstian19/herakleo/releases/latest"
	// DownloadBase + "v<version>/<asset>" is an asset's download URL.
	DownloadBase = "https://github.com/crstian19/herakleo/releases/download/"
	// SumsName lists every asset's SHA-256.
	SumsName = "SHA256SUMS"
	// SigName is the ed25519 signature of SumsName.
	SigName = SumsName + ".sig"

	maxAPIBody = 1 << 20
	maxNotes   = 16 << 10
)

// Release is the part of a GitHub release the updater uses.
type Release struct {
	// Version without the "v" ("0.5.0").
	Version string
	// Notes is the release text (Markdown from the changelog), capped.
	Notes  string
	Assets []string
}

// HasAsset reports whether the release carries the named file.
func (r Release) HasAsset(name string) bool {
	for _, a := range r.Assets {
		if a == name {
			return true
		}
	}
	return false
}

var tagRe = regexp.MustCompile(`^v(\d{1,4})\.(\d{1,4})\.(\d{1,4})$`)

// Latest asks GitHub for the newest release. Tags that are not vX.Y.Z are
// rejected: the version ends up in file names and URLs.
func Latest(ctx context.Context, c *http.Client, url string) (Release, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Release{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "herakleo-updater")
	resp, err := c.Do(req)
	if err != nil {
		return Release{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return Release{}, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	var body struct {
		Tag        string `json:"tag_name"`
		Body       string `json:"body"`
		Draft      bool   `json:"draft"`
		Prerelease bool   `json:"prerelease"`
		Assets     []struct {
			Name string `json:"name"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxAPIBody)).Decode(&body); err != nil {
		return Release{}, err
	}
	if body.Draft || body.Prerelease {
		return Release{}, errors.New("not a published release")
	}
	if !tagRe.MatchString(body.Tag) {
		return Release{}, fmt.Errorf("unexpected tag %q", body.Tag)
	}
	r := Release{Version: strings.TrimPrefix(body.Tag, "v"), Notes: body.Body}
	if len(r.Notes) > maxNotes {
		r.Notes = strings.ToValidUTF8(r.Notes[:maxNotes], "")
	}
	for _, a := range body.Assets {
		r.Assets = append(r.Assets, a.Name)
	}
	return r, nil
}

// Newer reports whether version a (x.y.z) is newer than b. A malformed
// version is never newer, so garbage cannot trigger an update.
func Newer(a, b string) bool {
	pa, okA := parse(a)
	pb, okB := parse(b)
	if !okA || !okB {
		return false
	}
	for i := range pa {
		if pa[i] != pb[i] {
			return pa[i] > pb[i]
		}
	}
	return false
}

func parse(v string) ([3]int, bool) {
	var out [3]int
	m := tagRe.FindStringSubmatch("v" + v)
	if m == nil {
		return out, false
	}
	for i := range out {
		out[i], _ = strconv.Atoi(m[i+1])
	}
	return out, true
}

// NewClient is the HTTP client for the API and the downloads: HTTPS only,
// redirects included (GitHub sends assets to its CDN).
func NewClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if req.URL.Scheme != "https" {
				return errors.New("redirect to a non-HTTPS URL")
			}
			if len(via) >= 5 {
				return errors.New("too many redirects")
			}
			return nil
		},
	}
}
