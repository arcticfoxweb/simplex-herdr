package gateway

import (
	"fmt"
	"strings"

	"simplex/internal/inbox"
)

// Format is the text typed into the agent's pane.
func Format(m inbox.Message) string {
	if m.Direction == "system" {
		return "simplex: " + m.Text
	}
	from := m.From
	if from == "" {
		from = "someone"
	}
	var b strings.Builder
	if m.Chat != "" && m.Chat != from {
		fmt.Fprintf(&b, "%s says in %s [%s]: %s", from, m.Chat, m.ID, m.Text)
	} else {
		fmt.Fprintf(&b, "%s says [%s]: %s", from, m.ID, m.Text)
	}
	if m.FilePath != "" {
		fmt.Fprintf(&b, "\nfile: %s", m.FilePath)
	} else if m.FileName != "" {
		status := m.FileStatus
		if status == "" {
			status = "downloading"
		}
		fmt.Fprintf(&b, "\nfile: %s (%s)", m.FileName, status)
	}
	return b.String()
}
