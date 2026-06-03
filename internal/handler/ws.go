package handler

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/qqgo/server/internal/model"
	pb "github.com/qqgo/server/internal/protocol"
	"github.com/qqgo/server/internal/service"
	ws "github.com/qqgo/server/pkg/websocket"
	"google.golang.org/protobuf/proto"
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
	onlineTracker interface {
		SetOnline(qq int64)
		SetOffline(qq int64)
		RefreshOnline(qq int64)
		GetInstance(qq int64) (string, bool)
	}
	pubsubRouter interface {
		Subscribe(channels ...string)
		Unsubscribe(channels ...string)
		PublishToUser(qq int64, msg *pb.WireMessage)
		PublishToGroup(groupID string, msg *pb.WireMessage)
	}
	instanceID string
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
	GetHistoryWithTarget(myQQ int64, targetQQ int64, offset int, limit int, fromTime string, toTime string) ([]*model.Message, bool, error)
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
	SearchMessages(myQQ int64, keyword string, targetQQ int64, groupID string, limit int) (*model.SearchResponse, error)
	BackupDB() ([]byte, string, error)
	CleanMessages(days int) (int64, error)
}

func NewHub(svc Service, onStatus func(int64, bool), maxConns int, rl interface {
	Allow(qq int64) bool
	Remove(qq int64)
}, online interface {
	SetOnline(qq int64)
	SetOffline(qq int64)
	RefreshOnline(qq int64)
	GetInstance(qq int64) (string, bool)
}, ps interface {
	Subscribe(channels ...string)
	Unsubscribe(channels ...string)
	PublishToUser(qq int64, msg *pb.WireMessage)
	PublishToGroup(groupID string, msg *pb.WireMessage)
}, instanceID string) *Hub {
	return &Hub{
		conns:         make(map[int64]*ws.Conn),
		groups:        make(map[string]map[string]bool),
		svc:           svc,
		onStatus:      onStatus,
		maxConns:      maxConns,
		rateLimiter:   rl,
		onlineTracker: online,
		pubsubRouter:  ps,
		instanceID:    instanceID,
	}
}

func (h *Hub) SetPubSubRouter(ps interface {
	Subscribe(channels ...string)
	Unsubscribe(channels ...string)
	PublishToUser(qq int64, msg *pb.WireMessage)
	PublishToGroup(groupID string, msg *pb.WireMessage)
}) {
	h.pubsubRouter = ps
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
		if msgType != websocket.BinaryMessage {
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
	var wire pb.WireMessage
	if err := proto.Unmarshal(data, &wire); err != nil {
		log.Printf("[dispatch] unmarshal error from %d: %v", c.QQ, err)
		return
	}

	if c.QQ != 0 && h.rateLimiter != nil {
		switch wire.Payload.(type) {
		case *pb.WireMessage_LoginRequest, *pb.WireMessage_RegisterRequest, *pb.WireMessage_Heartbeat:
		default:
			if !h.rateLimiter.Allow(c.QQ) {
				log.Printf("[ratelimit] qq=%d exceeded rate limit", c.QQ)
				h.writeServerAck(c, -1, wire.ClientSeq, "rate limit exceeded")
				return
			}
		}
	}

	switch p := wire.Payload.(type) {
	case *pb.WireMessage_LoginRequest:
		h.handleLogin(c, &wire, p.LoginRequest)
	case *pb.WireMessage_RegisterRequest:
		h.handleRegister(c, &wire, p.RegisterRequest)
	case *pb.WireMessage_RefreshTokenRequest:
		h.handleRefreshToken(c, &wire, p.RefreshTokenRequest)
	case *pb.WireMessage_Heartbeat:
		h.handleHeartbeat(c)
	case *pb.WireMessage_DeliveredAck:
		h.handleDeliveredAck(c, p.DeliveredAck)
	case *pb.WireMessage_FriendRequest:
		h.handleFriendRequest(c, p.FriendRequest)
	case *pb.WireMessage_FriendAccept:
		h.handleFriendAccept(c, p.FriendAccept)
	case *pb.WireMessage_FriendReject:
		h.handleFriendReject(c, p.FriendReject)
	case *pb.WireMessage_FriendDelete:
		h.handleFriendDelete(c, p.FriendDelete)
	case *pb.WireMessage_FriendListRequest:
		h.handleFriendList(c)
	case *pb.WireMessage_FriendSearchRequest:
		h.handleFriendSearch(c, p.FriendSearchRequest)
	case *pb.WireMessage_FriendMoveGroup:
		h.handleFriendMoveGroup(c, p.FriendMoveGroup)
	case *pb.WireMessage_FriendRemark:
		h.handleFriendRemark(c, p.FriendRemark)
	case *pb.WireMessage_FriendGroupsRequest:
		h.handleFriendGroups(c)
	case *pb.WireMessage_FriendCreateGroup:
		h.handleFriendCreateGroup(c, p.FriendCreateGroup)
	case *pb.WireMessage_FriendDeleteGroup:
		h.handleFriendDeleteGroup(c, p.FriendDeleteGroup)
	case *pb.WireMessage_CheckUserRequest:
		h.handleCheckUser(c, p.CheckUserRequest)
	case *pb.WireMessage_HistoryRequest:
		h.handleHistory(c, p.HistoryRequest)
	case *pb.WireMessage_GroupHistoryRequest:
		h.handleGroupHistory(c, p.GroupHistoryRequest)
	case *pb.WireMessage_SearchMessagesRequest:
		h.handleSearchMessages(c, p.SearchMessagesRequest)
	case *pb.WireMessage_SessionListRequest:
		h.handleSessionList(c)
	case *pb.WireMessage_GroupCreateRequest:
		h.handleGroupCreate(c, p.GroupCreateRequest)
	case *pb.WireMessage_GroupJoinRequest:
		h.handleGroupJoin(c, p.GroupJoinRequest)
	case *pb.WireMessage_GroupLeaveRequest:
		h.handleGroupLeave(c, p.GroupLeaveRequest)
	case *pb.WireMessage_GroupListRequest:
		h.handleGroupList(c)
	case *pb.WireMessage_GroupInfoRequest:
		h.handleGroupInfo(c, p.GroupInfoRequest)
	case *pb.WireMessage_TextMessage:
		h.handleChatMessage(c, &wire, p.TextMessage)
	case *pb.WireMessage_FileMessage:
		h.handleFileMessage(c, &wire, p.FileMessage)
	case *pb.WireMessage_ChangePasswordRequest:
		h.handleChangePassword(c, p.ChangePasswordRequest)
	case *pb.WireMessage_BlockUserRequest:
		h.handleBlockUser(c, p.BlockUserRequest)
	case *pb.WireMessage_UnblockUserRequest:
		h.handleUnblockUser(c, p.UnblockUserRequest)
	case *pb.WireMessage_BlacklistRequest:
		h.handleBlacklist(c)
	case *pb.WireMessage_ReadReceipt:
		h.handleReadReceipt(c, p.ReadReceipt)
	case *pb.WireMessage_RecallRequest:
		h.handleRecall(c, &wire, p.RecallRequest)
	case *pb.WireMessage_BackupRequest:
		h.handleBackup(c)
	case *pb.WireMessage_CleanRequest:
		h.handleClean(c, p.CleanRequest)
	default:
		log.Printf("[dispatch] unknown payload type %T", wire.Payload)
	}
}

func (h *Hub) writeWire(c *ws.Conn, wire *pb.WireMessage) {
	c.WriteProto(wire)
}

func (h *Hub) writeServerAck(c *ws.Conn, id int64, clientSeq int64, content string) {
	h.writeWire(c, &pb.WireMessage{
		Id:        id,
		ClientSeq: clientSeq,
		Payload:   &pb.WireMessage_ServerAck{ServerAck: &pb.ServerAck{Content: content}},
	})
}

func (h *Hub) writeError(c *ws.Conn, errMsg string) {
	h.writeWire(c, &pb.WireMessage{
		Payload: &pb.WireMessage_ServerAck{ServerAck: &pb.ServerAck{Content: errMsg}},
	})
}

func (h *Hub) handleRegister(c *ws.Conn, wire *pb.WireMessage, req *pb.RegisterRequest) {
	qqNumber, err := h.svc.Register(req.Nickname, req.Password)
	if err != nil {
		log.Printf("[register] failed: %v", err)
		h.writeWire(c, &pb.WireMessage{Payload: &pb.WireMessage_RegisterResponse{RegisterResponse: &pb.RegisterResponse{Code: 400, Message: err.Error()}}})
		return
	}

	log.Printf("[register] success qq=%d", qqNumber)
	h.writeWire(c, &pb.WireMessage{Payload: &pb.WireMessage_RegisterResponse{RegisterResponse: &pb.RegisterResponse{Code: 0, Message: "register ok", QqNumber: qqNumber}}})

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

	if h.onlineTracker != nil {
		h.onlineTracker.SetOnline(qqNumber)
	}
	if h.pubsubRouter != nil {
		h.pubsubRouter.Subscribe(fmt.Sprintf("ch:qq:%d", qqNumber))
	}

	h.writeWire(c, &pb.WireMessage{Payload: &pb.WireMessage_LoginResponse{LoginResponse: &pb.LoginResponse{
		Code: 0, Message: "ok", AccessToken: accessToken, RefreshToken: refreshToken,
		Online: int32(h.Count()), QqNumber: qqNumber, Nickname: req.Nickname,
	}}})

	if h.onStatus != nil {
		h.onStatus(qqNumber, true)
	}

	go h.pushOfflineMessages(qqNumber)
}

func (h *Hub) handleLogin(c *ws.Conn, wire *pb.WireMessage, req *pb.LoginRequest) {
	log.Printf("[login] attempting qq=%d", req.Qq)

	if req.Password != "" {
		accessToken, refreshToken, err := h.svc.Login(req.Qq, req.Password)
		if err != nil {
			log.Printf("[login] auth failed for qq=%d: %v", req.Qq, err)
			h.writeWire(c, &pb.WireMessage{Payload: &pb.WireMessage_LoginResponse{LoginResponse: &pb.LoginResponse{Code: 401, Message: "auth failed"}}})
			return
		}

		user, _ := h.svc.GetUserByQQ(req.Qq)
		nickname := ""
		if user != nil {
			nickname = user.Nickname
		}

		h.mu.Lock()
		if oldConn, ok := h.conns[req.Qq]; ok {
			oldConn.Close()
		}
		c.QQ = req.Qq
		c.Platform = req.Platform
		h.conns[req.Qq] = c
		h.mu.Unlock()

		if h.onlineTracker != nil {
			h.onlineTracker.SetOnline(req.Qq)
		}
		if h.pubsubRouter != nil {
			h.pubsubRouter.Subscribe(fmt.Sprintf("ch:qq:%d", req.Qq))
		}

		h.writeWire(c, &pb.WireMessage{Payload: &pb.WireMessage_LoginResponse{LoginResponse: &pb.LoginResponse{
			Code: 0, Message: "ok", AccessToken: accessToken, RefreshToken: refreshToken,
			Online: int32(h.Count()), QqNumber: req.Qq, Nickname: nickname,
		}}})

		if h.onStatus != nil {
			h.onStatus(req.Qq, true)
		}

		log.Printf("[login] qq=%d login, online: %d", req.Qq, h.Count())
		go h.pushOfflineMessages(req.Qq)
		return
	}

	if req.Token != "" {
		valid, err := h.svc.LoginWithToken(req.Qq, req.Token)
		if err != nil || !valid {
			msg := "auth failed"
			if err == service.ErrTokenExpired {
				msg = "token expired"
			}
			h.writeWire(c, &pb.WireMessage{Payload: &pb.WireMessage_LoginResponse{LoginResponse: &pb.LoginResponse{Code: 401, Message: msg}}})
			return
		}

		user, _ := h.svc.GetUserByQQ(req.Qq)
		nickname := ""
		if user != nil {
			nickname = user.Nickname
		}

		h.mu.Lock()
		if oldConn, ok := h.conns[req.Qq]; ok {
			oldConn.Close()
		}
		c.QQ = req.Qq
		c.Platform = req.Platform
		h.conns[req.Qq] = c
		h.mu.Unlock()

		if h.onlineTracker != nil {
			h.onlineTracker.SetOnline(req.Qq)
		}
		if h.pubsubRouter != nil {
			h.pubsubRouter.Subscribe(fmt.Sprintf("ch:qq:%d", req.Qq))
		}

		h.writeWire(c, &pb.WireMessage{Payload: &pb.WireMessage_LoginResponse{LoginResponse: &pb.LoginResponse{
			Code: 0, Message: "ok", AccessToken: req.Token,
			Online: int32(h.Count()), QqNumber: req.Qq, Nickname: nickname,
		}}})

		if h.onStatus != nil {
			h.onStatus(req.Qq, true)
		}

		log.Printf("[login] qq=%d login via token, online: %d", req.Qq, h.Count())
		go h.pushOfflineMessages(req.Qq)
		return
	}

	h.writeWire(c, &pb.WireMessage{Payload: &pb.WireMessage_LoginResponse{LoginResponse: &pb.LoginResponse{Code: 400, Message: "password or token required"}}})
}

func (h *Hub) handleRefreshToken(c *ws.Conn, wire *pb.WireMessage, req *pb.RefreshTokenRequest) {
	newAccessToken, err := h.svc.RefreshToken(req.Qq, req.RefreshToken)
	if err != nil {
		msg := "refresh token expired"
		if err == service.ErrInvalidToken {
			msg = "auth failed"
		}
		h.writeWire(c, &pb.WireMessage{Payload: &pb.WireMessage_RefreshTokenResponse{RefreshTokenResponse: &pb.RefreshTokenResponse{Code: 401, Message: msg}}})
		return
	}

	h.writeWire(c, &pb.WireMessage{Payload: &pb.WireMessage_RefreshTokenResponse{RefreshTokenResponse: &pb.RefreshTokenResponse{
		Code: 0, Message: "ok", AccessToken: newAccessToken,
	}}})
}

func (h *Hub) handleHeartbeat(c *ws.Conn) {
	h.writeWire(c, &pb.WireMessage{Payload: &pb.WireMessage_Heartbeat{Heartbeat: &pb.Heartbeat{Content: "pong"}}})
	if c.QQ != 0 && h.onlineTracker != nil {
		h.onlineTracker.RefreshOnline(c.QQ)
	}
}

func (h *Hub) handleChatMessage(c *ws.Conn, wire *pb.WireMessage, text *pb.TextMessage) {
	if wire.GroupId == "" {
		if _, err := h.svc.GetUserByQQ(wire.ToQq); err != nil {
			log.Printf("[chat] target QQ %d not found", wire.ToQq)
			h.writeServerAck(c, -1, wire.ClientSeq, "user not found")
			return
		}

		if h.svc.IsBlocked(wire.ToQq, c.QQ) {
			log.Printf("[chat] qq=%d has blocked qq=%d", wire.ToQq, c.QQ)
			h.writeServerAck(c, -1, wire.ClientSeq, "you are blocked by the recipient")
			return
		}

		if err := h.svc.CheckAndIncrementNonFriendMessage(c.QQ, wire.ToQq); err != nil {
			log.Printf("[chat] non-friend msg limit: from=%d to=%d err=%v", c.QQ, wire.ToQq, err)
			h.writeServerAck(c, -1, wire.ClientSeq, err.Error())
			return
		}
	} else {
		if !h.svc.IsGroupMember(wire.GroupId, c.QQ) {
			log.Printf("[chat] qq=%d not member of group %s", c.QQ, wire.GroupId)
			h.writeServerAck(c, -1, wire.ClientSeq, "not group member")
			return
		}
	}

	msg := &model.Message{
		MsgType: model.MsgTypeText,
		FromQQ:  c.QQ,
		ToQQ:    wire.ToQq,
		GroupID: wire.GroupId,
		Content: text.Content,
	}

	if err := h.svc.HandleMessage(context.Background(), msg); err != nil {
		log.Printf("[chat] store error: %v", err)
		h.writeWire(c, &pb.WireMessage{Payload: &pb.WireMessage_ServerAck{ServerAck: &pb.ServerAck{Content: "store failed"}}})
		return
	}

	h.writeServerAck(c, msg.ID, wire.ClientSeq, "ok")

	outWire := &pb.WireMessage{
		Id:       msg.ID,
		FromQq:   c.QQ,
		ToQq:     wire.ToQq,
		GroupId:  wire.GroupId,
		CreatedAt: msg.CreatedAt.Unix(),
		Payload:  &pb.WireMessage_TextMessage{TextMessage: text},
	}

	if wire.GroupId != "" {
		h.broadcastToGroup(outWire)
	} else {
		h.sendToUser(wire.ToQq, outWire)
	}
}

func (h *Hub) handleFileMessage(c *ws.Conn, wire *pb.WireMessage, file *pb.FileMessage) {
	if wire.GroupId == "" {
		if _, err := h.svc.GetUserByQQ(wire.ToQq); err != nil {
			h.writeServerAck(c, -1, wire.ClientSeq, "user not found")
			return
		}

		if h.svc.IsBlocked(wire.ToQq, c.QQ) {
			h.writeServerAck(c, -1, wire.ClientSeq, "you are blocked by the recipient")
			return
		}

		if err := h.svc.CheckAndIncrementNonFriendMessage(c.QQ, wire.ToQq); err != nil {
			h.writeServerAck(c, -1, wire.ClientSeq, err.Error())
			return
		}
	} else {
		if !h.svc.IsGroupMember(wire.GroupId, c.QQ) {
			h.writeServerAck(c, -1, wire.ClientSeq, "not group member")
			return
		}
	}

	msgType := model.MsgTypeFile
	content := fmt.Sprintf(`{"filename":"%s","size":%d,"data":"%s"}`, file.Filename, file.Size, string(file.Data))

	msg := &model.Message{
		MsgType: msgType,
		FromQQ:  c.QQ,
		ToQQ:    wire.ToQq,
		GroupID: wire.GroupId,
		Content: content,
	}

	if err := h.svc.HandleMessage(context.Background(), msg); err != nil {
		log.Printf("[chat] store error: %v", err)
		h.writeWire(c, &pb.WireMessage{Payload: &pb.WireMessage_ServerAck{ServerAck: &pb.ServerAck{Content: "store failed"}}})
		return
	}

	h.writeServerAck(c, msg.ID, wire.ClientSeq, "ok")

	outWire := &pb.WireMessage{
		Id:       msg.ID,
		FromQq:   c.QQ,
		ToQq:     wire.ToQq,
		GroupId:  wire.GroupId,
		CreatedAt: msg.CreatedAt.Unix(),
		Payload:  &pb.WireMessage_FileMessage{FileMessage: file},
	}

	if wire.GroupId != "" {
		h.broadcastToGroup(outWire)
	} else {
		h.sendToUser(wire.ToQq, outWire)
	}
}

func (h *Hub) handleDeliveredAck(c *ws.Conn, ack *pb.DeliveredAck) {
	if err := h.svc.MarkDelivered(ack.MessageId); err != nil {
		log.Printf("[delivered] mark error: %v", err)
		return
	}
	log.Printf("[delivered] message id=%d marked delivered", ack.MessageId)
}

func (h *Hub) isOnline(qq int64) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	_, ok := h.conns[qq]
	return ok
}

func (h *Hub) handleFriendRequest(c *ws.Conn, req *pb.FriendRequest) {
	if c.QQ == 0 {
		h.writeError(c, "not logged in")
		return
	}

	if err := h.svc.SendFriendRequest(c.QQ, req.ToQqNumber, req.Message); err != nil {
		h.writeError(c, err.Error())
		return
	}

	log.Printf("[friend] request from qq=%d to qq=%d", c.QQ, req.ToQqNumber)
	h.writeError(c, "friend request sent")
	h.notifyFriendRequest(c.QQ, req.ToQqNumber, req.Message)
}

func (h *Hub) handleFriendAccept(c *ws.Conn, req *pb.FriendAccept) {
	if c.QQ == 0 {
		h.writeError(c, "not logged in")
		return
	}

	if err := h.svc.AcceptFriend(c.QQ, req.ToQqNumber); err != nil {
		h.writeError(c, err.Error())
		return
	}

	log.Printf("[friend] qq=%d accepted qq=%d", c.QQ, req.ToQqNumber)
	h.writeError(c, "friend accepted")
	h.notifyFriendAccepted(c.QQ, req.ToQqNumber)
}

func (h *Hub) handleFriendReject(c *ws.Conn, req *pb.FriendReject) {
	if c.QQ == 0 {
		h.writeError(c, "not logged in")
		return
	}

	if err := h.svc.RejectFriend(c.QQ, req.ToQqNumber); err != nil {
		h.writeError(c, err.Error())
		return
	}

	log.Printf("[friend] qq=%d rejected qq=%d", c.QQ, req.ToQqNumber)
	h.writeError(c, "friend request rejected")
}

func (h *Hub) handleFriendDelete(c *ws.Conn, req *pb.FriendDelete) {
	if c.QQ == 0 {
		h.writeError(c, "not logged in")
		return
	}

	if err := h.svc.DeleteFriend(c.QQ, req.ToQqNumber); err != nil {
		h.writeError(c, err.Error())
		return
	}

	log.Printf("[friend] qq=%d deleted friend qq=%d", c.QQ, req.ToQqNumber)
	h.writeError(c, "friend deleted")
}

func (h *Hub) handleFriendList(c *ws.Conn) {
	if c.QQ == 0 {
		h.writeError(c, "not logged in")
		return
	}

	list, err := h.svc.GetFriendList(c.QQ, h.isOnline)
	if err != nil {
		h.writeError(c, err.Error())
		return
	}

	groups, _ := h.svc.GetFriendGroups(c.QQ)

	friends := make([]*pb.FriendInfo, 0, len(list))
	for _, f := range list {
		friends = append(friends, &pb.FriendInfo{
			QqNumber: f.QQNumber, Nickname: f.Nickname, Remark: f.Remark,
			GroupName: f.GroupName, Status: int32(f.Status), Online: f.Online,
		})
	}

	h.writeWire(c, &pb.WireMessage{Payload: &pb.WireMessage_FriendListResponse{FriendListResponse: &pb.FriendListResponse{
		Friends: friends, AllGroups: groups,
	}}})
}

func (h *Hub) handleFriendSearch(c *ws.Conn, req *pb.FriendSearchRequest) {
	if c.QQ == 0 {
		h.writeError(c, "not logged in")
		return
	}

	results, err := h.svc.SearchUsers(req.Keyword, h.isOnline)
	if err != nil {
		h.writeError(c, err.Error())
		return
	}

	pbResults := make([]*pb.UserSearchResult, 0, len(results))
	for _, r := range results {
		pbResults = append(pbResults, &pb.UserSearchResult{QqNumber: r.QQNumber, Nickname: r.Nickname, Online: r.Online})
	}

	h.writeWire(c, &pb.WireMessage{Payload: &pb.WireMessage_FriendSearchResponse{FriendSearchResponse: &pb.FriendSearchResponse{
		Results: pbResults,
	}}})
}

func (h *Hub) handleFriendMoveGroup(c *ws.Conn, req *pb.FriendMoveGroup) {
	if c.QQ == 0 {
		h.writeError(c, "not logged in")
		return
	}

	if err := h.svc.MoveFriendGroup(c.QQ, req.QqNumber, req.GroupName); err != nil {
		h.writeError(c, err.Error())
		return
	}

	h.writeError(c, "friend moved to "+req.GroupName)
}

func (h *Hub) handleFriendRemark(c *ws.Conn, req *pb.FriendRemark) {
	if c.QQ == 0 {
		h.writeError(c, "not logged in")
		return
	}

	if err := h.svc.SetRemark(c.QQ, req.QqNumber, req.Remark); err != nil {
		h.writeError(c, err.Error())
		return
	}

	h.writeError(c, "remark updated")
}

func (h *Hub) handleFriendGroups(c *ws.Conn) {
	if c.QQ == 0 {
		h.writeError(c, "not logged in")
		return
	}

	groups, err := h.svc.GetFriendGroups(c.QQ)
	if err != nil {
		h.writeError(c, err.Error())
		return
	}

	h.writeWire(c, &pb.WireMessage{Payload: &pb.WireMessage_FriendGroupsResponse{FriendGroupsResponse: &pb.FriendGroupsResponse{
		Groups: groups,
	}}})
}

func (h *Hub) handleFriendCreateGroup(c *ws.Conn, req *pb.FriendCreateGroup) {
	if c.QQ == 0 {
		h.writeError(c, "not logged in")
		return
	}
	if err := h.svc.CreateFriendGroup(c.QQ, req.Name); err != nil {
		h.writeError(c, err.Error())
		return
	}
	log.Printf("[friend] group created: qq=%d name=%s", c.QQ, req.Name)
	h.writeError(c, "group created")
}

func (h *Hub) handleFriendDeleteGroup(c *ws.Conn, req *pb.FriendDeleteGroup) {
	if c.QQ == 0 {
		h.writeError(c, "not logged in")
		return
	}
	if err := h.svc.DeleteFriendGroup(c.QQ, req.Name); err != nil {
		h.writeError(c, err.Error())
		return
	}
	log.Printf("[friend] group deleted: qq=%d name=%s", c.QQ, req.Name)
	h.writeError(c, "group deleted")
}

func (h *Hub) handleCheckUser(c *ws.Conn, req *pb.CheckUserRequest) {
	user, err := h.svc.GetUserByQQ(req.Qq)
	if err != nil {
		h.writeWire(c, &pb.WireMessage{Payload: &pb.WireMessage_CheckUserResponse{CheckUserResponse: &pb.CheckUserResponse{Code: 404, Message: "user not found"}}})
		return
	}

	h.writeWire(c, &pb.WireMessage{Payload: &pb.WireMessage_CheckUserResponse{CheckUserResponse: &pb.CheckUserResponse{
		Code: 0, Message: "ok", QqNumber: user.QQNumber, Nickname: user.Nickname, Online: h.isOnline(user.QQNumber),
	}}})
}

func (h *Hub) handleHistory(c *ws.Conn, req *pb.HistoryRequest) {
	if c.QQ == 0 {
		h.writeError(c, "not logged in")
		return
	}

	if req.Limit <= 0 {
		req.Limit = 30
	}

	targetUser, err := h.svc.GetUserByQQ(req.TargetQq)
	if err != nil {
		h.writeError(c, "user not found")
		return
	}

	msgs, hasMore, err := h.svc.GetHistoryWithTarget(c.QQ, req.TargetQq, int(req.Offset), int(req.Limit), req.FromTime, req.ToTime)
	if err != nil {
		log.Printf("[history] query error: %v", err)
		h.writeError(c, "query failed")
		return
	}

	historyMsgs := make([]*pb.HistoryMessage, 0, len(msgs))
	for _, m := range msgs {
		historyMsgs = append(historyMsgs, &pb.HistoryMessage{
			Id: m.ID, FromQq: m.FromQQ, ToQq: m.ToQQ, Content: m.Content, CreatedAt: m.CreatedAt.Unix(),
		})
	}

	h.writeWire(c, &pb.WireMessage{Payload: &pb.WireMessage_HistoryResponse{HistoryResponse: &pb.HistoryResponse{
		TargetQq: req.TargetQq, Nickname: targetUser.Nickname, Messages: historyMsgs, Offset: req.Offset, HasMore: hasMore,
	}}})
}

func (h *Hub) handleGroupHistory(c *ws.Conn, req *pb.GroupHistoryRequest) {
	if c.QQ == 0 {
		h.writeError(c, "not logged in")
		return
	}

	if req.Limit <= 0 {
		req.Limit = 30
	}

	if !h.svc.IsGroupMember(req.GroupId, c.QQ) {
		h.writeError(c, "not group member")
		return
	}

	groupInfo, err := h.svc.GetGroupInfo(req.GroupId)
	if err != nil {
		h.writeError(c, "group not found")
		return
	}

	msgs, hasMore, err := h.svc.GetGroupHistory(req.GroupId, int(req.Offset), int(req.Limit))
	if err != nil {
		log.Printf("[group-history] query error: %v", err)
		h.writeError(c, "query failed")
		return
	}

	historyMsgs := make([]*pb.HistoryMessage, 0, len(msgs))
	for _, m := range msgs {
		historyMsgs = append(historyMsgs, &pb.HistoryMessage{
			Id: m.ID, FromQq: m.FromQQ, ToQq: m.ToQQ, Content: m.Content, CreatedAt: m.CreatedAt.Unix(),
		})
	}

	h.writeWire(c, &pb.WireMessage{Payload: &pb.WireMessage_GroupHistoryResponse{GroupHistoryResponse: &pb.GroupHistoryResponse{
		GroupId: req.GroupId, GroupName: groupInfo.Name, Messages: historyMsgs, Offset: req.Offset, HasMore: hasMore,
	}}})
}

func (h *Hub) handleSessionList(c *ws.Conn) {
	if c.QQ == 0 {
		h.writeError(c, "not logged in")
		return
	}

	sessions, err := h.svc.GetSessions(c.QQ, h.isOnline)
	if err != nil {
		log.Printf("[sessions] query error: %v", err)
		h.writeError(c, "query failed")
		return
	}

	pbSessions := make([]*pb.SessionInfo, 0, len(sessions))
	for _, s := range sessions {
		pbSessions = append(pbSessions, &pb.SessionInfo{
			Type: s.Type, TargetQq: s.TargetQQ, GroupId: s.GroupID, Nickname: s.Nickname,
			LastMessage: s.LastMessage, LastTime: s.LastTime.Unix(), Online: s.Online, UnreadCount: int32(s.UnreadCount),
		})
	}

	h.writeWire(c, &pb.WireMessage{Payload: &pb.WireMessage_SessionListResponse{SessionListResponse: &pb.SessionListResponse{
		Sessions: pbSessions,
	}}})
}

func (h *Hub) handleSearchMessages(c *ws.Conn, req *pb.SearchMessagesRequest) {
	if c.QQ == 0 {
		h.writeError(c, "not logged in")
		return
	}

	if req.Keyword == "" {
		h.writeError(c, "keyword is required")
		return
	}

	resp, err := h.svc.SearchMessages(c.QQ, req.Keyword, req.TargetQq, req.GroupId, int(req.Limit))
	if err != nil {
		log.Printf("[search] query error: %v", err)
		h.writeError(c, "search failed")
		return
	}

	pbResults := make([]*pb.SearchResultItem, 0, len(resp.Results))
	for _, r := range resp.Results {
		item := &pb.SearchResultItem{
			MessageId: r.MessageID, FromQq: r.FromQQ, ToQq: r.ToQQ, GroupId: r.GroupID,
			Content: r.Content, CreatedAt: r.CreatedAt.Unix(),
		}
		if r.ContextBefore != nil {
			item.ContextBefore = &pb.HistoryMessage{
				Id: r.ContextBefore.ID, FromQq: r.ContextBefore.FromQQ, ToQq: r.ContextBefore.ToQQ,
				Content: r.ContextBefore.Content, CreatedAt: r.ContextBefore.CreatedAt.Unix(),
			}
		}
		if r.ContextAfter != nil {
			item.ContextAfter = &pb.HistoryMessage{
				Id: r.ContextAfter.ID, FromQq: r.ContextAfter.FromQQ, ToQq: r.ContextAfter.ToQQ,
				Content: r.ContextAfter.Content, CreatedAt: r.ContextAfter.CreatedAt.Unix(),
			}
		}
		pbResults = append(pbResults, item)
	}

	h.writeWire(c, &pb.WireMessage{Payload: &pb.WireMessage_SearchMessagesResponse{SearchMessagesResponse: &pb.SearchMessagesResponse{
		Keyword: resp.Keyword, Total: int32(resp.Total), Results: pbResults,
	}}})
}

func (h *Hub) handleGroupCreate(c *ws.Conn, req *pb.GroupCreateRequest) {
	if c.QQ == 0 {
		h.writeError(c, "not logged in")
		return
	}

	if req.Name == "" {
		h.writeError(c, "group name required")
		return
	}

	groupID, err := h.svc.CreateGroup(req.Name, c.QQ)
	if err != nil {
		h.writeError(c, err.Error())
		return
	}

	h.JoinGroup(c.QQ, groupID)
	log.Printf("[group] created group %s by qq=%d", groupID, c.QQ)

	h.writeWire(c, &pb.WireMessage{Payload: &pb.WireMessage_GroupCreateResponse{GroupCreateResponse: &pb.GroupCreateResponse{
		GroupId: groupID, Name: req.Name, Message: "group created",
	}}})
}

func (h *Hub) handleGroupJoin(c *ws.Conn, req *pb.GroupJoinRequest) {
	if c.QQ == 0 {
		h.writeError(c, "not logged in")
		return
	}

	if req.GroupId == "" {
		h.writeError(c, "group_id required")
		return
	}

	if err := h.svc.JoinGroup(req.GroupId, c.QQ); err != nil {
		h.writeError(c, err.Error())
		return
	}

	h.JoinGroup(c.QQ, req.GroupId)
	log.Printf("[group] qq=%d joined group %s", c.QQ, req.GroupId)
	h.writeError(c, "joined group "+req.GroupId)
}

func (h *Hub) handleGroupLeave(c *ws.Conn, req *pb.GroupLeaveRequest) {
	if c.QQ == 0 {
		h.writeError(c, "not logged in")
		return
	}

	if req.GroupId == "" {
		h.writeError(c, "group_id required")
		return
	}

	if err := h.svc.LeaveGroup(req.GroupId, c.QQ); err != nil {
		h.writeError(c, err.Error())
		return
	}

	h.LeaveGroup(c.QQ, req.GroupId)
	log.Printf("[group] qq=%d left group %s", c.QQ, req.GroupId)
	h.writeError(c, "left group "+req.GroupId)
}

func (h *Hub) handleGroupList(c *ws.Conn) {
	if c.QQ == 0 {
		h.writeError(c, "not logged in")
		return
	}

	groups, err := h.svc.GetGroupList(c.QQ)
	if err != nil {
		h.writeError(c, err.Error())
		return
	}

	pbGroups := make([]*pb.GroupInfo, 0, len(groups))
	for _, g := range groups {
		pbGroups = append(pbGroups, &pb.GroupInfo{
			GroupId: g.GroupID, Name: g.Name, OwnerQq: g.OwnerQQ, MemberCnt: int32(g.MemberCnt),
		})
	}

	h.writeWire(c, &pb.WireMessage{Payload: &pb.WireMessage_GroupListResponse{GroupListResponse: &pb.GroupListResponse{
		Groups: pbGroups,
	}}})
}

func (h *Hub) handleGroupInfo(c *ws.Conn, req *pb.GroupInfoRequest) {
	if c.QQ == 0 {
		h.writeError(c, "not logged in")
		return
	}

	if req.GroupId == "" {
		h.writeError(c, "group_id required")
		return
	}

	info, err := h.svc.GetGroupInfo(req.GroupId)
	if err != nil {
		h.writeError(c, err.Error())
		return
	}

	if !h.svc.IsGroupMember(req.GroupId, c.QQ) {
		h.writeError(c, "not group member")
		return
	}

	h.writeWire(c, &pb.WireMessage{Payload: &pb.WireMessage_GroupInfoResponse{GroupInfoResponse: &pb.GroupInfoResponse{
		GroupId: info.GroupID, Name: info.Name, OwnerQq: info.OwnerQQ, MemberCnt: int32(info.MemberCnt),
	}}})
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

	h.mu.RLock()
	conn, ok := h.conns[toUser.QQNumber]
	h.mu.RUnlock()

	if ok {
		h.writeWire(conn, &pb.WireMessage{Payload: &pb.WireMessage_FriendRequest{FriendRequest: &pb.FriendRequest{
			ToQqNumber: fromQQ, Message: fmt.Sprintf(`{"from_qq":%d,"from_nickname":"%s","message":"%s"}`, fromQQ, fromNickname, message),
		}}})
	}
}

func (h *Hub) notifyFriendAccepted(accepterQQ int64, requesterQQ int64) {
	requester, _ := h.svc.GetUserByQQ(requesterQQ)
	if requester == nil {
		return
	}

	accepter, _ := h.svc.GetUserByQQ(accepterQQ)
	_ = accepter

	h.mu.RLock()
	conn, ok := h.conns[requester.QQNumber]
	h.mu.RUnlock()

	if ok {
		h.writeWire(conn, &pb.WireMessage{Payload: &pb.WireMessage_FriendAccept{FriendAccept: &pb.FriendAccept{
			ToQqNumber: accepterQQ,
		}}})
	}
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

		sendWire := &pb.WireMessage{
			Id:        msg.ID,
			FromQq:    msg.FromQQ,
			ToQq:      msg.ToQQ,
			GroupId:   msg.GroupID,
			CreatedAt: msg.CreatedAt.Unix(),
		}

		switch msg.MsgType {
		case model.MsgTypeText:
			sendWire.Payload = &pb.WireMessage_TextMessage{TextMessage: &pb.TextMessage{Content: msg.Content}}
		case model.MsgTypeImage, model.MsgTypeFile:
			sendWire.Payload = &pb.WireMessage_FileMessage{FileMessage: &pb.FileMessage{
				Filename: msg.Content, Data: []byte(msg.Content),
			}}
		default:
			sendWire.Payload = &pb.WireMessage_TextMessage{TextMessage: &pb.TextMessage{Content: msg.Content}}
		}

		if err := conn.WriteProto(sendWire); err != nil {
			log.Printf("[offline] push to qq=%d error: %v", qq, err)
			return
		}
	}
}

func (h *Hub) sendToUser(qq int64, msg *pb.WireMessage) {
	h.mu.RLock()
	conn, ok := h.conns[qq]
	h.mu.RUnlock()

	if ok {
		if err := conn.WriteProto(msg); err != nil {
			log.Printf("[send] write to qq=%d error: %v", qq, err)
		}
		return
	}

	if h.pubsubRouter != nil && h.onlineTracker != nil {
		if instanceID, online := h.onlineTracker.GetInstance(qq); online && instanceID != h.instanceID {
			h.pubsubRouter.PublishToUser(qq, msg)
			return
		}
	}

	log.Printf("[send] target user qq=%d offline, saved to DB for later delivery", qq)
}

func (h *Hub) broadcastToGroup(msg *pb.WireMessage) {
	if h.pubsubRouter != nil && msg.GroupId != "" {
		h.pubsubRouter.PublishToGroup(msg.GroupId, msg)
	}

	members, err := h.svc.GetGroupMembers(msg.GroupId)
	if err != nil {
		return
	}

	for _, qq := range members {
		if qq != msg.FromQq {
			h.mu.RLock()
			conn, ok := h.conns[qq]
			h.mu.RUnlock()
			if ok {
				conn.WriteProto(msg)
			}
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

	if h.onlineTracker != nil {
		h.onlineTracker.SetOffline(qq)
	}
	if h.pubsubRouter != nil {
		h.pubsubRouter.Unsubscribe(fmt.Sprintf("ch:qq:%d", qq))
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

func (h *Hub) handleChangePassword(c *ws.Conn, req *pb.ChangePasswordRequest) {
	if c.QQ == 0 {
		h.writeError(c, "not logged in")
		return
	}

	if req.OldPassword == "" || req.NewPassword == "" {
		h.writeError(c, "old_password and new_password required")
		return
	}

	accessToken, refreshToken, err := h.svc.ChangePassword(c.QQ, req.OldPassword, req.NewPassword)
	if err != nil {
		h.writeError(c, err.Error())
		return
	}

	log.Printf("[changepw] qq=%d changed password", c.QQ)
	h.writeWire(c, &pb.WireMessage{Payload: &pb.WireMessage_ChangePasswordResponse{ChangePasswordResponse: &pb.ChangePasswordResponse{
		Code: 0, Message: "password changed", AccessToken: accessToken, RefreshToken: refreshToken,
	}}})
}

func (h *Hub) handleBlockUser(c *ws.Conn, req *pb.BlockUserRequest) {
	if c.QQ == 0 {
		h.writeError(c, "not logged in")
		return
	}

	if err := h.svc.BlockUser(c.QQ, req.Qq); err != nil {
		h.writeError(c, err.Error())
		return
	}

	log.Printf("[block] qq=%d blocked qq=%d", c.QQ, req.Qq)
	h.writeError(c, "user blocked")
}

func (h *Hub) handleUnblockUser(c *ws.Conn, req *pb.UnblockUserRequest) {
	if c.QQ == 0 {
		h.writeError(c, "not logged in")
		return
	}

	if err := h.svc.UnblockUser(c.QQ, req.Qq); err != nil {
		h.writeError(c, err.Error())
		return
	}

	log.Printf("[unblock] qq=%d unblocked qq=%d", c.QQ, req.Qq)
	h.writeError(c, "user unblocked")
}

func (h *Hub) handleBlacklist(c *ws.Conn) {
	if c.QQ == 0 {
		h.writeError(c, "not logged in")
		return
	}

	blocked, err := h.svc.GetBlacklist(c.QQ)
	if err != nil {
		h.writeError(c, err.Error())
		return
	}

	pbBlocked := make([]*pb.BlockedUserInfo, 0, len(blocked))
	for _, b := range blocked {
		pbBlocked = append(pbBlocked, &pb.BlockedUserInfo{QqNumber: b.QQNumber, Nickname: b.Nickname})
	}

	h.writeWire(c, &pb.WireMessage{Payload: &pb.WireMessage_BlacklistResponse{BlacklistResponse: &pb.BlacklistResponse{
		BlockedUsers: pbBlocked,
	}}})
}

func (h *Hub) handleReadReceipt(c *ws.Conn, receipt *pb.ReadReceipt) {
	if c.QQ == 0 {
		return
	}
	h.svc.MarkRead(receipt.MessageId)
}

func (h *Hub) handleRecall(c *ws.Conn, wire *pb.WireMessage, req *pb.RecallRequest) {
	if c.QQ == 0 {
		h.writeError(c, "not logged in")
		return
	}

	if err := h.svc.RecallMessage(c.QQ, req.MessageId); err != nil {
		h.writeError(c, err.Error())
		return
	}

	log.Printf("[recall] qq=%d recalled message id=%d", c.QQ, req.MessageId)
	h.writeError(c, "message recalled")

	notify := &pb.RecallNotify{MessageId: req.MessageId, FromQq: c.QQ, GroupId: wire.GroupId}

	if wire.GroupId != "" {
		members, err := h.svc.GetGroupMembers(wire.GroupId)
		if err == nil {
			for _, qq := range members {
				if qq != c.QQ {
					h.sendToUser(qq, &pb.WireMessage{Payload: &pb.WireMessage_RecallNotify{RecallNotify: notify}})
				}
			}
		}
	}
}

func (h *Hub) handleBackup(c *ws.Conn) {
	if c.QQ == 0 {
		h.writeError(c, "not logged in")
		return
	}

	data, filename, err := h.svc.BackupDB()
	if err != nil {
		log.Printf("[backup] failed: %v", err)
		h.writeError(c, "backup failed: "+err.Error())
		return
	}

	log.Printf("[backup] qq=%d exported %s (%d bytes)", c.QQ, filename, len(data))

	h.writeWire(c, &pb.WireMessage{Payload: &pb.WireMessage_BackupResponse{BackupResponse: &pb.BackupResponse{
		Code: 0, Message: "ok", Filename: filename, Size: int64(len(data)), Data: data,
	}}})
}

func (h *Hub) handleClean(c *ws.Conn, req *pb.CleanRequest) {
	if c.QQ == 0 {
		h.writeError(c, "not logged in")
		return
	}

	deleted, err := h.svc.CleanMessages(int(req.Days))
	if err != nil {
		log.Printf("[clean] failed: %v", err)
		h.writeError(c, "clean failed: "+err.Error())
		return
	}

	log.Printf("[clean] qq=%d deleted %d messages older than %d days", c.QQ, deleted, req.Days)

	h.writeWire(c, &pb.WireMessage{Payload: &pb.WireMessage_CleanResponse{CleanResponse: &pb.CleanResponse{
		Code: 0, Message: "ok", Deleted: deleted,
	}}})
}

func (h *Hub) handlePubSubMessage(qq int64, msg *pb.WireMessage) {
	if qq != 0 {
		h.mu.RLock()
		conn, ok := h.conns[qq]
		h.mu.RUnlock()
		if ok {
			conn.WriteProto(msg)
		}
		return
	}
	if msg.GroupId != "" {
		members, err := h.svc.GetGroupMembers(msg.GroupId)
		if err != nil {
			return
		}
		for _, memberQQ := range members {
			if memberQQ != msg.FromQq {
				h.mu.RLock()
				conn, ok := h.conns[memberQQ]
				h.mu.RUnlock()
				if ok {
					conn.WriteProto(msg)
				}
			}
		}
	}
}

func (h *Hub) HandlePubSubMessage(qq int64, msg *pb.WireMessage) {
	h.handlePubSubMessage(qq, msg)
}
