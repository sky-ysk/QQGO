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
	"github.com/qqgo/server/internal/service"
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
	mu         sync.RWMutex
	conns      map[int64]*ws.Conn
	groups     map[string]map[string]bool
	svc        Service
	onStatus   func(qq int64, online bool)
	maxConns   int
	rateLimiter interface {
		Allow(qq int64) bool
		Remove(qq int64)
	}
}

type Service interface {
	HandleMessage(ctx context.Context, msg *model.Message) error
	ValidateToken(qq int64, token string) (bool, error)
	GetOfflineMessages(qq int64) ([]*model.Message, error)
	MarkDelivered(messageID int64) error
	Register(nickname, password string) (int64, error)
	Login(qq int64, password string) (string, string, error)
	LoginWithToken(qq int64, token string) (bool, error)
	RefreshToken(qq int64, refreshToken string) (string, error)
	ClearRefreshToken(qq int64) error

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
	IsFriend(qq1 int64, qq2 int64) bool
	CheckAndIncrementNonFriendMessage(fromQQ int64, toQQ int64) error
	GetHistoryWithTarget(myQQ int64, targetQQ int64, offset int, limit int) ([]*model.Message, bool, error)
	CreateGroup(name string, ownerQQ int64) (string, error)
	JoinGroup(groupID string, qq int64) error
	LeaveGroup(groupID string, qq int64) error
	GetGroupMembers(groupID string) ([]int64, error)
	GetGroupList(qq int64) ([]model.GroupInfo, error)
	GetGroupInfo(groupID string) (*model.GroupInfo, error)
	IsGroupMember(groupID string, qq int64) bool
	GetSessions(qq int64, onlineFunc func(int64) bool) ([]model.SessionInfo, error)
	GetGroupHistory(groupID string, offset int, limit int) ([]*model.Message, bool, error)
	ChangePassword(qq int64, oldPassword, newPassword string) (string, string, error)
	BlockUser(qq int64, blockedQQ int64) error
	UnblockUser(qq int64, blockedQQ int64) error
	IsBlocked(qq int64, blockedQQ int64) bool
	GetBlacklist(qq int64) ([]model.BlockedUserInfo, error)
	MarkRead(messageID int64) error
	RecallMessage(qq int64, messageID int64) error
}

func NewHub(svc Service, onStatus func(int64, bool), maxConns int, rl interface {
	Allow(qq int64) bool
	Remove(qq int64)
}) *Hub {
	return &Hub{
		conns:       make(map[int64]*ws.Conn),
		groups:      make(map[string]map[string]bool),
		svc:         svc,
		onStatus:    onStatus,
		maxConns:    maxConns,
		rateLimiter: rl,
	}
}

func (h *Hub) ServeWS(w http.ResponseWriter, r *http.Request) {
	h.mu.RLock()
	currentConns := len(h.conns)
	h.mu.RUnlock()

	if currentConns >= h.maxConns {
		log.Printf("[conn] connection limit reached (%d/%d)", currentConns, h.maxConns)
		http.Error(w, "connection limit reached", http.StatusServiceUnavailable)
		return
	}

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

	if c.QQ != 0 && h.rateLimiter != nil {
		switch msg.MsgType {
		case model.MsgTypeLogin, model.MsgTypeRegister, model.MsgTypeHeartbeat:
		default:
			if !h.rateLimiter.Allow(c.QQ) {
				log.Printf("[ratelimit] qq=%d exceeded rate limit", c.QQ)
				c.WriteJSON(&model.Message{
					MsgType:   model.MsgTypeServerAck,
					ID:        -1,
					ClientSeq: msg.ClientSeq,
					Content:   "rate limit exceeded",
				})
				return
			}
		}
	}

	switch msg.MsgType {
	case model.MsgTypeLogin:
		h.handleLogin(c, &msg)
	case model.MsgTypeRegister:
		h.handleRegister(c, &msg)
	case model.MsgTypeRefreshToken:
		h.handleRefreshToken(c, &msg)
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
	case model.MsgTypeHistory:
		h.handleHistory(c, &msg)
	case model.MsgTypeGroupHistory:
		h.handleGroupHistory(c, &msg)
	case model.MsgTypeSessionList:
		h.handleSessionList(c, &msg)
	case model.MsgTypeGroupCreate:
		h.handleGroupCreate(c, &msg)
	case model.MsgTypeGroupJoin:
		h.handleGroupJoin(c, &msg)
	case model.MsgTypeGroupLeave:
		h.handleGroupLeave(c, &msg)
	case model.MsgTypeGroupList:
		h.handleGroupList(c, &msg)
	case model.MsgTypeGroupInfo:
		h.handleGroupInfo(c, &msg)
	case model.MsgTypeText, model.MsgTypeImage, model.MsgTypeFile:
		h.handleChatMessage(c, &msg)
	case model.MsgTypeChangePassword:
		h.handleChangePassword(c, &msg)
	case model.MsgTypeBlockUser:
		h.handleBlockUser(c, &msg)
	case model.MsgTypeUnblockUser:
		h.handleUnblockUser(c, &msg)
	case model.MsgTypeBlacklist:
		h.handleBlacklist(c, &msg)
	case model.MsgTypeReadReceipt:
		h.handleReadReceipt(c, &msg)
	case model.MsgTypeRecall:
		h.handleRecall(c, &msg)
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

	accessToken, refreshToken, err := h.svc.Login(qqNumber, req.Password)
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

	h.writeLoginAck(c, &model.LoginResponse{Code: 0, Message: "ok", AccessToken: accessToken, RefreshToken: refreshToken, Online: h.Count(), QQNumber: qqNumber, Nickname: req.Nickname})

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
		accessToken, refreshToken, err := h.svc.Login(req.QQ, req.Password)
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

		h.writeLoginAck(c, &model.LoginResponse{Code: 0, Message: "ok", AccessToken: accessToken, RefreshToken: refreshToken, Online: h.Count(), QQNumber: req.QQ, Nickname: nickname})

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
			msg := "auth failed"
			if err == service.ErrTokenExpired {
				msg = "token expired"
			}
			h.writeLoginAck(c, &model.LoginResponse{Code: 401, Message: msg})
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

		h.writeLoginAck(c, &model.LoginResponse{Code: 0, Message: "ok", AccessToken: req.Token, Online: h.Count(), QQNumber: req.QQ, Nickname: nickname})

		if h.onStatus != nil {
			h.onStatus(req.QQ, true)
		}

		log.Printf("[login] qq=%d login via token, online: %d", req.QQ, h.Count())
		go h.pushOfflineMessages(req.QQ)
		return
	}

	h.writeLoginAck(c, &model.LoginResponse{Code: 400, Message: "password or token required"})
}

func (h *Hub) handleRefreshToken(c *ws.Conn, msg *model.Message) {
	var req model.RefreshTokenRequest
	if err := json.Unmarshal([]byte(msg.Content), &req); err != nil {
		h.writeRefreshTokenAck(c, &model.RefreshTokenResponse{Code: 400, Message: "invalid payload"})
		return
	}

	newAccessToken, err := h.svc.RefreshToken(req.QQ, req.RefreshToken)
	if err != nil {
		msg := "refresh token expired"
		if err == service.ErrInvalidToken {
			msg = "auth failed"
		}
		h.writeRefreshTokenAck(c, &model.RefreshTokenResponse{Code: 401, Message: msg})
		return
	}

	h.writeRefreshTokenAck(c, &model.RefreshTokenResponse{
		Code: 0, Message: "ok", AccessToken: newAccessToken,
	})
}

func (h *Hub) writeRefreshTokenAck(c *ws.Conn, resp *model.RefreshTokenResponse) {
	payload, _ := json.Marshal(resp)
	c.WriteJSON(&model.Message{
		MsgType: model.MsgTypeRefreshTokenAck,
		Content: string(payload),
	})
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

		if h.svc.IsBlocked(msg.ToQQ, c.QQ) {
			log.Printf("[chat] qq=%d has blocked qq=%d", msg.ToQQ, c.QQ)
			c.WriteJSON(&model.Message{
				MsgType:   model.MsgTypeServerAck,
				ID:        -1,
				ClientSeq: msg.ClientSeq,
				Content:   "you are blocked by the recipient",
			})
			return
		}

		if err := h.svc.CheckAndIncrementNonFriendMessage(c.QQ, msg.ToQQ); err != nil {
			log.Printf("[chat] non-friend msg limit: from=%d to=%d err=%v", c.QQ, msg.ToQQ, err)
			c.WriteJSON(&model.Message{
				MsgType:   model.MsgTypeServerAck,
				ID:        -1,
				ClientSeq: msg.ClientSeq,
				Content:   err.Error(),
			})
			return
		}
	} else {
		if !h.svc.IsGroupMember(msg.GroupID, c.QQ) {
			log.Printf("[chat] qq=%d not member of group %s", c.QQ, msg.GroupID)
			c.WriteJSON(&model.Message{
				MsgType:   model.MsgTypeServerAck,
				ID:        -1,
				ClientSeq: msg.ClientSeq,
				Content:   "not group member",
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

func (h *Hub) handleHistory(c *ws.Conn, msg *model.Message) {
	if c.QQ == 0 {
		h.writeFriendError(c, "not logged in")
		return
	}

	var req model.HistoryRequest
	if err := json.Unmarshal([]byte(msg.Content), &req); err != nil {
		h.writeFriendError(c, "invalid payload")
		return
	}

	if req.Limit <= 0 {
		req.Limit = 30
	}

	targetUser, err := h.svc.GetUserByQQ(req.TargetQQ)
	if err != nil {
		h.writeFriendError(c, "user not found")
		return
	}

	msgs, hasMore, err := h.svc.GetHistoryWithTarget(c.QQ, req.TargetQQ, req.Offset, req.Limit)
	if err != nil {
		log.Printf("[history] query error: %v", err)
		h.writeFriendError(c, "query failed")
		return
	}

	historyMsgs := make([]model.HistoryMessage, 0, len(msgs))
	for _, m := range msgs {
		historyMsgs = append(historyMsgs, model.HistoryMessage{
			ID:        m.ID,
			FromQQ:    m.FromQQ,
			ToQQ:      m.ToQQ,
			Content:   m.Content,
			CreatedAt: m.CreatedAt,
		})
	}

	resp := model.HistoryResponse{
		TargetQQ: req.TargetQQ,
		Nickname: targetUser.Nickname,
		Messages: historyMsgs,
		Offset:   req.Offset,
		HasMore:  hasMore,
	}
	payload, _ := json.Marshal(resp)
	c.WriteJSON(&model.Message{
		MsgType: model.MsgTypeHistory,
		Content: string(payload),
	})
}

func (h *Hub) handleGroupHistory(c *ws.Conn, msg *model.Message) {
	if c.QQ == 0 {
		h.writeFriendError(c, "not logged in")
		return
	}

	var req model.GroupHistoryRequest
	if err := json.Unmarshal([]byte(msg.Content), &req); err != nil {
		h.writeFriendError(c, "invalid payload")
		return
	}

	if req.Limit <= 0 {
		req.Limit = 30
	}

	if !h.svc.IsGroupMember(req.GroupID, c.QQ) {
		h.writeFriendError(c, "not group member")
		return
	}

	groupInfo, err := h.svc.GetGroupInfo(req.GroupID)
	if err != nil {
		h.writeFriendError(c, "group not found")
		return
	}

	msgs, hasMore, err := h.svc.GetGroupHistory(req.GroupID, req.Offset, req.Limit)
	if err != nil {
		log.Printf("[group-history] query error: %v", err)
		h.writeFriendError(c, "query failed")
		return
	}

	historyMsgs := make([]model.HistoryMessage, 0, len(msgs))
	for _, m := range msgs {
		historyMsgs = append(historyMsgs, model.HistoryMessage{
			ID:        m.ID,
			FromQQ:    m.FromQQ,
			ToQQ:      m.ToQQ,
			Content:   m.Content,
			CreatedAt: m.CreatedAt,
		})
	}

	resp := model.GroupHistoryResponse{
		GroupID:   req.GroupID,
		GroupName: groupInfo.Name,
		Messages:  historyMsgs,
		Offset:    req.Offset,
		HasMore:   hasMore,
	}
	payload, _ := json.Marshal(resp)
	c.WriteJSON(&model.Message{
		MsgType: model.MsgTypeGroupHistory,
		Content: string(payload),
	})
}

func (h *Hub) handleSessionList(c *ws.Conn, msg *model.Message) {
	if c.QQ == 0 {
		h.writeFriendError(c, "not logged in")
		return
	}

	sessions, err := h.svc.GetSessions(c.QQ, h.isOnline)
	if err != nil {
		log.Printf("[sessions] query error: %v", err)
		h.writeFriendError(c, "query failed")
		return
	}

	resp := model.SessionListResponse{Sessions: sessions}
	payload, _ := json.Marshal(resp)
	c.WriteJSON(&model.Message{
		MsgType: model.MsgTypeSessionList,
		Content: string(payload),
	})
}

func (h *Hub) handleGroupCreate(c *ws.Conn, msg *model.Message) {
	if c.QQ == 0 {
		h.writeFriendError(c, "not logged in")
		return
	}

	var req model.GroupCreateRequest
	if err := json.Unmarshal([]byte(msg.Content), &req); err != nil {
		h.writeFriendError(c, "invalid payload")
		return
	}

	if req.Name == "" {
		h.writeFriendError(c, "group name required")
		return
	}

	groupID, err := h.svc.CreateGroup(req.Name, c.QQ)
	if err != nil {
		h.writeFriendError(c, err.Error())
		return
	}

	h.JoinGroup(c.QQ, groupID)
	log.Printf("[group] created group %s by qq=%d", groupID, c.QQ)

	resp := map[string]interface{}{
		"group_id": groupID,
		"name":     req.Name,
		"message":  "group created",
	}
	payload, _ := json.Marshal(resp)
	c.WriteJSON(&model.Message{
		MsgType: model.MsgTypeGroupCreate,
		Content: string(payload),
	})
}

func (h *Hub) handleGroupJoin(c *ws.Conn, msg *model.Message) {
	if c.QQ == 0 {
		h.writeFriendError(c, "not logged in")
		return
	}

	groupID := msg.Content
	if groupID == "" {
		h.writeFriendError(c, "group_id required")
		return
	}

	if err := h.svc.JoinGroup(groupID, c.QQ); err != nil {
		h.writeFriendError(c, err.Error())
		return
	}

	h.JoinGroup(c.QQ, groupID)
	log.Printf("[group] qq=%d joined group %s", c.QQ, groupID)
	h.writeFriendResult(c, "joined group "+groupID)
}

func (h *Hub) handleGroupLeave(c *ws.Conn, msg *model.Message) {
	if c.QQ == 0 {
		h.writeFriendError(c, "not logged in")
		return
	}

	groupID := msg.Content
	if groupID == "" {
		h.writeFriendError(c, "group_id required")
		return
	}

	if err := h.svc.LeaveGroup(groupID, c.QQ); err != nil {
		h.writeFriendError(c, err.Error())
		return
	}

	h.LeaveGroup(c.QQ, groupID)
	log.Printf("[group] qq=%d left group %s", c.QQ, groupID)
	h.writeFriendResult(c, "left group "+groupID)
}

func (h *Hub) handleGroupList(c *ws.Conn, msg *model.Message) {
	if c.QQ == 0 {
		h.writeFriendError(c, "not logged in")
		return
	}

	groups, err := h.svc.GetGroupList(c.QQ)
	if err != nil {
		h.writeFriendError(c, err.Error())
		return
	}

	resp := model.GroupListResponse{Groups: groups}
	payload, _ := json.Marshal(resp)
	c.WriteJSON(&model.Message{
		MsgType: model.MsgTypeGroupList,
		Content: string(payload),
	})
}

func (h *Hub) handleGroupInfo(c *ws.Conn, msg *model.Message) {
	if c.QQ == 0 {
		h.writeFriendError(c, "not logged in")
		return
	}

	groupID := msg.Content
	if groupID == "" {
		h.writeFriendError(c, "group_id required")
		return
	}

	info, err := h.svc.GetGroupInfo(groupID)
	if err != nil {
		h.writeFriendError(c, err.Error())
		return
	}

	if !h.svc.IsGroupMember(groupID, c.QQ) {
		h.writeFriendError(c, "not group member")
		return
	}

	payload, _ := json.Marshal(info)
	c.WriteJSON(&model.Message{
		MsgType: model.MsgTypeGroupInfo,
		Content: string(payload),
	})
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
	members, err := h.svc.GetGroupMembers(msg.GroupID)
	if err != nil {
		return
	}

	for _, qq := range members {
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

	if h.rateLimiter != nil {
		h.rateLimiter.Remove(qq)
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

func (h *Hub) Shutdown() {
	h.mu.Lock()
	defer h.mu.Unlock()

	for qq, conn := range h.conns {
		conn.Close()
		delete(h.conns, qq)
	}

	log.Printf("[hub] all connections closed, online was: %d", len(h.conns))
}

func (h *Hub) handleChangePassword(c *ws.Conn, msg *model.Message) {
	if c.QQ == 0 {
		h.writeFriendError(c, "not logged in")
		return
	}

	var req model.ChangePasswordRequest
	if err := json.Unmarshal([]byte(msg.Content), &req); err != nil {
		h.writeFriendError(c, "invalid payload")
		return
	}

	if req.OldPassword == "" || req.NewPassword == "" {
		h.writeFriendError(c, "old_password and new_password required")
		return
	}

	accessToken, refreshToken, err := h.svc.ChangePassword(c.QQ, req.OldPassword, req.NewPassword)
	if err != nil {
		h.writeFriendError(c, err.Error())
		return
	}

	log.Printf("[changepw] qq=%d changed password", c.QQ)
	payload, _ := json.Marshal(&model.ChangePasswordResponse{
		Code:         0,
		Message:      "password changed",
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
	})
	c.WriteJSON(&model.Message{
		MsgType: model.MsgTypeChangePasswordAck,
		Content: string(payload),
	})
}

func (h *Hub) handleBlockUser(c *ws.Conn, msg *model.Message) {
	if c.QQ == 0 {
		h.writeFriendError(c, "not logged in")
		return
	}

	qq, err := strconv.ParseInt(msg.Content, 10, 64)
	if err != nil {
		h.writeFriendError(c, "invalid QQ number")
		return
	}

	if err := h.svc.BlockUser(c.QQ, qq); err != nil {
		h.writeFriendError(c, err.Error())
		return
	}

	log.Printf("[block] qq=%d blocked qq=%d", c.QQ, qq)
	h.writeFriendResult(c, "user blocked")
}

func (h *Hub) handleUnblockUser(c *ws.Conn, msg *model.Message) {
	if c.QQ == 0 {
		h.writeFriendError(c, "not logged in")
		return
	}

	qq, err := strconv.ParseInt(msg.Content, 10, 64)
	if err != nil {
		h.writeFriendError(c, "invalid QQ number")
		return
	}

	if err := h.svc.UnblockUser(c.QQ, qq); err != nil {
		h.writeFriendError(c, err.Error())
		return
	}

	log.Printf("[unblock] qq=%d unblocked qq=%d", c.QQ, qq)
	h.writeFriendResult(c, "user unblocked")
}

func (h *Hub) handleBlacklist(c *ws.Conn, msg *model.Message) {
	if c.QQ == 0 {
		h.writeFriendError(c, "not logged in")
		return
	}

	blocked, err := h.svc.GetBlacklist(c.QQ)
	if err != nil {
		h.writeFriendError(c, err.Error())
		return
	}

	resp := model.BlacklistResponse{BlockedUsers: blocked}
	payload, _ := json.Marshal(resp)
	c.WriteJSON(&model.Message{
		MsgType: model.MsgTypeBlacklist,
		Content: string(payload),
	})
}

func (h *Hub) handleReadReceipt(c *ws.Conn, msg *model.Message) {
	if c.QQ == 0 {
		return
	}

	var req model.AckRequest
	if err := json.Unmarshal([]byte(msg.Content), &req); err != nil {
		return
	}

	h.svc.MarkRead(req.MessageID)
}

func (h *Hub) handleRecall(c *ws.Conn, msg *model.Message) {
	if c.QQ == 0 {
		h.writeFriendError(c, "not logged in")
		return
	}

	var req model.RecallRequest
	if err := json.Unmarshal([]byte(msg.Content), &req); err != nil {
		h.writeFriendError(c, "invalid payload")
		return
	}

	if err := h.svc.RecallMessage(c.QQ, req.MessageID); err != nil {
		h.writeFriendError(c, err.Error())
		return
	}

	log.Printf("[recall] qq=%d recalled message id=%d", c.QQ, req.MessageID)
	h.writeFriendResult(c, "message recalled")

	notify := model.RecallNotify{
		MessageID: req.MessageID,
		FromQQ:    c.QQ,
	}
	notifyData, _ := json.Marshal(notify)

	if msg.GroupID != "" {
		members, err := h.svc.GetGroupMembers(msg.GroupID)
		if err == nil {
			for _, qq := range members {
				if qq != c.QQ {
					h.sendToUser(qq, &model.Message{
						MsgType: model.MsgTypeRecallNotify,
						Content: string(notifyData),
					})
				}
			}
		}
	}
}
