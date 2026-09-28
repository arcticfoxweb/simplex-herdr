package qrterm

import (
	"regexp"
	"strings"
	"testing"

	"github.com/skip2/go-qrcode"
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
	width := cells(t, lines[0])
	for i, line := range lines {
		if !strings.HasSuffix(line, "\033[0m") {
			t.Fatalf("line %d does not reset color", i)
		}
		if strings.ContainsRune(line, ' ') {
			t.Fatalf("line %d has a plain space, so the quiet zone can be trimmed", i)
		}
		n := cells(t, line)
		if n != width {
			t.Fatalf("ragged row %d: %d vs %d", i, n, width)
		}
	}
	// Half-blocks: width in cells is about twice the line count, which is square on screen.
	if width < len(lines)*2-2 || width > len(lines)*2+2 {
		t.Fatalf("not square: %d wide by %d lines", width, len(lines))
	}
}

func TestRenderMatchesBitmap(t *testing.T) {
	const link = "https://smp6.simplex.im/a#aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	code, err := qrcode.New(link, qrcode.Medium)
	if err != nil {
		t.Fatal(err)
	}
	bits := code.Bitmap()
	got, err := Render(link)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(got, "\n"), "\n")
	if len(lines) != (len(bits)+1)/2 {
		t.Fatalf("lines %d for %d modules", len(lines), len(bits))
	}
	for i, line := range lines {
		tops, bots := moduleColors(t, line)
		y := i * 2
		if len(tops) != len(bits[y]) {
			t.Fatalf("row %d width %d, bitmap %d", y, len(tops), len(bits[y]))
		}
		for x := range tops {
			if tops[x] != bits[y][x] {
				t.Fatalf("top module (%d,%d) inverted or dropped", x, y)
			}
			if y+1 < len(bits) {
				if bots[x] != bits[y+1][x] {
					t.Fatalf("bottom module (%d,%d) inverted or dropped", x, y+1)
				}
			} else if bots[x] {
				t.Fatalf("padding row painted black at x %d", x)
			}
		}
	}
	// Finder origin sits inside the quiet zone and is a dark module.
	if !bits[4][4] {
		t.Fatal("expected a dark finder module")
	}
}

func cells(t *testing.T, line string) int {
	t.Helper()
	n := strings.Count(line, "▀")
	if n == 0 {
		t.Fatalf("no modules in %q", line)
	}
	return n
}

var cellRE = regexp.MustCompile(`\x1b\[38;2;(0;0;0|255;255;255)m\x1b\[48;2;(0;0;0|255;255;255)m▀`)

func moduleColors(t *testing.T, line string) (top, bot []bool) {
	t.Helper()
	matches := cellRE.FindAllStringSubmatch(line, -1)
	if len(matches) == 0 {
		t.Fatalf("no colored modules in line")
	}
	for _, m := range matches {
		top = append(top, m[1] == "0;0;0")
		bot = append(bot, m[2] == "0;0;0")
	}
	return top, bot
}
