package qrterm

import (
	"os"
	"strconv"
)

func colsFromEnv() int {
	n, err := strconv.Atoi(os.Getenv("COLUMNS"))
	if err != nil || n <= 0 {
		return 0
	}
	return n
}
