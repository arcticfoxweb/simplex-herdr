package gateway

import (
	"strings"
	"testing"
	"time"

	"simplex/internal/inbox"
)

type fakeScreen struct {
	snap Snap
	text string
	n    int
}

func (f *fakeScreen) Capture(string) (Snap, error) {
	f.n++
	return f.snap, nil
}

func (f *fakeScreen) Submit(_ string, text string) error {
	f.text = text
	return nil
}

func TestPumpTypesOneIdleMessage(t *testing.T) {
	t0 := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	screen := &fakeScreen{snap: Snap{Text: "ready\n>", CursorX: 1, CursorY: 1}}
	msg := inbox.Message{ID: "msg:direct:2:10", From: "bob", Text: "hello", FilePath: "/tmp/note.txt"}
	var w Watch
	screen.snap.At = t0
	id, err := Pump(&w, screen, "agents:0.0", time.Second, []inbox.Message{msg}, t0)
	if err != nil || id != "" {
		t.Fatalf("first look id=%q err=%v", id, err)
	}
	screen.snap.At = t0.Add(time.Second)
	id, err = Pump(&w, screen, "agents:0.0", time.Second, []inbox.Message{msg}, screen.snap.At)
	if err != nil {
		t.Fatal(err)
	}
	if id != msg.ID {
		t.Fatalf("id = %q", id)
	}
	if !strings.Contains(screen.text, "bob says [msg:direct:2:10]: hello") || !strings.Contains(screen.text, "file: /tmp/note.txt") {
		t.Fatalf("typed %q", screen.text)
	}
}

func TestPumpImmediate(t *testing.T) {
	t0 := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	screen := &fakeScreen{snap: Snap{Text: "ready\n>", At: t0}}
	msg := inbox.Message{ID: "m", From: "tangled", Chat: "Tangled Development", Text: "hi"}
	var w Watch
	if id, _ := Pump(&w, screen, "w1:p1", 0, []inbox.Message{msg}, t0); id != "" {
		t.Fatal("first look should wait for a second sample")
	}
	screen.snap.At = t0.Add(time.Millisecond)
	id, err := Pump(&w, screen, "w1:p1", 0, []inbox.Message{msg}, screen.snap.At)
	if err != nil {
		t.Fatal(err)
	}
	if id != "m" || !strings.Contains(screen.text, "tangled says in Tangled Development [m]: hi") {
		t.Fatalf("id=%q text=%q", id, screen.text)
	}
	if screen.n != 2 {
		t.Fatalf("captures = %d, want one look per pump", screen.n)
	}
}

func TestPumpSkipsDraft(t *testing.T) {
	t0 := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	screen := &fakeScreen{snap: Snap{Text: "ready\n>", CursorX: 1, CursorY: 1, At: t0}}
	var w Watch
	msg := inbox.Message{ID: "m", From: "bob", Text: "hello"}
	_, _ = Pump(&w, screen, "agents:0.0", time.Second, []inbox.Message{msg}, t0)
	screen.snap.Text = "ready\n> draft"
	screen.snap.CursorX = 7
	screen.snap.At = t0.Add(time.Second)
	id, err := Pump(&w, screen, "agents:0.0", time.Second, []inbox.Message{msg}, screen.snap.At)
	if err != nil {
		t.Fatal(err)
	}
	if id != "" || screen.text != "" {
		t.Fatalf("draft was overwritten: id=%q text=%q", id, screen.text)
	}
	screen.snap.At = t0.Add(5 * time.Second)
	id, _ = Pump(&w, screen, "agents:0.0", time.Second, []inbox.Message{msg}, screen.snap.At)
	if id != "" {
		t.Fatal("paused draft should still block delivery")
	}
}
