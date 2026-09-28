// Package rpc is the line protocol between the CLI, the MCP server, and the local daemon.
package rpc

import (
	"bufio"
	"encoding/json"
	"fmt"
	"time"

	"simplex/internal/control"
)

const MaxWaitSec = 120

type Request struct {
	ID      string   `json:"id"`
	Op      string   `json:"op"`
	To      string   `json:"to,omitempty"`
	Text    string   `json:"text,omitempty"`
	Link    string   `json:"link,omitempty"`
	Path    string   `json:"path,omitempty"`
	Limit   int      `json:"limit,omitempty"`
	All     bool     `json:"all,omitempty"`
	Ack     bool     `json:"ack,omitempty"`
	WaitSec int      `json:"waitSec,omitempty"`
	IDs     []string `json:"ids,omitempty"`
}

type Response struct {
	ID     string          `json:"id"`
	OK     bool            `json:"ok"`
	Error  string          `json:"error,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
}

func Call(sock string, req Request, timeout time.Duration) (Response, error) {
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	if req.WaitSec > 0 {
		extra := time.Duration(req.WaitSec)*time.Second + 15*time.Second
		if extra > timeout {
			timeout = extra
		}
	}
	conn, err := control.Dial(sock, 2*time.Second)
	if err != nil {
		return Response{}, fmt.Errorf("daemon is not running (%s)", err)
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(timeout)); err != nil {
		return Response{}, err
	}
	body, err := json.Marshal(req)
	if err != nil {
		return Response{}, err
	}
	if _, err := conn.Write(append(body, '\n')); err != nil {
		return Response{}, err
	}
	sc := bufio.NewScanner(conn)
	sc.Buffer(make([]byte, 64*1024), 4*1024*1024)
	if !sc.Scan() {
		if err := sc.Err(); err != nil {
			return Response{}, err
		}
		return Response{}, fmt.Errorf("daemon closed the connection")
	}
	var resp Response
	if err := json.Unmarshal(sc.Bytes(), &resp); err != nil {
		return Response{}, fmt.Errorf("bad daemon response: %w", err)
	}
	if !resp.OK {
		if resp.Error == "" {
			resp.Error = "unknown daemon error"
		}
		return resp, fmt.Errorf("%s", resp.Error)
	}
	return resp, nil
}
