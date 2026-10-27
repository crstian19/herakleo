package logocolor

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
)

// Same fixture as logo_color.rs: transparent + white + black + one red and
// one dull-blue half. Red is the most saturated, so it wins.
func TestPickMostSaturated(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 128, 128))
	for y := range 128 {
		for x := range 128 {
			var c color.NRGBA
			switch {
			case y < 32:
				c = color.NRGBA{}
			case y < 48:
				c = color.NRGBA{255, 255, 255, 255}
			case y < 64:
				c = color.NRGBA{0, 0, 0, 255}
			case x < 64:
				c = color.NRGBA{220, 30, 40, 255}
			default:
				c = color.NRGBA{80, 90, 140, 255}
			}
			img.SetNRGBA(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	got, err := FromBytes(buf.Bytes())
	if err != nil || got != "#dc1e28" {
		t.Fatalf("got %q, %v", got, err)
	}
}

func TestPickNothingUsable(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 8, 8)) // fully transparent
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	if got, _ := FromBytes(buf.Bytes()); got != "" {
		t.Fatalf("got %q", got)
	}
}
