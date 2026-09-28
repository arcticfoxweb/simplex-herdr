// Package gateway decides when a Herdr pane is idle and submits one inbox message to it.
package gateway

import (
	"strings"
	"time"
)

// Snap is one look at a Herdr pane.
type Snap struct {
	InMode  bool
	CursorX int
	CursorY int
	Text    string
	At      time.Time
}

// Watch remembers the last screen so a paused draft is not treated as an idle prompt.
type Watch struct {
	last        Snap
	have        bool
	inMode      bool
	typing      bool
	stableSince time.Time
}

// Observe reports whether the pane is idle at an empty prompt.
// Idle means the screen and cursor have been still for quiet, the pane is not in a
// mode, and the last change was not the user typing on the input line.
func (w *Watch) Observe(s Snap, quiet time.Duration) bool {
	if s.At.IsZero() {
		s.At = time.Now()
	}
	if !w.have {
		w.last = s
		w.have = true
		w.inMode = s.InMode
		w.stableSince = s.At
		return false
	}
	if s.InMode {
		w.inMode = true
		w.stableSince = time.Time{}
		w.last = s
		return false
	}
	if w.inMode {
		w.inMode = false
		w.typing = false
		w.stableSince = time.Time{}
		w.last = s
		return false
	}
	same := s.Text == w.last.Text && s.CursorX == w.last.CursorX && s.CursorY == w.last.CursorY
	if same {
		if w.stableSince.IsZero() {
			w.stableSince = w.last.At
		}
		w.last = s
		if w.typing {
			return false
		}
		return !w.stableSince.IsZero() && s.At.Sub(w.stableSince) >= quiet
	}
	if w.last.Text == s.Text || onlyLastLine(w.last.Text, s.Text) {
		w.typing = true
	} else {
		w.typing = false
	}
	w.stableSince = time.Time{}
	w.last = s
	return false
}

func onlyLastLine(a, b string) bool {
	la := trimTail(strings.Split(a, "\n"))
	lb := trimTail(strings.Split(b, "\n"))
	if len(la) == 0 || len(la) != len(lb) {
		return false
	}
	for i := 0; i < len(la)-1; i++ {
		if la[i] != lb[i] {
			return false
		}
	}
	return la[len(la)-1] != lb[len(lb)-1]
}

func trimTail(lines []string) []string {
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}
