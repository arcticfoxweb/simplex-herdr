package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"simplex/internal/inbox"
	"simplex/internal/jutil"
	"simplex/internal/profile"
	"simplex/internal/rpc"
)

func (d *Daemon) handle(req rpc.Request) rpc.Response {
	if req.WaitSec > rpc.MaxWaitSec {
		req.WaitSec = rpc.MaxWaitSec
	}
	if req.WaitSec < 0 {
		req.WaitSec = 0
	}
	var result any
	var err error
	switch req.Op {
	case "who":
		result = d.who()
	case "shutdown":
		go func() {
			time.Sleep(30 * time.Millisecond)
			if d.cancel != nil {
				d.cancel()
			}
		}()
		result = map[string]string{"status": "stopping"}
	case "address":
		result, err = d.opAddress()
	case "connect":
		result, err = d.opConnect(req.Link)
	case "contacts":
		result, err = d.opContacts()
	case "send":
		result, err = d.opSend(req.To, req.Text, "")
	case "send_file":
		result, err = d.opSend(req.To, req.Text, req.Path)
	case "inbox":
		result, err = d.opInbox(req)
	case "ack":
		n := d.box.Ack(req.IDs)
		result = map[string]any{"acked": n}
	default:
		err = fmt.Errorf("unknown op %q", req.Op)
	}
	resp := rpc.Response{ID: req.ID, OK: err == nil}
	if err != nil {
		resp.Error = err.Error()
		return resp
	}
	b, mErr := json.Marshal(result)
	if mErr != nil {
		return rpc.Response{ID: req.ID, OK: false, Error: mErr.Error()}
	}
	resp.Result = b
	return resp
}

func (d *Daemon) who() map[string]any {
	d.mu.Lock()
	status := d.status
	statusErr := d.statusErr
	gatewayErr := d.gatewayErr
	addr := d.address
	uid := d.userID
	d.mu.Unlock()
	meta := d.paths.LoadMeta()
	if addr == "" {
		addr = meta.Address
	}
	if status == "" {
		status = "starting"
	}
	now := time.Now()
	return map[string]any{
		"profile":      d.paths.Name,
		"status":       status,
		"address":      addr,
		"userId":       uid,
		"unread":       d.box.Unread(),
		"pending":      d.box.Pending(now),
		"pane":         meta.Target(),
		"error":        statusErr,
		"gatewayError": gatewayErr,
		"socket":       d.paths.Sock,
	}
}

func (d *Daemon) ready() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.status == "running" && d.client != nil {
		return nil
	}
	if d.statusErr != "" {
		return fmt.Errorf("%s", d.statusErr)
	}
	return fmt.Errorf("simplex is still starting")
}

func (d *Daemon) userIDNow() int64 {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.userID
}

func (d *Daemon) opAddress() (any, error) {
	d.mu.Lock()
	addr := d.address
	uid := d.userID
	running := d.status == "running" && d.client != nil
	d.mu.Unlock()
	var shortAddr string
	if running && uid != 0 {
		if shown, err := d.do(context.Background(), fmt.Sprintf("/_show_address %d", uid), 15*time.Second); err == nil {
			full, short := linksFrom(shown)
			if full != "" {
				addr = full
			}
			if short != "" {
				shortAddr = short
			}
			if full != "" || short != "" {
				_ = d.paths.Update(func(m *profile.Meta) {
					if full != "" {
						m.Address = full
					}
					if short != "" {
						m.Short = short
					}
				})
			}
		}
	}
	meta := d.paths.LoadMeta()
	if addr == "" {
		addr = meta.Address
	}
	if shortAddr == "" {
		shortAddr = meta.Short
	}
	if addr == "" && shortAddr == "" {
		if err := d.ready(); err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("address is not ready")
	}
	return map[string]any{"profile": d.paths.Name, "address": addr, "short": shortAddr}, nil
}

func (d *Daemon) opConnect(link string) (any, error) {
	if err := d.ready(); err != nil {
		return nil, err
	}
	link = strings.TrimSpace(link)
	if link == "" || strings.ContainsAny(link, " \r\n") {
		return nil, fmt.Errorf("connect needs one simplex link")
	}
	resp, err := d.do(context.Background(), "/connect "+link, 90*time.Second)
	if err != nil {
		return nil, err
	}
	return map[string]any{"status": jutil.Type(resp), "link": link}, nil
}

func (d *Daemon) opContacts() (any, error) {
	if err := d.ready(); err != nil {
		return nil, err
	}
	uid := d.userIDNow()
	resp, err := d.do(context.Background(), fmt.Sprintf("/_contacts %d", uid), 15*time.Second)
	if err != nil {
		return nil, err
	}
	var contacts []map[string]any
	for _, raw := range jutil.Slice(resp["contacts"]) {
		c := jutil.Obj(raw)
		status := jutil.Type(jutil.Obj(jutil.Obj(c["activeConn"])["connStatus"]))
		contacts = append(contacts, map[string]any{
			"id":     jutil.Int(c, "contactId"),
			"name":   jutil.Str(c, "localDisplayName"),
			"status": status,
		})
	}
	if contacts == nil {
		contacts = []map[string]any{}
	}
	return map[string]any{"contacts": contacts}, nil
}

func (d *Daemon) opSend(to, text, path string) (any, error) {
	if err := d.ready(); err != nil {
		return nil, err
	}
	to = strings.TrimSpace(to)
	if to == "" {
		return nil, fmt.Errorf("send needs a contact name")
	}
	text = strings.TrimRight(text, "\n")
	var filePath string
	if path != "" {
		abs, err := filepath.Abs(path)
		if err != nil {
			return nil, err
		}
		st, err := os.Stat(abs)
		if err != nil {
			return nil, err
		}
		if st.IsDir() {
			return nil, fmt.Errorf("%s is a directory", abs)
		}
		filePath = abs
		if strings.TrimSpace(text) == "" {
			text = filepath.Base(abs)
		}
	}
	if strings.TrimSpace(text) == "" {
		return nil, fmt.Errorf("send needs message text")
	}
	ref, kind, id, err := d.lookup(to)
	if err != nil {
		return nil, err
	}
	msg := map[string]any{
		"msgContent": map[string]any{"type": "text", "text": text},
		"mentions":   map[string]any{},
	}
	if filePath != "" {
		msg["fileSource"] = map[string]any{"filePath": filePath}
	}
	payload, err := json.Marshal([]any{msg})
	if err != nil {
		return nil, err
	}
	resp, err := d.do(context.Background(), fmt.Sprintf("/_send %s json %s", ref, payload), 45*time.Second)
	if err != nil {
		return nil, err
	}
	out := map[string]any{
		"to":     to,
		"status": jutil.Type(resp),
	}
	if kind == "group" {
		out["groupId"] = id
	} else {
		out["contactId"] = id
	}
	if filePath != "" {
		out["file"] = filePath
	}
	return out, nil
}

func (d *Daemon) lookup(to string) (ref, kind string, id int64, err error) {
	forceGroup := strings.HasPrefix(to, "#")
	name := strings.TrimSpace(strings.TrimPrefix(to, "#"))
	if name == "" {
		return "", "", 0, fmt.Errorf("send needs a contact or group name")
	}
	if !forceGroup {
		id, err = d.contactID(name)
		if err == nil {
			return fmt.Sprintf("@%d", id), "contact", id, nil
		}
		if !strings.HasPrefix(err.Error(), "no contact ") {
			return "", "", 0, err
		}
	}
	id, err = d.groupID(name)
	if err != nil {
		return "", "", 0, fmt.Errorf("no contact or group %q", name)
	}
	return fmt.Sprintf("#%d", id), "group", id, nil
}

func (d *Daemon) groupID(name string) (int64, error) {
	uid := d.userIDNow()
	resp, err := d.do(context.Background(), fmt.Sprintf("/_groups %d", uid), 15*time.Second)
	if err != nil {
		return 0, err
	}
	for _, raw := range jutil.Slice(resp["groups"]) {
		g := jutil.Obj(raw)
		n := jutil.Str(g, "localDisplayName")
		if n == "" {
			n = jutil.Str(jutil.Obj(g["groupProfile"]), "displayName")
		}
		if n == name {
			id := jutil.Int(g, "groupId")
			if id == 0 {
				return 0, fmt.Errorf("group %q has no id", name)
			}
			return id, nil
		}
	}
	return 0, fmt.Errorf("no group %q", name)
}

func (d *Daemon) contactID(name string) (int64, error) {
	uid := d.userIDNow()
	resp, err := d.do(context.Background(), fmt.Sprintf("/_contacts %d", uid), 15*time.Second)
	if err != nil {
		return 0, err
	}
	for _, raw := range jutil.Slice(resp["contacts"]) {
		c := jutil.Obj(raw)
		if jutil.Str(c, "localDisplayName") == name {
			id := jutil.Int(c, "contactId")
			if id == 0 {
				return 0, fmt.Errorf("contact %q has no id", name)
			}
			return id, nil
		}
	}
	return 0, fmt.Errorf("no contact %q", name)
}

func (d *Daemon) opInbox(req rpc.Request) (any, error) {
	wait := time.Duration(req.WaitSec) * time.Second
	deadline := time.Now().Add(wait)
	var msgs []inbox.Message
	for {
		msgs = d.box.List(req.All, req.Limit)
		if len(msgs) > 0 || req.WaitSec == 0 || !time.Now().Before(deadline) {
			break
		}
		done := make(chan struct{})
		var once sync.Once
		finish := func() { once.Do(func() { close(done) }) }
		timer := time.AfterFunc(time.Until(deadline), finish)
		d.box.Wait(done)
		timer.Stop()
		finish()
	}
	if req.Ack && len(msgs) > 0 {
		ids := make([]string, len(msgs))
		for i, m := range msgs {
			ids[i] = m.ID
			msgs[i].Unread = false
		}
		d.box.Ack(ids)
	}
	if msgs == nil {
		msgs = []inbox.Message{}
	}
	return map[string]any{"messages": msgs}, nil
}
