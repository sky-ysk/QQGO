package main

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	pb "github.com/qqgo/server/internal/protocol"
	"google.golang.org/protobuf/proto"
)

type testClient struct {
	conn       *websocket.Conn
	qq         int64
	nickname   string
	token      string
	lastMsg    string
	lastAck    string
	targetQQ   int64
	groupID    string
	msgCount   int
}

func newTestClient(t *testing.T) *testClient {
	conn, _, err := websocket.DefaultDialer.Dial("ws://localhost:8080/ws", nil)
	if err != nil {
		t.Fatalf("dial error: %v", err)
	}
	return &testClient{conn: conn}
}

func (c *testClient) send(wire *pb.WireMessage) error {
	data, err := proto.Marshal(wire)
	if err != nil {
		return err
	}
	return c.conn.WriteMessage(websocket.BinaryMessage, data)
}

func (c *testClient) read(timeout time.Duration) (*pb.WireMessage, error) {
	if c.conn == nil {
		return nil, fmt.Errorf("connection closed")
	}
	done := make(chan struct{})
	var wire *pb.WireMessage
	var readErr error

	go func() {
		_, data, err := c.conn.ReadMessage()
		if err != nil {
			readErr = err
		} else {
			var w pb.WireMessage
			if err := proto.Unmarshal(data, &w); err != nil {
				readErr = err
			} else {
				wire = &w
			}
		}
		close(done)
	}()

	select {
	case <-done:
		return wire, readErr
	case <-time.After(timeout):
		return nil, fmt.Errorf("read timeout")
	}
}

func (c *testClient) readUntilPayload(t *testing.T, match func(*pb.WireMessage) bool, timeout time.Duration) *pb.WireMessage {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		wire, err := c.read(500 * time.Millisecond)
		if err != nil {
			if strings.Contains(err.Error(), "closed") || strings.Contains(err.Error(), "close") {
				t.Fatalf("connection closed while waiting for payload")
			}
			continue
		}
		if match(wire) {
			return wire
		}
	}
	t.Fatalf("timeout waiting for matching payload")
	return nil
}

func (c *testClient) register(t *testing.T, password, nickname string) {
	c.send(&pb.WireMessage{Payload: &pb.WireMessage_RegisterRequest{RegisterRequest: &pb.RegisterRequest{
		Password: password, Nickname: nickname,
	}}})

	regWire := c.readUntilPayload(t, func(w *pb.WireMessage) bool {
		_, ok := w.Payload.(*pb.WireMessage_RegisterResponse)
		return ok
	}, 3*time.Second)
	regResp := regWire.Payload.(*pb.WireMessage_RegisterResponse).RegisterResponse
	if regResp.Code != 0 {
		t.Fatalf("register failed: %s", regResp.Message)
	}
	c.qq = regResp.QqNumber
	c.nickname = nickname

	loginWire := c.readUntilPayload(t, func(w *pb.WireMessage) bool {
		_, ok := w.Payload.(*pb.WireMessage_LoginResponse)
		return ok
	}, 3*time.Second)
	loginResp := loginWire.Payload.(*pb.WireMessage_LoginResponse).LoginResponse
	if loginResp.Code != 0 {
		t.Fatalf("auto-login after register failed: %s", loginResp.Message)
	}
	c.token = loginResp.AccessToken
	t.Logf("Registered: QQ=%d, nickname=%s", c.qq, c.nickname)
}

func (c *testClient) login(t *testing.T, qq int64, password string) {
	c.send(&pb.WireMessage{Payload: &pb.WireMessage_LoginRequest{LoginRequest: &pb.LoginRequest{
		Qq: qq, Password: password, Platform: "cli",
	}}})

	loginWire := c.readUntilPayload(t, func(w *pb.WireMessage) bool {
		_, ok := w.Payload.(*pb.WireMessage_LoginResponse)
		return ok
	}, 3*time.Second)
	loginResp := loginWire.Payload.(*pb.WireMessage_LoginResponse).LoginResponse
	if loginResp.Code != 0 {
		t.Fatalf("login failed: %s", loginResp.Message)
	}
	c.qq = loginResp.QqNumber
	c.nickname = loginResp.Nickname
	c.token = loginResp.AccessToken
}

func (c *testClient) loginWithToken(t *testing.T, qq int64, token string) {
	c.send(&pb.WireMessage{Payload: &pb.WireMessage_LoginRequest{LoginRequest: &pb.LoginRequest{
		Qq: qq, Token: token, Platform: "cli",
	}}})

	loginWire := c.readUntilPayload(t, func(w *pb.WireMessage) bool {
		_, ok := w.Payload.(*pb.WireMessage_LoginResponse)
		return ok
	}, 3*time.Second)
	loginResp := loginWire.Payload.(*pb.WireMessage_LoginResponse).LoginResponse
	if loginResp.Code != 0 {
		t.Fatalf("token login failed: %s", loginResp.Message)
	}
	c.qq = loginResp.QqNumber
	c.nickname = loginResp.Nickname
	c.token = loginResp.AccessToken
}

func (c *testClient) switchTo(t *testing.T, tqq int64) {
	c.send(&pb.WireMessage{Payload: &pb.WireMessage_CheckUserRequest{CheckUserRequest: &pb.CheckUserRequest{Qq: tqq}}})

	checkWire := c.readUntilPayload(t, func(w *pb.WireMessage) bool {
		_, ok := w.Payload.(*pb.WireMessage_CheckUserResponse)
		return ok
	}, 3*time.Second)
	checkResp := checkWire.Payload.(*pb.WireMessage_CheckUserResponse).CheckUserResponse
	if checkResp.Code != 0 {
		t.Fatalf("check user failed: %s", checkResp.Message)
	}
	c.targetQQ = tqq

	c.send(&pb.WireMessage{Payload: &pb.WireMessage_HistoryRequest{HistoryRequest: &pb.HistoryRequest{
		TargetQq: tqq, Offset: 0, Limit: 30,
	}}})
	c.readUntilPayload(t, func(w *pb.WireMessage) bool {
		_, ok := w.Payload.(*pb.WireMessage_HistoryResponse)
		return ok
	}, 3*time.Second)
}

func (c *testClient) sendText(t *testing.T, content string) string {
	c.msgCount++
	c.send(&pb.WireMessage{
		ClientSeq: int64(c.msgCount),
		FromQq:    c.qq,
		ToQq:      c.targetQQ,
		GroupId:   c.groupID,
		Payload:   &pb.WireMessage_TextMessage{TextMessage: &pb.TextMessage{Content: content}},
	})

	ackWire := c.readUntilPayload(t, func(w *pb.WireMessage) bool {
		_, ok := w.Payload.(*pb.WireMessage_ServerAck)
		return ok
	}, 3*time.Second)
	ack := ackWire.Payload.(*pb.WireMessage_ServerAck).ServerAck
	c.lastAck = ack.Content
	return ack.Content
}

func (c *testClient) receiveText(t *testing.T, timeout time.Duration) *pb.WireMessage {
	return c.readUntilPayload(t, func(w *pb.WireMessage) bool {
		_, ok := w.Payload.(*pb.WireMessage_TextMessage)
		return ok
	}, timeout)
}

func (c *testClient) close() {
	c.conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
	time.Sleep(100 * time.Millisecond)
	c.conn.Close()
}

func TestV06_AllFeatures(t *testing.T) {
	cleanup := setupTestDir(t)
	defer cleanup()

	serverCmd := exec.Command("go", "run", "./cmd/server")
	serverCmd.Dir = "/Users/yangshikang.6/Desktop/Code/Go/QQGO"
	serverCmd.Stdout = nil
	serverCmd.Stderr = nil
	if err := serverCmd.Start(); err != nil {
		t.Fatalf("failed to start server: %v", err)
	}
	defer func() {
		serverCmd.Process.Kill()
		serverCmd.Wait()
	}()

	time.Sleep(2 * time.Second)

	t.Run("1_TokenPersistence", func(t *testing.T) {
		clientA := newTestClient(t)
		defer clientA.close()
		clientA.register(t, "pass123", "TokenTest")

		savedQQ := clientA.qq
		savedToken := clientA.token

		clientA.close()
		time.Sleep(500 * time.Millisecond)

		clientA2 := newTestClient(t)
		defer clientA2.close()
		clientA2.loginWithToken(t, savedQQ, savedToken)

		if clientA2.qq != savedQQ {
			t.Fatalf("token login QQ mismatch: expected %d, got %d", savedQQ, clientA2.qq)
		}

		t.Logf("✓ Token persistence works: QQ=%d", savedQQ)
	})

	t.Run("2_BUG009_MessageDisplay", func(t *testing.T) {
		alice := newTestClient(t)
		defer alice.close()
		alice.register(t, "pass123", "Alice009")

		bob := newTestClient(t)
		defer bob.close()
		bob.register(t, "pass456", "Bob009")

		alice.switchTo(t, bob.qq)
		result := alice.sendText(t, "hello bob")
		if result != "ok" {
			t.Fatalf("send failed: %s", result)
		}

		msgWire := bob.receiveText(t, 3*time.Second)
		text := msgWire.Payload.(*pb.WireMessage_TextMessage).TextMessage
		if text.Content != "hello bob" {
			t.Fatalf("bob received wrong content: %s", text.Content)
		}
		if msgWire.FromQq != alice.qq {
			t.Fatalf("bob received wrong sender: %d", msgWire.FromQq)
		}

		t.Logf("✓ BUG-009: Message received correctly by Bob (QQ:%d -> QQ:%d)", msgWire.FromQq, msgWire.ToQq)
	})

	t.Run("3_BUG010_LeaveGroupExit", func(t *testing.T) {
		alice := newTestClient(t)
		defer alice.close()
		alice.register(t, "pass123", "Alice010")

		bob := newTestClient(t)
		defer bob.close()
		bob.register(t, "pass456", "Bob010")

		alice.send(&pb.WireMessage{Payload: &pb.WireMessage_GroupCreateRequest{GroupCreateRequest: &pb.GroupCreateRequest{Name: "test group 010"}}})
		createWire := alice.readUntilPayload(t, func(w *pb.WireMessage) bool {
			_, ok := w.Payload.(*pb.WireMessage_GroupCreateResponse)
			return ok
		}, 3*time.Second)
		groupID := createWire.Payload.(*pb.WireMessage_GroupCreateResponse).GroupCreateResponse.GroupId

		bob.send(&pb.WireMessage{Payload: &pb.WireMessage_GroupJoinRequest{GroupJoinRequest: &pb.GroupJoinRequest{GroupId: groupID}}})
		bob.readUntilPayload(t, func(w *pb.WireMessage) bool {
			_, ok := w.Payload.(*pb.WireMessage_ServerAck)
			return ok
		}, 3*time.Second)

		bob.send(&pb.WireMessage{Payload: &pb.WireMessage_GroupInfoRequest{GroupInfoRequest: &pb.GroupInfoRequest{GroupId: groupID}}})
		bob.readUntilPayload(t, func(w *pb.WireMessage) bool {
			_, ok := w.Payload.(*pb.WireMessage_GroupInfoResponse)
			return ok
		}, 3*time.Second)
		bob.groupID = groupID

		bob.send(&pb.WireMessage{Payload: &pb.WireMessage_GroupHistoryRequest{GroupHistoryRequest: &pb.GroupHistoryRequest{
			GroupId: groupID, Offset: 0, Limit: 30,
		}}})
		bob.readUntilPayload(t, func(w *pb.WireMessage) bool {
			_, ok := w.Payload.(*pb.WireMessage_GroupHistoryResponse)
			return ok
		}, 3*time.Second)

		bob.send(&pb.WireMessage{Payload: &pb.WireMessage_GroupLeaveRequest{GroupLeaveRequest: &pb.GroupLeaveRequest{GroupId: groupID}}})
		bob.readUntilPayload(t, func(w *pb.WireMessage) bool {
			_, ok := w.Payload.(*pb.WireMessage_ServerAck)
			return ok
		}, 3*time.Second)
		bob.groupID = ""

		bob.switchTo(t, alice.qq)
		result := bob.sendText(t, "private after leave")
		if result != "ok" {
			t.Fatalf("send after leavegroup failed: %s", result)
		}

		msgWire := alice.receiveText(t, 3*time.Second)
		if msgWire.GroupId != "" {
			t.Fatalf("message should not have group_id after leavegroup, got: %s", msgWire.GroupId)
		}

		t.Logf("✓ BUG-010: After leavegroup, messages go to private chat correctly")
	})

	t.Run("4_GroupHistory", func(t *testing.T) {
		alice := newTestClient(t)
		defer alice.close()
		alice.register(t, "pass123", "AliceGH")

		bob := newTestClient(t)
		defer bob.close()
		bob.register(t, "pass456", "BobGH")

		alice.send(&pb.WireMessage{Payload: &pb.WireMessage_GroupCreateRequest{GroupCreateRequest: &pb.GroupCreateRequest{Name: "history group"}}})
		createWire := alice.readUntilPayload(t, func(w *pb.WireMessage) bool {
			_, ok := w.Payload.(*pb.WireMessage_GroupCreateResponse)
			return ok
		}, 3*time.Second)
		groupID := createWire.Payload.(*pb.WireMessage_GroupCreateResponse).GroupCreateResponse.GroupId

		bob.send(&pb.WireMessage{Payload: &pb.WireMessage_GroupJoinRequest{GroupJoinRequest: &pb.GroupJoinRequest{GroupId: groupID}}})
		bob.readUntilPayload(t, func(w *pb.WireMessage) bool {
			_, ok := w.Payload.(*pb.WireMessage_ServerAck)
			return ok
		}, 3*time.Second)

		alice.groupID = groupID
		for i := 0; i < 5; i++ {
			alice.sendText(t, "group msg "+strconv.Itoa(i+1))
			time.Sleep(100 * time.Millisecond)
		}

		alice.groupID = ""
		alice.send(&pb.WireMessage{Payload: &pb.WireMessage_GroupHistoryRequest{GroupHistoryRequest: &pb.GroupHistoryRequest{
			GroupId: groupID, Offset: 0, Limit: 30,
		}}})
		histWire := alice.readUntilPayload(t, func(w *pb.WireMessage) bool {
			_, ok := w.Payload.(*pb.WireMessage_GroupHistoryResponse)
			return ok
		}, 3*time.Second)
		histResp := histWire.Payload.(*pb.WireMessage_GroupHistoryResponse).GroupHistoryResponse

		if len(histResp.Messages) != 5 {
			t.Fatalf("expected 5 group history messages, got %d", len(histResp.Messages))
		}

		t.Logf("✓ Group history: %d messages retrieved", len(histResp.Messages))
	})

	t.Run("5_LocalChatLog", func(t *testing.T) {
		t.Log("Skipped: local chat log is a client-side feature, verified by unit tests in localstore_test.go")
	})

	t.Run("6_GroupMemberValidation", func(t *testing.T) {
		alice := newTestClient(t)
		defer alice.close()
		alice.register(t, "pass123", "AliceGM")

		charlie := newTestClient(t)
		defer charlie.close()
		charlie.register(t, "pass789", "CharlieGM")

		alice.send(&pb.WireMessage{Payload: &pb.WireMessage_GroupCreateRequest{GroupCreateRequest: &pb.GroupCreateRequest{Name: "member test"}}})
		createWire := alice.readUntilPayload(t, func(w *pb.WireMessage) bool {
			_, ok := w.Payload.(*pb.WireMessage_GroupCreateResponse)
			return ok
		}, 3*time.Second)
		groupID := createWire.Payload.(*pb.WireMessage_GroupCreateResponse).GroupCreateResponse.GroupId

		charlie.groupID = groupID
		result := charlie.sendText(t, "non member msg")
		if result != "not group member" {
			t.Fatalf("expected 'not group member', got: %s", result)
		}

		charlie.send(&pb.WireMessage{Payload: &pb.WireMessage_GroupJoinRequest{GroupJoinRequest: &pb.GroupJoinRequest{GroupId: groupID}}})
		charlie.readUntilPayload(t, func(w *pb.WireMessage) bool {
			_, ok := w.Payload.(*pb.WireMessage_ServerAck)
			return ok
		}, 3*time.Second)

		result = charlie.sendText(t, "member msg")
		if result != "ok" {
			t.Fatalf("expected 'ok' after joining, got: %s", result)
		}

		t.Logf("✓ Group member validation: non-member rejected, member accepted")
	})

	t.Run("7_SessionList", func(t *testing.T) {
		alice := newTestClient(t)
		defer alice.close()
		alice.register(t, "pass123", "AliceSL")

		bob := newTestClient(t)
		defer bob.close()
		bob.register(t, "pass456", "BobSL")

		alice.switchTo(t, bob.qq)
		alice.sendText(t, "session test")
		time.Sleep(500 * time.Millisecond)

		alice.send(&pb.WireMessage{Payload: &pb.WireMessage_SessionListRequest{SessionListRequest: &pb.SessionListRequest{}}})
		sessWire := alice.readUntilPayload(t, func(w *pb.WireMessage) bool {
			_, ok := w.Payload.(*pb.WireMessage_SessionListResponse)
			return ok
		}, 3*time.Second)
		sessResp := sessWire.Payload.(*pb.WireMessage_SessionListResponse).SessionListResponse

		if len(sessResp.Sessions) == 0 {
			t.Fatal("session list should not be empty after sending messages")
		}

		foundBob := false
		for _, s := range sessResp.Sessions {
			if s.TargetQq == bob.qq {
				foundBob = true
				if s.LastMessage != "session test" {
					t.Fatalf("last message mismatch: expected 'session test', got '%s'", s.LastMessage)
				}
			}
		}
		if !foundBob {
			t.Fatal("session list should contain Bob's session")
		}

		t.Logf("✓ Session list: %d sessions, Bob's session found", len(sessResp.Sessions))
	})

	t.Run("8_NonFriendLimit", func(t *testing.T) {
		alice := newTestClient(t)
		defer alice.close()
		alice.register(t, "pass123", "AliceNF")

		bob := newTestClient(t)
		defer bob.close()
		bob.register(t, "pass456", "BobNF")

		alice.switchTo(t, bob.qq)
		result1 := alice.sendText(t, "first msg")
		if result1 != "ok" {
			t.Fatalf("first message should succeed: %s", result1)
		}

		result2 := alice.sendText(t, "second msg")
		if result2 == "ok" {
			t.Fatal("second message should fail for non-friend")
		}
		if !strings.Contains(result2, "not friend") {
			t.Fatalf("expected 'not friend' error, got: %s", result2)
		}

		t.Logf("✓ Non-friend limit: first msg ok, second msg blocked")
	})
}
