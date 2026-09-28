// Package mcp serves the simplex tools over stdio for CLI agents.
package mcp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"

	"simplex/internal/rpc"
)

const version = "0.1.0-alpha.1"

// Caller runs one daemon operation.
type Caller func(rpc.Request) (rpc.Response, error)

type tool struct {
	Name        string
	Description string
	Schema      map[string]any
	Call        func(args map[string]any) rpc.Request
}

func tools() []tool {
	return []tool{
		{
			Name:        "address",
			Description: "Return this agent's SimpleX contact address. Give it to another agent so they can connect.",
			Schema:      objSchema(nil),
			Call:        func(map[string]any) rpc.Request { return rpc.Request{Op: "address"} },
		},
		{
			Name:        "connect",
			Description: "Connect to another agent using their SimpleX address or one-time invitation. Connecting can take several seconds. Check contacts before sending.",
			Schema:      objSchema([]string{"link"}, prop("link", "SimpleX contact address or invitation link")),
			Call: func(args map[string]any) rpc.Request {
				return rpc.Request{Op: "connect", Link: str(args, "link")}
			},
		},
		{
			Name:        "contacts",
			Description: "List SimpleX contacts and whether each connection is ready for messages.",
			Schema:      objSchema(nil),
			Call:        func(map[string]any) rpc.Request { return rpc.Request{Op: "contacts"} },
		},
		{
			Name:        "send",
			Description: "Send a text message to a connected contact or a group by name. Prefix a group with # when it shares a name with a contact.",
			Schema: objSchema([]string{"to", "text"},
				prop("to", "Contact or group name"),
				prop("text", "Message text"),
			),
			Call: func(args map[string]any) rpc.Request {
				return rpc.Request{Op: "send", To: str(args, "to"), Text: str(args, "text")}
			},
		},
		{
			Name:        "send_file",
			Description: "Send a local file to a contact or group. Optional caption is the message text. The other side receives a local file path when the download finishes.",
			Schema: objSchema([]string{"to", "path"},
				prop("to", "Contact or group name"),
				prop("path", "Local file to send"),
				prop("caption", "Optional message text sent with the file"),
			),
			Call: func(args map[string]any) rpc.Request {
				return rpc.Request{Op: "send_file", To: str(args, "to"), Path: str(args, "path"), Text: str(args, "caption")}
			},
		},
		{
			Name:        "inbox",
			Description: "Read messages and files received from other agents. delivered=true means the text was already typed into this terminal. Reply once with send, then ack the id. Use wait_seconds to block until something is unread.",
			Schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"limit":        map[string]any{"type": "integer", "description": "Maximum messages to return"},
					"include_read": map[string]any{"type": "boolean", "description": "Include messages already acked"},
					"ack":          map[string]any{"type": "boolean", "description": "Mark the returned messages as read"},
					"wait_seconds": map[string]any{"type": "integer", "description": "Block until a message arrives, up to this many seconds"},
				},
			},
			Call: func(args map[string]any) rpc.Request {
				return rpc.Request{
					Op:      "inbox",
					Limit:   intArg(args, "limit"),
					All:     boolArg(args, "include_read"),
					Ack:     boolArg(args, "ack"),
					WaitSec: intArg(args, "wait_seconds"),
				}
			},
		},
		{
			Name:        "ack",
			Description: "Mark inbox message ids as read so they are not returned again.",
			Schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"ids": map[string]any{
						"type":        "array",
						"items":       map[string]any{"type": "string"},
						"description": "Message ids to mark read",
					},
				},
				"required": []string{"ids"},
			},
			Call: func(args map[string]any) rpc.Request {
				var ids []string
				switch raw := args["ids"].(type) {
				case []any:
					for _, v := range raw {
						if s, ok := v.(string); ok {
							ids = append(ids, s)
						}
					}
				}
				return rpc.Request{Op: "ack", IDs: ids}
			},
		},
	}
}

// Serve reads MCP JSON-RPC from r and writes responses to w.
func Serve(r io.Reader, w io.Writer, call Caller) error {
	in := bufio.NewReader(r)
	for {
		payload, framing, err := readMsg(in)
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
		if len(bytes.TrimSpace(payload)) == 0 {
			continue
		}
		var req map[string]any
		if err := json.Unmarshal(payload, &req); err != nil {
			b, _ := json.Marshal(errResult(nil, -32700, "parse error"))
			_ = writeMsg(w, framing, b)
			continue
		}
		resp, notify := dispatch(req, call)
		if notify {
			continue
		}
		body, err := json.Marshal(resp)
		if err != nil {
			return err
		}
		if err := writeMsg(w, framing, body); err != nil {
			return err
		}
	}
}

func dispatch(req map[string]any, call Caller) (resp map[string]any, notify bool) {
	method, _ := req["method"].(string)
	id := req["id"]
	params, _ := req["params"].(map[string]any)
	if _, ok := req["id"]; !ok {
		notify = true
	}
	switch method {
	case "initialize":
		proto := "2024-11-05"
		if p, _ := params["protocolVersion"].(string); p == "2024-11-05" || p == "2025-03-26" || p == "2025-06-18" {
			proto = p
		}
		return result(id, map[string]any{
			"protocolVersion": proto,
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": "simplex", "version": version},
		}), false
	case "notifications/initialized", "notifications/cancelled":
		return nil, true
	case "ping", "logging/setLevel":
		if notify {
			return nil, true
		}
		return result(id, map[string]any{}), false
	case "tools/list":
		var listed []map[string]any
		for _, t := range tools() {
			listed = append(listed, map[string]any{
				"name":        t.Name,
				"description": t.Description,
				"inputSchema": t.Schema,
			})
		}
		return result(id, map[string]any{"tools": listed}), false
	case "tools/call":
		name, _ := params["name"].(string)
		args, _ := params["arguments"].(map[string]any)
		if args == nil {
			args = map[string]any{}
		}
		var found *tool
		for i := range tools() {
			if tools()[i].Name == name {
				t := tools()[i]
				found = &t
				break
			}
		}
		if found == nil {
			return result(id, toolText(fmt.Sprintf("unknown tool %q", name), true)), false
		}
		daemonResp, err := call(found.Call(args))
		if err != nil {
			return result(id, toolText(err.Error(), true)), false
		}
		pretty := string(daemonResp.Result)
		var decoded any
		if json.Unmarshal(daemonResp.Result, &decoded) == nil {
			if b, err := json.MarshalIndent(decoded, "", "  "); err == nil {
				pretty = string(b)
			}
		}
		return result(id, toolText(pretty, false)), false
	default:
		if notify {
			return nil, true
		}
		return errResult(id, -32601, "method not found"), false
	}
}

func toolText(text string, isErr bool) map[string]any {
	out := map[string]any{
		"content": []map[string]any{{"type": "text", "text": text}},
	}
	if isErr {
		out["isError"] = true
	}
	return out
}

func result(id any, v any) map[string]any {
	return map[string]any{"jsonrpc": "2.0", "id": id, "result": v}
}

func errResult(id any, code int, msg string) map[string]any {
	return map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"error":   map[string]any{"code": code, "message": msg},
	}
}

func readMsg(r *bufio.Reader) ([]byte, string, error) {
	b, err := r.Peek(1)
	if err != nil {
		return nil, "", err
	}
	if b[0] == '{' {
		line, err := r.ReadBytes('\n')
		return bytes.TrimSpace(line), "line", err
	}
	var length int
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return nil, "", err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		if strings.HasPrefix(strings.ToLower(line), "content-length:") {
			length, _ = strconv.Atoi(strings.TrimSpace(line[len("content-length:"):]))
		}
	}
	if length < 0 || length > 8*1024*1024 {
		return nil, "", fmt.Errorf("bad content length")
	}
	buf := make([]byte, length)
	_, err = io.ReadFull(r, buf)
	return buf, "cl", err
}

func writeMsg(w io.Writer, framing string, payload []byte) error {
	if framing == "cl" {
		_, err := fmt.Fprintf(w, "Content-Length: %d\r\n\r\n%s", len(payload), payload)
		return err
	}
	_, err := w.Write(append(payload, '\n'))
	return err
}

func objSchema(required []string, props ...map[string]any) map[string]any {
	properties := map[string]any{}
	for _, p := range props {
		name, _ := p["name"].(string)
		delete(p, "name")
		properties[name] = p
	}
	s := map[string]any{"type": "object", "properties": properties}
	if len(required) > 0 {
		s["required"] = required
	}
	return s
}

func prop(name, desc string) map[string]any {
	return map[string]any{"name": name, "type": "string", "description": desc}
}

func str(args map[string]any, key string) string {
	s, _ := args[key].(string)
	return s
}

func boolArg(args map[string]any, key string) bool {
	b, _ := args[key].(bool)
	return b
}

func intArg(args map[string]any, key string) int {
	switch v := args[key].(type) {
	case float64:
		return int(v)
	case int:
		return v
	case string:
		n, _ := strconv.Atoi(v)
		return n
	default:
		return 0
	}
}
