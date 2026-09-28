package inbox

import (
	"testing"
	"time"
)

func TestDueWaitsForFilePath(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	s.Add(Message{
		ID: "msg:direct:2:1", From: "bob", Text: "see file", FileID: 9, FileName: "note.txt",
		FileStatus: "new", TS: now.Format(time.RFC3339),
	})
	if got := s.Due(now.Add(time.Second), 5); len(got) != 0 {
		t.Fatalf("file without a path should wait, got %+v", got)
	}
	if got := s.Due(now.Add(21*time.Second), 5); len(got) != 1 {
		t.Fatalf("wait should end, got %d", len(got))
	}
	s.Add(Message{ID: "msg:direct:2:1", FilePath: "/tmp/note.txt", FileStatus: "complete"})
	if got := s.Due(now.Add(21*time.Second), 5); len(got) != 1 || got[0].FilePath != "/tmp/note.txt" {
		t.Fatalf("path update should be due again, got %+v", got)
	}
	if !s.MarkDelivered("msg:direct:2:1") {
		t.Fatal("mark")
	}
	if got := s.Due(now.Add(21*time.Second), 5); len(got) != 0 {
		t.Fatalf("delivered message should leave the queue, got %+v", got)
	}
}
