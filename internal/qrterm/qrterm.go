// Package qrterm draws a QR code as text that stays small and square in a terminal.
package qrterm

import (
	"strings"

	"github.com/skip2/go-qrcode"
)

// One color setting for the whole line. Per-cell color codes make each row
// thousands of bytes long, the terminal wraps them, and the code will not scan.
// Foreground is white and background is black, so a dark terminal still shows
// dark modules as dark.
const lineColor = "\033[40;37;1m\033[38;5;231m\033[48;5;16m"

// Render returns a QR drawing of text. Each character covers two modules
// stacked vertically. The encoder is compiled into simplex; this does not
// shell out.
func Render(text string) (string, error) {
	code, err := qrcode.New(text, qrcode.Low)
	if err != nil {
		return "", err
	}
	bits := code.Bitmap()
	n := len(bits)
	if n == 0 || len(bits[0]) == 0 {
		return "", nil
	}
	var b strings.Builder
	for y := 0; y < n; y += 2 {
		b.WriteString(lineColor)
		for x := 0; x < n; x++ {
			bot := false
			if y+1 < n {
				bot = bits[y+1][x]
			}
			b.WriteString(glyph(bits[y][x], bot))
		}
		b.WriteString("\033[0m\n")
	}
	return b.String(), nil
}

// glyph uses a white foreground and a black background.
// true is a dark module.
func glyph(topBlack, botBlack bool) string {
	switch {
	case !topBlack && !botBlack:
		return "█"
	case !topBlack && botBlack:
		return "▀"
	case topBlack && !botBlack:
		return "▄"
	default:
		return " "
	}
}

// Cols reports how many terminal columns the first row occupies.
func Cols(rendered string) int {
	line, _, _ := strings.Cut(rendered, "\n")
	line = strings.TrimPrefix(line, lineColor)
	line = strings.TrimSuffix(line, "\033[0m")
	n := 0
	for _, r := range line {
		if r != '\033' {
			n++
		}
	}
	return n
}
