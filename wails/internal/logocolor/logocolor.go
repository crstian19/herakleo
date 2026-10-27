// Package logocolor picks a channel's accent colour from its logo, with the
// same rule tesdash uses at build time: the most saturated opaque pixel that
// is neither near-black nor near-white. The webview cannot do this itself:
// reading pixels of a cross-origin logo taints the canvas.
package logocolor

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	_ "image/gif" // register decoders
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"net/url"
	"time"

	_ "golang.org/x/image/webp"
)

const (
	maxLogoBytes = 4 << 20
	// Larger images are decompression bombs, not logos.
	maxLogoSide = 4096
	sample      = 64
)

var client = &http.Client{Timeout: 15 * time.Second}

// FromURL downloads a logo and returns its accent as "#rrggbb", or "" when
// the logo has no usable colour (all transparent, black or white).
func FromURL(ctx context.Context, rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", errors.New("logo URL not http(s)")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, http.NoBody)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "VLC/3.0.21 LibVLC/3.0.21")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("logo: %s", resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxLogoBytes+1))
	if err != nil {
		return "", err
	}
	if len(body) > maxLogoBytes {
		return "", errors.New("logo too large")
	}
	return FromBytes(body)
}

// FromBytes decodes PNG, JPEG, GIF or WebP and picks the accent colour.
func FromBytes(data []byte) (string, error) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	if cfg.Width > maxLogoSide || cfg.Height > maxLogoSide {
		return "", errors.New("logo dimensions too large")
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	return pick(img), nil
}

// pick samples a 64x64 nearest-neighbour grid (mirrored exactly in
// logo_color.rs) and keeps the most saturated pixel.
func pick(img image.Image) string {
	b := img.Bounds()
	if b.Dx() == 0 || b.Dy() == 0 {
		return ""
	}
	best, bestScore := [3]uint8{}, -1.0
	for sy := range sample {
		for sx := range sample {
			x := b.Min.X + (2*sx+1)*b.Dx()/(2*sample)
			y := b.Min.Y + (2*sy+1)*b.Dy()/(2*sample)
			r16, g16, b16, a16 := img.At(x, y).RGBA()
			if a16>>8 < 128 {
				continue
			}
			// RGBA() is alpha-premultiplied; undo it to get the drawn colour.
			r, g, bl := unpremultiply(r16, a16), unpremultiply(g16, a16), unpremultiply(b16, a16)
			l, s := lightSat(r, g, bl)
			if l < 0.15 || l > 0.92 {
				continue
			}
			if s > bestScore {
				best, bestScore = [3]uint8{r, g, bl}, s
			}
		}
	}
	if bestScore < 0 {
		return ""
	}
	return fmt.Sprintf("#%02x%02x%02x", best[0], best[1], best[2])
}

func unpremultiply(c, a uint32) uint8 {
	if a == 0 {
		return 0
	}
	return uint8(min(c*0xffff/a, 0xffff) >> 8)
}

// lightSat returns HSL lightness and saturation, as in tesdash's getSaturation.
func lightSat(r, g, b uint8) (l, s float64) {
	rn, gn, bn := float64(r)/255, float64(g)/255, float64(b)/255
	hi, lo := max(rn, gn, bn), min(rn, gn, bn)
	l = (hi + lo) / 2
	if hi == lo {
		return l, 0
	}
	d := hi - lo
	if l > 0.5 {
		return l, d / (2 - hi - lo)
	}
	return l, d / (hi + lo)
}
