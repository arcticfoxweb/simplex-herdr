// Package chattest is a fake simplex-chat WebSocket used by daemon tests.
package chattest

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
)

type Server struct {
	URL  string
	http *httptest.Server

	mu     sync.Mutex
	conns  []*client
	cmds   []string
	extra  []namedGroup
	nextID int64
}

type namedGroup struct {
	ID   int64
	Name string
}

type client struct {
	c  *websocket.Conn
	mu sync.Mutex
}

func Start() *Server {
	s := &Server{}
	s.http = httptest.NewServer(http.HandlerFunc(s.serve))
	s.URL = "ws" + strings.TrimPrefix(s.http.URL, "http")
	return s
}

func (s *Server) Close() {
	s.mu.Lock()
	conns := append([]*client(nil), s.conns...)
	s.mu.Unlock()
	for _, c := range conns {
		c.c.Close(websocket.StatusNormalClosure, "bye")
	}
	s.http.Close()
}

func (s *Server) Commands() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, len(s.cmds))
	copy(out, s.cmds)
	return out
}

func (s *Server) Push(resp map[string]any) {
	body, _ := json.Marshal(map[string]any{"resp": resp})
	s.mu.Lock()
	conns := append([]*client(nil), s.conns...)
	s.mu.Unlock()
	for _, c := range conns {
		c.write(body)
	}
}

func (c *client) write(body []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = c.c.Write(ctx, websocket.MessageText, body)
}

func (s *Server) serve(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
	if err != nil {
		return
	}
	c := &client{c: conn}
	s.mu.Lock()
	s.conns = append(s.conns, c)
	s.mu.Unlock()
	defer conn.Close(websocket.StatusNormalClosure, "bye")
	for {
		_, data, err := conn.Read(r.Context())
		if err != nil {
			return
		}
		var env struct {
			CorrID string `json:"corrId"`
			Cmd    string `json:"cmd"`
		}
		if json.Unmarshal(data, &env) != nil || env.CorrID == "" {
			continue
		}
		s.mu.Lock()
		s.cmds = append(s.cmds, env.Cmd)
		s.mu.Unlock()
		resp := s.reply(env.Cmd)
		body, _ := json.Marshal(map[string]any{"corrId": env.CorrID, "resp": resp})
		c.write(body)
	}
}

func groupDisplayName(cmd string) string {
	i := strings.IndexByte(cmd, '{')
	if i < 0 {
		return ""
	}
	var prof struct {
		DisplayName string `json:"displayName"`
	}
	if json.Unmarshal([]byte(cmd[i:]), &prof) != nil {
		return ""
	}
	return prof.DisplayName
}

func (s *Server) reply(cmd string) map[string]any {
	switch {
	case cmd == "/user":
		return map[string]any{
			"type": "activeUser",
			"user": map[string]any{"userId": 1, "localDisplayName": "alice"},
		}
	case strings.HasPrefix(cmd, "/_show_address"):
		return map[string]any{
			"type":      "chatCmdError",
			"chatError": map[string]any{"type": "NO_ADDRESS"},
		}
	case strings.HasPrefix(cmd, "/_address_settings"):
		return map[string]any{"type": "userContactLinkUpdated"}
	case strings.HasPrefix(cmd, "/_address"):
		return map[string]any{
			"type": "userContactLinkCreated",
			"connLinkContact": map[string]any{
				"connFullLink": "https://simplex.chat/contact#test-alice",
			},
		}
	case strings.HasPrefix(cmd, "/_get chats"):
		return map[string]any{"type": "apiChats", "chats": []any{}}
	case strings.HasPrefix(cmd, "/_groups"):
		s.mu.Lock()
		groups := []any{
			map[string]any{
				"groupInfo": map[string]any{
					"groupId":          1,
					"localDisplayName": "Tangled Development",
					"membership": map[string]any{
						"memberStatus": map[string]any{"type": "member"},
					},
				},
			},
		}
		for _, g := range s.extra {
			groups = append(groups, map[string]any{
				"groupInfo": map[string]any{
					"groupId":          g.ID,
					"localDisplayName": g.Name,
					"membership": map[string]any{
						"memberStatus": map[string]any{"type": "creator"},
					},
				},
			})
		}
		s.mu.Unlock()
		return map[string]any{"type": "groupsList", "groups": groups}
	case strings.HasPrefix(cmd, "/_group "):
		name := groupDisplayName(cmd)
		if name == "" {
			return map[string]any{
				"type":      "chatCmdError",
				"chatError": map[string]any{"type": "TEST", "cmd": cmd},
			}
		}
		s.mu.Lock()
		if s.nextID < 5 {
			s.nextID = 5
		}
		id := s.nextID
		s.nextID++
		s.extra = append(s.extra, namedGroup{ID: id, Name: name})
		s.mu.Unlock()
		return map[string]any{
			"type": "groupCreated",
			"groupInfo": map[string]any{
				"groupId":          id,
				"localDisplayName": name,
				"groupProfile": map[string]any{
					"displayName": name,
					"fullName":    "",
				},
				"membership": map[string]any{
					"memberStatus": map[string]any{"type": "creator"},
				},
			},
		}
	case strings.HasPrefix(cmd, "/_add #"):
		return map[string]any{"type": "sentGroupInvitation"}
	case strings.HasPrefix(cmd, "/_contacts"):
		return map[string]any{
			"type": "contactsList",
			"contacts": []any{
				map[string]any{
					"contactId":        2,
					"localDisplayName": "bob",
					"activeConn": map[string]any{
						"connStatus": map[string]any{"type": "ready"},
					},
				},
			},
		}
	case strings.HasPrefix(cmd, "/_send"):
		return map[string]any{"type": "newChatItems", "chatItems": []any{}}
	case strings.HasPrefix(cmd, "/connect"):
		return map[string]any{"type": "sentInvitation"}
	case strings.HasPrefix(cmd, "/_accept"):
		return map[string]any{"type": "acceptingContactRequest"}
	case strings.HasPrefix(cmd, "/_join "):
		return map[string]any{"type": "userJoinedGroup"}
	case strings.HasPrefix(cmd, "/fcancel "):
		return map[string]any{"type": "rcvFileCancelled"}
	case strings.Contains(cmd, "/freceive 78"):
		return map[string]any{
			"type": "chatCmdError",
			"chatError": map[string]any{
				"type": "error",
				"errorType": map[string]any{
					"type":    "fileAlreadyReceiving",
					"message": "already receiving",
				},
			},
		}
	case strings.HasPrefix(cmd, "/freceive"):
		return map[string]any{"type": "rcvFileAccepted"}
	default:
		return map[string]any{
			"type":      "chatCmdError",
			"chatError": map[string]any{"type": "TEST", "cmd": cmd},
		}
	}
}
