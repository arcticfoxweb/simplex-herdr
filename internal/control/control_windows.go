//go:build windows

package control

import (
	"context"
	"net"
	"time"

	"github.com/Microsoft/go-winio"
)

func dial(addr string, timeout time.Duration) (net.Conn, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return winio.DialPipeContext(ctx, addr)
}

func listen(addr string) (net.Listener, error) {
	return winio.ListenPipe(addr, nil)
}

func cleanup(string) {}
