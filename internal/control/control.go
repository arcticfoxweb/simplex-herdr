// Package control is the local socket between the simplex CLI and its daemon.
// Unix uses a filesystem socket. Windows uses a named pipe. Herdr plugins call
// the simplex CLI, so they do not have to know which one this is.
package control

import (
	"net"
	"time"
)

// Dial connects to the daemon listening on addr.
func Dial(addr string, timeout time.Duration) (net.Conn, error) {
	return dial(addr, timeout)
}

// Listen starts the daemon endpoint.
func Listen(addr string) (net.Listener, error) {
	return listen(addr)
}

// Cleanup removes a stale Unix socket. Named pipes have nothing to delete.
func Cleanup(addr string) {
	cleanup(addr)
}
