// Package qrterm renders QR codes in the terminal using Unicode block characters.
// Uses a pure-Go QR encoder with no external dependencies.
package qrterm

import (
	"fmt"
	"io"

	"github.com/skip2/go-qrcode"
)

// Print renders a QR code to the terminal.
func Print(w io.Writer, text string) error {
	qr, err := qrcode.New(text, qrcode.Medium)
	if err != nil {
		return fmt.Errorf("generate QR: %w", err)
	}

	bitmap := qr.Bitmap()
	rows := len(bitmap)

	// Use Unicode half-block characters: each char cell = 2 vertical modules
	// ▀ (upper half) = top black, bottom white
	// ▄ (lower half) = top white, bottom black
	// █ (full block) = both black
	// ' ' (space)    = both white

	// Add quiet zone
	for y := 0; y < rows; y += 2 {
		fmt.Fprint(w, "  ") // left margin
		for x := 0; x < len(bitmap[y]); x++ {
			top := bitmap[y][x]
			bot := false
			if y+1 < rows {
				bot = bitmap[y+1][x]
			}

			switch {
			case top && bot:
				fmt.Fprint(w, "█")
			case top && !bot:
				fmt.Fprint(w, "▀")
			case !top && bot:
				fmt.Fprint(w, "▄")
			default:
				fmt.Fprint(w, " ")
			}
		}
		fmt.Fprintln(w)
	}

	return nil
}
