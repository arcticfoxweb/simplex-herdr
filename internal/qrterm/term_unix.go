//go:build unix

package qrterm

import (
	"os"

	"golang.org/x/sys/unix"
)

// TermCols is the terminal width, or 0 when stdout is not a terminal.
func TermCols() int {
	ws, err := unix.IoctlGetWinsize(int(os.Stdout.Fd()), unix.TIOCGWINSZ)
	if err == nil && ws.Col > 0 {
		return int(ws.Col)
	}
	return colsFromEnv()
}
