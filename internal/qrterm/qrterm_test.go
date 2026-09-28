package qrterm

import (
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
		if strings.Count(line, "\033[38;5;231m") != 1 {
			t.Fatalf("line %d sets color %d times", i, strings.Count(line, "\033[38;5;231m"))
		}
		if !strings.HasSuffix(line, "\033[0m") {
			t.Fatalf("line %d does not reset color", i)
		}
		// A few kilobytes of color codes per row is what makes the code wrap.
		if len(line) > width*4+80 {
			t.Fatalf("line %d is %d bytes for %d columns", i, len(line), width)
		}
		body := glyphs(t, line)
		if strings.Trim(body, " ") != body {
			t.Fatalf("line %d quiet zone is a space and can be trimmed", i)
		}
		if cells(t, line) != width {
			t.Fatalf("ragged row %d", i)
		}
	}
	if width < len(lines)*2-2 || width > len(lines)*2+2 {
		t.Fatalf("not square: %d wide by %d lines", width, len(lines))
	}
	if Cols(got) != width {
		t.Fatalf("Cols %d, cells %d", Cols(got), width)
	}
}

func TestRenderMatchesBitmap(t *testing.T) {
	const link = "https://smp6.simplex.im/a#aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	code, err := qrcode.New(link, qrcode.Low)
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
		tops, bots := moduleColors(t, glyphs(t, line))
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
	if !bits[4][4] {
		t.Fatal("expected a dark finder module")
	}
}

func glyphs(t *testing.T, line string) string {
	t.Helper()
	if !strings.HasPrefix(line, lineColor) {
		t.Fatalf("line missing color prefix")
	}
	body := strings.TrimPrefix(line, lineColor)
	body = strings.TrimSuffix(body, "\033[0m")
	if strings.Contains(body, "\033") {
		t.Fatalf("color code inside the modules: %q", body)
	}
	return body
}

func cells(t *testing.T, line string) int {
	t.Helper()
	n := len([]rune(glyphs(t, line)))
	if n == 0 {
		t.Fatal("no modules")
	}
	return n
}

func moduleColors(t *testing.T, body string) (top, bot []bool) {
	t.Helper()
	for _, r := range body {
		switch r {
		case '█':
			top = append(top, false)
			bot = append(bot, false)
		case '▀':
			top = append(top, false)
			bot = append(bot, true)
		case '▄':
			top = append(top, true)
			bot = append(bot, false)
		case ' ':
			top = append(top, true)
			bot = append(bot, true)
		default:
			t.Fatalf("unexpected module rune %q", r)
		}
	}
	return top, bot
}
