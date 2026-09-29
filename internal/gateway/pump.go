package gateway

import (
	"time"

	"simplex/internal/inbox"
)

// Screen is how the gateway reads a Herdr pane and submits a prompt to it.
type Screen interface {
	Capture(target string) (Snap, error)
	Submit(target, text string) error
}

// Pump types the oldest due message when the pane is idle.
// The returned id is empty when nothing was submitted.
func Pump(w *Watch, screen Screen, target string, quiet time.Duration, due []inbox.Message, now time.Time) (string, error) {
	if target == "" || len(due) == 0 {
		return "", nil
	}
	// quiet <= 0 submits on the next stable idle look, with no extra delay.
	snap, err := screen.Capture(target)
	if err != nil {
		return "", err
	}
	if snap.At.IsZero() {
		snap.At = now
	}
	if !w.Observe(snap, quiet) {
		return "", nil
	}
	if err := screen.Submit(target, Format(due[0])); err != nil {
		return "", err
	}
	return due[0].ID, nil
}
