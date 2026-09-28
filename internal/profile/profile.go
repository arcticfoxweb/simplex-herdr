// Package profile is the on-disk layout for one SimpleX agent identity.
package profile

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"

	"simplex/internal/osutil"
)

var nameRe = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]{0,23}$`)

type Paths struct {
	Root    string
	Name    string
	Dir     string
	Sock    string
	Files   string
	PID     string
	Meta    string
	ChatLog string
	Log     string
}

type Meta struct {
	Name    string `json:"name"`
	UserID  int64  `json:"userId,omitempty"`
	Address string `json:"address,omitempty"`
	Short   string `json:"short,omitempty"`
	Port    int    `json:"port,omitempty"`
	Pane    string `json:"pane,omitempty"`
	Tmux    string `json:"tmux,omitempty"` // older name for Pane
	QuietMS int    `json:"quietMs,omitempty"`
}

// Target is the Herdr pane that receives messages.
func (m Meta) Target() string {
	if m.Pane != "" {
		return m.Pane
	}
	return m.Tmux
}

type configFile struct {
	Profile string `json:"profile"`
}

func Root() string {
	if h := os.Getenv("SIMPLEX_HOME"); h != "" {
		return h
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return filepath.Join("/tmp", "simplex")
	}
	return filepath.Join(dir, "simplex")
}

func Validate(name string) error {
	if !nameRe.MatchString(name) {
		return fmt.Errorf("profile name %q must start with a letter and use only letters, numbers, _ or - (max 24)", name)
	}
	return nil
}

func For(name string) (Paths, error) {
	if name == "" {
		name = DefaultName()
	}
	if err := Validate(name); err != nil {
		return Paths{}, err
	}
	dir := filepath.Join(Root(), "profiles", name)
	return Paths{
		Root:    Root(),
		Name:    name,
		Dir:     dir,
		Sock:    controlSock(name, dir),
		Files:   filepath.Join(dir, "files"),
		PID:     filepath.Join(dir, "daemon.pid"),
		Meta:    filepath.Join(dir, "profile.json"),
		ChatLog: filepath.Join(dir, "chat.log"),
		Log:     filepath.Join(dir, "daemon.log"),
	}, nil
}

func controlSock(name, dir string) string {
	if runtime.GOOS == "windows" {
		return `\\.\pipe\simplex-` + name
	}
	return filepath.Join(dir, "sock")
}

func (p Paths) Ensure() error {
	if err := os.MkdirAll(p.Files, 0o700); err != nil {
		return err
	}
	return os.MkdirAll(p.Dir, 0o700)
}

func (p Paths) LoadMeta() Meta {
	var m Meta
	b, err := os.ReadFile(p.Meta)
	if err != nil {
		return Meta{Name: p.Name}
	}
	_ = json.Unmarshal(b, &m)
	if m.Name == "" {
		m.Name = p.Name
	}
	return m
}

// Update reads profile.json, lets fn change it, and writes it back.
// A lock keeps the daemon and the CLI from dropping each other's fields.
func (p Paths) Update(fn func(*Meta)) error {
	if err := os.MkdirAll(p.Dir, 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(p.Dir, "meta.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := osutil.Lock(f); err != nil {
		return err
	}
	defer osutil.Unlock(f)
	m := p.LoadMeta()
	fn(&m)
	return p.SaveMeta(m)
}

func (p Paths) SaveMeta(m Meta) error {
	if m.Name == "" {
		m.Name = p.Name
	}
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	tmp := p.Meta + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, p.Meta)
}

func DefaultName() string {
	b, err := os.ReadFile(filepath.Join(Root(), "config.json"))
	if err != nil {
		return "default"
	}
	var cfg configFile
	if json.Unmarshal(b, &cfg) != nil || cfg.Profile == "" {
		return "default"
	}
	return cfg.Profile
}

func SetDefault(name string) error {
	if err := Validate(name); err != nil {
		return err
	}
	if err := os.MkdirAll(Root(), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(configFile{Profile: name}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(Root(), "config.json"), append(b, '\n'), 0o600)
}

func HasDefault() bool {
	_, err := os.Stat(filepath.Join(Root(), "config.json"))
	return err == nil
}

func List() ([]string, error) {
	dir := filepath.Join(Root(), "profiles")
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	return names, nil
}

func DBExists(dir string) bool {
	matches, _ := filepath.Glob(filepath.Join(dir, "db*_chat.db"))
	return len(matches) > 0
}
