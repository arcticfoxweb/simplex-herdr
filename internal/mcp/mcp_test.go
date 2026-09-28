package mcp

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"simplex/internal/rpc"
)

func TestMCPHandshakeAndCall(t *testing.T) {
	var got rpc.Request
	call := func(req rpc.Request) (rpc.Response, error) {
		got = req
		b, _ := json.Marshal(map[string]string{"to": req.To, "text": req.Text})
		return rpc.Response{OK: true, Result: b}, nil
	}
	in := strings.NewReader(strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"0"}}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"send","arguments":{"to":"bob","text":"hello"}}}`,
		"",
	}, "\n"))
	var out bytes.Buffer
	if err := Serve(in, &out, call); err != nil {
		t.Fatal(err)
	}
	dec := json.NewDecoder(&out)
	var init map[string]any
	if err := dec.Decode(&init); err != nil {
		t.Fatal(err)
	}
	result, _ := init["result"].(map[string]any)
	info, _ := result["serverInfo"].(map[string]any)
	if info["name"] != "simplex" {
		t.Fatalf("init = %v", init)
	}
	var list map[string]any
	if err := dec.Decode(&list); err != nil {
		t.Fatal(err)
	}
	tools, _ := list["result"].(map[string]any)["tools"].([]any)
	if len(tools) < 5 {
		t.Fatalf("tools = %v", tools)
	}
	var callResp map[string]any
	if err := dec.Decode(&callResp); err != nil {
		t.Fatal(err)
	}
	if got.Op != "send" || got.To != "bob" || got.Text != "hello" {
		t.Fatalf("daemon request = %+v", got)
	}
	content := callResp["result"].(map[string]any)["content"].([]any)
	text := content[0].(map[string]any)["text"].(string)
	if !strings.Contains(text, "bob") {
		t.Fatalf("tool text = %s", text)
	}
}

func TestMCPContentLength(t *testing.T) {
	call := func(req rpc.Request) (rpc.Response, error) {
		b, _ := json.Marshal(map[string]string{"ok": "yes"})
		return rpc.Response{OK: true, Result: b}, nil
	}
	body := `{"jsonrpc":"2.0","id":1,"method":"ping"}`
	in := strings.NewReader("Content-Length: " + strconv.Itoa(len(body)) + "\r\n\r\n" + body)
	var out bytes.Buffer
	if err := Serve(in, &out, call); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Content-Length:") || !strings.Contains(out.String(), `"id":1`) {
		t.Fatalf("framed response = %q", out.String())
	}
}
