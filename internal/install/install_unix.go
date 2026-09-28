//go:build !windows

package install

import (
	"context"
	"io"
)

func prepareRuntime(context.Context, string, io.Writer) error { return nil }
