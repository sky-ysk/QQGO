package main

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/qqgo/server/internal/model"
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

func (c *testClient) send(msg *model.Message) error {
	data, _ := json.Marshal(msg)
	return c.conn.WriteMessage(websocket.TextMessage, data)
}

func (c *testClient) read(timeout time.Duration) (*model.Message, error) {
	if c.conn == nil {
		return nil, fmt.Errorf("connection closed")
	}
	done := make(chan struct{})
	var msg *model.Message
	var readErr error

	go func() {
		_, data, err := c.conn.ReadMessage()
		if err != nil {
			readErr = err
		} else {
			var m model.Message
			if err := json.Unmarshal(data, &m); err != nil {
				readErr = err
			} else {
				msg = &m
			}
		}
		close(done)
	}()

	select {
	case <-done:
		return msg, readErr
	case <-time.After(timeout):
		return nil, fmt.Errorf("read timeout")
	}
}

func (c *testClient) readUntil(t *testing.T, msgType model.MessageType, timeout time.Duration) *model.Message {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		msg, err := c.read(500 * time.Millisecond)
		if err != nil {
			if strings.Contains(err.Error(), "closed") || strings.Contains(err.Error(), "close") {
				t.Fatalf("connection closed while waiting for msgType %d", msgType)
			}
			continue
		}
		if msg.MsgType == msgType {
			return msg
		}
	}
	t.Fatalf("timeout waiting for msgType %d", msgType)
	return nil
}

func (c *testClient) register(t *testing.T, password, nickname string) {
	payload, _ := json.Marshal(&model.RegisterRequest{
		Password: password,
		Nickname: nickname,
	})
	c.send(&model.Message{
		MsgType: model.MsgTypeRegister,
		Content: string(payload),
	})

	regAck := c.readUntil(t, model.MsgTypeRegisterAck, 3*time.Second)
	var regResp model.RegisterResponse
	json.Unmarshal([]byte(regAck.Content), &regResp)
	if regResp.Code != 0 {
		t.Fatalf("register failed: %s", regResp.Message)
	}
	c.qq = regResp.QQNumber
	c.nickname = nickname

	loginAck := c.readUntil(t, model.MsgTypeLoginAck, 3*time.Second)
	var loginResp model.LoginResponse
	json.Unmarshal([]byte(loginAck.Content), &loginResp)
	if loginResp.Code != 0 {
		t.Fatalf("auto-login after register failed: %s", loginResp.Message)
	}
	c.token = loginResp.Token
	t.Logf("Registered: QQ=%d, nickname=%s", c.qq, c.nickname)
}

func (c *testClient) login(t *testing.T, qq int64, password string) {
	payload, _ := json.Marshal(&model.LoginRequest{
		QQ:       qq,
		Password: password,
		Platform: "cli",
	})
	c.send(&model.Message{
		MsgType: model.MsgTypeLogin,
		Content: string(payload),
	})

	loginAck := c.readUntil(t, model.MsgTypeLoginAck, 3*time.Second)
	var loginResp model.LoginResponse
	json.Unmarshal([]byte(loginAck.Content), &loginResp)
	if loginResp.Code != 0 {
		t.Fatalf("login failed: %s", loginResp.Message)
	}
	c.qq = loginResp.QQNumber
	c.nickname = loginResp.Nickname
	c.token = loginResp.Token
}

func (c *testClient) loginWithToken(t *testing.T, qq int64, token string) {
	payload, _ := json.Marshal(&model.LoginRequest{
		QQ:       qq,
		Token:    token,
		Platform: "cli",
	})
	c.send(&model.Message{
		MsgType: model.MsgTypeLogin,
		Content: string(payload),
	})

	loginAck := c.readUntil(t, model.MsgTypeLoginAck, 3*time.Second)
	var loginResp model.LoginResponse
	json.Unmarshal([]byte(loginAck.Content), &loginResp)
	if loginResp.Code != 0 {
		t.Fatalf("token login failed: %s", loginResp.Message)
	}
	c.qq = loginResp.QQNumber
	c.nickname = loginResp.Nickname
	c.token = loginResp.Token
}

func (c *testClient) switchTo(t *testing.T, targetQQ int64) {
	c.send(&model.Message{
		MsgType: model.MsgTypeCheckUser,
		Content: strconv.FormatInt(targetQQ, 10),
	})

	resp := c.readUntil(t, model.MsgTypeCheckUser, 3*time.Second)
	var checkResp model.CheckUserResponse
	json.Unmarshal([]byte(resp.Content), &checkResp)
	if checkResp.Code != 0 {
		t.Fatalf("check user failed: %s", checkResp.Message)
	}
	c.targetQQ = targetQQ

	histPayload, _ := json.Marshal(&model.HistoryRequest{
		TargetQQ: targetQQ,
		Offset:   0,
		Limit:    30,
	})
	c.send(&model.Message{
		MsgType: model.MsgTypeHistory,
		Content: string(histPayload),
	})
	c.readUntil(t, model.MsgTypeHistory, 3*time.Second)
}

func (c *testClient) sendText(t *testing.T, content string) string {
	c.msgCount++
	msg := &model.Message{
		ClientSeq: int64(c.msgCount),
		MsgType:   model.MsgTypeText,
		FromQQ:    c.qq,
		ToQQ:      c.targetQQ,
		GroupID:   c.groupID,
		Content:   content,
	}
	c.send(msg)

	ack := c.readUntil(t, model.MsgTypeServerAck, 3*time.Second)
	c.lastAck = ack.Content
	return ack.Content
}

func (c *testClient) receiveText(t *testing.T, timeout time.Duration) *model.Message {
	return c.readUntil(t, model.MsgTypeText, timeout)
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

		msg := bob.receiveText(t, 3*time.Second)
		if msg.Content != "hello bob" {
			t.Fatalf("bob received wrong content: %s", msg.Content)
		}
		if msg.FromQQ != alice.qq {
			t.Fatalf("bob received wrong sender: %d", msg.FromQQ)
		}

		t.Logf("✓ BUG-009: Message received correctly by Bob (QQ:%d -> QQ:%d)", msg.FromQQ, msg.ToQQ)
	})

	t.Run("3_BUG010_LeaveGroupExit", func(t *testing.T) {
		alice := newTestClient(t)
		defer alice.close()
		alice.register(t, "pass123", "Alice010")

		bob := newTestClient(t)
		defer bob.close()
		bob.register(t, "pass456", "Bob010")

		payload, _ := json.Marshal(&model.GroupCreateRequest{Name: "test group 010"})
		alice.send(&model.Message{
			MsgType: model.MsgTypeGroupCreate,
			Content: string(payload),
		})
		createResp := alice.readUntil(t, model.MsgTypeGroupCreate, 3*time.Second)
		var groupResult map[string]interface{}
		json.Unmarshal([]byte(createResp.Content), &groupResult)
		groupID := groupResult["group_id"].(string)

		bob.send(&model.Message{
			MsgType: model.MsgTypeGroupJoin,
			Content: groupID,
		})
		bob.readUntil(t, model.MsgTypeServerAck, 3*time.Second)

		bob.send(&model.Message{
			MsgType: model.MsgTypeGroupInfo,
			Content: groupID,
		})
		bob.readUntil(t, model.MsgTypeGroupInfo, 3*time.Second)
		bob.groupID = groupID

		ghPayload, _ := json.Marshal(&model.GroupHistoryRequest{
			GroupID: groupID,
			Offset:  0,
			Limit:   30,
		})
		bob.send(&model.Message{
			MsgType: model.MsgTypeGroupHistory,
			Content: string(ghPayload),
		})
		bob.readUntil(t, model.MsgTypeGroupHistory, 3*time.Second)

		bob.send(&model.Message{
			MsgType: model.MsgTypeGroupLeave,
			Content: groupID,
		})
		bob.readUntil(t, model.MsgTypeServerAck, 3*time.Second)
		bob.groupID = ""

		bob.switchTo(t, alice.qq)
		result := bob.sendText(t, "private after leave")
		if result != "ok" {
			t.Fatalf("send after leavegroup failed: %s", result)
		}

		msg := alice.receiveText(t, 3*time.Second)
		if msg.GroupID != "" {
			t.Fatalf("message should not have group_id after leavegroup, got: %s", msg.GroupID)
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

		payload, _ := json.Marshal(&model.GroupCreateRequest{Name: "history group"})
		alice.send(&model.Message{
			MsgType: model.MsgTypeGroupCreate,
			Content: string(payload),
		})
		createResp := alice.readUntil(t, model.MsgTypeGroupCreate, 3*time.Second)
		var groupResult map[string]interface{}
		json.Unmarshal([]byte(createResp.Content), &groupResult)
		groupID := groupResult["group_id"].(string)

		bob.send(&model.Message{
			MsgType: model.MsgTypeGroupJoin,
			Content: groupID,
		})
		bob.readUntil(t, model.MsgTypeServerAck, 3*time.Second)

		alice.groupID = groupID
		for i := 0; i < 5; i++ {
			alice.sendText(t, "group msg "+strconv.Itoa(i+1))
			time.Sleep(100 * time.Millisecond)
		}

		alice.groupID = ""
		ghPayload, _ := json.Marshal(&model.GroupHistoryRequest{
			GroupID: groupID,
			Offset:  0,
			Limit:   30,
		})
		alice.send(&model.Message{
			MsgType: model.MsgTypeGroupHistory,
			Content: string(ghPayload),
		})
		histResp := alice.readUntil(t, model.MsgTypeGroupHistory, 3*time.Second)
		var histResult model.GroupHistoryResponse
		json.Unmarshal([]byte(histResp.Content), &histResult)

		if len(histResult.Messages) != 5 {
			t.Fatalf("expected 5 group history messages, got %d", len(histResult.Messages))
		}

		t.Logf("✓ Group history: %d messages retrieved", len(histResult.Messages))
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

		payload, _ := json.Marshal(&model.GroupCreateRequest{Name: "member test"})
		alice.send(&model.Message{
			MsgType: model.MsgTypeGroupCreate,
			Content: string(payload),
		})
		createResp := alice.readUntil(t, model.MsgTypeGroupCreate, 3*time.Second)
		var groupResult map[string]interface{}
		json.Unmarshal([]byte(createResp.Content), &groupResult)
		groupID := groupResult["group_id"].(string)

		charlie.groupID = groupID
		result := charlie.sendText(t, "non member msg")
		if result != "not group member" {
			t.Fatalf("expected 'not group member', got: %s", result)
		}

		charlie.send(&model.Message{
			MsgType: model.MsgTypeGroupJoin,
			Content: groupID,
		})
		charlie.readUntil(t, model.MsgTypeServerAck, 3*time.Second)

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

		alice.send(&model.Message{
			MsgType: model.MsgTypeSessionList,
		})
		sessResp := alice.readUntil(t, model.MsgTypeSessionList, 3*time.Second)
		var sessResult model.SessionListResponse
		json.Unmarshal([]byte(sessResp.Content), &sessResult)

		if len(sessResult.Sessions) == 0 {
			t.Fatal("session list should not be empty after sending messages")
		}

		foundBob := false
		for _, s := range sessResult.Sessions {
			if s.TargetQQ == bob.qq {
				foundBob = true
				if s.LastMessage != "session test" {
					t.Fatalf("last message mismatch: expected 'session test', got '%s'", s.LastMessage)
				}
			}
		}
		if !foundBob {
			t.Fatal("session list should contain Bob's session")
		}

		t.Logf("✓ Session list: %d sessions, Bob's session found", len(sessResult.Sessions))
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
