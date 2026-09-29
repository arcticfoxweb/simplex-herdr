package daemon

import (
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
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

func TestSendImageUsesPreview(t *testing.T) {
	sock, srv, paths := startDaemon(t, 0)
	img := filepath.Join(paths.Dir, "pic.png")
	writeTinyPNG(t, img)
	if _, err := rpc.Call(sock, rpc.Request{Op: "send_file", To: "bob", Path: img}, 5*time.Second); err != nil {
		t.Fatal(err)
	}
	cmd := waitCmdContains(t, srv, `"type":"image"`)
	if !strings.Contains(cmd, `"text":""`) || !strings.Contains(cmd, `"image":"`) || !strings.Contains(cmd, img) {
		t.Fatalf("image send = %s", cmd)
	}
	if strings.Contains(cmd, `"text":"pic.png"`) {
		t.Fatalf("empty caption was replaced with the file name: %s", cmd)
	}

	notes := filepath.Join(paths.Dir, "notes.txt")
	if err := os.WriteFile(notes, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := rpc.Call(sock, rpc.Request{Op: "send_file", To: "bob", Path: notes}, 5*time.Second); err != nil {
		t.Fatal(err)
	}
	textCmd := waitCmdContains(t, srv, notes)
	if !strings.Contains(textCmd, `"type":"text"`) || !strings.Contains(textCmd, `"text":"notes.txt"`) {
		t.Fatalf("file row send = %s", textCmd)
	}
	if strings.Contains(textCmd, `"type":"image"`) {
		t.Fatalf("non-image was sent as an image: %s", textCmd)
	}
}

func TestFileReceiveUsesProfilePath(t *testing.T) {
	sock, srv, paths := startDaemon(t, 0)
	pushFile(srv, 77, 19, "voice.ogg")
	cmd := waitCmdContains(t, srv, "/freceive 77 approved_relays=on")
	if !strings.Contains(cmd, filepath.Join(paths.Files, "voice.ogg")) {
		t.Fatalf("receive = %s", cmd)
	}
	_ = sock
}

func TestFileAlreadyReceivingWaits(t *testing.T) {
	sock, srv, _ := startDaemon(t, 0)
	pushFile(srv, 78, 20, "voice.ogg")
	waitCmdContains(t, srv, "/freceive 78 approved_relays=on")
	time.Sleep(300 * time.Millisecond)
	for _, cmd := range srv.Commands() {
		if strings.Contains(cmd, "/fcancel") {
			t.Fatalf("already receiving was cancelled: %v", srv.Commands())
		}
	}
	_ = sock
}

func TestFileStaysOutOfPaneUntilComplete(t *testing.T) {
	logPath := writeFakeHerdr(t)
	sock, srv, paths := startDaemon(t, 0)
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
				"meta":    map[string]any{"itemId": 21, "itemTs": "2026-09-27T00:00:00Z", "itemText": "voice note"},
				"content": map[string]any{"type": "rcvMsgContent", "msgContent": map[string]any{"type": "text", "text": "voice note"}},
				"file": map[string]any{
					"fileId":     9,
					"fileName":   "note.txt",
					"fileStatus": map[string]any{"type": "rcvInvitation"},
				},
			},
		}},
	})
	waitCmd(t, srv, "/freceive 9")
	time.Sleep(400 * time.Millisecond)
	if b, _ := os.ReadFile(logPath); strings.Contains(string(b), "voice note") {
		t.Fatalf("invitation was typed before rcvComplete:\n%s", b)
	}
	srv.Push(map[string]any{
		"type": "rcvFileComplete",
		"chatItem": map[string]any{
			"chatInfo": map[string]any{
				"type":    "direct",
				"contact": map[string]any{"contactId": 2, "localDisplayName": "bob"},
			},
			"chatItem": map[string]any{
				"chatDir": map[string]any{"type": "directRcv"},
				"meta":    map[string]any{"itemId": 21, "itemTs": "2026-09-27T00:00:00Z", "itemText": "voice note"},
				"content": map[string]any{"type": "rcvMsgContent", "msgContent": map[string]any{"type": "text", "text": "voice note"}},
				"file":    map[string]any{"fileId": 9, "fileName": "note.txt", "fileStatus": map[string]any{"type": "rcvComplete"}, "filePath": "/tmp/note.txt"},
			},
		},
	})
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		b, _ := os.ReadFile(logPath)
		if strings.Contains(string(b), "voice note") && strings.Contains(string(b), "file: /tmp/note.txt") {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	b, _ := os.ReadFile(logPath)
	t.Fatalf("completed file was not typed\nlog:\n%s\ndaemon:\n%s", b, readLog(t, paths.Log))
	_ = sock
}

func TestGroupsJoinAndInvite(t *testing.T) {
	sock, srv, _ := startDaemon(t, 0)
	resp, err := rpc.Call(sock, rpc.Request{Op: "groups"}, 3*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(resp.Result), `"name":"Tangled Development"`) || !strings.Contains(string(resp.Result), `"status":"member"`) {
		t.Fatalf("groups = %s", resp.Result)
	}
	if _, err := rpc.Call(sock, rpc.Request{Op: "join", To: "Tangled Development"}, 3*time.Second); err != nil {
		t.Fatal(err)
	}
	waitCmd(t, srv, "/_join #1")
	if _, err := rpc.Call(sock, rpc.Request{Op: "join", To: "#1"}, 3*time.Second); err != nil {
		t.Fatal(err)
	}

	srv.Push(map[string]any{
		"type":    "receivedGroupInvitation",
		"contact": map[string]any{"localDisplayName": "tangled"},
		"groupInfo": map[string]any{
			"groupId":          4,
			"localDisplayName": "Agents",
		},
	})
	waitInbox(t, sock, `group invite Agents. Join with: simplex join "Agents"`, "")
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

func waitCmdContains(t *testing.T, srv *chattest.Server, part string) string {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		for _, cmd := range srv.Commands() {
			if strings.Contains(cmd, part) {
				return cmd
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("missing %s in %v", part, srv.Commands())
	return ""
}

func pushFile(srv *chattest.Server, fileID, itemID int64, name string) {
	srv.Push(map[string]any{
		"type": "newChatItems",
		"chatItems": []any{map[string]any{
			"chatInfo": map[string]any{
				"type":    "direct",
				"contact": map[string]any{"contactId": 2, "localDisplayName": "bob"},
			},
			"chatItem": map[string]any{
				"chatDir": map[string]any{"type": "directRcv"},
				"meta":    map[string]any{"itemId": itemID, "itemTs": "2026-09-27T00:00:00Z", "itemText": "voice"},
				"content": map[string]any{"type": "rcvMsgContent", "msgContent": map[string]any{"type": "text", "text": "voice"}},
				"file": map[string]any{
					"fileId":     fileID,
					"fileName":   name,
					"fileStatus": map[string]any{"type": "rcvInvitation"},
				},
			},
		}},
	})
}

func writeTinyPNG(t *testing.T, path string) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 200, G: 10, B: 10, A: 255})
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}

func writeFakeHerdr(t *testing.T) string {
	t.Helper()
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
	return logPath
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
