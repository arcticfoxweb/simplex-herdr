// Package inbox stores messages the daemon has seen, including file paths once a download finishes.
package inbox

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Message struct {
	ID         string `json:"id"`
	From       string `json:"from"`
	Chat       string `json:"chat"`
	Direction  string `json:"direction"`
	Text       string `json:"text"`
	FileID     int64  `json:"fileId,omitempty"`
	FileName   string `json:"fileName,omitempty"`
	FilePath   string `json:"filePath,omitempty"`
	FileStatus string `json:"fileStatus,omitempty"`
	ContactID  int64  `json:"contactId,omitempty"`
	ItemID     int64  `json:"itemId,omitempty"`
	TS         string `json:"ts"`
	Unread     bool   `json:"unread"`
	Delivered  bool   `json:"delivered,omitempty"`
}

type Store struct {
	mu    sync.Mutex
	cond  *sync.Cond
	path  string
	order []string
	msgs  map[string]Message
}

func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	s := &Store{
		path: filepath.Join(dir, "inbox.json"),
		msgs: map[string]Message{},
	}
	s.cond = sync.NewCond(&s.mu)
	if b, err := os.ReadFile(s.path); err == nil && len(b) > 0 {
		var disk struct {
			Messages []Message `json:"messages"`
		}
		if err := json.Unmarshal(b, &disk); err != nil {
			return nil, err
		}
		for _, m := range disk.Messages {
			if m.ID == "" {
				continue
			}
			s.order = append(s.order, m.ID)
			s.msgs[m.ID] = m
		}
	}
	return s, nil
}

// Add records a message. A second add with the same id updates text and file fields
// and does not mark an already-read message unread again.
func (s *Store) Add(m Message) {
	if m.ID == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if prev, ok := s.msgs[m.ID]; ok {
		changed := false
		if m.Text != "" && m.Text != prev.Text {
			prev.Text = m.Text
			changed = true
		}
		if m.FileID != 0 && prev.FileID == 0 {
			prev.FileID = m.FileID
			changed = true
		}
		if m.FileName != "" && m.FileName != prev.FileName {
			prev.FileName = m.FileName
			changed = true
		}
		if m.FilePath != "" && m.FilePath != prev.FilePath {
			prev.FilePath = m.FilePath
			// The terminal copy was missing the path. Let the gateway type it again.
			prev.Delivered = false
			changed = true
		}
		if m.FileStatus != "" && m.FileStatus != prev.FileStatus {
			prev.FileStatus = m.FileStatus
			changed = true
		}
		if !changed {
			return
		}
		s.msgs[m.ID] = prev
		s.saveLocked()
		s.cond.Broadcast()
		return
	}
	if m.TS == "" {
		m.TS = time.Now().UTC().Format(time.RFC3339)
	}
	if m.Direction == "" {
		m.Direction = "in"
	}
	m.Unread = true
	s.order = append(s.order, m.ID)
	s.msgs[m.ID] = m
	s.trimLocked()
	s.saveLocked()
	s.cond.Broadcast()
}

func (s *Store) trimLocked() {
	const capN = 2000
	if len(s.order) <= capN {
		return
	}
	drop := len(s.order) - capN
	kept := make([]string, 0, capN)
	removed := 0
	for _, id := range s.order {
		m := s.msgs[id]
		if removed < drop && !m.Unread {
			delete(s.msgs, id)
			removed++
			continue
		}
		kept = append(kept, id)
	}
	s.order = kept
}

func (s *Store) List(all bool, limit int) []Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.listLocked(all, limit)
}

func (s *Store) listLocked(all bool, limit int) []Message {
	if limit <= 0 {
		limit = 20
	}
	var out []Message
	for _, id := range s.order {
		m := s.msgs[id]
		if !all && !m.Unread {
			continue
		}
		out = append(out, m)
	}
	if len(out) > limit {
		out = out[len(out)-limit:]
	}
	return out
}

func (s *Store) Unread() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, m := range s.msgs {
		if m.Unread {
			n++
		}
	}
	return n
}

// AddSeen records history that should not be typed into a running terminal.
func (s *Store) AddSeen(m Message) {
	if m.ID == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.msgs[m.ID]; ok {
		return
	}
	m.Unread = false
	m.Delivered = true
	if m.TS == "" {
		m.TS = time.Now().UTC().Format(time.RFC3339)
	}
	if m.Direction == "" {
		m.Direction = "in"
	}
	s.order = append(s.order, m.ID)
	s.msgs[m.ID] = m
	s.trimLocked()
	s.saveLocked()
}

// Due returns unread messages that still need to be typed into the terminal.
// A file is held until the download reports rcvComplete and a local path exists.
func (s *Store) Due(now time.Time, limit int) []Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	if limit <= 0 {
		limit = 1
	}
	var out []Message
	for _, id := range s.order {
		m := s.msgs[id]
		if !messageDue(m, now) {
			continue
		}
		out = append(out, m)
		if len(out) >= limit {
			break
		}
	}
	return out
}

func messageDue(m Message, now time.Time) bool {
	_ = now
	if !m.Unread || m.Delivered || m.Direction == "call" || m.Direction == "notice" || m.Direction == "system" {
		return false
	}
	if m.FileID != 0 && !fileReady(m) {
		return false
	}
	return true
}

func fileReady(m Message) bool {
	switch m.FileStatus {
	case "complete", "rcvComplete":
		return m.FilePath != "" && (filepath.IsAbs(m.FilePath) || strings.ContainsAny(m.FilePath, `/\`))
	default:
		return false
	}
}

func (s *Store) MarkDelivered(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, ok := s.msgs[id]
	if !ok || m.Delivered {
		return false
	}
	m.Delivered = true
	s.msgs[id] = m
	s.saveLocked()
	return true
}

func (s *Store) Pending(now time.Time) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, m := range s.msgs {
		if messageDue(m, now) {
			n++
		}
	}
	return n
}

// Ack marks ids read. It returns how many changed.
func (s *Store) Ack(ids []string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, id := range ids {
		m, ok := s.msgs[id]
		if !ok || !m.Unread {
			continue
		}
		m.Unread = false
		s.msgs[id] = m
		n++
	}
	if n > 0 {
		s.saveLocked()
	}
	return n
}

// Wait blocks until a message is added or updated, or ctx ends.
func (s *Store) Wait(done <-chan struct{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	awake := make(chan struct{})
	go func() {
		select {
		case <-done:
		case <-awake:
			return
		}
		s.mu.Lock()
		s.cond.Broadcast()
		s.mu.Unlock()
	}()
	s.cond.Wait()
	close(awake)
}

func (s *Store) saveLocked() {
	disk := struct {
		Messages []Message `json:"messages"`
	}{}
	for _, id := range s.order {
		disk.Messages = append(disk.Messages, s.msgs[id])
	}
	b, err := json.MarshalIndent(disk, "", "  ")
	if err != nil {
		return
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return
	}
	_ = os.Rename(tmp, s.path)
}
