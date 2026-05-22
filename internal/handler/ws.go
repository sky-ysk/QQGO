package handler

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
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
	conns    map[int64]*ws.Conn
	groups   map[string]map[string]bool
	svc      Service
	onStatus func(qq int64, online bool)
}

type Service interface {
	HandleMessage(ctx context.Context, msg *model.Message) error
	ValidateToken(qq int64, token string) (bool, error)
	GetGroupMembers(groupID string) ([]string, error)
	GetOfflineMessages(qq int64) ([]*model.Message, error)
	MarkDelivered(messageID int64) error
	Register(nickname, password string) (int64, error)
	Login(qq int64, password string) (string, error)
	LoginWithToken(qq int64, token string) (bool, error)

	SendFriendRequest(fromQQ int64, toQQ int64, message string) error
	AcceptFriend(qq int64, fromQQ int64) error
	RejectFriend(qq int64, fromQQ int64) error
	DeleteFriend(qq int64, friendQQ int64) error
	GetFriendList(qq int64, onlineFunc func(int64) bool) ([]model.FriendInfo, error)
	SearchUsers(keyword string, onlineFunc func(int64) bool) ([]model.UserSearchResult, error)
	MoveFriendGroup(qq int64, friendQQ int64, groupName string) error
	GetFriendGroups(qq int64) ([]string, error)
	SetRemark(qq int64, friendQQ int64, remark string) error
	CreateFriendGroup(qq int64, name string) error
	DeleteFriendGroup(qq int64, name string) error
	GetUserByQQ(qq int64) (*model.User, error)
}

func NewHub(svc Service, onStatus func(int64, bool)) *Hub {
	return &Hub{
		conns:    make(map[int64]*ws.Conn),
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

	log.Printf("[conn] %d read loop exited", c.QQ)
	if c.QQ != 0 {
		h.RemoveUser(c.QQ)
	}
}

func (h *Hub) dispatch(c *ws.Conn, data []byte) {
	var msg model.Message
	if err := json.Unmarshal(data, &msg); err != nil {
		log.Printf("[dispatch] unmarshal error from %d: %v", c.QQ, err)
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
	case model.MsgTypeFriendCreateGroup:
		h.handleFriendCreateGroup(c, &msg)
	case model.MsgTypeFriendDeleteGroup:
		h.handleFriendDeleteGroup(c, &msg)
	case model.MsgTypeCheckUser:
		h.handleCheckUser(c, &msg)
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

	qqNumber, err := h.svc.Register(req.Nickname, req.Password)
	if err != nil {
		log.Printf("[register] failed: %v", err)
		h.writeRegisterAck(c, &model.RegisterResponse{Code: 400, Message: err.Error()}, 0)
		return
	}

	log.Printf("[register] success qq=%d", qqNumber)
	h.writeRegisterAck(c, &model.RegisterResponse{Code: 0, Message: "register ok", QQNumber: qqNumber}, qqNumber)

	token, err := h.svc.Login(qqNumber, req.Password)
	if err != nil {
		log.Printf("[register] auto-login failed for qq=%d: %v", qqNumber, err)
		return
	}

	h.mu.Lock()
	if oldConn, ok := h.conns[qqNumber]; ok {
		oldConn.Close()
	}
	c.QQ = qqNumber
	c.Platform = "cli"
	h.conns[qqNumber] = c
	h.mu.Unlock()

	h.writeLoginAck(c, &model.LoginResponse{Code: 0, Message: "ok", Token: token, Online: h.Count(), QQNumber: qqNumber, Nickname: req.Nickname})

	if h.onStatus != nil {
		h.onStatus(qqNumber, true)
	}

	go h.pushOfflineMessages(qqNumber)
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

	log.Printf("[login] attempting qq=%d", req.QQ)

	if req.Password != "" {
		token, err := h.svc.Login(req.QQ, req.Password)
		if err != nil {
			log.Printf("[login] auth failed for qq=%d: %v", req.QQ, err)
			h.writeLoginAck(c, &model.LoginResponse{Code: 401, Message: "auth failed"})
			return
		}

		user, _ := h.svc.GetUserByQQ(req.QQ)
		nickname := ""
		if user != nil {
			nickname = user.Nickname
		}

		h.mu.Lock()
		if oldConn, ok := h.conns[req.QQ]; ok {
			oldConn.Close()
		}
		c.QQ = req.QQ
		c.Platform = req.Platform
		h.conns[req.QQ] = c
		h.mu.Unlock()

		h.writeLoginAck(c, &model.LoginResponse{Code: 0, Message: "ok", Token: token, Online: h.Count(), QQNumber: req.QQ, Nickname: nickname})

		if h.onStatus != nil {
			h.onStatus(req.QQ, true)
		}

		log.Printf("[login] qq=%d login, online: %d", req.QQ, h.Count())
		go h.pushOfflineMessages(req.QQ)
		return
	}

	if req.Token != "" {
		valid, err := h.svc.LoginWithToken(req.QQ, req.Token)
		if err != nil || !valid {
			h.writeLoginAck(c, &model.LoginResponse{Code: 401, Message: "auth failed"})
			return
		}

		user, _ := h.svc.GetUserByQQ(req.QQ)
		nickname := ""
		if user != nil {
			nickname = user.Nickname
		}

		h.mu.Lock()
		if oldConn, ok := h.conns[req.QQ]; ok {
			oldConn.Close()
		}
		c.QQ = req.QQ
		c.Platform = req.Platform
		h.conns[req.QQ] = c
		h.mu.Unlock()

		h.writeLoginAck(c, &model.LoginResponse{Code: 0, Message: "ok", Token: req.Token, Online: h.Count(), QQNumber: req.QQ, Nickname: nickname})

		if h.onStatus != nil {
			h.onStatus(req.QQ, true)
		}

		log.Printf("[login] qq=%d login via token, online: %d", req.QQ, h.Count())
		go h.pushOfflineMessages(req.QQ)
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
	msg.FromQQ = c.QQ

	if msg.GroupID == "" {
		if _, err := h.svc.GetUserByQQ(msg.ToQQ); err != nil {
			log.Printf("[chat] target QQ %d not found", msg.ToQQ)
			c.WriteJSON(&model.Message{
				MsgType:   model.MsgTypeServerAck,
				ID:        -1,
				ClientSeq: msg.ClientSeq,
				Content:   "user not found",
			})
			return
		}
	}

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
		h.sendToUser(msg.ToQQ, msg)
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

func (h *Hub) isOnline(qq int64) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	_, ok := h.conns[qq]
	return ok
}

func (h *Hub) handleFriendRequest(c *ws.Conn, msg *model.Message) {
	if c.QQ == 0 {
		h.writeFriendError(c, "not logged in")
		return
	}

	var req model.FriendRequestPayload
	if err := json.Unmarshal([]byte(msg.Content), &req); err != nil {
		h.writeFriendError(c, "invalid payload")
		return
	}

	if err := h.svc.SendFriendRequest(c.QQ, req.ToQQNumber, req.Message); err != nil {
		h.writeFriendError(c, err.Error())
		return
	}

	log.Printf("[friend] request from qq=%d to qq=%d", c.QQ, req.ToQQNumber)
	h.writeFriendResult(c, "friend request sent")
	h.notifyFriendRequest(c.QQ, req.ToQQNumber, req.Message)
}

func (h *Hub) handleFriendAccept(c *ws.Conn, msg *model.Message) {
	if c.QQ == 0 {
		h.writeFriendError(c, "not logged in")
		return
	}

	var req model.FriendRequestPayload
	if err := json.Unmarshal([]byte(msg.Content), &req); err != nil {
		h.writeFriendError(c, "invalid payload")
		return
	}

	if err := h.svc.AcceptFriend(c.QQ, req.ToQQNumber); err != nil {
		h.writeFriendError(c, err.Error())
		return
	}

	log.Printf("[friend] qq=%d accepted qq=%d", c.QQ, req.ToQQNumber)
	h.writeFriendResult(c, "friend accepted")
	h.notifyFriendAccepted(c.QQ, req.ToQQNumber)
}

func (h *Hub) handleFriendReject(c *ws.Conn, msg *model.Message) {
	if c.QQ == 0 {
		h.writeFriendError(c, "not logged in")
		return
	}

	var req model.FriendRequestPayload
	if err := json.Unmarshal([]byte(msg.Content), &req); err != nil {
		h.writeFriendError(c, "invalid payload")
		return
	}

	if err := h.svc.RejectFriend(c.QQ, req.ToQQNumber); err != nil {
		h.writeFriendError(c, err.Error())
		return
	}

	log.Printf("[friend] qq=%d rejected qq=%d", c.QQ, req.ToQQNumber)
	h.writeFriendResult(c, "friend request rejected")
}

func (h *Hub) handleFriendDelete(c *ws.Conn, msg *model.Message) {
	if c.QQ == 0 {
		h.writeFriendError(c, "not logged in")
		return
	}

	var req model.FriendRequestPayload
	if err := json.Unmarshal([]byte(msg.Content), &req); err != nil {
		h.writeFriendError(c, "invalid payload")
		return
	}

	if err := h.svc.DeleteFriend(c.QQ, req.ToQQNumber); err != nil {
		h.writeFriendError(c, err.Error())
		return
	}

	log.Printf("[friend] qq=%d deleted friend qq=%d", c.QQ, req.ToQQNumber)
	h.writeFriendResult(c, "friend deleted")
}

func (h *Hub) handleFriendList(c *ws.Conn, msg *model.Message) {
	if c.QQ == 0 {
		h.writeFriendError(c, "not logged in")
		return
	}

	list, err := h.svc.GetFriendList(c.QQ, h.isOnline)
	if err != nil {
		h.writeFriendError(c, err.Error())
		return
	}

	groups, _ := h.svc.GetFriendGroups(c.QQ)

	resp := model.FriendListResponse{Friends: list, AllGroups: groups}
	payload, _ := json.Marshal(resp)
	c.WriteJSON(&model.Message{
		MsgType: model.MsgTypeFriendList,
		Content: string(payload),
	})
}

func (h *Hub) handleFriendSearch(c *ws.Conn, msg *model.Message) {
	if c.QQ == 0 {
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
	if c.QQ == 0 {
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

	if err := h.svc.MoveFriendGroup(c.QQ, req.QQNumber, req.GroupName); err != nil {
		h.writeFriendError(c, err.Error())
		return
	}

	h.writeFriendResult(c, "friend moved to "+req.GroupName)
}

func (h *Hub) handleFriendRemark(c *ws.Conn, msg *model.Message) {
	if c.QQ == 0 {
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

	if err := h.svc.SetRemark(c.QQ, req.QQNumber, req.Remark); err != nil {
		h.writeFriendError(c, err.Error())
		return
	}

	h.writeFriendResult(c, "remark updated")
}

func (h *Hub) handleFriendGroups(c *ws.Conn, msg *model.Message) {
	if c.QQ == 0 {
		h.writeFriendError(c, "not logged in")
		return
	}

	groups, err := h.svc.GetFriendGroups(c.QQ)
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

func (h *Hub) handleFriendCreateGroup(c *ws.Conn, msg *model.Message) {
	if c.QQ == 0 {
		h.writeFriendError(c, "not logged in")
		return
	}
	if err := h.svc.CreateFriendGroup(c.QQ, msg.Content); err != nil {
		h.writeFriendError(c, err.Error())
		return
	}
	log.Printf("[friend] group created: qq=%d name=%s", c.QQ, msg.Content)
	h.writeFriendResult(c, "group created")
}

func (h *Hub) handleFriendDeleteGroup(c *ws.Conn, msg *model.Message) {
	if c.QQ == 0 {
		h.writeFriendError(c, "not logged in")
		return
	}
	if err := h.svc.DeleteFriendGroup(c.QQ, msg.Content); err != nil {
		h.writeFriendError(c, err.Error())
		return
	}
	log.Printf("[friend] group deleted: qq=%d name=%s", c.QQ, msg.Content)
	h.writeFriendResult(c, "group deleted")
}

func (h *Hub) handleCheckUser(c *ws.Conn, msg *model.Message) {
	qq, err := strconv.ParseInt(msg.Content, 10, 64)
	if err != nil {
		payload, _ := json.Marshal(&model.CheckUserResponse{Code: 400, Message: "invalid QQ number"})
		c.WriteJSON(&model.Message{MsgType: model.MsgTypeCheckUser, Content: string(payload)})
		return
	}

	user, err := h.svc.GetUserByQQ(qq)
	if err != nil {
		payload, _ := json.Marshal(&model.CheckUserResponse{Code: 404, Message: "user not found"})
		c.WriteJSON(&model.Message{MsgType: model.MsgTypeCheckUser, Content: string(payload)})
		return
	}

	payload, _ := json.Marshal(&model.CheckUserResponse{
		Code:     0,
		Message:  "ok",
		QQNumber: user.QQNumber,
		Nickname: user.Nickname,
		Online:   h.isOnline(user.QQNumber),
	})
	c.WriteJSON(&model.Message{MsgType: model.MsgTypeCheckUser, Content: string(payload)})
}

func (h *Hub) notifyFriendRequest(fromQQ int64, toQQ int64, message string) {
	toUser, _ := h.svc.GetUserByQQ(toQQ)
	if toUser == nil {
		return
	}

	fromUser, _ := h.svc.GetUserByQQ(fromQQ)
	fromNickname := ""
	if fromUser != nil {
		fromNickname = fromUser.Nickname
	}

	notify := map[string]interface{}{
		"type":          "friend_request",
		"from_qq":       fromQQ,
		"from_nickname": fromNickname,
		"message":       message,
	}
	payload, _ := json.Marshal(notify)

	h.mu.RLock()
	conn, ok := h.conns[toUser.QQNumber]
	h.mu.RUnlock()

	if ok {
		conn.WriteJSON(&model.Message{
			MsgType: model.MsgTypeFriendRequest,
			Content: string(payload),
		})
	}
}

func (h *Hub) notifyFriendAccepted(accepterQQ int64, requesterQQ int64) {
	requester, _ := h.svc.GetUserByQQ(requesterQQ)
	if requester == nil {
		return
	}

	accepter, _ := h.svc.GetUserByQQ(accepterQQ)

	notify := map[string]interface{}{
		"type":              "friend_accepted",
		"accepter_qq":       accepterQQ,
		"accepter_nickname": "",
	}
	if accepter != nil {
		notify["accepter_nickname"] = accepter.Nickname
	}

	payload, _ := json.Marshal(notify)

	h.mu.RLock()
	conn, ok := h.conns[requester.QQNumber]
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

func (h *Hub) pushOfflineMessages(qq int64) {
	msgs, err := h.svc.GetOfflineMessages(qq)
	if err != nil {
		log.Printf("[offline] query error for qq=%d: %v", qq, err)
		return
	}

	if len(msgs) == 0 {
		return
	}

	log.Printf("[offline] pushing %d messages to qq=%d", len(msgs), qq)

	for _, msg := range msgs {
		h.mu.RLock()
		conn, ok := h.conns[qq]
		h.mu.RUnlock()

		if !ok {
			return
		}

		sendMsg := &model.Message{
			ID:        msg.ID,
			MsgType:   msg.MsgType,
			FromQQ:    msg.FromQQ,
			ToQQ:      msg.ToQQ,
			GroupID:   msg.GroupID,
			Content:   msg.Content,
			Delivered: msg.Delivered,
			CreatedAt: msg.CreatedAt,
		}

		if err := conn.WriteJSON(sendMsg); err != nil {
			log.Printf("[offline] push to qq=%d error: %v", qq, err)
			return
		}
	}
}

func (h *Hub) sendToUser(qq int64, msg *model.Message) {
	h.mu.RLock()
	conn, ok := h.conns[qq]
	h.mu.RUnlock()

	if !ok {
		log.Printf("[send] target user qq=%d offline, saved to DB for later delivery", qq)
		return
	}

	if err := conn.WriteJSON(msg); err != nil {
		log.Printf("[send] write to qq=%d error: %v", qq, err)
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
		qq, err := strconv.ParseInt(uid, 10, 64)
		if err != nil {
			continue
		}
		if qq != msg.FromQQ {
			h.sendToUser(qq, msg)
		}
	}
}

func (h *Hub) RemoveUser(qq int64) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if conn, ok := h.conns[qq]; ok {
		conn.Close()
		delete(h.conns, qq)
	}

	for gid, members := range h.groups {
		delete(members, strconv.FormatInt(qq, 10))
		if len(members) == 0 {
			delete(h.groups, gid)
		}
	}

	if h.onStatus != nil {
		h.onStatus(qq, false)
	}

	log.Printf("[conn] user qq=%d disconnected, online: %d", qq, len(h.conns))
}

func (h *Hub) Count() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.conns)
}

func (h *Hub) JoinGroup(qq int64, groupID string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.groups[groupID] == nil {
		h.groups[groupID] = make(map[string]bool)
	}
	h.groups[groupID][strconv.FormatInt(qq, 10)] = true
}

func (h *Hub) LeaveGroup(qq int64, groupID string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if members, ok := h.groups[groupID]; ok {
		delete(members, strconv.FormatInt(qq, 10))
		if len(members) == 0 {
			delete(h.groups, groupID)
		}
	}
}
