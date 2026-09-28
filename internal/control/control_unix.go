//go:build !windows

package control

import (
	"net"
	"os"
	"time"
)

func dial(addr string, timeout time.Duration) (net.Conn, error) {
	return net.DialTimeout("unix", addr, timeout)
}

func listen(addr string) (net.Listener, error) {
	return net.Listen("unix", addr)
}

func cleanup(addr string) {
	_ = os.Remove(addr)
}
