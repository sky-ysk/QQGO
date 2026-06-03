package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/qqgo/server/internal/model"
	pb "github.com/qqgo/server/internal/protocol"
	"github.com/qqgo/server/internal/service"
	ws "github.com/qqgo/server/pkg/websocket"
	"google.golang.org/protobuf/proto"
)

type mockService struct {
	mu             sync.Mutex
	registerQQ     int64
	registerErr    error
	loginAccess    string
	loginRefresh   string
	loginErr       error
	loginTokenOK   bool
	loginTokenErr  error
	user           *model.User
	userErr        error
	isBlocked      bool
	isFriend       bool
	isGroupMember  bool
	handledMsgs    []*model.Message
	refreshAccess  string
	refreshErr     error
	friendList     []model.FriendInfo
	friendGroups   []string
	searchResults  []model.UserSearchResult
	historyMsgs    []*model.Message
	historyMore    bool
	groupInfo      *model.GroupInfo
	groupInfoErr   error
	groupList      []model.GroupInfo
	groupMembers   []int64
	sessions       []model.SessionInfo
	searchResp     *model.SearchResponse
	searchErr      error
	blacklist      []model.BlockedUserInfo
	offlineMsgs    []*model.Message
	changePwAccess string
	changePwRefresh string
	changePwErr    error
	blockErr       error
	unblockErr     error
	recallErr      error
	acceptErr      error
	rejectErr      error
	deleteErr      error
	sendReqErr     error
	moveErr        error
	remarkErr      error
	createGrpErr   error
	deleteGrpErr   error
	joinGrpErr     error
	leaveGrpErr    error
	cleanDeleted   int64
	cleanErr       error
	backupData     []byte
	backupFile     string
	backupErr      error
}

func (m *mockService) HandleMessage(ctx context.Context, msg *model.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	msg.ID = int64(len(m.handledMsgs) + 1)
	msg.CreatedAt = time.Now()
	m.handledMsgs = append(m.handledMsgs, msg)
	return nil
}
func (m *mockService) ValidateToken(qq int64, token string) (bool, error) { return true, nil }
func (m *mockService) GetOfflineMessages(qq int64) ([]*model.Message, error) { return m.offlineMsgs, nil }
func (m *mockService) MarkDelivered(messageID int64) error                   { return nil }
func (m *mockService) Register(nickname, password string) (int64, error) {
	if m.registerErr != nil {
		return 0, m.registerErr
	}
	return m.registerQQ, nil
}
func (m *mockService) Login(qq int64, password string) (string, string, error) {
	if m.loginErr != nil {
		return "", "", m.loginErr
	}
	return m.loginAccess, m.loginRefresh, nil
}
func (m *mockService) LoginWithToken(qq int64, token string) (bool, error) {
	return m.loginTokenOK, m.loginTokenErr
}
func (m *mockService) RefreshToken(qq int64, refreshToken string) (string, error) {
	return m.refreshAccess, m.refreshErr
}
func (m *mockService) ClearRefreshToken(qq int64) error { return nil }
func (m *mockService) SendFriendRequest(fromQQ, toQQ int64, message string) error { return m.sendReqErr }
func (m *mockService) AcceptFriend(qq, fromQQ int64) error                        { return m.acceptErr }
func (m *mockService) RejectFriend(qq, fromQQ int64) error                        { return m.rejectErr }
func (m *mockService) DeleteFriend(qq, friendQQ int64) error                      { return m.deleteErr }
func (m *mockService) GetFriendList(qq int64, onlineFunc func(int64) bool) ([]model.FriendInfo, error) {
	return m.friendList, nil
}
func (m *mockService) SearchUsers(keyword string, onlineFunc func(int64) bool) ([]model.UserSearchResult, error) {
	return m.searchResults, nil
}
func (m *mockService) MoveFriendGroup(qq, friendQQ int64, groupName string) error { return m.moveErr }
func (m *mockService) GetFriendGroups(qq int64) ([]string, error)                 { return m.friendGroups, nil }
func (m *mockService) SetRemark(qq, friendQQ int64, remark string) error          { return m.remarkErr }
func (m *mockService) CreateFriendGroup(qq int64, name string) error              { return m.createGrpErr }
func (m *mockService) DeleteFriendGroup(qq int64, name string) error              { return m.deleteGrpErr }
func (m *mockService) GetUserByQQ(qq int64) (*model.User, error) {
	if m.userErr != nil {
		return nil, m.userErr
	}
	return m.user, nil
}
func (m *mockService) IsFriend(qq1, qq2 int64) bool      { return m.isFriend }
func (m *mockService) CheckAndIncrementNonFriendMessage(fromQQ, toQQ int64) error { return nil }
func (m *mockService) CreateGroup(name string, ownerQQ int64) (string, error) { return "G1", nil }
func (m *mockService) JoinGroup(groupID string, qq int64) error               { return m.joinGrpErr }
func (m *mockService) LeaveGroup(groupID string, qq int64) error              { return m.leaveGrpErr }
func (m *mockService) GetGroupMembers(groupID string) ([]int64, error)        { return m.groupMembers, nil }
func (m *mockService) GetGroupList(qq int64) ([]model.GroupInfo, error)       { return m.groupList, nil }
func (m *mockService) GetGroupInfo(groupID string) (*model.GroupInfo, error)  { return m.groupInfo, m.groupInfoErr }
func (m *mockService) IsGroupMember(groupID string, qq int64) bool            { return m.isGroupMember }
func (m *mockService) GetSessions(qq int64, onlineFunc func(int64) bool) ([]model.SessionInfo, error) {
	return m.sessions, nil
}
func (m *mockService) GetGroupHistory(groupID string, offset, limit int) ([]*model.Message, bool, error) {
	return m.historyMsgs, m.historyMore, nil
}
func (m *mockService) ChangePassword(qq int64, oldPw, newPw string) (string, string, error) {
	if m.changePwErr != nil {
		return "", "", m.changePwErr
	}
	return m.changePwAccess, m.changePwRefresh, nil
}
func (m *mockService) BlockUser(qq, blockedQQ int64) error    { return m.blockErr }
func (m *mockService) UnblockUser(qq, blockedQQ int64) error  { return m.unblockErr }
func (m *mockService) IsBlocked(qq, blockedQQ int64) bool     { return m.isBlocked }
func (m *mockService) GetBlacklist(qq int64) ([]model.BlockedUserInfo, error) { return m.blacklist, nil }
func (m *mockService) MarkRead(messageID int64) error         { return nil }
func (m *mockService) RecallMessage(qq, messageID int64) error { return m.recallErr }
func (m *mockService) SearchMessages(myQQ int64, keyword string, targetQQ int64, groupID string, limit int) (*model.SearchResponse, error) {
	return m.searchResp, m.searchErr
}
func (m *mockService) BackupDB() ([]byte, string, error) {
	if m.backupErr != nil {
		return nil, "", m.backupErr
	}
	if m.backupData != nil {
		return m.backupData, m.backupFile, nil
	}
	return []byte("backup"), "test.db", nil
}
func (m *mockService) CleanMessages(days int) (int64, error) {
	if m.cleanErr != nil {
		return 0, m.cleanErr
	}
	return m.cleanDeleted, nil
}
func (m *mockService) GetHistoryWithTarget(myQQ, targetQQ int64, offset, limit int, fromTime, toTime string) ([]*model.Message, bool, error) {
	return m.historyMsgs, m.historyMore, nil
}

type testClient struct {
	conn *websocket.Conn
}

func newTestClient(t *testing.T, serverURL string) *testClient {
	u := url.URL{Scheme: "ws", Host: strings.TrimPrefix(serverURL, "http://"), Path: "/ws"}
	conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		t.Fatalf("websocket dial failed: %v", err)
	}
	return &testClient{conn: conn}
}

func (tc *testClient) send(t *testing.T, msg *pb.WireMessage) {
	data, err := proto.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	if err := tc.conn.WriteMessage(websocket.BinaryMessage, data); err != nil {
		t.Fatalf("write failed: %v", err)
	}
}

func (tc *testClient) recv(t *testing.T, timeout time.Duration) *pb.WireMessage {
	tc.conn.SetReadDeadline(time.Now().Add(timeout))
	_, data, err := tc.conn.ReadMessage()
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	var wire pb.WireMessage
	if err := proto.Unmarshal(data, &wire); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	return &wire
}

func (tc *testClient) close() {
	tc.conn.Close()
}

func setupTestServer(t *testing.T, svc *mockService) (*httptest.Server, *Hub) {
	hub := NewHub(svc, nil, 10, nil, nil, nil, "test-instance")
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", hub.ServeWS)
	server := httptest.NewServer(mux)
	t.Cleanup(func() {
		server.Close()
		hub.Shutdown()
	})
	return server, hub
}

func TestConnectionLimit(t *testing.T) {
	svc := &mockService{}
	hub := NewHub(svc, nil, 2, nil, nil, nil, "")

	hub.conns[10001] = ws.NewConn(nil)
	hub.conns[10002] = ws.NewConn(nil)

	req := httptest.NewRequest("GET", "/ws", nil)
	w := httptest.NewRecorder()
	hub.ServeWS(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "connection limit reached") {
		t.Fatalf("expected 'connection limit reached' in body, got: %s", w.Body.String())
	}
}

func TestWSHeartbeat(t *testing.T) {
	svc := &mockService{}
	server, _ := setupTestServer(t, svc)

	client := newTestClient(t, server.URL)
	defer client.close()

	client.send(t, &pb.WireMessage{Payload: &pb.WireMessage_Heartbeat{Heartbeat: &pb.Heartbeat{Content: "ping"}}})

	resp := client.recv(t, 2*time.Second)
	hb := resp.GetHeartbeat()
	if hb == nil {
		t.Fatal("expected heartbeat response")
	}
	if hb.Content != "pong" {
		t.Fatalf("expected 'pong', got '%s'", hb.Content)
	}
}

func TestWSRegisterAndLogin(t *testing.T) {
	svc := &mockService{
		registerQQ:  10001,
		loginAccess: "access-token-123",
		loginRefresh: "refresh-token-456",
		user:        &model.User{QQNumber: 10001, Nickname: "alice"},
	}
	server, _ := setupTestServer(t, svc)

	client := newTestClient(t, server.URL)
	defer client.close()

	client.send(t, &pb.WireMessage{Payload: &pb.WireMessage_RegisterRequest{RegisterRequest: &pb.RegisterRequest{
		Nickname: "alice", Password: "password123",
	}}})

	resp := client.recv(t, 2*time.Second)
	regResp := resp.GetRegisterResponse()
	if regResp == nil {
		t.Fatal("expected register response")
	}
	if regResp.Code != 0 {
		t.Fatalf("expected code 0, got %d: %s", regResp.Code, regResp.Message)
	}
	if regResp.QqNumber != 10001 {
		t.Fatalf("expected qq 10001, got %d", regResp.QqNumber)
	}

	resp = client.recv(t, 2*time.Second)
	loginResp := resp.GetLoginResponse()
	if loginResp == nil {
		t.Fatal("expected auto-login response after register")
	}
	if loginResp.Code != 0 {
		t.Fatalf("expected code 0, got %d", loginResp.Code)
	}
	if loginResp.AccessToken != "access-token-123" {
		t.Fatalf("expected access token, got '%s'", loginResp.AccessToken)
	}
}

func TestWSLoginPassword(t *testing.T) {
	svc := &mockService{
		loginAccess:  "tok-access",
		loginRefresh: "tok-refresh",
		user:         &model.User{QQNumber: 10001, Nickname: "alice"},
	}
	server, _ := setupTestServer(t, svc)

	client := newTestClient(t, server.URL)
	defer client.close()

	client.send(t, &pb.WireMessage{Payload: &pb.WireMessage_LoginRequest{LoginRequest: &pb.LoginRequest{
		Qq: 10001, Password: "password123", Platform: "test",
	}}})

	resp := client.recv(t, 2*time.Second)
	loginResp := resp.GetLoginResponse()
	if loginResp == nil {
		t.Fatal("expected login response")
	}
	if loginResp.Code != 0 {
		t.Fatalf("expected code 0, got %d: %s", loginResp.Code, loginResp.Message)
	}
	if loginResp.Nickname != "alice" {
		t.Fatalf("expected nickname 'alice', got '%s'", loginResp.Nickname)
	}
}

func TestWSLoginAuthFailed(t *testing.T) {
	svc := &mockService{
		loginErr: errors.New("auth failed"),
	}
	server, _ := setupTestServer(t, svc)

	client := newTestClient(t, server.URL)
	defer client.close()

	client.send(t, &pb.WireMessage{Payload: &pb.WireMessage_LoginRequest{LoginRequest: &pb.LoginRequest{
		Qq: 10001, Password: "wrong",
	}}})

	resp := client.recv(t, 2*time.Second)
	loginResp := resp.GetLoginResponse()
	if loginResp == nil {
		t.Fatal("expected login response")
	}
	if loginResp.Code != 401 {
		t.Fatalf("expected code 401, got %d", loginResp.Code)
	}
}

func TestWSLoginToken(t *testing.T) {
	svc := &mockService{
		loginTokenOK: true,
		user:         &model.User{QQNumber: 10001, Nickname: "alice"},
	}
	server, _ := setupTestServer(t, svc)

	client := newTestClient(t, server.URL)
	defer client.close()

	client.send(t, &pb.WireMessage{Payload: &pb.WireMessage_LoginRequest{LoginRequest: &pb.LoginRequest{
		Qq: 10001, Token: "valid-jwt-token", Platform: "test",
	}}})

	resp := client.recv(t, 2*time.Second)
	loginResp := resp.GetLoginResponse()
	if loginResp == nil {
		t.Fatal("expected login response")
	}
	if loginResp.Code != 0 {
		t.Fatalf("expected code 0, got %d: %s", loginResp.Code, loginResp.Message)
	}
}

func TestWSLoginTokenExpired(t *testing.T) {
	svc := &mockService{
		loginTokenOK:  false,
		loginTokenErr: service.ErrTokenExpired,
	}
	server, _ := setupTestServer(t, svc)

	client := newTestClient(t, server.URL)
	defer client.close()

	client.send(t, &pb.WireMessage{Payload: &pb.WireMessage_LoginRequest{LoginRequest: &pb.LoginRequest{
		Qq: 10001, Token: "expired-token",
	}}})

	resp := client.recv(t, 2*time.Second)
	loginResp := resp.GetLoginResponse()
	if loginResp.Code != 401 {
		t.Fatalf("expected code 401, got %d", loginResp.Code)
	}
	if loginResp.Message != "token expired" {
		t.Fatalf("expected 'token expired', got '%s'", loginResp.Message)
	}
}

func TestWSLoginNoCredentials(t *testing.T) {
	svc := &mockService{}
	server, _ := setupTestServer(t, svc)

	client := newTestClient(t, server.URL)
	defer client.close()

	client.send(t, &pb.WireMessage{Payload: &pb.WireMessage_LoginRequest{LoginRequest: &pb.LoginRequest{
		Qq: 10001,
	}}})

	resp := client.recv(t, 2*time.Second)
	loginResp := resp.GetLoginResponse()
	if loginResp.Code != 400 {
		t.Fatalf("expected code 400, got %d", loginResp.Code)
	}
}

func TestWSChatMessage(t *testing.T) {
	svc := &mockService{
		user: &model.User{QQNumber: 10002, Nickname: "bob"},
	}
	server, hub := setupTestServer(t, svc)

	client := newTestClient(t, server.URL)
	defer client.close()

	client.send(t, &pb.WireMessage{Payload: &pb.WireMessage_LoginRequest{LoginRequest: &pb.LoginRequest{
		Qq: 10001, Password: "pass",
	}}})
	client.recv(t, 2*time.Second)

	client.send(t, &pb.WireMessage{
		ToQq: 10002,
		Payload: &pb.WireMessage_TextMessage{TextMessage: &pb.TextMessage{Content: "hello bob"}},
	})

	resp := client.recv(t, 2*time.Second)
	ack := resp.GetServerAck()
	if ack == nil {
		t.Fatal("expected server ack")
	}
	if ack.Content != "ok" {
		t.Fatalf("expected 'ok', got '%s'", ack.Content)
	}

	svc.mu.Lock()
	msgCount := len(svc.handledMsgs)
	svc.mu.Unlock()
	if msgCount != 1 {
		t.Fatalf("expected 1 handled message, got %d", msgCount)
	}

	_ = hub
}

func TestWSChatMessageUserNotFound(t *testing.T) {
	svc := &mockService{
		userErr: errors.New("not found"),
	}
	server, _ := setupTestServer(t, svc)

	client := newTestClient(t, server.URL)
	defer client.close()

	client.send(t, &pb.WireMessage{Payload: &pb.WireMessage_LoginRequest{LoginRequest: &pb.LoginRequest{
		Qq: 10001, Password: "pass",
	}}})
	client.recv(t, 2*time.Second)

	svc.userErr = errors.New("not found")
	client.send(t, &pb.WireMessage{
		ToQq: 99999,
		Payload: &pb.WireMessage_TextMessage{TextMessage: &pb.TextMessage{Content: "hello"}},
	})

	resp := client.recv(t, 2*time.Second)
	ack := resp.GetServerAck()
	if ack == nil {
		t.Fatal("expected server ack")
	}
	if ack.Content != "user not found" {
		t.Fatalf("expected 'user not found', got '%s'", ack.Content)
	}
}

func TestWSChatMessageBlocked(t *testing.T) {
	svc := &mockService{
		user:      &model.User{QQNumber: 10002, Nickname: "bob"},
		isBlocked: true,
	}
	server, _ := setupTestServer(t, svc)

	client := newTestClient(t, server.URL)
	defer client.close()

	client.send(t, &pb.WireMessage{Payload: &pb.WireMessage_LoginRequest{LoginRequest: &pb.LoginRequest{
		Qq: 10001, Password: "pass",
	}}})
	client.recv(t, 2*time.Second)

	client.send(t, &pb.WireMessage{
		ToQq: 10002,
		Payload: &pb.WireMessage_TextMessage{TextMessage: &pb.TextMessage{Content: "hello"}},
	})

	resp := client.recv(t, 2*time.Second)
	ack := resp.GetServerAck()
	if ack == nil || !strings.Contains(ack.Content, "blocked") {
		t.Fatalf("expected blocked message, got: %v", ack)
	}
}

func TestWSRefreshToken(t *testing.T) {
	svc := &mockService{refreshAccess: "new-access-tok"}
	server, _ := setupTestServer(t, svc)

	client := newTestClient(t, server.URL)
	defer client.close()

	client.send(t, &pb.WireMessage{Payload: &pb.WireMessage_RefreshTokenRequest{RefreshTokenRequest: &pb.RefreshTokenRequest{
		Qq: 10001, RefreshToken: "valid-refresh",
	}}})

	resp := client.recv(t, 2*time.Second)
	refreshResp := resp.GetRefreshTokenResponse()
	if refreshResp == nil {
		t.Fatal("expected refresh token response")
	}
	if refreshResp.Code != 0 {
		t.Fatalf("expected code 0, got %d", refreshResp.Code)
	}
	if refreshResp.AccessToken != "new-access-tok" {
		t.Fatalf("expected new access token, got '%s'", refreshResp.AccessToken)
	}
}

func TestWSRefreshTokenInvalid(t *testing.T) {
	svc := &mockService{refreshErr: service.ErrInvalidToken}
	server, _ := setupTestServer(t, svc)

	client := newTestClient(t, server.URL)
	defer client.close()

	client.send(t, &pb.WireMessage{Payload: &pb.WireMessage_RefreshTokenRequest{RefreshTokenRequest: &pb.RefreshTokenRequest{
		Qq: 10001, RefreshToken: "bad-token",
	}}})

	resp := client.recv(t, 2*time.Second)
	refreshResp := resp.GetRefreshTokenResponse()
	if refreshResp.Code != 401 {
		t.Fatalf("expected code 401, got %d", refreshResp.Code)
	}
}

func TestWSRegisterFailed(t *testing.T) {
	svc := &mockService{registerErr: errors.New("db error")}
	server, _ := setupTestServer(t, svc)

	client := newTestClient(t, server.URL)
	defer client.close()

	client.send(t, &pb.WireMessage{Payload: &pb.WireMessage_RegisterRequest{RegisterRequest: &pb.RegisterRequest{
		Nickname: "alice", Password: "pass",
	}}})

	resp := client.recv(t, 2*time.Second)
	regResp := resp.GetRegisterResponse()
	if regResp.Code != 400 {
		t.Fatalf("expected code 400, got %d", regResp.Code)
	}
}

func TestHubCountAndRemove(t *testing.T) {
	svc := &mockService{}
	hub := NewHub(svc, nil, 10, nil, nil, nil, "")

	if hub.Count() != 0 {
		t.Fatalf("expected 0 connections, got %d", hub.Count())
	}

	dummyConn := ws.NewConn(nil)
	hub.mu.Lock()
	hub.conns[10001] = dummyConn
	hub.mu.Unlock()

	if hub.Count() != 1 {
		t.Fatalf("expected 1 connection, got %d", hub.Count())
	}

	hub.RemoveUser(10001)

	if hub.Count() != 0 {
		t.Fatalf("expected 0 connections after remove, got %d", hub.Count())
	}
}

func TestHubJoinLeaveGroup(t *testing.T) {
	svc := &mockService{}
	hub := NewHub(svc, nil, 10, nil, nil, nil, "")

	hub.JoinGroup(10001, "G1")
	hub.JoinGroup(10002, "G1")

	hub.mu.RLock()
	members := hub.groups["G1"]
	hub.mu.RUnlock()
	if len(members) != 2 {
		t.Fatalf("expected 2 group members, got %d", len(members))
	}

	hub.LeaveGroup(10001, "G1")

	hub.mu.RLock()
	members = hub.groups["G1"]
	hub.mu.RUnlock()
	if len(members) != 1 {
		t.Fatalf("expected 1 group member after leave, got %d", len(members))
	}

	hub.LeaveGroup(10002, "G1")
	hub.mu.RLock()
	_, exists := hub.groups["G1"]
	hub.mu.RUnlock()
	if exists {
		t.Fatal("group should be deleted when empty")
	}
}

func TestHandlePubSubMessage(t *testing.T) {
	svc := &mockService{}
	hub := NewHub(svc, nil, 10, nil, nil, nil, "")

	msg := &pb.WireMessage{Id: 1, FromQq: 10001, ToQq: 10002}
	hub.HandlePubSubMessage(10002, msg)

	hub.HandlePubSubMessage(0, &pb.WireMessage{Id: 2, FromQq: 10001, GroupId: "G1"})
}

func TestWSDispatchInvalidProto(t *testing.T) {
	svc := &mockService{}
	server, _ := setupTestServer(t, svc)

	client := newTestClient(t, server.URL)
	defer client.close()

	client.conn.WriteMessage(websocket.BinaryMessage, []byte("not-protobuf"))

	client.send(t, &pb.WireMessage{Payload: &pb.WireMessage_Heartbeat{Heartbeat: &pb.Heartbeat{Content: "ping"}}})
	resp := client.recv(t, 2*time.Second)
	if resp.GetHeartbeat() == nil {
		t.Fatal("expected heartbeat after invalid proto (should be ignored)")
	}
}

func TestWSGroupChatMessage(t *testing.T) {
	svc := &mockService{
		user:          &model.User{QQNumber: 10001, Nickname: "alice"},
		isGroupMember: true,
	}
	server, _ := setupTestServer(t, svc)

	client := newTestClient(t, server.URL)
	defer client.close()

	client.send(t, &pb.WireMessage{Payload: &pb.WireMessage_LoginRequest{LoginRequest: &pb.LoginRequest{
		Qq: 10001, Password: "pass",
	}}})
	client.recv(t, 2*time.Second)

	client.send(t, &pb.WireMessage{
		GroupId: "G1",
		Payload: &pb.WireMessage_TextMessage{TextMessage: &pb.TextMessage{Content: "hello group"}},
	})

	resp := client.recv(t, 2*time.Second)
	ack := resp.GetServerAck()
	if ack == nil {
		t.Fatal("expected server ack")
	}
	if ack.Content != "ok" {
		t.Fatalf("expected 'ok', got '%s'", ack.Content)
	}
}

func TestWSGroupChatNotMember(t *testing.T) {
	svc := &mockService{
		user:          &model.User{QQNumber: 10001, Nickname: "alice"},
		isGroupMember: false,
	}
	server, _ := setupTestServer(t, svc)

	client := newTestClient(t, server.URL)
	defer client.close()

	client.send(t, &pb.WireMessage{Payload: &pb.WireMessage_LoginRequest{LoginRequest: &pb.LoginRequest{
		Qq: 10001, Password: "pass",
	}}})
	client.recv(t, 2*time.Second)

	client.send(t, &pb.WireMessage{
		GroupId: "G1",
		Payload: &pb.WireMessage_TextMessage{TextMessage: &pb.TextMessage{Content: "hello group"}},
	})

	resp := client.recv(t, 2*time.Second)
	ack := resp.GetServerAck()
	if ack == nil {
		t.Fatal("expected server ack")
	}
	if !strings.Contains(ack.Content, "not group member") {
		t.Fatalf("expected 'not group member', got '%s'", ack.Content)
	}
}

func loginClient(t *testing.T, serverURL string) *testClient {
	client := newTestClient(t, serverURL)
	client.send(t, &pb.WireMessage{Payload: &pb.WireMessage_LoginRequest{LoginRequest: &pb.LoginRequest{
		Qq: 10001, Password: "pass",
	}}})
	client.recv(t, 2*time.Second)
	return client
}

func TestWSFriendRequest(t *testing.T) {
	svc := &mockService{user: &model.User{QQNumber: 10001, Nickname: "alice"}}
	server, _ := setupTestServer(t, svc)
	client := loginClient(t, server.URL)
	defer client.close()

	client.send(t, &pb.WireMessage{Payload: &pb.WireMessage_FriendRequest{FriendRequest: &pb.FriendRequest{ToQqNumber: 10002, Message: "hi"}}})
	resp := client.recv(t, 2*time.Second)
	ack := resp.GetServerAck()
	if ack == nil || !strings.Contains(ack.Content, "friend request sent") {
		t.Fatalf("expected 'friend request sent', got: %v", ack)
	}
}

func TestWSFriendAccept(t *testing.T) {
	svc := &mockService{user: &model.User{QQNumber: 10001, Nickname: "alice"}}
	server, _ := setupTestServer(t, svc)
	client := loginClient(t, server.URL)
	defer client.close()

	client.send(t, &pb.WireMessage{Payload: &pb.WireMessage_FriendAccept{FriendAccept: &pb.FriendAccept{ToQqNumber: 10002}}})
	resp := client.recv(t, 2*time.Second)
	ack := resp.GetServerAck()
	if ack == nil || !strings.Contains(ack.Content, "friend accepted") {
		t.Fatalf("expected 'friend accepted', got: %v", ack)
	}
}

func TestWSFriendReject(t *testing.T) {
	svc := &mockService{user: &model.User{QQNumber: 10001, Nickname: "alice"}}
	server, _ := setupTestServer(t, svc)
	client := loginClient(t, server.URL)
	defer client.close()

	client.send(t, &pb.WireMessage{Payload: &pb.WireMessage_FriendReject{FriendReject: &pb.FriendReject{ToQqNumber: 10002}}})
	resp := client.recv(t, 2*time.Second)
	ack := resp.GetServerAck()
	if ack == nil || !strings.Contains(ack.Content, "reject") {
		t.Fatalf("expected reject ack, got: %v", ack)
	}
}

func TestWSFriendDelete(t *testing.T) {
	svc := &mockService{user: &model.User{QQNumber: 10001, Nickname: "alice"}}
	server, _ := setupTestServer(t, svc)
	client := loginClient(t, server.URL)
	defer client.close()

	client.send(t, &pb.WireMessage{Payload: &pb.WireMessage_FriendDelete{FriendDelete: &pb.FriendDelete{ToQqNumber: 10002}}})
	resp := client.recv(t, 2*time.Second)
	ack := resp.GetServerAck()
	if ack == nil || !strings.Contains(ack.Content, "friend deleted") {
		t.Fatalf("expected 'friend deleted', got: %v", ack)
	}
}

func TestWSFriendList(t *testing.T) {
	svc := &mockService{
		user:         &model.User{QQNumber: 10001, Nickname: "alice"},
		friendList:   []model.FriendInfo{{QQNumber: 10002, Nickname: "bob", GroupName: "我的好友", Status: 1}},
		friendGroups: []string{"我的好友", "同事"},
	}
	server, _ := setupTestServer(t, svc)
	client := loginClient(t, server.URL)
	defer client.close()

	client.send(t, &pb.WireMessage{Payload: &pb.WireMessage_FriendListRequest{FriendListRequest: &pb.FriendListRequest{}}})
	resp := client.recv(t, 2*time.Second)
	flResp := resp.GetFriendListResponse()
	if flResp == nil {
		t.Fatal("expected friend list response")
	}
	if len(flResp.Friends) != 1 {
		t.Fatalf("expected 1 friend, got %d", len(flResp.Friends))
	}
	if flResp.Friends[0].QqNumber != 10002 {
		t.Fatalf("expected friend qq 10002, got %d", flResp.Friends[0].QqNumber)
	}
	if len(flResp.AllGroups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(flResp.AllGroups))
	}
}

func TestWSFriendSearch(t *testing.T) {
	svc := &mockService{
		user:          &model.User{QQNumber: 10001, Nickname: "alice"},
		searchResults: []model.UserSearchResult{{QQNumber: 10002, Nickname: "bob", Online: true}},
	}
	server, _ := setupTestServer(t, svc)
	client := loginClient(t, server.URL)
	defer client.close()

	client.send(t, &pb.WireMessage{Payload: &pb.WireMessage_FriendSearchRequest{FriendSearchRequest: &pb.FriendSearchRequest{Keyword: "bob"}}})
	resp := client.recv(t, 2*time.Second)
	fsResp := resp.GetFriendSearchResponse()
	if fsResp == nil {
		t.Fatal("expected friend search response")
	}
	if len(fsResp.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(fsResp.Results))
	}
	if fsResp.Results[0].QqNumber != 10002 {
		t.Fatalf("expected qq 10002, got %d", fsResp.Results[0].QqNumber)
	}
}

func TestWSFriendMoveGroup(t *testing.T) {
	svc := &mockService{user: &model.User{QQNumber: 10001, Nickname: "alice"}}
	server, _ := setupTestServer(t, svc)
	client := loginClient(t, server.URL)
	defer client.close()

	client.send(t, &pb.WireMessage{Payload: &pb.WireMessage_FriendMoveGroup{FriendMoveGroup: &pb.FriendMoveGroup{QqNumber: 10002, GroupName: "同事"}}})
	resp := client.recv(t, 2*time.Second)
	ack := resp.GetServerAck()
	if ack == nil || !strings.Contains(ack.Content, "moved") {
		t.Fatalf("expected 'moved' ack, got: %v", ack)
	}
}

func TestWSFriendRemark(t *testing.T) {
	svc := &mockService{user: &model.User{QQNumber: 10001, Nickname: "alice"}}
	server, _ := setupTestServer(t, svc)
	client := loginClient(t, server.URL)
	defer client.close()

	client.send(t, &pb.WireMessage{Payload: &pb.WireMessage_FriendRemark{FriendRemark: &pb.FriendRemark{QqNumber: 10002, Remark: "小鲍"}}})
	resp := client.recv(t, 2*time.Second)
	ack := resp.GetServerAck()
	if ack == nil || !strings.Contains(ack.Content, "remark") {
		t.Fatalf("expected remark ack, got: %v", ack)
	}
}

func TestWSFriendGroups(t *testing.T) {
	svc := &mockService{
		user:         &model.User{QQNumber: 10001, Nickname: "alice"},
		friendGroups: []string{"我的好友", "同事"},
	}
	server, _ := setupTestServer(t, svc)
	client := loginClient(t, server.URL)
	defer client.close()

	client.send(t, &pb.WireMessage{Payload: &pb.WireMessage_FriendGroupsRequest{FriendGroupsRequest: &pb.FriendGroupsRequest{}}})
	resp := client.recv(t, 2*time.Second)
	fgResp := resp.GetFriendGroupsResponse()
	if fgResp == nil {
		t.Fatal("expected friend groups response")
	}
	if len(fgResp.Groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(fgResp.Groups))
	}
}

func TestWSFriendCreateGroup(t *testing.T) {
	svc := &mockService{user: &model.User{QQNumber: 10001, Nickname: "alice"}}
	server, _ := setupTestServer(t, svc)
	client := loginClient(t, server.URL)
	defer client.close()

	client.send(t, &pb.WireMessage{Payload: &pb.WireMessage_FriendCreateGroup{FriendCreateGroup: &pb.FriendCreateGroup{Name: "同事"}}})
	resp := client.recv(t, 2*time.Second)
	ack := resp.GetServerAck()
	if ack == nil || !strings.Contains(ack.Content, "group created") {
		t.Fatalf("expected 'group created', got: %v", ack)
	}
}

func TestWSFriendDeleteGroup(t *testing.T) {
	svc := &mockService{user: &model.User{QQNumber: 10001, Nickname: "alice"}}
	server, _ := setupTestServer(t, svc)
	client := loginClient(t, server.URL)
	defer client.close()

	client.send(t, &pb.WireMessage{Payload: &pb.WireMessage_FriendDeleteGroup{FriendDeleteGroup: &pb.FriendDeleteGroup{Name: "同事"}}})
	resp := client.recv(t, 2*time.Second)
	ack := resp.GetServerAck()
	if ack == nil || !strings.Contains(ack.Content, "group deleted") {
		t.Fatalf("expected 'group deleted', got: %v", ack)
	}
}

func TestWSCheckUser(t *testing.T) {
	svc := &mockService{
		user: &model.User{QQNumber: 10002, Nickname: "bob"},
	}
	server, _ := setupTestServer(t, svc)
	client := loginClient(t, server.URL)
	defer client.close()

	client.send(t, &pb.WireMessage{Payload: &pb.WireMessage_CheckUserRequest{CheckUserRequest: &pb.CheckUserRequest{Qq: 10002}}})
	resp := client.recv(t, 2*time.Second)
	cuResp := resp.GetCheckUserResponse()
	if cuResp == nil {
		t.Fatal("expected check user response")
	}
	if cuResp.Code != 0 {
		t.Fatalf("expected code 0, got %d", cuResp.Code)
	}
	if cuResp.Nickname != "bob" {
		t.Fatalf("expected nickname 'bob', got '%s'", cuResp.Nickname)
	}
}

func TestWSCheckUserNotFound(t *testing.T) {
	svc := &mockService{
		user:    &model.User{QQNumber: 10001, Nickname: "alice"},
		userErr: errors.New("not found"),
	}
	server, _ := setupTestServer(t, svc)
	client := loginClient(t, server.URL)
	defer client.close()

	svc.userErr = errors.New("not found")
	client.send(t, &pb.WireMessage{Payload: &pb.WireMessage_CheckUserRequest{CheckUserRequest: &pb.CheckUserRequest{Qq: 99999}}})
	resp := client.recv(t, 2*time.Second)
	cuResp := resp.GetCheckUserResponse()
	if cuResp == nil {
		t.Fatal("expected check user response")
	}
	if cuResp.Code != 404 {
		t.Fatalf("expected code 404, got %d", cuResp.Code)
	}
}

func TestWSHistory(t *testing.T) {
	svc := &mockService{
		user:        &model.User{QQNumber: 10002, Nickname: "bob"},
		historyMsgs: []*model.Message{{ID: 1, FromQQ: 10001, ToQQ: 10002, Content: "hello", CreatedAt: time.Now()}},
		historyMore: false,
	}
	server, _ := setupTestServer(t, svc)
	client := loginClient(t, server.URL)
	defer client.close()

	client.send(t, &pb.WireMessage{Payload: &pb.WireMessage_HistoryRequest{HistoryRequest: &pb.HistoryRequest{TargetQq: 10002, Offset: 0, Limit: 30}}})
	resp := client.recv(t, 2*time.Second)
	hResp := resp.GetHistoryResponse()
	if hResp == nil {
		t.Fatal("expected history response")
	}
	if len(hResp.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(hResp.Messages))
	}
	if hResp.Messages[0].Content != "hello" {
		t.Fatalf("expected 'hello', got '%s'", hResp.Messages[0].Content)
	}
}

func TestWSGroupHistory(t *testing.T) {
	svc := &mockService{
		user:          &model.User{QQNumber: 10001, Nickname: "alice"},
		isGroupMember: true,
		groupInfo:     &model.GroupInfo{GroupID: "G1", Name: "test group", OwnerQQ: 10001, MemberCnt: 2},
		historyMsgs:   []*model.Message{{ID: 1, FromQQ: 10001, Content: "group msg", CreatedAt: time.Now()}},
	}
	server, _ := setupTestServer(t, svc)
	client := loginClient(t, server.URL)
	defer client.close()

	client.send(t, &pb.WireMessage{Payload: &pb.WireMessage_GroupHistoryRequest{GroupHistoryRequest: &pb.GroupHistoryRequest{GroupId: "G1", Offset: 0, Limit: 30}}})
	resp := client.recv(t, 2*time.Second)
	ghResp := resp.GetGroupHistoryResponse()
	if ghResp == nil {
		t.Fatal("expected group history response")
	}
	if ghResp.GroupName != "test group" {
		t.Fatalf("expected 'test group', got '%s'", ghResp.GroupName)
	}
	if len(ghResp.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(ghResp.Messages))
	}
}

func TestWSGroupHistoryNotMember(t *testing.T) {
	svc := &mockService{
		user:          &model.User{QQNumber: 10001, Nickname: "alice"},
		isGroupMember: false,
	}
	server, _ := setupTestServer(t, svc)
	client := loginClient(t, server.URL)
	defer client.close()

	client.send(t, &pb.WireMessage{Payload: &pb.WireMessage_GroupHistoryRequest{GroupHistoryRequest: &pb.GroupHistoryRequest{GroupId: "G1"}}})
	resp := client.recv(t, 2*time.Second)
	ack := resp.GetServerAck()
	if ack == nil || !strings.Contains(ack.Content, "not group member") {
		t.Fatalf("expected 'not group member', got: %v", ack)
	}
}

func TestWSSessionList(t *testing.T) {
	svc := &mockService{
		user:     &model.User{QQNumber: 10001, Nickname: "alice"},
		sessions: []model.SessionInfo{{Type: "private", TargetQQ: 10002, Nickname: "bob", LastMessage: "hi", LastTime: time.Now()}},
	}
	server, _ := setupTestServer(t, svc)
	client := loginClient(t, server.URL)
	defer client.close()

	client.send(t, &pb.WireMessage{Payload: &pb.WireMessage_SessionListRequest{SessionListRequest: &pb.SessionListRequest{}}})
	resp := client.recv(t, 2*time.Second)
	slResp := resp.GetSessionListResponse()
	if slResp == nil {
		t.Fatal("expected session list response")
	}
	if len(slResp.Sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(slResp.Sessions))
	}
	if slResp.Sessions[0].Nickname != "bob" {
		t.Fatalf("expected 'bob', got '%s'", slResp.Sessions[0].Nickname)
	}
}

func TestWSSearchMessages(t *testing.T) {
	svc := &mockService{
		user: &model.User{QQNumber: 10001, Nickname: "alice"},
		searchResp: &model.SearchResponse{
			Keyword: "hello",
			Total:   1,
			Results: []model.SearchResultItem{{MessageID: 1, FromQQ: 10001, ToQQ: 10002, Content: "hello world", CreatedAt: time.Now()}},
		},
	}
	server, _ := setupTestServer(t, svc)
	client := loginClient(t, server.URL)
	defer client.close()

	client.send(t, &pb.WireMessage{Payload: &pb.WireMessage_SearchMessagesRequest{SearchMessagesRequest: &pb.SearchMessagesRequest{Keyword: "hello", Limit: 50}}})
	resp := client.recv(t, 2*time.Second)
	smResp := resp.GetSearchMessagesResponse()
	if smResp == nil {
		t.Fatal("expected search messages response")
	}
	if smResp.Total != 1 {
		t.Fatalf("expected total 1, got %d", smResp.Total)
	}
	if smResp.Results[0].Content != "hello world" {
		t.Fatalf("expected 'hello world', got '%s'", smResp.Results[0].Content)
	}
}

func TestWSSearchMessagesEmptyKeyword(t *testing.T) {
	svc := &mockService{user: &model.User{QQNumber: 10001, Nickname: "alice"}}
	server, _ := setupTestServer(t, svc)
	client := loginClient(t, server.URL)
	defer client.close()

	client.send(t, &pb.WireMessage{Payload: &pb.WireMessage_SearchMessagesRequest{SearchMessagesRequest: &pb.SearchMessagesRequest{Keyword: ""}}})
	resp := client.recv(t, 2*time.Second)
	ack := resp.GetServerAck()
	if ack == nil || !strings.Contains(ack.Content, "keyword") {
		t.Fatalf("expected keyword error, got: %v", ack)
	}
}

func TestWSGroupCreate(t *testing.T) {
	svc := &mockService{user: &model.User{QQNumber: 10001, Nickname: "alice"}}
	server, _ := setupTestServer(t, svc)
	client := loginClient(t, server.URL)
	defer client.close()

	client.send(t, &pb.WireMessage{Payload: &pb.WireMessage_GroupCreateRequest{GroupCreateRequest: &pb.GroupCreateRequest{Name: "my group"}}})
	resp := client.recv(t, 2*time.Second)
	gcResp := resp.GetGroupCreateResponse()
	if gcResp == nil {
		t.Fatal("expected group create response")
	}
	if gcResp.GroupId != "G1" {
		t.Fatalf("expected group id 'G1', got '%s'", gcResp.GroupId)
	}
}

func TestWSGroupCreateEmptyName(t *testing.T) {
	svc := &mockService{user: &model.User{QQNumber: 10001, Nickname: "alice"}}
	server, _ := setupTestServer(t, svc)
	client := loginClient(t, server.URL)
	defer client.close()

	client.send(t, &pb.WireMessage{Payload: &pb.WireMessage_GroupCreateRequest{GroupCreateRequest: &pb.GroupCreateRequest{Name: ""}}})
	resp := client.recv(t, 2*time.Second)
	ack := resp.GetServerAck()
	if ack == nil || !strings.Contains(ack.Content, "name required") {
		t.Fatalf("expected 'name required', got: %v", ack)
	}
}

func TestWSGroupJoin(t *testing.T) {
	svc := &mockService{user: &model.User{QQNumber: 10001, Nickname: "alice"}}
	server, _ := setupTestServer(t, svc)
	client := loginClient(t, server.URL)
	defer client.close()

	client.send(t, &pb.WireMessage{Payload: &pb.WireMessage_GroupJoinRequest{GroupJoinRequest: &pb.GroupJoinRequest{GroupId: "G1"}}})
	resp := client.recv(t, 2*time.Second)
	ack := resp.GetServerAck()
	if ack == nil || !strings.Contains(ack.Content, "joined") {
		t.Fatalf("expected 'joined', got: %v", ack)
	}
}

func TestWSGroupLeave(t *testing.T) {
	svc := &mockService{user: &model.User{QQNumber: 10001, Nickname: "alice"}}
	server, _ := setupTestServer(t, svc)
	client := loginClient(t, server.URL)
	defer client.close()

	client.send(t, &pb.WireMessage{Payload: &pb.WireMessage_GroupLeaveRequest{GroupLeaveRequest: &pb.GroupLeaveRequest{GroupId: "G1"}}})
	resp := client.recv(t, 2*time.Second)
	ack := resp.GetServerAck()
	if ack == nil || !strings.Contains(ack.Content, "left") {
		t.Fatalf("expected 'left', got: %v", ack)
	}
}

func TestWSGroupList(t *testing.T) {
	svc := &mockService{
		user:      &model.User{QQNumber: 10001, Nickname: "alice"},
		groupList: []model.GroupInfo{{GroupID: "G1", Name: "test", OwnerQQ: 10001, MemberCnt: 3}},
	}
	server, _ := setupTestServer(t, svc)
	client := loginClient(t, server.URL)
	defer client.close()

	client.send(t, &pb.WireMessage{Payload: &pb.WireMessage_GroupListRequest{GroupListRequest: &pb.GroupListRequest{}}})
	resp := client.recv(t, 2*time.Second)
	glResp := resp.GetGroupListResponse()
	if glResp == nil {
		t.Fatal("expected group list response")
	}
	if len(glResp.Groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(glResp.Groups))
	}
	if glResp.Groups[0].Name != "test" {
		t.Fatalf("expected 'test', got '%s'", glResp.Groups[0].Name)
	}
}

func TestWSGroupInfo(t *testing.T) {
	svc := &mockService{
		user:          &model.User{QQNumber: 10001, Nickname: "alice"},
		isGroupMember: true,
		groupInfo:     &model.GroupInfo{GroupID: "G1", Name: "test group", OwnerQQ: 10001, MemberCnt: 5},
	}
	server, _ := setupTestServer(t, svc)
	client := loginClient(t, server.URL)
	defer client.close()

	client.send(t, &pb.WireMessage{Payload: &pb.WireMessage_GroupInfoRequest{GroupInfoRequest: &pb.GroupInfoRequest{GroupId: "G1"}}})
	resp := client.recv(t, 2*time.Second)
	giResp := resp.GetGroupInfoResponse()
	if giResp == nil {
		t.Fatal("expected group info response")
	}
	if giResp.Name != "test group" {
		t.Fatalf("expected 'test group', got '%s'", giResp.Name)
	}
	if giResp.MemberCnt != 5 {
		t.Fatalf("expected 5 members, got %d", giResp.MemberCnt)
	}
}

func TestWSChangePassword(t *testing.T) {
	svc := &mockService{
		user:           &model.User{QQNumber: 10001, Nickname: "alice"},
		changePwAccess: "new-access",
		changePwRefresh: "new-refresh",
	}
	server, _ := setupTestServer(t, svc)
	client := loginClient(t, server.URL)
	defer client.close()

	client.send(t, &pb.WireMessage{Payload: &pb.WireMessage_ChangePasswordRequest{ChangePasswordRequest: &pb.ChangePasswordRequest{OldPassword: "old", NewPassword: "new"}}})
	resp := client.recv(t, 2*time.Second)
	cpResp := resp.GetChangePasswordResponse()
	if cpResp == nil {
		t.Fatal("expected change password response")
	}
	if cpResp.Code != 0 {
		t.Fatalf("expected code 0, got %d", cpResp.Code)
	}
	if cpResp.AccessToken != "new-access" {
		t.Fatalf("expected 'new-access', got '%s'", cpResp.AccessToken)
	}
}

func TestWSChangePasswordMissingFields(t *testing.T) {
	svc := &mockService{user: &model.User{QQNumber: 10001, Nickname: "alice"}}
	server, _ := setupTestServer(t, svc)
	client := loginClient(t, server.URL)
	defer client.close()

	client.send(t, &pb.WireMessage{Payload: &pb.WireMessage_ChangePasswordRequest{ChangePasswordRequest: &pb.ChangePasswordRequest{OldPassword: "", NewPassword: ""}}})
	resp := client.recv(t, 2*time.Second)
	ack := resp.GetServerAck()
	if ack == nil || !strings.Contains(ack.Content, "required") {
		t.Fatalf("expected 'required' error, got: %v", ack)
	}
}

func TestWSBlockUser(t *testing.T) {
	svc := &mockService{user: &model.User{QQNumber: 10001, Nickname: "alice"}}
	server, _ := setupTestServer(t, svc)
	client := loginClient(t, server.URL)
	defer client.close()

	client.send(t, &pb.WireMessage{Payload: &pb.WireMessage_BlockUserRequest{BlockUserRequest: &pb.BlockUserRequest{Qq: 10002}}})
	resp := client.recv(t, 2*time.Second)
	ack := resp.GetServerAck()
	if ack == nil || !strings.Contains(ack.Content, "blocked") {
		t.Fatalf("expected 'blocked', got: %v", ack)
	}
}

func TestWSUnblockUser(t *testing.T) {
	svc := &mockService{user: &model.User{QQNumber: 10001, Nickname: "alice"}}
	server, _ := setupTestServer(t, svc)
	client := loginClient(t, server.URL)
	defer client.close()

	client.send(t, &pb.WireMessage{Payload: &pb.WireMessage_UnblockUserRequest{UnblockUserRequest: &pb.UnblockUserRequest{Qq: 10002}}})
	resp := client.recv(t, 2*time.Second)
	ack := resp.GetServerAck()
	if ack == nil || !strings.Contains(ack.Content, "unblocked") {
		t.Fatalf("expected 'unblocked', got: %v", ack)
	}
}

func TestWSBlacklist(t *testing.T) {
	svc := &mockService{
		user:      &model.User{QQNumber: 10001, Nickname: "alice"},
		blacklist: []model.BlockedUserInfo{{QQNumber: 10002, Nickname: "bob"}},
	}
	server, _ := setupTestServer(t, svc)
	client := loginClient(t, server.URL)
	defer client.close()

	client.send(t, &pb.WireMessage{Payload: &pb.WireMessage_BlacklistRequest{BlacklistRequest: &pb.BlacklistRequest{}}})
	resp := client.recv(t, 2*time.Second)
	blResp := resp.GetBlacklistResponse()
	if blResp == nil {
		t.Fatal("expected blacklist response")
	}
	if len(blResp.BlockedUsers) != 1 {
		t.Fatalf("expected 1 blocked user, got %d", len(blResp.BlockedUsers))
	}
	if blResp.BlockedUsers[0].QqNumber != 10002 {
		t.Fatalf("expected qq 10002, got %d", blResp.BlockedUsers[0].QqNumber)
	}
}

func TestWSDeliveredAck(t *testing.T) {
	svc := &mockService{user: &model.User{QQNumber: 10001, Nickname: "alice"}}
	server, _ := setupTestServer(t, svc)
	client := loginClient(t, server.URL)
	defer client.close()

	client.send(t, &pb.WireMessage{Payload: &pb.WireMessage_DeliveredAck{DeliveredAck: &pb.DeliveredAck{MessageId: 42}}})
	time.Sleep(100 * time.Millisecond)
}

func TestWSReadReceipt(t *testing.T) {
	svc := &mockService{user: &model.User{QQNumber: 10001, Nickname: "alice"}}
	server, _ := setupTestServer(t, svc)
	client := loginClient(t, server.URL)
	defer client.close()

	client.send(t, &pb.WireMessage{Payload: &pb.WireMessage_ReadReceipt{ReadReceipt: &pb.ReadReceipt{MessageId: 42}}})
	time.Sleep(100 * time.Millisecond)
}

func TestWSRecall(t *testing.T) {
	svc := &mockService{user: &model.User{QQNumber: 10001, Nickname: "alice"}}
	server, _ := setupTestServer(t, svc)
	client := loginClient(t, server.URL)
	defer client.close()

	client.send(t, &pb.WireMessage{Payload: &pb.WireMessage_RecallRequest{RecallRequest: &pb.RecallRequest{MessageId: 1}}})
	resp := client.recv(t, 2*time.Second)
	ack := resp.GetServerAck()
	if ack == nil || !strings.Contains(ack.Content, "recalled") {
		t.Fatalf("expected 'recalled', got: %v", ack)
	}
}

func TestWSRecallFailed(t *testing.T) {
	svc := &mockService{
		user:      &model.User{QQNumber: 10001, Nickname: "alice"},
		recallErr: errors.New("too old"),
	}
	server, _ := setupTestServer(t, svc)
	client := loginClient(t, server.URL)
	defer client.close()

	client.send(t, &pb.WireMessage{Payload: &pb.WireMessage_RecallRequest{RecallRequest: &pb.RecallRequest{MessageId: 1}}})
	resp := client.recv(t, 2*time.Second)
	ack := resp.GetServerAck()
	if ack == nil || !strings.Contains(ack.Content, "too old") {
		t.Fatalf("expected 'too old', got: %v", ack)
	}
}

func TestWSBackup(t *testing.T) {
	svc := &mockService{
		user:       &model.User{QQNumber: 10001, Nickname: "alice"},
		backupData: []byte("sqlite data here"),
		backupFile: "backup.db",
	}
	server, _ := setupTestServer(t, svc)
	client := loginClient(t, server.URL)
	defer client.close()

	client.send(t, &pb.WireMessage{Payload: &pb.WireMessage_BackupRequest{BackupRequest: &pb.BackupRequest{}}})
	resp := client.recv(t, 2*time.Second)
	bkResp := resp.GetBackupResponse()
	if bkResp == nil {
		t.Fatal("expected backup response")
	}
	if bkResp.Code != 0 {
		t.Fatalf("expected code 0, got %d", bkResp.Code)
	}
	if bkResp.Filename != "backup.db" {
		t.Fatalf("expected 'backup.db', got '%s'", bkResp.Filename)
	}
	if bkResp.Size != 16 {
		t.Fatalf("expected size 16, got %d", bkResp.Size)
	}
}

func TestWSClean(t *testing.T) {
	svc := &mockService{
		user:         &model.User{QQNumber: 10001, Nickname: "alice"},
		cleanDeleted: 42,
	}
	server, _ := setupTestServer(t, svc)
	client := loginClient(t, server.URL)
	defer client.close()

	client.send(t, &pb.WireMessage{Payload: &pb.WireMessage_CleanRequest{CleanRequest: &pb.CleanRequest{Days: 30}}})
	resp := client.recv(t, 2*time.Second)
	clResp := resp.GetCleanResponse()
	if clResp == nil {
		t.Fatal("expected clean response")
	}
	if clResp.Code != 0 {
		t.Fatalf("expected code 0, got %d", clResp.Code)
	}
	if clResp.Deleted != 42 {
		t.Fatalf("expected 42 deleted, got %d", clResp.Deleted)
	}
}

func TestWSFileMessage(t *testing.T) {
	svc := &mockService{user: &model.User{QQNumber: 10002, Nickname: "bob"}}
	server, _ := setupTestServer(t, svc)
	client := loginClient(t, server.URL)
	defer client.close()

	client.send(t, &pb.WireMessage{
		ToQq: 10002,
		Payload: &pb.WireMessage_FileMessage{FileMessage: &pb.FileMessage{
			Filename: "test.txt", Size: 4, Data: []byte("data"),
		}},
	})
	resp := client.recv(t, 2*time.Second)
	ack := resp.GetServerAck()
	if ack == nil || ack.Content != "ok" {
		t.Fatalf("expected 'ok', got: %v", ack)
	}
}

func TestWSFileMessageGroupNotMember(t *testing.T) {
	svc := &mockService{
		user:          &model.User{QQNumber: 10001, Nickname: "alice"},
		isGroupMember: false,
	}
	server, _ := setupTestServer(t, svc)
	client := loginClient(t, server.URL)
	defer client.close()

	client.send(t, &pb.WireMessage{
		GroupId: "G1",
		Payload: &pb.WireMessage_FileMessage{FileMessage: &pb.FileMessage{
			Filename: "test.txt", Size: 4, Data: []byte("data"),
		}},
	})
	resp := client.recv(t, 2*time.Second)
	ack := resp.GetServerAck()
	if ack == nil || !strings.Contains(ack.Content, "not group member") {
		t.Fatalf("expected 'not group member', got: %v", ack)
	}
}

func TestWSNotLoggedIn(t *testing.T) {
	svc := &mockService{}
	server, _ := setupTestServer(t, svc)
	client := newTestClient(t, server.URL)
	defer client.close()

	tests := []struct {
		name string
		msg  *pb.WireMessage
	}{
		{"friend request", &pb.WireMessage{Payload: &pb.WireMessage_FriendRequest{FriendRequest: &pb.FriendRequest{ToQqNumber: 10002}}}},
		{"friend accept", &pb.WireMessage{Payload: &pb.WireMessage_FriendAccept{FriendAccept: &pb.FriendAccept{ToQqNumber: 10002}}}},
		{"friend reject", &pb.WireMessage{Payload: &pb.WireMessage_FriendReject{FriendReject: &pb.FriendReject{ToQqNumber: 10002}}}},
		{"friend delete", &pb.WireMessage{Payload: &pb.WireMessage_FriendDelete{FriendDelete: &pb.FriendDelete{ToQqNumber: 10002}}}},
		{"friend list", &pb.WireMessage{Payload: &pb.WireMessage_FriendListRequest{FriendListRequest: &pb.FriendListRequest{}}}},
		{"friend search", &pb.WireMessage{Payload: &pb.WireMessage_FriendSearchRequest{FriendSearchRequest: &pb.FriendSearchRequest{Keyword: "test"}}}},
		{"friend move", &pb.WireMessage{Payload: &pb.WireMessage_FriendMoveGroup{FriendMoveGroup: &pb.FriendMoveGroup{QqNumber: 10002, GroupName: "g"}}}},
		{"friend remark", &pb.WireMessage{Payload: &pb.WireMessage_FriendRemark{FriendRemark: &pb.FriendRemark{QqNumber: 10002, Remark: "r"}}}},
		{"friend groups", &pb.WireMessage{Payload: &pb.WireMessage_FriendGroupsRequest{FriendGroupsRequest: &pb.FriendGroupsRequest{}}}},
		{"friend create group", &pb.WireMessage{Payload: &pb.WireMessage_FriendCreateGroup{FriendCreateGroup: &pb.FriendCreateGroup{Name: "g"}}}},
		{"friend delete group", &pb.WireMessage{Payload: &pb.WireMessage_FriendDeleteGroup{FriendDeleteGroup: &pb.FriendDeleteGroup{Name: "g"}}}},
		{"history", &pb.WireMessage{Payload: &pb.WireMessage_HistoryRequest{HistoryRequest: &pb.HistoryRequest{TargetQq: 10002}}}},
		{"group history", &pb.WireMessage{Payload: &pb.WireMessage_GroupHistoryRequest{GroupHistoryRequest: &pb.GroupHistoryRequest{GroupId: "G1"}}}},
		{"session list", &pb.WireMessage{Payload: &pb.WireMessage_SessionListRequest{SessionListRequest: &pb.SessionListRequest{}}}},
		{"search", &pb.WireMessage{Payload: &pb.WireMessage_SearchMessagesRequest{SearchMessagesRequest: &pb.SearchMessagesRequest{Keyword: "test"}}}},
		{"group create", &pb.WireMessage{Payload: &pb.WireMessage_GroupCreateRequest{GroupCreateRequest: &pb.GroupCreateRequest{Name: "g"}}}},
		{"group join", &pb.WireMessage{Payload: &pb.WireMessage_GroupJoinRequest{GroupJoinRequest: &pb.GroupJoinRequest{GroupId: "G1"}}}},
		{"group leave", &pb.WireMessage{Payload: &pb.WireMessage_GroupLeaveRequest{GroupLeaveRequest: &pb.GroupLeaveRequest{GroupId: "G1"}}}},
		{"group list", &pb.WireMessage{Payload: &pb.WireMessage_GroupListRequest{GroupListRequest: &pb.GroupListRequest{}}}},
		{"group info", &pb.WireMessage{Payload: &pb.WireMessage_GroupInfoRequest{GroupInfoRequest: &pb.GroupInfoRequest{GroupId: "G1"}}}},
		{"change password", &pb.WireMessage{Payload: &pb.WireMessage_ChangePasswordRequest{ChangePasswordRequest: &pb.ChangePasswordRequest{OldPassword: "a", NewPassword: "b"}}}},
		{"block", &pb.WireMessage{Payload: &pb.WireMessage_BlockUserRequest{BlockUserRequest: &pb.BlockUserRequest{Qq: 10002}}}},
		{"unblock", &pb.WireMessage{Payload: &pb.WireMessage_UnblockUserRequest{UnblockUserRequest: &pb.UnblockUserRequest{Qq: 10002}}}},
		{"blacklist", &pb.WireMessage{Payload: &pb.WireMessage_BlacklistRequest{BlacklistRequest: &pb.BlacklistRequest{}}}},
		{"recall", &pb.WireMessage{Payload: &pb.WireMessage_RecallRequest{RecallRequest: &pb.RecallRequest{MessageId: 1}}}},
		{"backup", &pb.WireMessage{Payload: &pb.WireMessage_BackupRequest{BackupRequest: &pb.BackupRequest{}}}},
		{"clean", &pb.WireMessage{Payload: &pb.WireMessage_CleanRequest{CleanRequest: &pb.CleanRequest{Days: 30}}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc2 := &mockService{}
			server2, _ := setupTestServer(t, svc2)
			c := newTestClient(t, server2.URL)
			defer c.close()

			c.send(t, tt.msg)
			resp := c.recv(t, 2*time.Second)
			ack := resp.GetServerAck()
			if ack == nil || !strings.Contains(ack.Content, "not logged in") {
				t.Fatalf("[%s] expected 'not logged in', got: %v", tt.name, ack)
			}
		})
	}
}
