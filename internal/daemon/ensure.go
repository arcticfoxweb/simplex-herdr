package daemon

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"simplex/internal/osutil"
	"simplex/internal/profile"
	"simplex/internal/rpc"
)

// Ensure starts the profile daemon if it is not already running and waits until it is ready.
func Ensure(p profile.Paths) error {
	if _, err := rpc.Call(p.Sock, rpc.Request{Op: "who"}, 2*time.Second); err != nil {
		exe, err := os.Executable()
		if err != nil {
			return err
		}
		cmd := exec.Command(exe, "--profile", p.Name, "up")
		cmd.Env = os.Environ()
		osutil.Detach(cmd)
		devnull, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
		if err != nil {
			return err
		}
		defer devnull.Close()
		cmd.Stdin = devnull
		cmd.Stdout = devnull
		cmd.Stderr = devnull
		if err := cmd.Start(); err != nil {
			return err
		}
		_ = cmd.Process.Release()
	}
	return WaitReady(p.Sock, 90*time.Second)
}

// WaitReady polls who until status is running, or returns a fatal setup error early.
func WaitReady(sock string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	last := "timed out waiting for simplex"
	for time.Now().Before(deadline) {
		resp, err := rpc.Call(sock, rpc.Request{Op: "who"}, 3*time.Second)
		if err != nil {
			last = err.Error()
		} else {
			var m map[string]any
			if json.Unmarshal(resp.Result, &m) == nil {
				switch m["status"] {
				case "running":
					return nil
				case "error":
					if s, _ := m["error"].(string); s != "" {
						if strings.Contains(s, "not installed") || missingChatDLL(s) {
							return fmt.Errorf("%s", s)
						}
						last = s
					}
				default:
					if s, _ := m["error"].(string); s != "" {
						last = s
					} else if status, _ := m["status"].(string); status != "" {
						last = status
					}
				}
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	return fmt.Errorf("%s", last)
}

// missingChatDLL is Windows STATUS_DLL_NOT_FOUND. Retrying will not find the DLL.
func missingChatDLL(s string) bool {
	low := strings.ToLower(s)
	return strings.Contains(low, "c0000135") || strings.Contains(s, "3221225781")
}
