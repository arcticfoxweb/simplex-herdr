package daemon

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"simplex/internal/chattest"
	"simplex/internal/profile"
	"simplex/internal/rpc"
)

func TestRoundTrip(t *testing.T) {
	sock, srv, _ := startDaemon(t, 0)
	resp, err := rpc.Call(sock, rpc.Request{Op: "address"}, 3*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	var addr struct {
		Address string `json:"address"`
	}
	if err := json.Unmarshal(resp.Result, &addr); err != nil {
		t.Fatal(err)
	}
	if addr.Address != "https://simplex.chat/contact#test-alice" {
		t.Fatalf("address = %s", addr.Address)
	}

	resp, err = rpc.Call(sock, rpc.Request{Op: "contacts"}, 3*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(resp.Result), "bob") {
		t.Fatalf("contacts = %s", resp.Result)
	}

	if _, err := rpc.Call(sock, rpc.Request{Op: "send", To: "bob", Text: "hi"}, 3*time.Second); err != nil {
		t.Fatal(err)
	}
	if _, err := rpc.Call(sock, rpc.Request{Op: "send", To: "Tangled Development", Text: "hi group"}, 3*time.Second); err != nil {
		t.Fatal(err)
	}
	if _, err := rpc.Call(sock, rpc.Request{Op: "connect", Link: "https://simplex.chat/contact#bob"}, 3*time.Second); err != nil {
		t.Fatal(err)
	}
	waitCmd(t, srv, "/_send @2")
	waitCmd(t, srv, "/_send #1")
	waitCmd(t, srv, "/connect ")

	srv.Push(map[string]any{
		"type": "newChatItems",
		"chatItems": []any{map[string]any{
			"chatInfo": map[string]any{
				"type":    "direct",
				"contact": map[string]any{"contactId": 2, "localDisplayName": "bob"},
			},
			"chatItem": map[string]any{
				"chatDir": map[string]any{"type": "directRcv"},
				"meta":    map[string]any{"itemId": 10, "itemTs": "2026-09-27T00:00:00Z", "itemText": "hello"},
				"content": map[string]any{"type": "rcvMsgContent", "msgContent": map[string]any{"type": "text", "text": "hello"}},
				"file":    map[string]any{"fileId": 9, "fileName": "note.txt", "fileStatus": "new"},
			},
		}},
	})
	waitCmd(t, srv, "/freceive 9")
	srv.Push(map[string]any{
		"type": "rcvFileComplete",
		"chatItem": map[string]any{
			"chatInfo": map[string]any{
				"type":    "direct",
				"contact": map[string]any{"contactId": 2, "localDisplayName": "bob"},
			},
			"chatItem": map[string]any{
				"chatDir": map[string]any{"type": "directRcv"},
				"meta":    map[string]any{"itemId": 10, "itemTs": "2026-09-27T00:00:00Z", "itemText": "hello"},
				"file":    map[string]any{"fileId": 9, "fileName": "note.txt", "fileStatus": "complete", "filePath": "/tmp/note.txt"},
			},
		},
	})
	waitInbox(t, sock, "hello", "/tmp/note.txt")

	if _, err := rpc.Call(sock, rpc.Request{Op: "inbox", Ack: true}, 3*time.Second); err != nil {
		t.Fatal(err)
	}
	resp, err = rpc.Call(sock, rpc.Request{Op: "inbox"}, 3*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	var box struct {
		Messages []any `json:"messages"`
	}
	if err := json.Unmarshal(resp.Result, &box); err != nil {
		t.Fatal(err)
	}
	if len(box.Messages) != 0 {
		t.Fatalf("inbox after ack = %s", resp.Result)
	}
}

func TestGatewayTypesIntoIdlePane(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "herdr.log")
	script := filepath.Join(dir, "herdr")
	body := `#!/bin/sh
echo "$*" >> "$HERDR_LOG"
if [ "$1" = agent ] && [ "$2" = get ]; then
  printf '%s\n' '{"result":{"agent":{"agent_status":"idle"}}}'
  exit 0
fi
if [ "$1" = pane ] && [ "$2" = read ]; then
  printf 'agent ready\n'
  exit 0
fi
if [ "$1" = agent ] && [ "$2" = prompt ]; then
  shift 3
  printf '%s\n' "$*" >> "$HERDR_LOG"
  exit 0
fi
exit 1
`
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+":/usr/bin:/bin")
	t.Setenv("HERDR_BIN_PATH", script)
	t.Setenv("HERDR_LOG", logPath)

	sock, srv, paths := startDaemon(t, 100*time.Millisecond)
	if err := paths.Update(func(m *profile.Meta) { m.Pane = "w1:p1" }); err != nil {
		t.Fatal(err)
	}
	srv.Push(map[string]any{
		"type": "newChatItems",
		"chatItems": []any{map[string]any{
			"chatInfo": map[string]any{
				"type":    "direct",
				"contact": map[string]any{"contactId": 2, "localDisplayName": "bob"},
			},
			"chatItem": map[string]any{
				"chatDir": map[string]any{"type": "directRcv"},
				"meta":    map[string]any{"itemId": 10, "itemTs": "2026-09-27T00:00:00Z", "itemText": "hello"},
				"content": map[string]any{"type": "rcvMsgContent", "msgContent": map[string]any{"type": "text", "text": "hello"}},
			},
		}},
	})

	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		b, _ := os.ReadFile(logPath)
		if strings.Contains(string(b), "bob says [msg:direct:2:10]: hello") {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	b, _ := os.ReadFile(logPath)
	t.Fatalf("pane was not given the message\nlog:\n%s\ndaemon:\n%s", b, readLog(t, paths.Log))
	_ = sock
}

func startDaemon(t *testing.T, quiet time.Duration) (string, *chattest.Server, profile.Paths) {
	t.Helper()
	srv := chattest.Start()
	t.Cleanup(srv.Close)
	t.Setenv("SIMPLEX_HOME", t.TempDir())
	paths, err := profile.For("alice")
	if err != nil {
		t.Fatal(err)
	}
	if err := paths.Ensure(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		_ = Run(ctx, Config{Paths: paths, WSURL: srv.URL, Quiet: quiet})
		close(done)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Error("daemon did not stop")
		}
	})
	waitRunning(t, paths.Sock, paths.Log)
	return paths.Sock, srv, paths
}

func waitRunning(t *testing.T, sock, logPath string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	var last string
	for time.Now().Before(deadline) {
		resp, err := rpc.Call(sock, rpc.Request{Op: "who"}, 2*time.Second)
		if err != nil {
			last = err.Error()
		} else {
			var m map[string]any
			_ = json.Unmarshal(resp.Result, &m)
			if m["status"] == "running" {
				return
			}
			last, _ = m["error"].(string)
			if last == "" {
				last, _ = m["status"].(string)
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("daemon not running: %s\nlog:\n%s", last, readLog(t, logPath))
}

func waitCmd(t *testing.T, srv *chattest.Server, prefix string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		for _, cmd := range srv.Commands() {
			if strings.HasPrefix(cmd, prefix) {
				return
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("missing %s in %v", prefix, srv.Commands())
}

func waitInbox(t *testing.T, sock, text, path string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	var last string
	for time.Now().Before(deadline) {
		resp, err := rpc.Call(sock, rpc.Request{Op: "inbox", All: true, Limit: 20}, 2*time.Second)
		if err != nil {
			last = err.Error()
		} else {
			last = string(resp.Result)
			var body struct {
				Messages []struct {
					Text     string `json:"text"`
					FilePath string `json:"filePath"`
				} `json:"messages"`
			}
			_ = json.Unmarshal(resp.Result, &body)
			for _, m := range body.Messages {
				if m.Text == text && m.FilePath == path {
					return
				}
			}
		}
		time.Sleep(30 * time.Millisecond)
	}
	t.Fatalf("inbox missing %q %q\nlast: %s", text, path, last)
}

func readLog(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		return err.Error()
	}
	return string(b)
}
