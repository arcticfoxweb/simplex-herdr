// Package chat is a small client for the simplex-chat WebSocket API.
package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/coder/websocket"

	"simplex/internal/jutil"
)

type Client struct {
	conn *websocket.Conn

	mu      sync.Mutex
	cmdMu   sync.Mutex
	pending map[string]chan map[string]any
	next    atomic.Int64
}

func Dial(ctx context.Context, url string) (*Client, error) {
	conn, _, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		return nil, err
	}
	conn.SetReadLimit(8 * 1024 * 1024)
	return &Client{
		conn:    conn,
		pending: map[string]chan map[string]any{},
	}, nil
}

func (c *Client) Close() error {
	return c.conn.Close(websocket.StatusNormalClosure, "bye")
}

// Read demuxes command responses and events until the socket closes.
// onEvent is called without the read loop blocked, so it may send more commands.
func (c *Client) Read(ctx context.Context, onEvent func(map[string]any)) error {
	for {
		_, data, err := c.conn.Read(ctx)
		if err != nil {
			return err
		}
		var env struct {
			CorrID string          `json:"corrId"`
			Resp   json.RawMessage `json:"resp"`
		}
		if err := json.Unmarshal(data, &env); err != nil || len(env.Resp) == 0 {
			continue
		}
		if env.CorrID != "" {
			c.deliver(env.CorrID, env.Resp)
			continue
		}
		var event map[string]any
		if err := json.Unmarshal(env.Resp, &event); err != nil {
			continue
		}
		if onEvent != nil {
			go onEvent(event)
		}
	}
}

func (c *Client) deliver(id string, raw json.RawMessage) {
	var event map[string]any
	if err := json.Unmarshal(raw, &event); err != nil {
		return
	}
	c.mu.Lock()
	ch := c.pending[id]
	delete(c.pending, id)
	c.mu.Unlock()
	if ch != nil {
		ch <- event
	}
}

// Do sends one chat command and waits for the matching response.
// Commands are serialized because the CLI processes them one at a time.
func (c *Client) Do(ctx context.Context, cmd string) (map[string]any, error) {
	c.cmdMu.Lock()
	defer c.cmdMu.Unlock()

	id := fmt.Sprintf("%d", c.next.Add(1))
	ch := make(chan map[string]any, 1)
	c.mu.Lock()
	c.pending[id] = ch
	c.mu.Unlock()

	body, err := json.Marshal(map[string]string{"corrId": id, "cmd": cmd})
	if err != nil {
		c.forget(id)
		return nil, err
	}
	if err := c.conn.Write(ctx, websocket.MessageText, body); err != nil {
		c.forget(id)
		return nil, err
	}
	select {
	case <-ctx.Done():
		c.forget(id)
		return nil, ctx.Err()
	case resp := <-ch:
		if jutil.Type(resp) == "chatCmdError" {
			return nil, fmt.Errorf("%s", compact(resp["chatError"]))
		}
		return resp, nil
	}
}

func (c *Client) forget(id string) {
	c.mu.Lock()
	delete(c.pending, id)
	c.mu.Unlock()
}

func compact(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprint(v)
	}
	return string(b)
}
