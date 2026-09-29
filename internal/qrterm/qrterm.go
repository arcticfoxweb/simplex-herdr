// Package qrterm writes a QR code as a PNG. The encoder is compiled into simplex.
package qrterm

import "github.com/skip2/go-qrcode"

// PNG encodes text as a square PNG. Each module is 8 pixels, and the quiet
// zone is the 4-module white margin inside the image, so the margin survives
// being saved or pasted.
func PNG(text string) ([]byte, error) {
	code, err := qrcode.New(text, qrcode.Medium)
	if err != nil {
		return nil, err
	}
	return code.PNG(-8)
}
