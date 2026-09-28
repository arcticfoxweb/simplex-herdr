// Package qrterm draws a QR code as text that stays small and square in a terminal.
package qrterm

import (
	"strings"

	"github.com/skip2/go-qrcode"
)

// Render returns a QR drawing of text. Each character covers two modules
// stacked vertically, so the block looks square instead of a tall page.
func Render(text string) (string, error) {
	code, err := qrcode.New(text, qrcode.Low)
	if err != nil {
		return "", err
	}
	bits := code.Bitmap()
	n := len(bits)
	if n == 0 {
		return "", nil
	}
	var b strings.Builder
	for y := 0; y < n; y += 2 {
		for x := 0; x < n; x++ {
			top := bits[y][x]
			bot := false
			if y+1 < n {
				bot = bits[y+1][x]
			}
			switch {
			case top && bot:
				b.WriteString("█")
			case top:
				b.WriteString("▀")
			case bot:
				b.WriteString("▄")
			default:
				b.WriteString(" ")
			}
		}
		b.WriteByte('\n')
	}
	return b.String(), nil
}
