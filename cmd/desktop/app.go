package main

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/qqgo/server/internal/protocol"
	"github.com/wailsapp/wails/v2/pkg/runtime"
	"google.golang.org/protobuf/proto"
)

type App struct {
	ctx  context.Context
	conn *websocket.Conn
	qq   int64
	mu   sync.Mutex
	seq  int64
}

func NewApp() *App {
	return &App{}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

func (a *App) shutdown(ctx context.Context) {
	a.Disconnect()
}

func (a *App) Connect(addr string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.conn != nil {
		a.conn.Close()
	}

	conn, _, err := websocket.DefaultDialer.Dial(addr, nil)
	if err != nil {
		return err
	}

	a.conn = conn
	go a.readLoop()
	go a.heartbeatLoop()

	return nil
}

func (a *App) Disconnect() {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.conn != nil {
		a.conn.Close()
		a.conn = nil
	}
}

func (a *App) sendWire(wire *protocol.WireMessage) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.conn == nil {
		return fmt.Errorf("not connected")
	}

	data, err := proto.Marshal(wire)
	if err != nil {
		return err
	}

	return a.conn.WriteMessage(websocket.BinaryMessage, data)
}

func (a *App) nextSeq() int64 {
	a.seq++
	return a.seq
}

func (a *App) readLoop() {
	defer func() {
		runtime.EventsEmit(a.ctx, "connection-lost")
	}()

	for {
		_, data, err := a.conn.ReadMessage()
		if err != nil {
			log.Printf("[ws] read error: %v", err)
			return
		}

		var wire protocol.WireMessage
		if err := proto.Unmarshal(data, &wire); err != nil {
			log.Printf("[ws] unmarshal error: %v", err)
			continue
		}

		a.handleMessage(&wire)
	}
}

func (a *App) heartbeatLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		a.mu.Lock()
		if a.conn == nil {
			a.mu.Unlock()
			return
		}
		a.mu.Unlock()

		a.sendWire(&protocol.WireMessage{
			Payload: &protocol.WireMessage_Heartbeat{
				Heartbeat: &protocol.Heartbeat{Content: "ping"},
			},
		})
	}
}

func (a *App) handleMessage(wire *protocol.WireMessage) {
	switch p := wire.Payload.(type) {
	case *protocol.WireMessage_LoginResponse:
		if p.LoginResponse.Code == 0 {
			a.qq = p.LoginResponse.QqNumber
			runtime.EventsEmit(a.ctx, "login-success", map[string]interface{}{
				"qq":          p.LoginResponse.QqNumber,
				"nickname":    p.LoginResponse.Nickname,
				"accessToken": p.LoginResponse.AccessToken,
			})
		} else {
			runtime.EventsEmit(a.ctx, "login-failed", map[string]interface{}{
				"message": p.LoginResponse.Message,
			})
		}

	case *protocol.WireMessage_RegisterResponse:
		if p.RegisterResponse.Code == 0 {
			runtime.EventsEmit(a.ctx, "register-success", map[string]interface{}{
				"qq": p.RegisterResponse.QqNumber,
			})
		} else {
			runtime.EventsEmit(a.ctx, "register-failed", map[string]interface{}{
				"message": p.RegisterResponse.Message,
			})
		}

	case *protocol.WireMessage_SessionListResponse:
		sessions := make([]map[string]interface{}, 0, len(p.SessionListResponse.Sessions))
		for _, s := range p.SessionListResponse.Sessions {
			sessions = append(sessions, map[string]interface{}{
				"type":        s.Type,
				"targetQQ":    s.TargetQq,
				"groupID":     s.GroupId,
				"nickname":    s.Nickname,
				"lastMessage": s.LastMessage,
				"lastTime":    s.LastTime,
				"online":      s.Online,
			})
		}
		runtime.EventsEmit(a.ctx, "sessions-updated", sessions)

	case *protocol.WireMessage_FriendListResponse:
		friends := make([]map[string]interface{}, 0, len(p.FriendListResponse.Friends))
		for _, f := range p.FriendListResponse.Friends {
			friends = append(friends, map[string]interface{}{
				"qqNumber":  f.QqNumber,
				"nickname":  f.Nickname,
				"groupName": f.GroupName,
				"online":    f.Online,
			})
		}
		runtime.EventsEmit(a.ctx, "friends-loaded", friends)

	case *protocol.WireMessage_HistoryResponse:
		msgs := make([]map[string]interface{}, 0, len(p.HistoryResponse.Messages))
		for _, m := range p.HistoryResponse.Messages {
			msgs = append(msgs, map[string]interface{}{
				"id":        m.Id,
				"fromQQ":    m.FromQq,
				"toQQ":      m.ToQq,
				"content":   m.Content,
				"createdAt": m.CreatedAt,
			})
		}
		runtime.EventsEmit(a.ctx, "history-loaded", msgs)

	case *protocol.WireMessage_TextMessage:
		runtime.EventsEmit(a.ctx, "message-received", map[string]interface{}{
			"id":        wire.Id,
			"fromQQ":    wire.FromQq,
			"toQQ":      wire.ToQq,
			"groupID":   wire.GroupId,
			"content":   p.TextMessage.Content,
			"createdAt": wire.CreatedAt,
		})

	case *protocol.WireMessage_ServerAck:
		runtime.EventsEmit(a.ctx, "message-ack", map[string]interface{}{
			"id":        wire.Id,
			"clientSeq": wire.ClientSeq,
		})
	}
}

func (a *App) Login(qq int64, password string) error {
	return a.sendWire(&protocol.WireMessage{
		ClientSeq: a.nextSeq(),
		Payload: &protocol.WireMessage_LoginRequest{
			LoginRequest: &protocol.LoginRequest{
				Qq:       qq,
				Password: password,
				Platform: "desktop",
			},
		},
	})
}

func (a *App) Register(nickname, password string) error {
	return a.sendWire(&protocol.WireMessage{
		ClientSeq: a.nextSeq(),
		Payload: &protocol.WireMessage_RegisterRequest{
			RegisterRequest: &protocol.RegisterRequest{
				Nickname: nickname,
				Password: password,
			},
		},
	})
}

func (a *App) SendMessage(toQQ int64, content string) error {
	return a.sendWire(&protocol.WireMessage{
		ClientSeq: a.nextSeq(),
		ToQq:      toQQ,
		Payload: &protocol.WireMessage_TextMessage{
			TextMessage: &protocol.TextMessage{
				Content: content,
			},
		},
	})
}

func (a *App) SendGroupMessage(groupID, content string) error {
	return a.sendWire(&protocol.WireMessage{
		ClientSeq: a.nextSeq(),
		GroupId:   groupID,
		Payload: &protocol.WireMessage_TextMessage{
			TextMessage: &protocol.TextMessage{
				Content: content,
			},
		},
	})
}

func (a *App) GetSessions() error {
	return a.sendWire(&protocol.WireMessage{
		ClientSeq: a.nextSeq(),
		Payload: &protocol.WireMessage_SessionListRequest{
			SessionListRequest: &protocol.SessionListRequest{},
		},
	})
}

func (a *App) GetFriendList() error {
	return a.sendWire(&protocol.WireMessage{
		ClientSeq: a.nextSeq(),
		Payload: &protocol.WireMessage_FriendListRequest{
			FriendListRequest: &protocol.FriendListRequest{},
		},
	})
}

func (a *App) GetHistory(targetQQ int64, offset, limit int32) error {
	return a.sendWire(&protocol.WireMessage{
		ClientSeq: a.nextSeq(),
		Payload: &protocol.WireMessage_HistoryRequest{
			HistoryRequest: &protocol.HistoryRequest{
				TargetQq: targetQQ,
				Offset:   offset,
				Limit:    limit,
			},
		},
	})
}
