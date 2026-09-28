package qrterm

import (
	"strings"
	"testing"
)

func TestShortLinkIsSmallAndSquare(t *testing.T) {
	const link = "https://smp6.simplex.im/a#aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	got, err := Render(link)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(got, "\n"), "\n")
	if len(lines) < 8 || len(lines) > 24 {
		t.Fatalf("height %d, want a small block", len(lines))
	}
	width := len([]rune(lines[0]))
	for _, line := range lines {
		if len([]rune(line)) != width {
			t.Fatalf("ragged row %d vs %d", len([]rune(line)), width)
		}
	}
	// Half-blocks: width in cells is about twice the line count, which is square on screen.
	if width < len(lines)*2-2 || width > len(lines)*2+2 {
		t.Fatalf("not square: %d wide by %d lines", width, len(lines))
	}
}
