// Package daemon runs one simplex-chat process, keeps the inbox, and submits
// new messages to an idle Herdr agent pane.
package daemon

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"simplex/internal/chat"
	"simplex/internal/control"
	"simplex/internal/gateway"
	"simplex/internal/inbox"
	"simplex/internal/install"
	"simplex/internal/jutil"
	"simplex/internal/osutil"
	"simplex/internal/profile"
	"simplex/internal/rpc"
)

// ErrAlready means another daemon holds this profile.
var ErrAlready = errors.New("daemon already running")

type Config struct {
	Paths   profile.Paths
	WSURL   string
	ChatBin string
	Quiet   time.Duration
}

type Daemon struct {
	paths  profile.Paths
	wsURL  string
	bin    string
	quiet  time.Duration
	log    *log.Logger
	cancel context.CancelFunc

	mu         sync.Mutex
	client     *chat.Client
	box        *inbox.Store
	status     string
	statusErr  string
	gatewayErr string
	address    string
	userID     int64
	accepted   map[int64]bool
	child      *exec.Cmd
	hold       *os.File
}

func Run(ctx context.Context, cfg Config) error {
	if err := cfg.Paths.Ensure(); err != nil {
		return err
	}
	lock, err := lockFile(filepath.Join(cfg.Paths.Dir, "daemon.lock"))
	if err != nil {
		return ErrAlready
	}
	defer lock.Close()

	lf, err := os.OpenFile(cfg.Paths.Log, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer lf.Close()

	box, err := inbox.Open(cfg.Paths.Dir)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(ctx)
	d := &Daemon{
		paths:    cfg.Paths,
		wsURL:    cfg.WSURL,
		bin:      cfg.ChatBin,
		quiet:    cfg.Quiet,
		log:      log.New(lf, "", log.LstdFlags),
		cancel:   cancel,
		box:      box,
		status:   "starting",
		accepted: map[int64]bool{},
	}
	defer cancel()
	defer d.killChild()

	if err := os.WriteFile(cfg.Paths.PID, []byte(fmt.Sprintf("%d\n", os.Getpid())), 0o600); err != nil {
		return err
	}
	defer os.Remove(cfg.Paths.PID)

	control.Cleanup(cfg.Paths.Sock)
	ln, err := control.Listen(cfg.Paths.Sock)
	if err != nil {
		return err
	}
	defer ln.Close()
	defer control.Cleanup(cfg.Paths.Sock)
	if runtime.GOOS != "windows" {
		_ = os.Chmod(cfg.Paths.Sock, 0o600)
	}

	d.log.Printf("profile %s listening", cfg.Paths.Name)
	go d.supervise(ctx)
	go d.gatewayLoop(ctx)
	return d.serve(ctx, ln)
}

func lockFile(path string) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := osutil.LockNB(f); err != nil {
		f.Close()
		return nil, err
	}
	return f, nil
}

func (d *Daemon) serve(ctx context.Context, ln net.Listener) error {
	go func() {
		<-ctx.Done()
		_ = ln.Close()
	}()
	for {
		conn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		go d.serveConn(conn)
	}
}

func (d *Daemon) serveConn(conn net.Conn) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(3 * time.Minute))
	sc := bufio.NewScanner(conn)
	sc.Buffer(make([]byte, 64*1024), 4*1024*1024)
	if !sc.Scan() {
		return
	}
	var req rpc.Request
	if err := json.Unmarshal(sc.Bytes(), &req); err != nil {
		writeResp(conn, rpc.Response{OK: false, Error: "bad request"})
		return
	}
	writeResp(conn, d.handle(req))
}

func writeResp(conn net.Conn, resp rpc.Response) {
	b, err := json.Marshal(resp)
	if err != nil {
		return
	}
	_, _ = conn.Write(append(b, '\n'))
}

func (d *Daemon) supervise(ctx context.Context) {
	backoff := time.Second
	for {
		if ctx.Err() != nil {
			return
		}
		d.setStatus("starting", "")
		err := d.runOnce(ctx)
		if ctx.Err() != nil {
			return
		}
		d.setError(err)
		d.log.Printf("chat session ended: %v", err)
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		if backoff < 15*time.Second {
			backoff *= 2
		}
	}
}

func (d *Daemon) runOnce(ctx context.Context) error {
	if d.wsURL != "" {
		return d.session(ctx, d.wsURL)
	}
	return d.runProcess(ctx)
}

func (d *Daemon) runProcess(ctx context.Context) error {
	bin := d.bin
	if bin == "" {
		var err error
		bin, err = install.Bin()
		if err != nil {
			return err
		}
	}
	port, err := freePort()
	if err != nil {
		return err
	}
	if err := d.paths.Update(func(m *profile.Meta) { m.Port = port }); err != nil {
		return err
	}
	args := []string{
		"-p", fmt.Sprintf("%d", port),
		"-d", "db",
		"-y",
		"--mute",
		"--files-folder", d.paths.Files,
		"--auto-accept-files", "104857600",
	}
	if !profile.DBExists(d.paths.Dir) {
		args = append(args, "--create-bot-display-name", d.paths.Name, "--create-bot-allow-files")
	}
	logf, err := os.OpenFile(d.paths.ChatLog, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer logf.Close()
	pr, pw, err := os.Pipe()
	if err != nil {
		return err
	}
	defer pr.Close()

	cmd := exec.Command(bin, args...)
	cmd.Dir = d.paths.Dir
	cmd.Stdin = pr
	cmd.Stdout = logf
	cmd.Stderr = logf
	osutil.ChildGroup(cmd)
	if err := cmd.Start(); err != nil {
		_ = pw.Close()
		return err
	}
	d.setChild(cmd, pw)
	exitc := make(chan error, 1)
	go func() { exitc <- cmd.Wait() }()
	defer d.killChild()

	addr := fmt.Sprintf("127.0.0.1:%d", port)
	if err := waitPort(ctx, addr, 60*time.Second); err != nil {
		select {
		case werr := <-exitc:
			return fmt.Errorf("simplex-chat exited: %v\n%s", werr, tail(d.paths.ChatLog, 30))
		default:
			return fmt.Errorf("%w\n%s", err, tail(d.paths.ChatLog, 30))
		}
	}
	sessCtx, sessCancel := context.WithCancel(ctx)
	defer sessCancel()
	go func() {
		select {
		case <-exitc:
			sessCancel()
		case <-sessCtx.Done():
		}
	}()
	return d.session(sessCtx, "ws://"+addr)
}

func (d *Daemon) session(ctx context.Context, wsURL string) error {
	dialCtx, dialCancel := context.WithTimeout(ctx, 10*time.Second)
	client, err := chat.Dial(dialCtx, wsURL)
	dialCancel()
	if err != nil {
		return err
	}
	defer client.Close()
	d.setClient(client)
	defer d.setClient(nil)

	errc := make(chan error, 1)
	go func() { errc <- client.Read(ctx, d.onEvent) }()
	if err := d.bootstrap(ctx); err != nil {
		_ = client.Close()
		return err
	}
	select {
	case <-ctx.Done():
		_ = client.Close()
		return ctx.Err()
	case err := <-errc:
		return err
	}
}

func (d *Daemon) bootstrap(ctx context.Context) error {
	var user map[string]any
	var err error
	for i := 0; i < 20; i++ {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		user, err = d.do(ctx, "/user", 5*time.Second)
		if err == nil {
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(300 * time.Millisecond):
		}
	}
	if err != nil {
		return fmt.Errorf("profile: %w", err)
	}
	uid := jutil.Int(jutil.Obj(user["user"]), "userId")
	if uid == 0 {
		return fmt.Errorf("simplex did not return a user id")
	}

	link := ""
	shown, showErr := d.do(ctx, fmt.Sprintf("/_show_address %d", uid), 20*time.Second)
	if showErr == nil {
		link = linkFrom(shown)
	}
	if link == "" {
		created, err := d.do(ctx, fmt.Sprintf("/_address %d", uid), 60*time.Second)
		if err != nil {
			return fmt.Errorf("create address: %w", err)
		}
		link = linkFrom(created)
	}
	if link == "" {
		return fmt.Errorf("simplex did not return an address")
	}
	settings := `{"businessAddress":false,"autoAccept":{"acceptIncognito":false}}`
	if _, err := d.do(ctx, fmt.Sprintf("/_address_settings %d %s", uid, settings), 30*time.Second); err != nil {
		d.log.Printf("address settings: %v", err)
	}
	if chats, err := d.do(ctx, fmt.Sprintf(`/_get chats %d count=50 {"type":"filters","favorite":false,"unread":false}`, uid), 15*time.Second); err == nil {
		for _, m := range inbox.MessagesFromEvent(chats) {
			d.box.AddSeen(m)
		}
	} else {
		d.log.Printf("history: %v", err)
	}
	if err := d.paths.Update(func(m *profile.Meta) {
		m.Name = d.paths.Name
		m.UserID = uid
		m.Address = link
	}); err != nil {
		return err
	}
	d.setRunning(link, uid)
	d.log.Printf("ready")
	return nil
}

func (d *Daemon) onEvent(ev map[string]any) {
	switch jutil.Type(ev) {
	case "receivedContactRequest":
		id := jutil.Int(jutil.Obj(ev["contactRequest"]), "contactRequestId")
		if id == 0 {
			return
		}
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if _, err := d.do(ctx, fmt.Sprintf("/_accept %d", id), 30*time.Second); err != nil {
				d.log.Printf("accept contact: %v", err)
			}
		}()
	case "receivedGroupInvitation":
		g := jutil.Obj(ev["groupInfo"])
		name := jutil.Str(g, "localDisplayName")
		if name == "" {
			name = jutil.Str(jutil.Obj(g["groupProfile"]), "displayName")
		}
		id := jutil.Int(g, "groupId")
		if name == "" || id == 0 {
			return
		}
		from := jutil.Str(jutil.Obj(ev["contact"]), "localDisplayName")
		if from == "" {
			from = name
		}
		d.box.Add(inbox.Message{
			ID:        fmt.Sprintf("sys:invite:%d", id),
			From:      from,
			Chat:      name,
			Direction: "in",
			Text:      fmt.Sprintf("group invite %s. Join with: simplex join %q", name, name),
		})
	case "contactConnected", "contactSndReady":
		c := jutil.Obj(ev["contact"])
		name := jutil.Str(c, "localDisplayName")
		id := jutil.Int(c, "contactId")
		if name == "" || id == 0 {
			return
		}
		text := name + " connected"
		if jutil.Type(ev) == "contactSndReady" {
			text = name + " is ready"
		}
		d.box.Add(inbox.Message{
			ID:        fmt.Sprintf("sys:%s:%d", jutil.Type(ev), id),
			From:      name,
			Chat:      name,
			Direction: "system",
			Text:      text,
		})
	default:
		for _, m := range inbox.MessagesFromEvent(ev) {
			if m.Direction == "call" {
				d.rejectCall(m)
				m.Text = "rejected call from " + m.From
				d.box.AddSeen(m)
				continue
			}
			m = d.resolveFile(m)
			d.box.Add(m)
			if receiveFile(m) {
				d.acceptFile(m)
			}
		}
	}
}

func (d *Daemon) resolveFile(m inbox.Message) inbox.Message {
	if m.FilePath == "" || filepath.IsAbs(m.FilePath) {
		return m
	}
	cand := filepath.Join(d.paths.Files, filepath.Base(m.FilePath))
	if st, err := os.Stat(cand); err == nil && !st.IsDir() {
		m.FilePath = cand
		return m
	}
	if m.FileName == "" {
		m.FileName = m.FilePath
	}
	if !fileDone(m.FileStatus) {
		m.FilePath = ""
	}
	return m
}

func fileDone(status string) bool {
	switch status {
	case "complete", "rcvComplete":
		return true
	default:
		return false
	}
}

func receiveFile(m inbox.Message) bool {
	if m.FileID == 0 {
		return false
	}
	switch m.FileStatus {
	case "complete", "cancelled", "rcvComplete":
		return false
	}
	if filepath.IsAbs(m.FilePath) {
		if st, err := os.Stat(m.FilePath); err == nil && !st.IsDir() {
			return false
		}
	}
	return true
}

func (d *Daemon) rejectCall(m inbox.Message) {
	if m.ContactID == 0 {
		d.log.Printf("call from %s has no contact id", m.From)
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if _, err := d.do(ctx, fmt.Sprintf("/_call reject @%d", m.ContactID), 15*time.Second); err != nil {
			d.log.Printf("reject call: %v", err)
		}
	}()
}

func (d *Daemon) acceptFile(m inbox.Message) {
	id := m.FileID
	d.mu.Lock()
	if d.accepted[id] {
		d.mu.Unlock()
		return
	}
	d.accepted[id] = true
	d.mu.Unlock()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		err := d.receiveFile(ctx, id, receiveDest(d.paths.Files, m))
		if err == nil || fileAlreadyReceiving(err) {
			return
		}
		d.log.Printf("receive file %d: %v", id, err)
	}()
}

func receiveDest(dir string, m inbox.Message) string {
	name := filepath.Base(m.FileName)
	if name == "." || name == ".." || name == string(filepath.Separator) {
		name = ""
	}
	if name == "" {
		name = fmt.Sprintf("%d", m.FileID)
	}
	return filepath.Join(dir, name)
}

func (d *Daemon) receiveFile(ctx context.Context, id int64, dest string) error {
	cmd := fmt.Sprintf("/freceive %d approved_relays=on %s", id, dest)
	_, err := d.do(ctx, cmd, 20*time.Second)
	return err
}

func fileAlreadyReceiving(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "filealreadyreceiving") || strings.Contains(s, "already receiving")
}

func (d *Daemon) gatewayLoop(ctx context.Context) {
	tick := time.NewTicker(200 * time.Millisecond)
	defer tick.Stop()
	var watch gateway.Watch
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			d.pump(&watch)
		}
	}
}

func (d *Daemon) pump(watch *gateway.Watch) {
	meta := d.paths.LoadMeta()
	target := strings.TrimSpace(meta.Target())
	if target == "" || d.box == nil {
		return
	}
	quiet := d.quiet
	if quiet <= 0 && meta.QuietMS > 0 {
		quiet = time.Duration(meta.QuietMS) * time.Millisecond
	}
	now := time.Now()
	due := d.box.Due(now, 1)
	id, err := gateway.Pump(watch, gateway.Herdr{}, target, quiet, due, now)
	d.mu.Lock()
	if err != nil {
		d.gatewayErr = err.Error()
	} else if id != "" {
		d.gatewayErr = ""
	}
	d.mu.Unlock()
	if id != "" {
		d.box.MarkDelivered(id)
		d.log.Printf("typed %s into %s", id, target)
	}
}

func (d *Daemon) do(ctx context.Context, cmd string, timeout time.Duration) (map[string]any, error) {
	c := d.getClient()
	if c == nil {
		return nil, errors.New("simplex is still starting")
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	d.log.Printf("cmd %s", verb(cmd))
	return c.Do(ctx, cmd)
}

func verb(cmd string) string {
	if i := strings.IndexByte(cmd, ' '); i > 0 {
		return cmd[:i]
	}
	return cmd
}

func linkFrom(m map[string]any) string {
	full, _ := linksFrom(m)
	return full
}

func linksFrom(m map[string]any) (full, short string) {
	if m == nil {
		return "", ""
	}
	c := jutil.Obj(m["connLinkContact"])
	if len(c) == 0 {
		c = jutil.Obj(jutil.Obj(m["contactLink"])["connLinkContact"])
	}
	return jutil.Str(c, "connFullLink"), jutil.Str(c, "connShortLink")
}

func (d *Daemon) setClient(c *chat.Client) {
	d.mu.Lock()
	d.client = c
	d.mu.Unlock()
}

func (d *Daemon) getClient() *chat.Client {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.client
}

func (d *Daemon) setChild(cmd *exec.Cmd, hold *os.File) {
	d.mu.Lock()
	d.child = cmd
	d.hold = hold
	d.mu.Unlock()
}

func (d *Daemon) killChild() {
	d.mu.Lock()
	cmd := d.child
	hold := d.hold
	d.child = nil
	d.hold = nil
	d.mu.Unlock()
	if hold != nil {
		_ = hold.Close()
	}
	if cmd == nil || cmd.Process == nil {
		return
	}
	osutil.StopTree(cmd.Process.Pid)
}

func (d *Daemon) setStatus(status, errText string) {
	d.mu.Lock()
	d.status = status
	if errText == "" && status == "starting" {
		d.statusErr = ""
	}
	d.mu.Unlock()
}

func (d *Daemon) setError(err error) {
	if err == nil {
		return
	}
	d.mu.Lock()
	d.status = "error"
	d.statusErr = err.Error()
	d.mu.Unlock()
}

func (d *Daemon) setRunning(addr string, uid int64) {
	d.mu.Lock()
	d.status = "running"
	d.statusErr = ""
	d.address = addr
	d.userID = uid
	d.mu.Unlock()
}

func freePort() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	return port, nil
}

func waitPort(ctx context.Context, addr string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var last error
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if time.Now().After(deadline) {
			if last == nil {
				last = errors.New("timed out")
			}
			return fmt.Errorf("simplex-chat port %s: %w", addr, last)
		}
		c, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
		if err == nil {
			_ = c.Close()
			return nil
		}
		last = err
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
}

func tail(path string, n int) string {
	b, err := os.ReadFile(path)
	if err != nil || len(b) == 0 {
		return ""
	}
	lines := strings.Split(string(b), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}
