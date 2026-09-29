package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"simplex/internal/profile"
)

const maxCapture = 256 * 1024

// Herdr drives the herdr CLI. A pane is idle when herdr reports the agent as
// idle or done, and the visible screen has stopped changing.
type Herdr struct{}

func ValidTarget(target string) error {
	if target == "" {
		return fmt.Errorf("herdr pane is empty")
	}
	if len(target) > 200 || strings.ContainsAny(target, "\r\n \t") {
		return fmt.Errorf("bad herdr pane %q", target)
	}
	return nil
}

// RememberHerdr resolves the herdr binary and stores the absolute path where
// a running daemon can read it. A detached daemon does not see a later PATH change.
func RememberHerdr() string {
	bin := resolveHerdr()
	if bin == "" {
		return ""
	}
	abs, err := filepath.Abs(bin)
	if err == nil {
		bin = abs
	}
	if err := os.MkdirAll(profile.Root(), 0o700); err != nil {
		return bin
	}
	_ = os.WriteFile(herdrPathFile(), []byte(bin+"\n"), 0o600)
	return bin
}

func herdrBin() string {
	if p := resolveHerdr(); p != "" {
		return p
	}
	return "herdr"
}

func resolveHerdr() string {
	if p := strings.TrimSpace(os.Getenv("HERDR_BIN_PATH")); p != "" && fileExists(p) {
		return p
	}
	if b, err := os.ReadFile(herdrPathFile()); err == nil {
		if p := strings.TrimSpace(string(b)); p != "" && fileExists(p) {
			return p
		}
	}
	if p, err := exec.LookPath("herdr"); err == nil {
		return p
	}
	if runtime.GOOS == "windows" {
		if p, err := exec.LookPath("herdr.exe"); err == nil {
			return p
		}
	}
	for _, p := range herdrCandidates() {
		if fileExists(p) {
			return p
		}
	}
	return ""
}

func herdrPathFile() string {
	return filepath.Join(profile.Root(), "herdr.path")
}

func herdrCandidates() []string {
	home, _ := os.UserHomeDir()
	if runtime.GOOS == "windows" {
		local := os.Getenv("LOCALAPPDATA")
		return []string{
			filepath.Join(local, "Programs", "Herdr", "bin", "herdr.exe"),
			filepath.Join(home, ".herdr", "packages", "standalone", "current", "herdr.exe"),
		}
	}
	return []string{
		filepath.Join(home, ".local", "bin", "herdr"),
		"/usr/local/bin/herdr",
	}
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

func (Herdr) Capture(target string) (Snap, error) {
	if err := ValidTarget(target); err != nil {
		return Snap{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	statusOut, err := exec.CommandContext(ctx, herdrBin(), "agent", "get", target).Output()
	if err != nil {
		return Snap{}, fmt.Errorf("herdr agent get %s: %w", target, err)
	}
	status, err := parseAgentStatus(statusOut)
	if err != nil {
		return Snap{}, err
	}
	ctx2, cancel2 := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel2()
	read := exec.CommandContext(ctx2, herdrBin(), "pane", "read", "--source", "visible", "--format", "text", "--lines", "200", target)
	out, err := read.Output()
	if err != nil {
		return Snap{}, fmt.Errorf("herdr pane read %s: %w", target, err)
	}
	if len(out) > maxCapture {
		out = out[len(out)-maxCapture:]
	}
	return Snap{
		InMode: !agentIdle(status),
		Text:   string(out),
		At:     time.Now(),
	}, nil
}

func (Herdr) Submit(target, text string) error {
	if err := ValidTarget(target); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, herdrBin(), "agent", "prompt", target, text)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(buf.String())
		if msg == "" {
			return fmt.Errorf("herdr agent prompt %s: %w", target, err)
		}
		return fmt.Errorf("herdr agent prompt %s: %w: %s", target, err, msg)
	}
	return nil
}

func agentIdle(status string) bool {
	switch status {
	case "idle", "done":
		return true
	default:
		return false
	}
}

func parseAgentStatus(out []byte) (string, error) {
	for _, line := range bytes.Split(out, []byte("\n")) {
		line = bytes.TrimSpace(line)
		if len(line) == 0 || line[0] != '{' {
			continue
		}
		var body struct {
			Result struct {
				Agent struct {
					Status string `json:"agent_status"`
				} `json:"agent"`
			} `json:"result"`
		}
		if json.Unmarshal(line, &body) != nil {
			continue
		}
		if body.Result.Agent.Status != "" {
			return body.Result.Agent.Status, nil
		}
	}
	snippet := strings.TrimSpace(string(out))
	if len(snippet) > 200 {
		snippet = snippet[:200]
	}
	return "", fmt.Errorf("herdr agent get: no agent_status in %q", snippet)
}
