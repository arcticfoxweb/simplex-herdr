package gateway

import (
	"testing"
	"time"
)

func TestIdleAfterQuiet(t *testing.T) {
	var w Watch
	t0 := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	base := Snap{Text: "ready\n>", CursorX: 1, CursorY: 1, At: t0}
	if w.Observe(base, 2*time.Second) {
		t.Fatal("first sample is not idle")
	}
	base.At = t0.Add(2 * time.Second)
	if !w.Observe(base, 2*time.Second) {
		t.Fatal("stable pane should be idle")
	}
}

func TestTypingBlocksDelivery(t *testing.T) {
	var w Watch
	t0 := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	quiet := 2 * time.Second
	empty := Snap{Text: "ready\n>", CursorX: 1, CursorY: 1, At: t0}
	w.Observe(empty, quiet)
	typed := empty
	typed.Text = "ready\n> hello"
	typed.CursorX = 7
	typed.At = t0.Add(time.Second)
	if w.Observe(typed, quiet) {
		t.Fatal("typing should not be idle")
	}
	typed.At = t0.Add(6 * time.Second)
	if w.Observe(typed, quiet) {
		t.Fatal("a paused draft should stay protected")
	}
}

func TestOutputThenIdle(t *testing.T) {
	var w Watch
	t0 := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	quiet := 2 * time.Second
	w.Observe(Snap{Text: "ready\n> hello", CursorX: 7, At: t0}, quiet)
	// User hits enter. The pane gains history, which clears the typing flag.
	next := Snap{Text: "ready\nhello\nworking\n>", CursorX: 1, At: t0.Add(time.Second)}
	if w.Observe(next, quiet) {
		t.Fatal("fresh output is not idle yet")
	}
	next.At = t0.Add(4 * time.Second)
	if !w.Observe(next, quiet) {
		t.Fatal("settled output should be idle")
	}
}

func TestCopyMode(t *testing.T) {
	var w Watch
	t0 := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	quiet := time.Second
	base := Snap{Text: "ready\n>", At: t0}
	w.Observe(base, quiet)
	base.At = t0.Add(time.Second)
	if !w.Observe(base, quiet) {
		t.Fatal("expected idle")
	}
	mode := base
	mode.InMode = true
	mode.At = t0.Add(2 * time.Second)
	if w.Observe(mode, quiet) {
		t.Fatal("copy mode is not idle")
	}
	back := base
	back.At = t0.Add(3 * time.Second)
	if w.Observe(back, quiet) {
		t.Fatal("leaving copy mode needs a fresh settle")
	}
	back.At = t0.Add(5 * time.Second)
	if !w.Observe(back, quiet) {
		t.Fatal("expected idle after leaving copy mode")
	}
}
