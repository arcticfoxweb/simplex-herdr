package inbox

import (
	"encoding/json"
	"testing"
)

func TestMessagesFromNewChatItems(t *testing.T) {
	const raw = `{
	  "type": "newChatItems",
	  "chatItems": [{
	    "chatInfo": {"type": "direct", "contact": {"contactId": 2, "localDisplayName": "bob"}},
	    "chatItem": {
	      "chatDir": {"type": "directRcv"},
	      "meta": {"itemId": 10, "itemTs": "2026-09-27T00:00:00Z", "itemText": "hello"},
	      "content": {"type": "rcvMsgContent", "msgContent": {"type": "text", "text": "hello"}},
	      "file": {"fileId": 9, "fileName": "note.txt", "fileStatus": "new"}
	    }
	  }, {
	    "chatInfo": {"type": "direct", "contact": {"contactId": 2, "localDisplayName": "bob"}},
	    "chatItem": {
	      "chatDir": {"type": "directSnd"},
	      "meta": {"itemId": 11, "itemText": "echo"}
	    }
	  }]
	}`
	ev := decode(t, raw)
	got := MessagesFromEvent(ev)
	if len(got) != 1 {
		t.Fatalf("got %d messages, want 1 inbound", len(got))
	}
	m := got[0]
	if m.From != "bob" || m.Text != "hello" || m.FileID != 9 || m.FileName != "note.txt" {
		t.Fatalf("message = %+v", m)
	}
	if m.ID != "msg:direct:2:10" {
		t.Fatalf("id = %s", m.ID)
	}
}

func TestMessagesFromChatsAndFileComplete(t *testing.T) {
	chats := decode(t, `{
	  "type": "apiChats",
	  "chats": [{
	    "chatInfo": {"type": "group", "groupInfo": {"groupId": 5, "localDisplayName": "agents"}},
	    "chatItems": [{
	      "chatDir": {"type": "groupRcv", "groupMember": {"displayName": "carol"}},
	      "meta": {"itemId": 3, "itemTs": "2026-09-27T01:00:00Z", "itemText": "ping"}
	    }]
	  }]
	}`)
	got := MessagesFromEvent(chats)
	if len(got) != 1 || got[0].From != "carol" || got[0].Chat != "agents" {
		t.Fatalf("group message = %+v", got)
	}

	done := decode(t, `{
	  "type": "rcvFileComplete",
	  "chatItem": {
	    "chatInfo": {"type": "direct", "contact": {"contactId": 2, "localDisplayName": "bob"}},
	    "chatItem": {
	      "chatDir": {"type": "directRcv"},
	      "meta": {"itemId": 10, "itemText": "hello"},
	      "file": {"fileId": 9, "fileName": "note.txt", "fileStatus": "complete", "filePath": "/tmp/note.txt"}
	    }
	  }
	}`)
	got = MessagesFromEvent(done)
	if len(got) != 1 || got[0].FilePath != "/tmp/note.txt" || got[0].FileStatus != "complete" {
		t.Fatalf("file complete = %+v", got)
	}
}

func TestGroupSpeakerNestedFileAndSkipNotices(t *testing.T) {
	ev := decode(t, `{
	  "type": "newChatItems",
	  "chatItems": [{
	    "chatInfo": {"type": "group", "groupInfo": {"groupId": 1, "localDisplayName": "Tangled Development"}},
	    "chatItem": {
	      "chatDir": {"type": "groupRcv", "groupMember": {"localDisplayName": "tangled"}},
	      "meta": {"itemId": 16, "itemText": "Tell me what this is"},
	      "content": {"type": "rcvMsgContent", "msgContent": {"type": "text", "text": "Tell me what this is"}},
	      "file": {"fileId": 4, "fileStatus": {"type": "rcvInvitation"}, "fileSource": {"filePath": "letter.pdf"}}
	    }
	  }, {
	    "chatInfo": {"type": "group", "groupInfo": {"groupId": 1, "localDisplayName": "Tangled Development"}},
	    "chatItem": {
	      "chatDir": {"type": "groupRcv", "groupMember": {"localDisplayName": "tangled"}},
	      "meta": {"itemId": 25, "itemText": "Files and media: on"},
	      "content": {"type": "rcvGroupFeature"}
	    }
	  }, {
	    "chatInfo": {"type": "direct", "contact": {"contactId": 3, "localDisplayName": "tangled"}},
	    "chatItem": {
	      "chatDir": {"type": "directRcv"},
	      "meta": {"itemId": 20, "itemText": "incoming call: calling..."},
	      "content": {"type": "rcvCall"}
	    }
	  }]
	}`)
	got := MessagesFromEvent(ev)
	if len(got) != 2 {
		t.Fatalf("got %d messages: %+v", len(got), got)
	}
	if got[0].From != "tangled" || got[0].Chat != "Tangled Development" || got[0].FileName != "letter.pdf" || got[0].FilePath != "" || got[0].FileStatus != "rcvInvitation" {
		t.Fatalf("file message = %+v", got[0])
	}
	if got[1].Direction != "call" || got[1].ContactID != 3 || got[1].From != "tangled" {
		t.Fatalf("call = %+v", got[1])
	}
}

func TestStoreDedupAndAck(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	s.Add(Message{ID: "a", From: "bob", Text: "one", FileID: 9, FileStatus: "new"})
	s.Add(Message{ID: "a", Text: "one", FilePath: "/tmp/note.txt", FileStatus: "complete"})
	got := s.List(false, 10)
	if len(got) != 1 || got[0].FilePath != "/tmp/note.txt" || !got[0].Unread {
		t.Fatalf("after update = %+v", got)
	}
	if n := s.Ack([]string{"a"}); n != 1 {
		t.Fatalf("ack = %d", n)
	}
	if unread := s.List(false, 10); len(unread) != 0 {
		t.Fatalf("still unread: %+v", unread)
	}
	s2, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	all := s2.List(true, 10)
	if len(all) != 1 || all[0].Unread || all[0].FilePath != "/tmp/note.txt" {
		t.Fatalf("reloaded = %+v", all)
	}
}

func decode(t *testing.T, raw string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatal(err)
	}
	return m
}
