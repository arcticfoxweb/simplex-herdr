package inbox

import (
	"fmt"
	"strings"

	"simplex/internal/jutil"
)

// MessagesFromEvent pulls inbound chat items out of a SimpleX event or API response.
// Outbound items are ignored. The same helper accepts newChatItems and apiChats.
func MessagesFromEvent(ev map[string]any) []Message {
	switch jutil.Type(ev) {
	case "newChatItems":
		return fromAChatItems(jutil.Slice(ev["chatItems"]))
	case "apiChats":
		return fromAChats(jutil.Slice(ev["chats"]))
	case "chatItemUpdated", "rcvFileComplete", "rcvFileAccepted":
		if item := jutil.Obj(ev["chatItem"]); item != nil {
			return fromAChatItems([]any{item})
		}
	}
	return nil
}

func fromAChats(chats []any) []Message {
	var out []Message
	for _, raw := range chats {
		chat := jutil.Obj(raw)
		info := jutil.Obj(chat["chatInfo"])
		for _, rawItem := range jutil.Slice(chat["chatItems"]) {
			if m, ok := itemMessage(info, jutil.Obj(rawItem)); ok {
				out = append(out, m)
			}
		}
	}
	return out
}

func fromAChatItems(items []any) []Message {
	var out []Message
	for _, raw := range items {
		wrap := jutil.Obj(raw)
		info := jutil.Obj(wrap["chatInfo"])
		item := jutil.Obj(wrap["chatItem"])
		if item == nil {
			item = wrap
		}
		if m, ok := itemMessage(info, item); ok {
			out = append(out, m)
		}
	}
	return out
}

func itemMessage(info, item map[string]any) (Message, bool) {
	if item == nil {
		return Message{}, false
	}
	dir := jutil.Obj(item["chatDir"])
	dirType := jutil.Type(dir)
	if dirType != "" && !strings.Contains(strings.ToLower(dirType), "rcv") {
		return Message{}, false
	}
	meta := jutil.Obj(item["meta"])
	itemID := jutil.Int(meta, "itemId")
	content := jutil.Obj(item["content"])
	switch jutil.Type(content) {
	case "", "rcvMsgContent":
	case "rcvCall":
		return callMessage(info, dir, meta, itemID), true
	default:
		// Join text, feature toggles, and other setup events are not messages.
		return Message{}, false
	}
	text := jutil.Str(meta, "itemText")
	if text == "" {
		text = jutil.Str(jutil.Obj(content["msgContent"]), "text")
	}
	file := jutil.Obj(item["file"])
	fileID := jutil.Int(file, "fileId")
	fileName := nestedString(file["fileName"])
	if fileName == "" {
		fileName = nestedString(file["fileDescrText"])
	}
	filePath := nestedString(file["filePath"])
	if filePath == "" {
		filePath = nestedString(file["fileSource"])
	}
	fileStatus := statusString(file["fileStatus"])
	if filePath != "" && !strings.Contains(filePath, "/") && fileStatus != "complete" {
		if fileName == "" {
			fileName = filePath
		}
		filePath = ""
	}
	if text == "" && fileName == "" && fileID == 0 {
		return Message{}, false
	}

	chatName, chatID, from := party(info, dir)
	if from == "" {
		from = chatName
	}
	if chatName == "" {
		chatName = from
	}
	kind := "direct"
	if strings.Contains(strings.ToLower(jutil.Type(info)), "group") || strings.Contains(strings.ToLower(dirType), "group") {
		kind = "group"
	}
	id := fmt.Sprintf("msg:%s:%d:%d", kind, chatID, itemID)
	msg := Message{
		ID:         id,
		From:       from,
		Chat:       chatName,
		Direction:  "in",
		Text:       text,
		FileID:     fileID,
		FileName:   fileName,
		FilePath:   filePath,
		FileStatus: fileStatus,
		ItemID:     itemID,
		TS:         jutil.Str(meta, "itemTs"),
	}
	if kind == "direct" {
		msg.ContactID = chatID
	}
	return msg, true
}

func callMessage(info, dir, meta map[string]any, itemID int64) Message {
	chatName, chatID, from := party(info, dir)
	if from == "" {
		from = chatName
	}
	return Message{
		ID:        fmt.Sprintf("msg:direct:%d:%d", chatID, itemID),
		From:      from,
		Chat:      chatName,
		Direction: "call",
		Text:      "call from " + from,
		ContactID: chatID,
		ItemID:    itemID,
		TS:        jutil.Str(meta, "itemTs"),
	}
}

func party(info, dir map[string]any) (chatName string, chatID int64, from string) {
	switch jutil.Type(info) {
	case "group":
		g := jutil.Obj(info["groupInfo"])
		chatName = jutil.Str(g, "localDisplayName")
		if chatName == "" {
			chatName = jutil.Str(jutil.Obj(g["groupProfile"]), "displayName")
		}
		chatID = jutil.Int(g, "groupId")
	default:
		c := jutil.Obj(info["contact"])
		chatName = jutil.Str(c, "localDisplayName")
		chatID = jutil.Int(c, "contactId")
	}
	member := jutil.Obj(dir["groupMember"])
	from = jutil.Str(member, "localDisplayName")
	if from == "" {
		from = jutil.Str(jutil.Obj(member["memberProfile"]), "displayName")
	}
	if from == "" {
		from = jutil.Str(member, "displayName")
	}
	return chatName, chatID, from
}

func nestedString(v any) string {
	switch t := v.(type) {
	case string:
		if strings.HasPrefix(t, "map[") {
			return ""
		}
		return t
	case map[string]any:
		for _, key := range []string{"filePath", "fileName", "text"} {
			if s := nestedString(t[key]); s != "" {
				return s
			}
		}
	}
	return ""
}

func statusString(v any) string {
	switch t := v.(type) {
	case string:
		if strings.HasPrefix(t, "map[") {
			return ""
		}
		return t
	case map[string]any:
		return jutil.Type(t)
	}
	return ""
}
