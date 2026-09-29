package preview

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestJPEGBase64ShrinksPNG(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wide.png")
	src := image.NewRGBA(image.Rect(0, 0, 400, 10))
	src.Set(0, 0, color.RGBA{R: 255, A: 255})
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, src); err != nil {
		t.Fatal(err)
	}
	f.Close()

	b64, ok := JPEGBase64(path)
	if !ok || b64 == "" {
		t.Fatal("png should become a jpeg preview")
	}
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		t.Fatal(err)
	}
	img, err := jpeg.Decode(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds().Dx() != 160 || img.Bounds().Dy() != 4 {
		t.Fatalf("preview size %v", img.Bounds())
	}
}

func TestJPEGBase64AcceptsGIF(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a.gif")
	img := image.NewPaletted(image.Rect(0, 0, 2, 2), color.Palette{color.Black, color.White})
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := gif.Encode(f, img, nil); err != nil {
		f.Close()
		t.Fatal(err)
	}
	f.Close()
	if _, ok := JPEGBase64(path); !ok {
		t.Fatal("gif should become a jpeg preview")
	}
}

func TestJPEGBase64AcceptsWebP(t *testing.T) {
	raw, err := hex.DecodeString("524946463c000000574542505650382030000000d001009d012a0200020001402625a00274ba01f80003b000fef2eb7ffcd815cd73eff7ffd2e0fd2e0fd2e0ffd2900000")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "a.webp")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, ok := JPEGBase64(path); !ok {
		t.Fatal("webp should become a jpeg preview")
	}
}

func TestJPEGBase64RejectsNonImage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notes.jpg")
	if err := os.WriteFile(path, []byte("not a picture"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, ok := JPEGBase64(path); ok {
		t.Fatal("undecodable file must not get a fake preview")
	}
}
