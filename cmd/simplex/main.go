package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"simplex/internal/agentdoc"
	"simplex/internal/daemon"
	"simplex/internal/gateway"
	"simplex/internal/install"
	"simplex/internal/mcp"
	"simplex/internal/osutil"
	"simplex/internal/profile"
	"simplex/internal/qrterm"
	"simplex/internal/rpc"
)

const version = "0.1.0-alpha.1"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "simplex: %s\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	profileName := os.Getenv("SIMPLEX_PROFILE")
	jsonOut := false
	var rest []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--profile":
			if i+1 >= len(args) {
				return errors.New("--profile needs a name")
			}
			i++
			profileName = args[i]
		case strings.HasPrefix(a, "--profile="):
			profileName = strings.TrimPrefix(a, "--profile=")
		case a == "--json":
			jsonOut = true
		case a == "-h" || a == "--help" || a == "help":
			fmt.Fprint(os.Stdout, agentdoc.Text)
			return nil
		case strings.HasPrefix(a, "-"):
			return fmt.Errorf("unknown flag %s", a)
		default:
			rest = append(rest, args[i:]...)
			i = len(args)
		}
	}
	if len(rest) == 0 {
		fmt.Fprint(os.Stdout, agentdoc.Text)
		return nil
	}
	switch rest[0] {
	case "version":
		fmt.Println(version)
		return nil
	case "install":
		_, err := install.Download(context.Background(), os.Stdout)
		return err
	case "profiles":
		return cmdProfiles()
	case "use":
		if len(rest) < 2 {
			return errors.New("use needs a profile name")
		}
		return profile.SetDefault(rest[1])
	case "init":
		return cmdInit(profileName, rest[1:], jsonOut)
	case "gateway":
		return cmdGateway(profileName, rest[1:])
	case "up":
		return cmdUp(profileName)
	case "down":
		return cmdDown(profileName)
	case "plugin":
		return cmdPlugin(profileName, rest[1:])
	case "mcp":
		p, err := profile.For(profileName)
		if err != nil {
			return err
		}
		return mcp.Serve(os.Stdin, os.Stdout, func(req rpc.Request) (rpc.Response, error) {
			return callOp(p, req)
		})
	case "qr":
		p, err := profile.For(profileName)
		if err != nil {
			return err
		}
		return cmdQR(p)
	case "address", "connect", "contacts", "send", "send-file", "inbox", "ack", "status":
		p, err := profile.For(profileName)
		if err != nil {
			return err
		}
		return cmdRPC(p, rest, jsonOut)
	default:
		return fmt.Errorf("unknown command %q\n\n%s", rest[0], agentdoc.Text)
	}
}

func cmdInit(fallback string, args []string, jsonOut bool) error {
	pane, _, args := pullFlag(args, "--pane")
	if pane == "" {
		var ok bool
		pane, ok, args = pullFlag(args, "--tmux")
		_ = ok
	}
	name := fallback
	if len(args) > 0 {
		name = args[0]
	}
	if name == "" {
		name = "default"
	}
	p, err := profile.For(name)
	if err != nil {
		return err
	}
	if err := p.Ensure(); err != nil {
		return err
	}
	if !profile.HasDefault() {
		if err := profile.SetDefault(name); err != nil {
			return err
		}
	}
	if pane != "" {
		if err := gateway.ValidTarget(pane); err != nil {
			return err
		}
		if err := p.Update(func(m *profile.Meta) {
			m.Pane = pane
			m.Tmux = ""
		}); err != nil {
			return err
		}
	}
	if err := daemon.Ensure(p); err != nil {
		return err
	}
	resp, err := callOp(p, rpc.Request{Op: "address"})
	if err != nil {
		return err
	}
	var body struct {
		Address string `json:"address"`
	}
	if err := json.Unmarshal(resp.Result, &body); err != nil {
		return err
	}
	if jsonOut {
		return emit(true, resp.Result, nil)
	}
	fmt.Printf("profile: %s\naddress: %s\n\n", p.Name, body.Address)
	fmt.Printf("Other agent:\n  simplex --profile OTHER connect %q\n\n", body.Address)
	fmt.Printf("QR of the short link:\n  simplex --profile %s qr\n\n", p.Name)
	if pane == "" {
		fmt.Printf("Deliver incoming messages into this agent's Herdr pane:\n  simplex --profile %s gateway PANE\n", p.Name)
		fmt.Printf("  herdr plugin action invoke simplex.agents.attach\n\n")
	} else {
		fmt.Printf("Incoming messages are submitted to Herdr pane %s when that agent is idle.\n\n", pane)
	}
	fmt.Printf("Send:\n  simplex --profile %s send NAME \"hello\"\n", p.Name)
	fmt.Printf("  herdr plugin pane open --plugin simplex.agents --entrypoint send --env SIMPLEX_TO=NAME --env SIMPLEX_TEXT=hello\n\n")
	fmt.Printf("MCP, any agent, one server per profile:\n  simplex mcp --profile %s\n", p.Name)
	return nil
}

func cmdGateway(profileName string, args []string) error {
	quietStr, _, args := pullFlag(args, "--quiet")
	p, err := profile.For(profileName)
	if err != nil {
		return err
	}
	if err := p.Ensure(); err != nil {
		return err
	}
	var quietMS int
	if quietStr != "" {
		d, err := time.ParseDuration(quietStr)
		if err != nil {
			return fmt.Errorf("--quiet: %w", err)
		}
		if d < 200*time.Millisecond {
			return errors.New("--quiet must be at least 200ms")
		}
		quietMS = int(d / time.Millisecond)
	}
	if len(args) == 0 {
		meta := p.LoadMeta()
		if meta.Target() == "" {
			fmt.Println("gateway off")
			return nil
		}
		if meta.QuietMS == 0 {
			fmt.Printf("%s\nimmediate\n", meta.Target())
		} else {
			fmt.Printf("%s\nquiet: %dms\n", meta.Target(), meta.QuietMS)
		}
		return nil
	}
	target := args[0]
	if target == "off" {
		target = ""
	} else if err := gateway.ValidTarget(target); err != nil {
		return err
	}
	if err := p.Update(func(m *profile.Meta) {
		m.Pane = target
		m.Tmux = ""
		if quietMS > 0 {
			m.QuietMS = quietMS
		}
	}); err != nil {
		return err
	}
	if target == "" {
		fmt.Println("gateway off")
		return nil
	}
	fmt.Printf("gateway %s\n", target)
	return nil
}

func cmdUp(profileName string) error {
	p, err := profile.For(profileName)
	if err != nil {
		return err
	}
	if err := p.Ensure(); err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	err = daemon.Run(ctx, daemon.Config{Paths: p})
	if errors.Is(err, daemon.ErrAlready) {
		fmt.Println("already running")
		return nil
	}
	if errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}

func cmdDown(profileName string) error {
	p, err := profile.For(profileName)
	if err != nil {
		return err
	}
	if _, err := rpc.Call(p.Sock, rpc.Request{Op: "shutdown"}, 3*time.Second); err == nil {
		fmt.Println("stopped")
		return nil
	}
	b, err := os.ReadFile(p.PID)
	if err != nil {
		fmt.Println("not running")
		return nil
	}
	pid, _ := strconv.Atoi(strings.TrimSpace(string(b)))
	if pid <= 0 {
		fmt.Println("not running")
		return nil
	}
	if err := osutil.StopTree(pid); err != nil {
		fmt.Println("not running")
		return nil
	}
	fmt.Println("stopped")
	return nil
}

func cmdPlugin(profileName string, args []string) error {
	if len(args) == 0 {
		return errors.New("plugin needs attach, startup, start, compose, send, or status")
	}
	switch args[0] {
	case "attach":
		return pluginAttach(profileName)
	case "startup", "start":
		return pluginStartup()
	case "compose", "send":
		return pluginCompose(profileName)
	case "status":
		p, err := profile.For(profileName)
		if err != nil {
			return err
		}
		rpcErr := cmdRPC(p, []string{"status"}, false)
		fmt.Fprint(os.Stdout, "\n")
		fmt.Fprint(os.Stdout, agentdoc.Text)
		return rpcErr
	default:
		return fmt.Errorf("unknown plugin command %q", args[0])
	}
}

func pluginAttach(profileName string) error {
	pane := herdrPane()
	if pane == "" {
		return errors.New("no Herdr pane. Run this from a Herdr pane, or pass one to simplex gateway")
	}
	if err := gateway.ValidTarget(pane); err != nil {
		return err
	}
	name := profileName
	if name == "" {
		name = "default"
	}
	p, err := profile.For(name)
	if err != nil {
		return err
	}
	if err := p.Ensure(); err != nil {
		return err
	}
	if !profile.HasDefault() {
		if err := profile.SetDefault(name); err != nil {
			return err
		}
	}
	if err := p.Update(func(m *profile.Meta) {
		m.Pane = pane
		m.Tmux = ""
	}); err != nil {
		return err
	}
	if err := daemon.Ensure(p); err != nil {
		return err
	}
	fmt.Printf("simplex profile %s is attached to Herdr pane %s\n", p.Name, pane)
	return cmdRPC(p, []string{"address"}, false)
}

func pluginStartup() error {
	names, err := profile.List()
	if err != nil {
		return err
	}
	if len(names) == 0 {
		fmt.Println("simplex: no profiles")
		return nil
	}
	p, err := profile.For("")
	if err != nil {
		return err
	}
	if st, err := os.Stat(p.Dir); err != nil || !st.IsDir() {
		fmt.Println("simplex: default profile is not set up")
		return nil
	}
	if err := daemon.Ensure(p); err != nil {
		return err
	}
	fmt.Printf("simplex profile %s is running\n", p.Name)
	return nil
}

func pluginCompose(profileName string) error {
	to := strings.TrimSpace(os.Getenv("SIMPLEX_TO"))
	text := strings.TrimSpace(os.Getenv("SIMPLEX_TEXT"))
	if to == "" || text == "" {
		in := bufio.NewReader(os.Stdin)
		fmt.Fprint(os.Stderr, "To: ")
		line, err := in.ReadString('\n')
		if err != nil {
			return err
		}
		to = strings.TrimSpace(line)
		fmt.Fprint(os.Stderr, "Message: ")
		line, err = in.ReadString('\n')
		if err != nil && err != io.EOF {
			return err
		}
		text = strings.TrimSpace(line)
	}
	if to == "" || text == "" {
		return errors.New("need a recipient and a message")
	}
	p, err := profile.For(profileName)
	if err != nil {
		return err
	}
	resp, err := callOp(p, rpc.Request{Op: "send", To: to, Text: text})
	if err != nil {
		return err
	}
	fmt.Printf("sent to %s\n", to)
	_ = resp
	return nil
}

func herdrPane() string {
	if pane := os.Getenv("HERDR_PANE_ID"); pane != "" {
		return pane
	}
	raw := os.Getenv("HERDR_PLUGIN_CONTEXT_JSON")
	if raw == "" {
		return ""
	}
	var ctx struct {
		PaneID string `json:"pane_id"`
		Pane   struct {
			ID string `json:"id"`
		} `json:"pane"`
		FocusedPane struct {
			ID string `json:"id"`
		} `json:"focused_pane"`
	}
	if json.Unmarshal([]byte(raw), &ctx) != nil {
		return ""
	}
	if ctx.PaneID != "" {
		return ctx.PaneID
	}
	if ctx.Pane.ID != "" {
		return ctx.Pane.ID
	}
	return ctx.FocusedPane.ID
}

func cmdQR(p profile.Paths) error {
	resp, err := callOp(p, rpc.Request{Op: "address"})
	if err != nil {
		return err
	}
	var body struct {
		Address string `json:"address"`
		Short   string `json:"short"`
	}
	if err := json.Unmarshal(resp.Result, &body); err != nil {
		return err
	}
	link := body.Short
	if link == "" {
		return errors.New("no short link yet. Run simplex address once, then simplex qr")
	}
	pic, err := qrterm.Render(link)
	if err != nil {
		return err
	}
	if cols := qrterm.TermCols(); cols > 0 && cols < qrterm.Cols(pic) {
		fmt.Fprintf(os.Stderr, "pane is %d columns and this QR is %d. Widen the pane and run simplex qr again. A wrapped code will not scan.\n", cols, qrterm.Cols(pic))
	}
	fmt.Print(pic)
	fmt.Println(link)
	return nil
}

func cmdProfiles() error {
	names, err := profile.List()
	if err != nil {
		return err
	}
	def := profile.DefaultName()
	if len(names) == 0 {
		fmt.Println("no profiles")
		return nil
	}
	for _, name := range names {
		if name == def {
			fmt.Printf("%s (default)\n", name)
			continue
		}
		fmt.Println(name)
	}
	return nil
}

func cmdRPC(p profile.Paths, args []string, jsonOut bool) error {
	req, human, err := buildReq(args)
	if err != nil {
		return err
	}
	resp, err := callOp(p, req)
	if err != nil {
		return err
	}
	return emit(jsonOut, resp.Result, human)
}

func buildReq(args []string) (rpc.Request, func(), error) {
	switch args[0] {
	case "address":
		return rpc.Request{Op: "address"}, func() {}, nil
	case "status":
		return rpc.Request{Op: "who"}, func() {}, nil
	case "contacts":
		return rpc.Request{Op: "contacts"}, func() {}, nil
	case "connect":
		if len(args) < 2 {
			return rpc.Request{}, nil, errors.New("connect needs a simplex link")
		}
		return rpc.Request{Op: "connect", Link: args[1]}, func() {}, nil
	case "send":
		if len(args) < 3 {
			return rpc.Request{}, nil, errors.New("send needs a contact and text")
		}
		text := strings.Join(args[2:], " ")
		if text == "-" {
			b, err := io.ReadAll(os.Stdin)
			if err != nil {
				return rpc.Request{}, nil, err
			}
			text = string(b)
		}
		return rpc.Request{Op: "send", To: args[1], Text: text}, func() {}, nil
	case "send-file":
		if len(args) < 3 {
			return rpc.Request{}, nil, errors.New("send-file needs a contact and a path")
		}
		caption := ""
		if len(args) > 3 {
			caption = strings.Join(args[3:], " ")
		}
		return rpc.Request{Op: "send_file", To: args[1], Path: args[2], Text: caption}, func() {}, nil
	case "ack":
		if len(args) < 2 {
			return rpc.Request{}, nil, errors.New("ack needs message ids")
		}
		return rpc.Request{Op: "ack", IDs: args[1:]}, func() {}, nil
	case "inbox":
		req := rpc.Request{Op: "inbox"}
		inboxArgs := args[1:]
		var waitStr string
		waitStr, _, inboxArgs = pullFlag(inboxArgs, "--wait")
		var rest []string
		for _, a := range inboxArgs {
			switch a {
			case "--all":
				req.All = true
			case "--ack":
				req.Ack = true
			default:
				rest = append(rest, a)
			}
		}
		if len(rest) > 0 {
			return rpc.Request{}, nil, fmt.Errorf("unknown inbox argument %q", rest[0])
		}
		if waitStr != "" {
			d, err := time.ParseDuration(waitStr)
			if err != nil {
				return rpc.Request{}, nil, fmt.Errorf("--wait: %w", err)
			}
			sec := int(d.Round(time.Second) / time.Second)
			if sec < 1 {
				sec = 1
			}
			req.WaitSec = sec
		}
		return req, func() {}, nil
	default:
		return rpc.Request{}, nil, fmt.Errorf("unknown command %q", args[0])
	}
}

func callOp(p profile.Paths, req rpc.Request) (rpc.Response, error) {
	if err := daemon.Ensure(p); err != nil {
		return rpc.Response{}, err
	}
	timeout := 30 * time.Second
	switch req.Op {
	case "connect":
		timeout = 90 * time.Second
	case "send", "send_file":
		timeout = 50 * time.Second
	}
	if req.WaitSec > 0 {
		extra := time.Duration(req.WaitSec)*time.Second + 15*time.Second
		if extra > timeout {
			timeout = extra
		}
	}
	return rpc.Call(p.Sock, req, timeout)
}

func emit(jsonOut bool, raw json.RawMessage, _ func()) error {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		fmt.Println(string(raw))
		return nil
	}
	if jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(v)
	}
	printHuman(v)
	return nil
}

func printHuman(v any) {
	m, ok := v.(map[string]any)
	if !ok {
		fmt.Println(v)
		return
	}
	if addr, ok := m["address"].(string); ok && len(m) <= 3 && m["contacts"] == nil && m["messages"] == nil && m["status"] == nil {
		fmt.Println(addr)
		return
	}
	if status, ok := m["status"].(string); ok && m["profile"] != nil && m["socket"] != nil {
		fmt.Printf("profile: %s\nstatus: %s\n", m["profile"], status)
		if addr, _ := m["address"].(string); addr != "" {
			fmt.Printf("address: %s\n", addr)
		}
		fmt.Printf("unread: %v\npending: %v\n", num(m["unread"]), num(m["pending"]))
		if pane, _ := m["pane"].(string); pane != "" {
			fmt.Printf("pane: %s\n", pane)
		} else {
			fmt.Println("pane: off")
		}
		if errText, _ := m["error"].(string); errText != "" {
			fmt.Printf("error: %s\n", errText)
		}
		if gerr, _ := m["gatewayError"].(string); gerr != "" {
			fmt.Printf("gateway: %s\n", gerr)
		}
		return
	}
	if _, ok := m["contacts"]; ok {
		list, _ := m["contacts"].([]any)
		if len(list) == 0 {
			fmt.Println("no contacts")
			return
		}
		for _, raw := range list {
			c, _ := raw.(map[string]any)
			fmt.Printf("%s\t%s\tid=%v\n", c["name"], c["status"], num(c["id"]))
		}
		return
	}
	if _, ok := m["messages"]; ok {
		printMessages(m["messages"])
		return
	}
	if to, ok := m["to"].(string); ok {
		if file, ok := m["file"].(string); ok {
			fmt.Printf("sent %s to %s\n", file, to)
			return
		}
		fmt.Printf("sent to %s\n", to)
		return
	}
	if status, ok := m["status"].(string); ok && m["link"] != nil {
		fmt.Printf("connecting (%s)\n", status)
		return
	}
	if n, ok := m["acked"]; ok {
		fmt.Printf("acked %v\n", num(n))
		return
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

func printMessages(raw any) {
	list, _ := raw.([]any)
	if len(list) == 0 {
		fmt.Println("inbox empty")
		return
	}
	for _, item := range list {
		m, _ := item.(map[string]any)
		flag := "read"
		if unread, _ := m["unread"].(bool); unread {
			flag = "unread"
		}
		if delivered, _ := m["delivered"].(bool); delivered {
			flag += ", in terminal"
		}
		fmt.Printf("%s [%s] %s (%s)\n%s\n", m["from"], m["id"], m["ts"], flag, m["text"])
		if path, _ := m["filePath"].(string); path != "" {
			fmt.Printf("file: %s\n", path)
		} else if name, _ := m["fileName"].(string); name != "" {
			fmt.Printf("file: %s (%s)\n", name, m["fileStatus"])
		}
		fmt.Println()
	}
}

func num(v any) any {
	f, ok := v.(float64)
	if !ok {
		return v
	}
	return int64(f)
}

func pullFlag(args []string, name string) (string, bool, []string) {
	var rest []string
	var val string
	found := false
	for i := 0; i < len(args); i++ {
		if args[i] == name && i+1 < len(args) {
			val = args[i+1]
			found = true
			i++
			continue
		}
		if strings.HasPrefix(args[i], name+"=") {
			val = strings.TrimPrefix(args[i], name+"=")
			found = true
			continue
		}
		rest = append(rest, args[i])
	}
	return val, found, rest
}
