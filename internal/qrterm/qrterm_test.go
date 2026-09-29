package qrterm

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func TestPNGHasQuietZone(t *testing.T) {
	const link = "https://smp6.simplex.im/a#aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	raw, err := PNG(link)
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	b := img.Bounds()
	if b.Dx() < 80 || b.Dx() != b.Dy() {
		t.Fatalf("size %dx%d, want a square", b.Dx(), b.Dy())
	}
	margin := quietMargin(img)
	// 4 modules at 8px each. A pasted terminal drawing loses this margin.
	if margin < 32 {
		t.Fatalf("quiet zone is %dpx, want at least 32", margin)
	}
}

func quietMargin(img image.Image) int {
	b := img.Bounds()
	darkX := b.Dx()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if dark(img.At(x, y)) && x-b.Min.X < darkX {
				darkX = x - b.Min.X
			}
		}
	}
	return darkX
}

func dark(c color.Color) bool {
	r, g, b, _ := c.RGBA()
	return r < 0x8000 && g < 0x8000 && b < 0x8000
}
