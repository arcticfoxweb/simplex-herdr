//go:build windows

package qrterm

// TermCols is the terminal width, or 0 when it is not known.
func TermCols() int {
	return colsFromEnv()
}
