// Package qrterm draws a QR code as text that stays small and square in a terminal.
package qrterm

import (
	"strings"

	"github.com/skip2/go-qrcode"
)

// Render returns a QR drawing of text. Each character is one column and two
// modules tall, so the block stays small and square. Modules are painted with
// explicit black and white: a dark terminal draws a plain block in the
// foreground color, which inverts the code, drops the quiet zone into the
// background, and will not scan.
func Render(text string) (string, error) {
	code, err := qrcode.New(text, qrcode.Medium)
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
		for x := 0; x < n; x++ {
			bot := false
			if y+1 < n {
				bot = bits[y+1][x]
			}
			writeCell(&b, bits[y][x], bot)
		}
		b.WriteString("\033[0m\n")
	}
	return b.String(), nil
}

// writeCell draws the top module in the foreground and the bottom module in
// the background of U+2580. Every cell is the same glyph, so a font that
// treats block characters as double-width cannot shear rows apart. 256-color
// is set first and truecolor after it, so a terminal that only understands
// one of them still gets real black (16 / #000) and white (231 / #fff)
// instead of the theme's remapped gray.
func writeCell(b *strings.Builder, topBlack, botBlack bool) {
	b.WriteString("\033[38;5;")
	b.WriteString(ansi256(topBlack))
	b.WriteString("m\033[48;5;")
	b.WriteString(ansi256(botBlack))
	b.WriteString("m\033[38;2;")
	b.WriteString(rgb(topBlack))
	b.WriteString("m\033[48;2;")
	b.WriteString(rgb(botBlack))
	b.WriteString("m▀")
}

func ansi256(black bool) string {
	if black {
		return "16"
	}
	return "231"
}

func rgb(black bool) string {
	if black {
		return "0;0;0"
	}
	return "255;255;255"
}
