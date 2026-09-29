// Package preview builds the base64 JPEG a SimpleX image message needs.
package preview

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/jpeg"
	"os"

	_ "image/gif"
	_ "image/png"

	_ "golang.org/x/image/webp"
)

// JPEGBase64 returns a small JPEG preview for a picture file.
// The second result is false when the file is not a jpeg, png, or gif.
func JPEGBase64(path string) (string, bool) {
	f, err := os.Open(path)
	if err != nil {
		return "", false
	}
	defer f.Close()
	src, _, err := image.Decode(f)
	if err != nil {
		return "", false
	}
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w < 1 || h < 1 {
		return "", false
	}
	const maxEdge = 160
	nw, nh := w, h
	if w > maxEdge || h > maxEdge {
		if w >= h {
			nw = maxEdge
			nh = h * maxEdge / w
		} else {
			nh = maxEdge
			nw = w * maxEdge / h
		}
	}
	if nw < 1 {
		nw = 1
	}
	if nh < 1 {
		nh = 1
	}
	dst := image.NewRGBA(image.Rect(0, 0, nw, nh))
	for y := 0; y < nh; y++ {
		sy := b.Min.Y + y*h/nh
		for x := 0; x < nw; x++ {
			sx := b.Min.X + x*w/nw
			dst.Set(x, y, src.At(sx, sy))
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: 60}); err != nil {
		return "", false
	}
	return base64.StdEncoding.EncodeToString(buf.Bytes()), true
}
