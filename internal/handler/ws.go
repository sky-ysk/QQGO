package handler

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/qqgo/server/internal/model"
	ws "github.com/qqgo/server/pkg/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

type Hub struct {
	mu       sync.RWMutex
	conns    map[string]*ws.Conn
	groups   map[string]map[string]bool
	svc      Service
	onStatus func(uid string, online bool)
}

type Service interface {
	HandleMessage(ctx context.Context, msg *model.Message) error
	ValidateToken(uid, token string) (bool, error)
	GetGroupMembers(groupID string) ([]string, error)
	GetOfflineMessages(uid string) ([]*model.Message, error)
	MarkDelivered(messageID int64) error
	Register(uid, password, nickname string) (int64, error)
	Login(uid, password string) (string, int64, error)
	LoginWithToken(uid, token string) (bool, error)

	SendFriendRequest(fromUID string, toQQNumber int64, message string) error
	AcceptFriend(uid string, fromQQNumber int64) error
	RejectFriend(uid string, fromQQNumber int64) error
	DeleteFriend(uid string, friendQQNumber int64) error
	GetFriendList(uid string, onlineFunc func(string) bool) ([]model.FriendInfo, error)
	SearchUsers(keyword string, onlineFunc func(string) bool) ([]model.UserSearchResult, error)
	MoveFriendGroup(uid string, friendQQNumber int64, groupName string) error
	GetFriendGroups(uid string) ([]string, error)
	SetRemark(uid string, friendQQNumber int64, remark string) error
	GetUserByUID(uid string) (*model.User, error)
	GetUserByQQNumber(qqNumber int64) (*model.User, error)
}

func NewHub(svc Service, onStatus func(string, bool)) *Hub {
	return &Hub{
		conns:    make(map[string]*ws.Conn),
		groups:   make(map[string]map[string]bool),
		svc:      svc,
		onStatus: onStatus,
	}
}

func (h *Hub) ServeWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("upgrade error: %v", err)
		return
	}

	c := ws.NewConn(conn)
	h.handleConnection(c)
}

func (h *Hub) handleConnection(c *ws.Conn) {
	log.Printf("[conn] new connection from %s", c.WS.RemoteAddr())

	go c.WriteLoop()
	c.ReadLoop(func(msgType int, data []byte) {
		if msgType != websocket.TextMessage {
			return
		}
		h.dispatch(c, data)
	})

	log.Printf("[conn] %s read loop exited", c.UID)
	if c.UID != "" {
		h.RemoveUser(c.UID)
	}
}

func (h *Hub) dispatch(c *ws.Conn, data []byte) {
	var msg model.Message
	if err := json.Unmarshal(data, &msg); err != nil {
		log.Printf("[dispatch] unmarshal error from %s: %v", c.UID, err)
		return
	}

	switch msg.MsgType {
	case model.MsgTypeLogin:
		h.handleLogin(c, &msg)
	case model.MsgTypeRegister:
		h.handleRegister(c, &msg)
	case model.MsgTypeHeartbeat:
		h.handleHeartbeat(c)
	case model.MsgTypeDelivered:
		h.handleDeliveredAck(c, &msg)
	case model.MsgTypeFriendRequest:
		h.handleFriendRequest(c, &msg)
	case model.MsgTypeFriendAccept:
		h.handleFriendAccept(c, &msg)
	case model.MsgTypeFriendReject:
		h.handleFriendReject(c, &msg)
	case model.MsgTypeFriendDelete:
		h.handleFriendDelete(c, &msg)
	case model.MsgTypeFriendList:
		h.handleFriendList(c, &msg)
	case model.MsgTypeFriendSearch:
		h.handleFriendSearch(c, &msg)
	case model.MsgTypeFriendMoveGroup:
		h.handleFriendMoveGroup(c, &msg)
	case model.MsgTypeFriendRemark:
		h.handleFriendRemark(c, &msg)
	case model.MsgTypeFriendGroups:
		h.handleFriendGroups(c, &msg)
	case model.MsgTypeText, model.MsgTypeImage, model.MsgTypeFile:
		h.handleChatMessage(c, &msg)
	default:
		log.Printf("[dispatch] unknown msgType=%d", msg.MsgType)
	}
}

func (h *Hub) handleRegister(c *ws.Conn, msg *model.Message) {
	var req model.RegisterRequest
	if err := json.Unmarshal([]byte(msg.Content), &req); err != nil {
		h.writeRegisterAck(c, &model.RegisterResponse{Code: 400, Message: "invalid register payload"}, 0)
		return
	}

	qqNumber, err := h.svc.Register(req.UID, req.Password, req.Nickname)
	if err != nil {
		log.Printf("[register] failed uid=%s: %v", req.UID, err)
		h.writeRegisterAck(c, &model.RegisterResponse{Code: 400, Message: err.Error()}, 0)
		return
	}

	log.Printf("[register] success uid=%s qq=%d", req.UID, qqNumber)
	h.writeRegisterAck(c, &model.RegisterResponse{Code: 0, Message: "register ok", QQNumber: qqNumber}, qqNumber)

	token, _, err := h.svc.Login(req.UID, req.Password)
	if err != nil {
		log.Printf("[register] auto-login failed for %s: %v", req.UID, err)
		return
	}

	h.mu.Lock()
	if oldConn, ok := h.conns[req.UID]; ok {
		oldConn.Close()
	}
	c.UID = req.UID
	c.Platform = "cli"
	h.conns[req.UID] = c
	h.mu.Unlock()

	h.writeLoginAck(c, &model.LoginResponse{Code: 0, Message: "ok", Token: token, Online: h.Count(), QQNumber: qqNumber})

	if h.onStatus != nil {
		h.onStatus(req.UID, true)
	}

	go h.pushOfflineMessages(req.UID)
}

func (h *Hub) writeRegisterAck(c *ws.Conn, resp *model.RegisterResponse, qqNumber int64) {
	payload, _ := json.Marshal(resp)
	c.WriteJSON(&model.Message{
		MsgType: model.MsgTypeRegisterAck,
		Content: string(payload),
	})
}

func (h *Hub) handleLogin(c *ws.Conn, msg *model.Message) {
	var req model.LoginRequest
	if err := json.Unmarshal([]byte(msg.Content), &req); err != nil {
		h.writeLoginAck(c, &model.LoginResponse{Code: 400, Message: "invalid login payload"})
		return
	}

	log.Printf("[login] attempting uid=%s", req.UID)

	if req.Password != "" {
		token, qqNumber, err := h.svc.Login(req.UID, req.Password)
		if err != nil {
			log.Printf("[login] auth failed for %s: %v", req.UID, err)
			h.writeLoginAck(c, &model.LoginResponse{Code: 401, Message: "auth failed"})
			return
		}

		h.mu.Lock()
		if oldConn, ok := h.conns[req.UID]; ok {
			oldConn.Close()
		}
		c.UID = req.UID
		c.Platform = req.Platform
		h.conns[req.UID] = c
		h.mu.Unlock()

		h.writeLoginAck(c, &model.LoginResponse{Code: 0, Message: "ok", Token: token, Online: h.Count(), QQNumber: qqNumber})

		if h.onStatus != nil {
			h.onStatus(req.UID, true)
		}

		log.Printf("[login] user %s login, online: %d", req.UID, h.Count())
		go h.pushOfflineMessages(req.UID)
		return
	}

	if req.Token != "" {
		valid, err := h.svc.LoginWithToken(req.UID, req.Token)
		if err != nil || !valid {
			h.writeLoginAck(c, &model.LoginResponse{Code: 401, Message: "auth failed"})
			return
		}

		user, _ := h.svc.GetUserByUID(req.UID)

		h.mu.Lock()
		if oldConn, ok := h.conns[req.UID]; ok {
			oldConn.Close()
		}
		c.UID = req.UID
		c.Platform = req.Platform
		h.conns[req.UID] = c
		h.mu.Unlock()

		qqNumber := int64(0)
		if user != nil {
			qqNumber = user.QQNumber
		}
		h.writeLoginAck(c, &model.LoginResponse{Code: 0, Message: "ok", Token: req.Token, Online: h.Count(), QQNumber: qqNumber})

		if h.onStatus != nil {
			h.onStatus(req.UID, true)
		}

		log.Printf("[login] user %s login via token, online: %d", req.UID, h.Count())
		go h.pushOfflineMessages(req.UID)
		return
	}

	h.writeLoginAck(c, &model.LoginResponse{Code: 400, Message: "password or token required"})
}

func (h *Hub) writeLoginAck(c *ws.Conn, resp *model.LoginResponse) {
	payload, _ := json.Marshal(resp)
	c.WriteJSON(&model.Message{
		MsgType: model.MsgTypeLoginAck,
		Content: string(payload),
	})
}

func (h *Hub) handleHeartbeat(c *ws.Conn) {
	c.WriteJSON(&model.Message{
		MsgType: model.MsgTypeHeartbeat,
		Content: "pong",
	})
}

func (h *Hub) handleChatMessage(c *ws.Conn, msg *model.Message) {
	msg.FromUID = c.UID

	if err := h.svc.HandleMessage(context.Background(), msg); err != nil {
		log.Printf("[chat] store error: %v", err)
		c.WriteJSON(&model.Message{
			MsgType: model.MsgTypeServerAck,
			ID:      -1,
			Content: "store failed",
		})
		return
	}

	c.WriteJSON(&model.Message{
		MsgType:   model.MsgTypeServerAck,
		ID:        msg.ID,
		ClientSeq: msg.ClientSeq,
		Content:   "ok",
	})

	if msg.GroupID != "" {
		h.broadcastToGroup(msg)
	} else {
		h.sendToUser(msg.ToUID, msg)
	}
}

func (h *Hub) handleDeliveredAck(c *ws.Conn, msg *model.Message) {
	var ack model.AckRequest
	if err := json.Unmarshal([]byte(msg.Content), &ack); err != nil {
		log.Printf("[delivered] parse ack error: %v", err)
		return
	}

	if err := h.svc.MarkDelivered(ack.MessageID); err != nil {
		log.Printf("[delivered] mark error: %v", err)
		return
	}

	log.Printf("[delivered] message id=%d marked delivered", ack.MessageID)
}

func (h *Hub) isOnline(uid string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	_, ok := h.conns[uid]
	return ok
}

func (h *Hub) handleFriendRequest(c *ws.Conn, msg *model.Message) {
	if c.UID == "" {
		h.writeFriendError(c, "not logged in")
		return
	}

	var req model.FriendRequestPayload
	if err := json.Unmarshal([]byte(msg.Content), &req); err != nil {
		h.writeFriendError(c, "invalid payload")
		return
	}

	if err := h.svc.SendFriendRequest(c.UID, req.ToQQNumber, req.Message); err != nil {
		h.writeFriendError(c, err.Error())
		return
	}

	log.Printf("[friend] request from %s to qq=%d", c.UID, req.ToQQNumber)
	h.writeFriendResult(c, "friend request sent")
	h.notifyFriendRequest(c.UID, req.ToQQNumber, req.Message)
}

func (h *Hub) handleFriendAccept(c *ws.Conn, msg *model.Message) {
	if c.UID == "" {
		h.writeFriendError(c, "not logged in")
		return
	}

	var req model.FriendRequestPayload
	if err := json.Unmarshal([]byte(msg.Content), &req); err != nil {
		h.writeFriendError(c, "invalid payload")
		return
	}

	if err := h.svc.AcceptFriend(c.UID, req.ToQQNumber); err != nil {
		h.writeFriendError(c, err.Error())
		return
	}

	log.Printf("[friend] %s accepted qq=%d", c.UID, req.ToQQNumber)
	h.writeFriendResult(c, "friend accepted")
	h.notifyFriendAccepted(c.UID, req.ToQQNumber)
}

func (h *Hub) handleFriendReject(c *ws.Conn, msg *model.Message) {
	if c.UID == "" {
		h.writeFriendError(c, "not logged in")
		return
	}

	var req model.FriendRequestPayload
	if err := json.Unmarshal([]byte(msg.Content), &req); err != nil {
		h.writeFriendError(c, "invalid payload")
		return
	}

	if err := h.svc.RejectFriend(c.UID, req.ToQQNumber); err != nil {
		h.writeFriendError(c, err.Error())
		return
	}

	log.Printf("[friend] %s rejected qq=%d", c.UID, req.ToQQNumber)
	h.writeFriendResult(c, "friend request rejected")
}

func (h *Hub) handleFriendDelete(c *ws.Conn, msg *model.Message) {
	if c.UID == "" {
		h.writeFriendError(c, "not logged in")
		return
	}

	var req model.FriendRequestPayload
	if err := json.Unmarshal([]byte(msg.Content), &req); err != nil {
		h.writeFriendError(c, "invalid payload")
		return
	}

	if err := h.svc.DeleteFriend(c.UID, req.ToQQNumber); err != nil {
		h.writeFriendError(c, err.Error())
		return
	}

	log.Printf("[friend] %s deleted friend qq=%d", c.UID, req.ToQQNumber)
	h.writeFriendResult(c, "friend deleted")
}

func (h *Hub) handleFriendList(c *ws.Conn, msg *model.Message) {
	if c.UID == "" {
		h.writeFriendError(c, "not logged in")
		return
	}

	list, err := h.svc.GetFriendList(c.UID, h.isOnline)
	if err != nil {
		h.writeFriendError(c, err.Error())
		return
	}

	resp := model.FriendListResponse{Friends: list}
	payload, _ := json.Marshal(resp)
	c.WriteJSON(&model.Message{
		MsgType: model.MsgTypeFriendList,
		Content: string(payload),
	})
}

func (h *Hub) handleFriendSearch(c *ws.Conn, msg *model.Message) {
	if c.UID == "" {
		h.writeFriendError(c, "not logged in")
		return
	}

	keyword := msg.Content
	results, err := h.svc.SearchUsers(keyword, h.isOnline)
	if err != nil {
		h.writeFriendError(c, err.Error())
		return
	}

	payload, _ := json.Marshal(results)
	c.WriteJSON(&model.Message{
		MsgType: model.MsgTypeFriendSearch,
		Content: string(payload),
	})
}

func (h *Hub) handleFriendMoveGroup(c *ws.Conn, msg *model.Message) {
	if c.UID == "" {
		h.writeFriendError(c, "not logged in")
		return
	}

	var req struct {
		QQNumber  int64  `json:"qq_number"`
		GroupName string `json:"group_name"`
	}
	if err := json.Unmarshal([]byte(msg.Content), &req); err != nil {
		h.writeFriendError(c, "invalid payload")
		return
	}

	if err := h.svc.MoveFriendGroup(c.UID, req.QQNumber, req.GroupName); err != nil {
		h.writeFriendError(c, err.Error())
		return
	}

	h.writeFriendResult(c, "friend moved to "+req.GroupName)
}

func (h *Hub) handleFriendRemark(c *ws.Conn, msg *model.Message) {
	if c.UID == "" {
		h.writeFriendError(c, "not logged in")
		return
	}

	var req struct {
		QQNumber int64  `json:"qq_number"`
		Remark   string `json:"remark"`
	}
	if err := json.Unmarshal([]byte(msg.Content), &req); err != nil {
		h.writeFriendError(c, "invalid payload")
		return
	}

	if err := h.svc.SetRemark(c.UID, req.QQNumber, req.Remark); err != nil {
		h.writeFriendError(c, err.Error())
		return
	}

	h.writeFriendResult(c, "remark updated")
}

func (h *Hub) handleFriendGroups(c *ws.Conn, msg *model.Message) {
	if c.UID == "" {
		h.writeFriendError(c, "not logged in")
		return
	}

	groups, err := h.svc.GetFriendGroups(c.UID)
	if err != nil {
		h.writeFriendError(c, err.Error())
		return
	}

	resp := model.FriendGroupListResponse{Groups: groups}
	payload, _ := json.Marshal(resp)
	c.WriteJSON(&model.Message{
		MsgType: model.MsgTypeFriendGroups,
		Content: string(payload),
	})
}

func (h *Hub) notifyFriendRequest(fromUID string, toQQNumber int64, message string) {
	toUser, _ := h.svc.GetUserByQQNumber(toQQNumber)
	if toUser == nil {
		return
	}

	fromUser, _ := h.svc.GetUserByUID(fromUID)
	fromNickname := fromUID
	fromQQNumber := int64(0)
	if fromUser != nil {
		fromNickname = fromUser.Nickname
		fromQQNumber = fromUser.QQNumber
	}

	notify := map[string]interface{}{
		"type":         "friend_request",
		"from_uid":     fromUID,
		"from_qq":      fromQQNumber,
		"from_nickname": fromNickname,
		"message":      message,
	}
	payload, _ := json.Marshal(notify)

	h.mu.RLock()
	conn, ok := h.conns[toUser.UID]
	h.mu.RUnlock()

	if ok {
		conn.WriteJSON(&model.Message{
			MsgType: model.MsgTypeFriendRequest,
			Content: string(payload),
		})
	}
}

func (h *Hub) notifyFriendAccepted(fromUID string, fromQQNumber int64) {
	fromUser, _ := h.svc.GetUserByQQNumber(fromQQNumber)
	if fromUser == nil {
		return
	}

	accepter, _ := h.svc.GetUserByUID(fromUID)

	notify := map[string]interface{}{
		"type":           "friend_accepted",
		"accepter_uid":   fromUID,
		"accepter_qq":    int64(0),
		"accepter_nickname": fromUID,
	}
	if accepter != nil {
		notify["accepter_qq"] = accepter.QQNumber
		notify["accepter_nickname"] = accepter.Nickname
	}

	payload, _ := json.Marshal(notify)

	h.mu.RLock()
	conn, ok := h.conns[fromUser.UID]
	h.mu.RUnlock()

	if ok {
		conn.WriteJSON(&model.Message{
			MsgType: model.MsgTypeFriendAccept,
			Content: string(payload),
		})
	}
}

func (h *Hub) writeFriendError(c *ws.Conn, errMsg string) {
	c.WriteJSON(&model.Message{
		MsgType: model.MsgTypeServerAck,
		Content: errMsg,
	})
}

func (h *Hub) writeFriendResult(c *ws.Conn, result string) {
	c.WriteJSON(&model.Message{
		MsgType: model.MsgTypeServerAck,
		Content: result,
	})
}

func (h *Hub) pushOfflineMessages(uid string) {
	msgs, err := h.svc.GetOfflineMessages(uid)
	if err != nil {
		log.Printf("[offline] query error for %s: %v", uid, err)
		return
	}

	if len(msgs) == 0 {
		return
	}

	log.Printf("[offline] pushing %d messages to %s", len(msgs), uid)

	for _, msg := range msgs {
		h.mu.RLock()
		conn, ok := h.conns[uid]
		h.mu.RUnlock()

		if !ok {
			return
		}

		sendMsg := &model.Message{
			ID:        msg.ID,
			MsgType:   msg.MsgType,
			FromUID:   msg.FromUID,
			ToUID:     msg.ToUID,
			GroupID:   msg.GroupID,
			Content:   msg.Content,
			Delivered: msg.Delivered,
			CreatedAt: msg.CreatedAt,
		}

		if err := conn.WriteJSON(sendMsg); err != nil {
			log.Printf("[offline] push to %s error: %v", uid, err)
			return
		}
	}
}

func (h *Hub) sendToUser(uid string, msg *model.Message) {
	h.mu.RLock()
	conn, ok := h.conns[uid]
	h.mu.RUnlock()

	if !ok {
		log.Printf("[send] target user %s offline, saved to DB for later delivery", uid)
		return
	}

	if err := conn.WriteJSON(msg); err != nil {
		log.Printf("[send] write to %s error: %v", uid, err)
	}
}

func (h *Hub) broadcastToGroup(msg *model.Message) {
	h.mu.RLock()
	members, ok := h.groups[msg.GroupID]
	h.mu.RUnlock()

	if !ok {
		return
	}

	for uid := range members {
		if uid != msg.FromUID {
			h.sendToUser(uid, msg)
		}
	}
}

func (h *Hub) RemoveUser(uid string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if conn, ok := h.conns[uid]; ok {
		conn.Close()
		delete(h.conns, uid)
	}

	for gid, members := range h.groups {
		delete(members, uid)
		if len(members) == 0 {
			delete(h.groups, gid)
		}
	}

	if h.onStatus != nil {
		h.onStatus(uid, false)
	}

	log.Printf("[conn] user %s disconnected, online: %d", uid, len(h.conns))
}

func (h *Hub) Count() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.conns)
}

func (h *Hub) JoinGroup(uid, groupID string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.groups[groupID] == nil {
		h.groups[groupID] = make(map[string]bool)
	}
	h.groups[groupID][uid] = true
}

func (h *Hub) LeaveGroup(uid, groupID string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if members, ok := h.groups[groupID]; ok {
		delete(members, uid)
		if len(members) == 0 {
			delete(h.groups, groupID)
		}
	}
}
