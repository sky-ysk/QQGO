package service

import (
	"fmt"
	"testing"
	"time"

	"github.com/qqgo/server/internal/config"
	"github.com/qqgo/server/internal/model"
	"github.com/qqgo/server/internal/store"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}

	if err := db.AutoMigrate(
		&model.User{},
		&model.Friend{},
		&model.FriendGroup{},
		&model.MessageCount{},
		&model.Group{},
		&model.GroupMember{},
		&model.Message{},
		&model.Blacklist{},
	); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	InitJWT(config.JWTConfig{Secret: "test-secret", AccessTTL: 900, RefreshTTLDays: 7})

	if err := store.InitFTS(db); err != nil {
		t.Logf("FTS init warning: %v (search tests may fail)", err)
	}

	return db
}

func TestNonFriendMessageLimit(t *testing.T) {
	db := setupTestDB(t)
	svc := NewChatService(db, "")

	qq1, err := svc.Register("alice", "password123")
	if err != nil {
		t.Fatalf("register alice failed: %v", err)
	}

	qq2, err := svc.Register("bob", "password456")
	if err != nil {
		t.Fatalf("register bob failed: %v", err)
	}

	if svc.IsFriend(qq1, qq2) {
		t.Fatal("should not be friends yet")
	}

	err = svc.CheckAndIncrementNonFriendMessage(qq1, qq2)
	if err != nil {
		t.Fatalf("first message should succeed: %v", err)
	}

	err = svc.CheckAndIncrementNonFriendMessage(qq1, qq2)
	if err == nil {
		t.Fatal("second message should fail with limit error")
	}
	if err != ErrNonFriendMsgLimit {
		t.Fatalf("expected ErrNonFriendMsgLimit, got: %v", err)
	}

	err = svc.CheckAndIncrementNonFriendMessage(qq2, qq1)
	if err != nil {
		t.Fatalf("bob's first message to alice should succeed: %v", err)
	}

	err = svc.SendFriendRequest(qq1, qq2, "hello")
	if err != nil {
		t.Fatalf("send friend request failed: %v", err)
	}

	err = svc.AcceptFriend(qq2, qq1)
	if err != nil {
		t.Fatalf("accept friend failed: %v", err)
	}

	if !svc.IsFriend(qq1, qq2) {
		t.Fatal("should be friends now")
	}

	err = svc.CheckAndIncrementNonFriendMessage(qq1, qq2)
	if err != nil {
		t.Fatalf("friends should send unlimited messages: %v", err)
	}
	err = svc.CheckAndIncrementNonFriendMessage(qq1, qq2)
	if err != nil {
		t.Fatalf("friends should send unlimited messages (2nd): %v", err)
	}
}

func TestClearMessageCountsOnAcceptFriend(t *testing.T) {
	db := setupTestDB(t)
	svc := NewChatService(db, "")

	qq1, _ := svc.Register("charlie", "password123")
	qq2, _ := svc.Register("dave", "password456")

	svc.CheckAndIncrementNonFriendMessage(qq1, qq2)

	var mc model.MessageCount
	db.Where("from_qq = ? AND to_qq = ?", qq1, qq2).First(&mc)
	if mc.Count != 1 {
		t.Fatalf("expected count 1, got %d", mc.Count)
	}

	svc.SendFriendRequest(qq1, qq2, "hello")
	svc.AcceptFriend(qq2, qq1)

	var count int64
	db.Model(&model.MessageCount{}).Where("from_qq = ? AND to_qq = ?", qq1, qq2).Count(&count)
	if count != 0 {
		t.Fatalf("message counts should be cleared after accepting friend, got %d records", count)
	}
}

func TestDeleteFriendThenMessageLimit(t *testing.T) {
	db := setupTestDB(t)
	svc := NewChatService(db, "")

	qq1, _ := svc.Register("eve", "password123")
	qq2, _ := svc.Register("frank", "password456")

	svc.SendFriendRequest(qq1, qq2, "hello")
	svc.AcceptFriend(qq2, qq1)

	if !svc.IsFriend(qq1, qq2) {
		t.Fatal("should be friends")
	}

	svc.DeleteFriend(qq1, qq2)

	if svc.IsFriend(qq1, qq2) {
		t.Fatal("should not be friends after delete")
	}

	err := svc.CheckAndIncrementNonFriendMessage(qq1, qq2)
	if err != nil {
		t.Fatalf("first message after delete should succeed: %v", err)
	}

	err = svc.CheckAndIncrementNonFriendMessage(qq1, qq2)
	if err == nil {
		t.Fatal("second message after delete should fail")
	}
	if err != ErrNonFriendMsgLimit {
		t.Fatalf("expected ErrNonFriendMsgLimit, got: %v", err)
	}
}

func TestGetHistoryWithTarget(t *testing.T) {
	db := setupTestDB(t)
	svc := NewChatService(db, "")

	qq1, _ := svc.Register("alice", "password123")
	qq2, _ := svc.Register("bob", "password456")

	for i := 1; i <= 5; i++ {
		svc.HandleMessage(nil, &model.Message{
			MsgType: model.MsgTypeText,
			FromQQ:  qq1,
			ToQQ:    qq2,
			Content: "hello " + string(rune('0'+i)),
		})
		svc.HandleMessage(nil, &model.Message{
			MsgType: model.MsgTypeText,
			FromQQ:  qq2,
			ToQQ:    qq1,
			Content: "reply " + string(rune('0'+i)),
		})
	}

	msgs, hasMore, err := svc.GetHistoryWithTarget(qq1, qq2, 0, 30, "", "")
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if len(msgs) != 10 {
		t.Fatalf("expected 10 messages, got %d", len(msgs))
	}
	if hasMore {
		t.Fatal("should not have more")
	}

	msgs, hasMore, err = svc.GetHistoryWithTarget(qq1, qq2, 0, 5, "", "")
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if len(msgs) != 5 {
		t.Fatalf("expected 5 messages, got %d", len(msgs))
	}
	if !hasMore {
		t.Fatal("should have more")
	}

	msgs, hasMore, err = svc.GetHistoryWithTarget(qq1, qq2, 5, 5, "", "")
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if len(msgs) != 5 {
		t.Fatalf("expected 5 messages on page 2, got %d", len(msgs))
	}
	if hasMore {
		t.Fatal("should not have more on page 2")
	}

	msgs, _, err = svc.GetHistoryWithTarget(qq1, qq2, 100, 30, "", "")
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if len(msgs) != 0 {
		t.Fatalf("expected 0 messages for out of range offset, got %d", len(msgs))
	}
}

func TestGroupChat(t *testing.T) {
	db := setupTestDB(t)
	svc := NewChatService(db, "")

	qq1, _ := svc.Register("alice", "password123")
	qq2, _ := svc.Register("bob", "password456")
	_, _ = svc.Register("charlie", "password789")

	groupID, err := svc.CreateGroup("test group", qq1)
	if err != nil {
		t.Fatalf("create group failed: %v", err)
	}

	members, err := svc.GetGroupMembers(groupID)
	if err != nil {
		t.Fatalf("get members failed: %v", err)
	}
	if len(members) != 1 {
		t.Fatalf("expected 1 member, got %d", len(members))
	}
	if members[0] != qq1 {
		t.Fatalf("expected owner qq=%d, got %d", qq1, members[0])
	}

	if !svc.IsGroupMember(groupID, qq1) {
		t.Fatal("alice should be group member")
	}
	if svc.IsGroupMember(groupID, qq2) {
		t.Fatal("bob should not be group member yet")
	}

	err = svc.JoinGroup(groupID, qq2)
	if err != nil {
		t.Fatalf("join group failed: %v", err)
	}

	members, _ = svc.GetGroupMembers(groupID)
	if len(members) != 2 {
		t.Fatalf("expected 2 members, got %d", len(members))
	}

	err = svc.JoinGroup(groupID, qq2)
	if err == nil {
		t.Fatal("should fail when joining again")
	}

	groups, err := svc.GetGroupList(qq2)
	if err != nil {
		t.Fatalf("get group list failed: %v", err)
	}
	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}
	if groups[0].GroupID != groupID {
		t.Fatalf("expected group %s, got %s", groupID, groups[0].GroupID)
	}

	err = svc.LeaveGroup(groupID, qq2)
	if err != nil {
		t.Fatalf("leave group failed: %v", err)
	}

	if svc.IsGroupMember(groupID, qq2) {
		t.Fatal("bob should not be group member after leaving")
	}

	err = svc.LeaveGroup(groupID, qq1)
	if err == nil {
		t.Fatal("owner should not be able to leave")
	}
}

func TestGetGroupHistory(t *testing.T) {
	db := setupTestDB(t)
	svc := NewChatService(db, "")

	qq1, _ := svc.Register("alice", "password123")
	qq2, _ := svc.Register("bob", "password456")

	groupID, _ := svc.CreateGroup("history test", qq1)
	svc.JoinGroup(groupID, qq2)

	for i := 1; i <= 5; i++ {
		svc.HandleMessage(nil, &model.Message{
			MsgType: model.MsgTypeText,
			FromQQ:  qq1,
			ToQQ:    0,
			GroupID: groupID,
			Content: "group msg " + string(rune('0'+i)),
		})
		svc.HandleMessage(nil, &model.Message{
			MsgType: model.MsgTypeText,
			FromQQ:  qq2,
			ToQQ:    0,
			GroupID: groupID,
			Content: "reply " + string(rune('0'+i)),
		})
	}

	msgs, hasMore, err := svc.GetGroupHistory(groupID, 0, 30)
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if len(msgs) != 10 {
		t.Fatalf("expected 10 messages, got %d", len(msgs))
	}
	if hasMore {
		t.Fatal("should not have more")
	}

	msgs, hasMore, err = svc.GetGroupHistory(groupID, 0, 5)
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if len(msgs) != 5 {
		t.Fatalf("expected 5 messages, got %d", len(msgs))
	}
	if !hasMore {
		t.Fatal("should have more")
	}

	msgs, hasMore, err = svc.GetGroupHistory(groupID, 5, 5)
	if err != nil {
		t.Fatalf("query page 2 failed: %v", err)
	}
	if len(msgs) != 5 {
		t.Fatalf("expected 5 messages on page 2, got %d", len(msgs))
	}
	if hasMore {
		t.Fatal("should not have more on page 2")
	}

	msgs, _, err = svc.GetGroupHistory("nonexistent", 0, 30)
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if len(msgs) != 0 {
		t.Fatalf("expected 0 messages for nonexistent group, got %d", len(msgs))
	}

	msgs, _, err = svc.GetGroupHistory(groupID, 100, 30)
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if len(msgs) != 0 {
		t.Fatalf("expected 0 messages for out of range offset, got %d", len(msgs))
	}
}

func TestGroupHistoryOnlyReturnsChatMessages(t *testing.T) {
	db := setupTestDB(t)
	svc := NewChatService(db, "")

	qq1, _ := svc.Register("alice", "password123")

	groupID, _ := svc.CreateGroup("filter test", qq1)

	svc.HandleMessage(nil, &model.Message{
		MsgType: model.MsgTypeText,
		FromQQ:  qq1,
		GroupID: groupID,
		Content: "text message",
	})
	svc.HandleMessage(nil, &model.Message{
		MsgType: model.MsgTypeFriendRequest,
		FromQQ:  qq1,
		GroupID: groupID,
		Content: "should be filtered",
	})

	msgs, _, err := svc.GetGroupHistory(groupID, 0, 30)
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message (friend request filtered), got %d", len(msgs))
	}
	if msgs[0].Content != "text message" {
		t.Fatalf("expected text message, got %s", msgs[0].Content)
	}
}

func createTestUser(db *gorm.DB, nickname string, hash string) int64 {
	token := fmt.Sprintf("test_token_%s", nickname)
	user := &model.User{
		PasswordHash: hash,
		Token:        token,
		Nickname:     nickname,
	}
	db.Create(user)
	qqNumber := int64(model.QQNumberBase) + int64(user.ID)
	db.Model(user).Updates(map[string]interface{}{
		"qq_number": qqNumber,
		"token":     token,
	})
	return qqNumber
}

func TestFriendLimit500(t *testing.T) {
	db := setupTestDB(t)
	svc := NewChatService(db, "")

	hash := "$2a$04$xxxxxxxxxxxxxxxxxxxxxO"

	qqA := createTestUser(db, "alice", hash)
	qqB := createTestUser(db, "bob_last", hash)
	qqC := createTestUser(db, "charlie_fail", hash)

	for i := 0; i < 499; i++ {
		qqOther := createTestUser(db, fmt.Sprintf("friend_%d", i), hash)
		db.Create(&model.Friend{
			QQ:       qqA,
			FriendQQ: qqOther,
			Status:   model.FriendStatusAccepted,
			GroupName: "我的好友",
		})
		db.Create(&model.Friend{
			QQ:       qqOther,
			FriendQQ: qqA,
			Status:   model.FriendStatusAccepted,
			GroupName: "我的好友",
		})
	}

	var countA int64
	db.Model(&model.Friend{}).Where("qq = ? AND status = ?", qqA, model.FriendStatusAccepted).Count(&countA)
	if countA != 499 {
		t.Fatalf("alice should have 499 friends, got %d", countA)
	}

	err := svc.SendFriendRequest(qqB, qqA, "hello from bob")
	if err != nil {
		t.Fatalf("bob's friend request should succeed: %v", err)
	}

	err = svc.AcceptFriend(qqA, qqB)
	if err != nil {
		t.Fatalf("alice should accept bob (499+1=500): %v", err)
	}

	var countAfter int64
	db.Model(&model.Friend{}).Where("qq = ? AND status = ?", qqA, model.FriendStatusAccepted).Count(&countAfter)
	if countAfter != 500 {
		t.Fatalf("alice should have 500 friends after accepting bob, got %d", countAfter)
	}

	err = svc.SendFriendRequest(qqC, qqA, "hello from charlie")
	if err != nil {
		t.Fatalf("charlie's friend request should succeed (pending): %v", err)
	}

	err = svc.AcceptFriend(qqA, qqC)
	if err != ErrFriendLimit {
		t.Fatalf("alice should NOT accept charlie (500+1=501), expected ErrFriendLimit, got: %v", err)
	}
}

func TestOfflineFriendRequest(t *testing.T) {
	db := setupTestDB(t)
	svc := NewChatService(db, "")

	qqA, err := svc.Register("alice", "password123")
	if err != nil {
		t.Fatalf("register alice failed: %v", err)
	}

	qqB, err := svc.Register("bob", "password456")
	if err != nil {
		t.Fatalf("register bob failed: %v", err)
	}

	err = svc.SendFriendRequest(qqA, qqB, "hello from alice")
	if err != nil {
		t.Fatalf("friend request should succeed: %v", err)
	}

	var pending model.Friend
	result := db.Where("qq = ? AND friend_qq = ? AND status = ?", qqA, qqB, model.FriendStatusPending).First(&pending)
	if result.Error != nil {
		t.Fatalf("pending friend request should exist in DB: %v", result.Error)
	}

	onlineFunc := func(qq int64) bool { return false }
	friendList, err := svc.GetFriendList(qqB, onlineFunc)
	if err != nil {
		t.Fatalf("get friend list failed: %v", err)
	}

	found := false
	for _, f := range friendList {
		if f.QQNumber == qqA && f.Status == model.FriendStatusPending && f.GroupName == "待处理" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("bob should see alice's pending request in 待处理 group, got %d items", len(friendList))
	}

	err = svc.AcceptFriend(qqB, qqA)
	if err != nil {
		t.Fatalf("bob should accept alice: %v", err)
	}

	friendListB, _ := svc.GetFriendList(qqB, onlineFunc)
	foundAccepted := false
	for _, f := range friendListB {
		if f.QQNumber == qqA && f.Status == model.FriendStatusAccepted {
			foundAccepted = true
			break
		}
	}
	if !foundAccepted {
		t.Fatal("bob should see alice as accepted friend")
	}

	friendListA, _ := svc.GetFriendList(qqA, onlineFunc)
	foundReverse := false
	for _, f := range friendListA {
		if f.QQNumber == qqB && f.Status == model.FriendStatusAccepted {
			foundReverse = true
			break
		}
	}
	if !foundReverse {
		t.Fatal("alice should see bob as accepted friend")
	}
}

func TestChangePassword(t *testing.T) {
	db := setupTestDB(t)
	svc := NewChatService(db, "")

	qq, err := svc.Register("alice", "oldpassword")
	if err != nil {
		t.Fatalf("register failed: %v", err)
	}

	accessTok, refreshTok, err := svc.ChangePassword(qq, "oldpassword", "newpassword")
	if err != nil {
		t.Fatalf("change password failed: %v", err)
	}
	if accessTok == "" {
		t.Fatal("new access token should not be empty")
	}
	if refreshTok == "" {
		t.Fatal("new refresh token should not be empty")
	}

	_, _, err = svc.Login(qq, "newpassword")
	if err != nil {
		t.Fatalf("login with new password should succeed: %v", err)
	}

	_, _, err = svc.Login(qq, "oldpassword")
	if err != ErrAuthFailed {
		t.Fatalf("login with old password should fail, got: %v", err)
	}
}

func TestChangePasswordWrongOld(t *testing.T) {
	db := setupTestDB(t)
	svc := NewChatService(db, "")

	qq, _ := svc.Register("bob", "correctpassword")

	_, _, err := svc.ChangePassword(qq, "wrongpassword", "newpassword")
	if err != ErrAuthFailed {
		t.Fatalf("should fail with wrong old password, got: %v", err)
	}
}

func TestBlockUser(t *testing.T) {
	db := setupTestDB(t)
	svc := NewChatService(db, "")

	qqA, _ := svc.Register("alice", "password123")
	qqB, _ := svc.Register("bob", "password456")

	if svc.IsBlocked(qqA, qqB) {
		t.Fatal("bob should not be blocked yet")
	}

	err := svc.BlockUser(qqA, qqB)
	if err != nil {
		t.Fatalf("block should succeed: %v", err)
	}

	if !svc.IsBlocked(qqA, qqB) {
		t.Fatal("bob should be blocked now")
	}

	err = svc.BlockUser(qqA, qqB)
	if err == nil {
		t.Fatal("blocking again should fail")
	}
}

func TestBlockSelf(t *testing.T) {
	db := setupTestDB(t)
	svc := NewChatService(db, "")

	qq, _ := svc.Register("selfblock", "password123")

	err := svc.BlockUser(qq, qq)
	if err == nil {
		t.Fatal("blocking self should fail")
	}
}

func TestUnblockUser(t *testing.T) {
	db := setupTestDB(t)
	svc := NewChatService(db, "")

	qqA, _ := svc.Register("alice", "password123")
	qqB, _ := svc.Register("bob", "password456")

	svc.BlockUser(qqA, qqB)

	if !svc.IsBlocked(qqA, qqB) {
		t.Fatal("bob should be blocked")
	}

	err := svc.UnblockUser(qqA, qqB)
	if err != nil {
		t.Fatalf("unblock should succeed: %v", err)
	}

	if svc.IsBlocked(qqA, qqB) {
		t.Fatal("bob should not be blocked after unblock")
	}

	err = svc.UnblockUser(qqA, qqB)
	if err == nil {
		t.Fatal("unblocking non-blocked user should fail")
	}
}

func TestGetBlacklist(t *testing.T) {
	db := setupTestDB(t)
	svc := NewChatService(db, "")

	qqA, _ := svc.Register("alice", "password123")
	qqB, _ := svc.Register("bob", "password456")
	qqC, _ := svc.Register("charlie", "password789")

	svc.BlockUser(qqA, qqB)
	svc.BlockUser(qqA, qqC)

	list, err := svc.GetBlacklist(qqA)
	if err != nil {
		t.Fatalf("get blacklist failed: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 blocked users, got %d", len(list))
	}

	foundB := false
	foundC := false
	for _, u := range list {
		if u.QQNumber == qqB {
			foundB = true
		}
		if u.QQNumber == qqC {
			foundC = true
		}
	}
	if !foundB || !foundC {
		t.Fatal("both bob and charlie should be in blacklist")
	}

	emptyList, _ := svc.GetBlacklist(qqB)
	if len(emptyList) != 0 {
		t.Fatal("bob's blacklist should be empty")
	}
}

func TestMarkRead(t *testing.T) {
	db := setupTestDB(t)
	svc := NewChatService(db, "")

	qq1, _ := svc.Register("alice", "password123")
	qq2, _ := svc.Register("bob", "password456")

	svc.HandleMessage(nil, &model.Message{
		MsgType: model.MsgTypeText,
		FromQQ:  qq1,
		ToQQ:    qq2,
		Content: "hello",
	})

	var msg model.Message
	db.Where("from_qq = ? AND to_qq = ?", qq1, qq2).First(&msg)
	if msg.ReadAt != nil {
		t.Fatal("message should not be read yet")
	}

	err := svc.MarkRead(msg.ID)
	if err != nil {
		t.Fatalf("mark read failed: %v", err)
	}

	db.Where("id = ?", msg.ID).First(&msg)
	if msg.ReadAt == nil {
		t.Fatal("message should be marked as read")
	}
}

func TestRecallMessage(t *testing.T) {
	db := setupTestDB(t)
	svc := NewChatService(db, "")

	qq1, _ := svc.Register("alice", "password123")
	qq2, _ := svc.Register("bob", "password456")

	svc.HandleMessage(nil, &model.Message{
		MsgType: model.MsgTypeText,
		FromQQ:  qq1,
		ToQQ:    qq2,
		Content: "hello",
	})

	var msg model.Message
	db.Where("from_qq = ? AND to_qq = ?", qq1, qq2).First(&msg)

	err := svc.RecallMessage(qq1, msg.ID)
	if err != nil {
		t.Fatalf("recall should succeed: %v", err)
	}

	db.Where("id = ?", msg.ID).First(&msg)
	if !msg.IsRecalled {
		t.Fatal("message should be recalled")
	}
}

func TestRecallMessageNotSender(t *testing.T) {
	db := setupTestDB(t)
	svc := NewChatService(db, "")

	qq1, _ := svc.Register("alice", "password123")
	qq2, _ := svc.Register("bob", "password456")

	svc.HandleMessage(nil, &model.Message{
		MsgType: model.MsgTypeText,
		FromQQ:  qq1,
		ToQQ:    qq2,
		Content: "hello",
	})

	var msg model.Message
	db.Where("from_qq = ? AND to_qq = ?", qq1, qq2).First(&msg)

	err := svc.RecallMessage(qq2, msg.ID)
	if err == nil {
		t.Fatal("non-sender should not be able to recall")
	}
}

func TestRecalledMessageNotInHistory(t *testing.T) {
	db := setupTestDB(t)
	svc := NewChatService(db, "")

	qq1, _ := svc.Register("alice", "password123")
	qq2, _ := svc.Register("bob", "password456")

	svc.HandleMessage(nil, &model.Message{
		MsgType: model.MsgTypeText,
		FromQQ:  qq1,
		ToQQ:    qq2,
		Content: "msg1",
	})
	svc.HandleMessage(nil, &model.Message{
		MsgType: model.MsgTypeText,
		FromQQ:  qq1,
		ToQQ:    qq2,
		Content: "msg2",
	})

	var msg1 model.Message
	db.Where("content = ?", "msg1").First(&msg1)

	svc.RecallMessage(qq1, msg1.ID)

	msgs, _, err := svc.GetHistoryWithTarget(qq1, qq2, 0, 30, "", "")
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message (recalled filtered), got %d", len(msgs))
	}
	if msgs[0].Content != "msg2" {
		t.Fatalf("expected msg2, got %s", msgs[0].Content)
	}
}

func TestLoginReturnsDualToken(t *testing.T) {
	db := setupTestDB(t)
	svc := NewChatService(db, "")

	qq, _ := svc.Register("alice", "password123")

	accessTok, refreshTok, err := svc.Login(qq, "password123")
	if err != nil {
		t.Fatalf("login failed: %v", err)
	}
	if accessTok == "" {
		t.Fatal("access token should not be empty")
	}
	if refreshTok == "" {
		t.Fatal("refresh token should not be empty")
	}

	parsedQQ, err := ValidateAccessToken(accessTok)
	if err != nil {
		t.Fatalf("validate access token failed: %v", err)
	}
	if parsedQQ != qq {
		t.Fatalf("expected qq %d, got %d", qq, parsedQQ)
	}
}

func TestLoginWithTokenJWT(t *testing.T) {
	db := setupTestDB(t)
	svc := NewChatService(db, "")

	qq, _ := svc.Register("alice", "password123")
	accessTok, _, _ := svc.Login(qq, "password123")

	valid, err := svc.LoginWithToken(qq, accessTok)
	if err != nil {
		t.Fatalf("login with jwt token failed: %v", err)
	}
	if !valid {
		t.Fatal("should be valid")
	}

	valid, err = svc.LoginWithToken(qq+1, accessTok)
	if err != nil || valid {
		t.Fatal("should be invalid for wrong qq")
	}

	valid, err = svc.LoginWithToken(qq, "old_sha256_token_abc123")
	if err == nil && valid {
		t.Fatal("old SHA256 token should fail")
	}
}

func TestRefreshToken(t *testing.T) {
	db := setupTestDB(t)
	svc := NewChatService(db, "")

	qq, _ := svc.Register("alice", "password123")
	_, refreshTok, _ := svc.Login(qq, "password123")

	newAccessTok, err := svc.RefreshToken(qq, refreshTok)
	if err != nil {
		t.Fatalf("refresh failed: %v", err)
	}
	if newAccessTok == "" {
		t.Fatal("new access token should not be empty")
	}

	parsedQQ, err := ValidateAccessToken(newAccessTok)
	if err != nil {
		t.Fatalf("validate new access token failed: %v", err)
	}
	if parsedQQ != qq {
		t.Fatalf("expected qq %d, got %d", qq, parsedQQ)
	}
}

func TestRefreshTokenInvalid(t *testing.T) {
	db := setupTestDB(t)
	svc := NewChatService(db, "")

	qq, _ := svc.Register("alice", "password123")

	_, err := svc.RefreshToken(qq, "wrong_refresh_token")
	if err != ErrInvalidToken {
		t.Fatalf("expected ErrInvalidToken, got: %v", err)
	}
}

func TestRefreshTokenExpired(t *testing.T) {
	db := setupTestDB(t)
	svc := NewChatService(db, "")

	qq, _ := svc.Register("alice", "password123")
	_, refreshTok, _ := svc.Login(qq, "password123")

	var user model.User
	db.Where("qq_number = ?", qq).First(&user)
	db.Model(&user).Update("updated_at", time.Now().Add(-8*24*time.Hour))

	_, err := svc.RefreshToken(qq, refreshTok)
	if err != ErrTokenExpired {
		t.Fatalf("expected ErrTokenExpired, got: %v", err)
	}
}

func TestClearRefreshToken(t *testing.T) {
	db := setupTestDB(t)
	svc := NewChatService(db, "")

	qq, _ := svc.Register("alice", "password123")
	_, refreshTok, _ := svc.Login(qq, "password123")

	if refreshTok == "" {
		t.Fatal("refresh token should exist")
	}

	svc.ClearRefreshToken(qq)

	var user model.User
	db.Where("qq_number = ?", qq).First(&user)
	if user.RefreshToken != "" {
		t.Fatal("refresh token should be cleared")
	}
}

func TestHistoryTimeRange(t *testing.T) {
	db := setupTestDB(t)
	svc := NewChatService(db, "")

	qq1, _ := svc.Register("alice", "password123")
	qq2, _ := svc.Register("bob", "password456")

	now := time.Now().UTC()
	msgs := []model.Message{
		{MsgType: 1, FromQQ: qq1, ToQQ: qq2, Content: "msg1", CreatedAt: now.Add(-48 * time.Hour)},
		{MsgType: 1, FromQQ: qq2, ToQQ: qq1, Content: "msg2", CreatedAt: now.Add(-24 * time.Hour)},
		{MsgType: 1, FromQQ: qq1, ToQQ: qq2, Content: "msg3", CreatedAt: now.Add(-1 * time.Hour)},
	}
	for _, m := range msgs {
		svc.db.Create(&m)
	}

	result, hasMore, err := svc.GetHistoryWithTarget(qq1, qq2, 0, 10, "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(result))
	}
	if hasMore {
		t.Fatal("should not have more")
	}

	fromTime := now.Add(-30 * time.Hour).Format("2006-01-02T15:04:05")
	result, _, err = svc.GetHistoryWithTarget(qq1, qq2, 0, 10, fromTime, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 messages with --from, got %d", len(result))
	}

	toTime := now.Add(-12 * time.Hour).Format("2006-01-02")
	result, _, err = svc.GetHistoryWithTarget(qq1, qq2, 0, 10, fromTime, toTime)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 message with --from/--to, got %d", len(result))
	}
	if result[0].Content != "msg2" {
		t.Fatalf("expected msg2, got %s", result[0].Content)
	}

	toTimeDate := now.Format("2006-01-02")
	result, _, err = svc.GetHistoryWithTarget(qq1, qq2, 0, 10, fromTime, toTimeDate)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 messages with date --to, got %d", len(result))
	}

	futureFrom := now.Add(24 * time.Hour).Format("2006-01-02")
	result, hasMore, err = svc.GetHistoryWithTarget(qq1, qq2, 0, 10, futureFrom, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Fatalf("expected 0 messages, got %d", len(result))
	}
	if hasMore {
		t.Fatal("should not have more when no results")
	}
}

func ftsAvailable(db *gorm.DB) bool {
	var count int64
	err := db.Raw("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='messages_fts'").Scan(&count).Error
	return err == nil && count > 0
}

func TestSearchMessages(t *testing.T) {
	db := setupTestDB(t)
	if !ftsAvailable(db) {
		t.Skip("FTS5 not available in test environment")
	}
	svc := NewChatService(db, "")

	qq1, _ := svc.Register("alice", "password123")
	qq2, _ := svc.Register("bob", "password456")
	qq3, _ := svc.Register("charlie", "password789")

	now := time.Now().UTC()
	msgs := []model.Message{
		{MsgType: 1, FromQQ: qq1, ToQQ: qq2, Content: "你好，项目进度怎么样了？", CreatedAt: now.Add(-2 * time.Hour)},
		{MsgType: 1, FromQQ: qq2, ToQQ: qq1, Content: "完成，项目已经80%了", CreatedAt: now.Add(-1 * time.Hour)},
		{MsgType: 1, FromQQ: qq1, ToQQ: qq2, Content: "太好了，下周上线", CreatedAt: now.Add(-30 * time.Minute)},
		{MsgType: 1, FromQQ: qq1, ToQQ: qq3, Content: "项目，新计划什么时候开始", CreatedAt: now.Add(-10 * time.Minute)},
	}
	for _, m := range msgs {
		svc.db.Create(&m)
	}

	time.Sleep(100 * time.Millisecond)

	resp, err := svc.SearchMessages(qq1, "项目", 0, "", 50)
	if err != nil {
		t.Fatalf("search error: %v", err)
	}
	if resp.Total < 3 {
		t.Fatalf("expected at least 3 results for '项目', got %d", resp.Total)
	}

	resp, err = svc.SearchMessages(qq1, "项目", qq2, "", 50)
	if err != nil {
		t.Fatalf("search error: %v", err)
	}
	if resp.Total < 2 {
		t.Fatalf("expected at least 2 results in private chat, got %d", resp.Total)
	}
	for _, r := range resp.Results {
		if r.ToQQ == qq3 || r.FromQQ == qq3 {
			t.Fatal("should not include messages with charlie in private search")
		}
	}

	resp, err = svc.SearchMessages(qq1, "不存在的关键词", 0, "", 50)
	if err != nil {
		t.Fatalf("search error: %v", err)
	}
	if resp.Total != 0 {
		t.Fatalf("expected 0 results, got %d", resp.Total)
	}

	resp, err = svc.SearchMessages(qq1, "完成", qq2, "", 50)
	if err != nil {
		t.Fatalf("search error: %v", err)
	}
	if resp.Total < 1 {
		t.Fatal("expected at least 1 result for '完成'")
	}
	firstResult := resp.Results[0]
	if firstResult.ContextBefore == nil {
		t.Fatal("expected context before")
	}
	if firstResult.ContextBefore.Content != "你好，项目进度怎么样了？" {
		t.Fatalf("unexpected context before: %s", firstResult.ContextBefore.Content)
	}
	if firstResult.ContextAfter == nil {
		t.Fatal("expected context after")
	}
	if firstResult.ContextAfter.Content != "太好了，下周上线" {
		t.Fatalf("unexpected context after: %s", firstResult.ContextAfter.Content)
	}
}

func TestSearchRecalledMessages(t *testing.T) {
	db := setupTestDB(t)
	if !ftsAvailable(db) {
		t.Skip("FTS5 not available in test environment")
	}
	svc := NewChatService(db, "")

	qq1, _ := svc.Register("alice", "password123")
	qq2, _ := svc.Register("bob", "password456")

	now := time.Now().UTC()
	msgs := []model.Message{
		{MsgType: 1, FromQQ: qq1, ToQQ: qq2, Content: "这条消息会被撤回", CreatedAt: now.Add(-1 * time.Hour)},
		{MsgType: 1, FromQQ: qq1, ToQQ: qq2, Content: "这条消息正常", CreatedAt: now},
	}
	for _, m := range msgs {
		svc.db.Create(&m)
	}

	svc.db.Model(&model.Message{}).Where("content = ?", "这条消息会被撤回").Update("is_recalled", true)

	time.Sleep(100 * time.Millisecond)

	resp, err := svc.SearchMessages(qq1, "撤回", 0, "", 50)
	if err != nil {
		t.Fatalf("search error: %v", err)
	}
	if resp.Total != 0 {
		t.Fatalf("recalled messages should not appear in search, got %d results", resp.Total)
	}
}

func TestGetGroupInfo(t *testing.T) {
	db := setupTestDB(t)
	svc := NewChatService(db, "")

	qq1, _ := svc.Register("alice", "pass123")

	groupID, err := svc.CreateGroup("test group", qq1)
	if err != nil {
		t.Fatalf("create group failed: %v", err)
	}

	info, err := svc.GetGroupInfo(groupID)
	if err != nil {
		t.Fatalf("get group info failed: %v", err)
	}
	if info.Name != "test group" {
		t.Fatalf("expected name 'test group', got '%s'", info.Name)
	}
	if info.OwnerQQ != qq1 {
		t.Fatalf("expected owner %d, got %d", qq1, info.OwnerQQ)
	}
	if info.MemberCnt != 1 {
		t.Fatalf("expected 1 member, got %d", info.MemberCnt)
	}

	_, err = svc.GetGroupInfo("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent group")
	}
}

func TestGetSessions(t *testing.T) {
	db := setupTestDB(t)
	svc := NewChatService(db, "")

	qq1, _ := svc.Register("alice", "pass123")
	qq2, _ := svc.Register("bob", "pass456")
	qq3, _ := svc.Register("charlie", "pass789")

	svc.HandleMessage(nil, &model.Message{FromQQ: qq1, ToQQ: qq2, Content: "hello bob", MsgType: model.MsgTypeText})
	time.Sleep(10 * time.Millisecond)
	svc.HandleMessage(nil, &model.Message{FromQQ: qq1, ToQQ: qq3, Content: "hello charlie", MsgType: model.MsgTypeText})

	online := func(qq int64) bool { return false }
	sessions, err := svc.GetSessions(qq1, online)
	if err != nil {
		t.Fatalf("get sessions failed: %v", err)
	}
	if len(sessions) < 2 {
		t.Fatalf("expected at least 2 sessions, got %d", len(sessions))
	}

	sessions2, err := svc.GetSessions(qq2, online)
	if err != nil {
		t.Fatalf("get sessions for bob failed: %v", err)
	}
	if len(sessions2) < 1 {
		t.Fatalf("bob should have at least 1 session, got %d", len(sessions2))
	}
}

func TestCleanMessages(t *testing.T) {
	db := setupTestDB(t)
	svc := NewChatService(db, "")

	qq1, _ := svc.Register("alice", "pass123")
	qq2, _ := svc.Register("bob", "pass456")

	oldTime := time.Now().AddDate(0, 0, -10)
	svc.db.Create(&model.Message{FromQQ: qq1, ToQQ: qq2, Content: "old msg", MsgType: model.MsgTypeText, CreatedAt: oldTime})
	svc.db.Create(&model.Message{FromQQ: qq1, ToQQ: qq2, Content: "new msg", MsgType: model.MsgTypeText})

	deleted, err := svc.CleanMessages(5)
	if err != nil {
		t.Fatalf("clean failed: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("expected 1 deleted, got %d", deleted)
	}

	var remaining int64
	svc.db.Model(&model.Message{}).Where("content IN ?", []string{"old msg", "new msg"}).Count(&remaining)
	if remaining != 1 {
		t.Fatalf("expected 1 remaining test message, got %d", remaining)
	}

	_, err = svc.CleanMessages(0)
	if err == nil {
		t.Fatal("expected error for days=0")
	}

	_, err = svc.CleanMessages(-1)
	if err == nil {
		t.Fatal("expected error for negative days")
	}
}

func TestCleanMessagesNoMatch(t *testing.T) {
	db := setupTestDB(t)
	svc := NewChatService(db, "")

	qq1, _ := svc.Register("alice", "pass123")
	qq2, _ := svc.Register("bob", "pass456")

	svc.db.Create(&model.Message{FromQQ: qq1, ToQQ: qq2, Content: "recent msg", MsgType: model.MsgTypeText})

	deleted, err := svc.CleanMessages(30)
	if err != nil {
		t.Fatalf("clean failed: %v", err)
	}
	if deleted != 0 {
		t.Fatalf("expected 0 deleted, got %d", deleted)
	}
}

func TestSearchMessagesBasic(t *testing.T) {
	db := setupTestDB(t)
	if !ftsAvailable(db) {
		t.Skip("FTS5 not available in test environment")
	}
	svc := NewChatService(db, "")

	qq1, _ := svc.Register("alice", "pass123")
	qq2, _ := svc.Register("bob", "pass456")

	svc.HandleMessage(nil, &model.Message{FromQQ: qq1, ToQQ: qq2, Content: "hello world", MsgType: model.MsgTypeText})
	svc.HandleMessage(nil, &model.Message{FromQQ: qq2, ToQQ: qq1, Content: "hi there", MsgType: model.MsgTypeText})
	svc.HandleMessage(nil, &model.Message{FromQQ: qq1, ToQQ: qq2, Content: "hello again", MsgType: model.MsgTypeText})

	resp, err := svc.SearchMessages(qq1, "hello", 0, "", 50)
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	if resp.Keyword != "hello" {
		t.Fatalf("expected keyword 'hello', got '%s'", resp.Keyword)
	}
	if resp.Total < 2 {
		t.Fatalf("expected at least 2 results for 'hello', got %d", resp.Total)
	}
}

func TestSearchMessagesWithTarget(t *testing.T) {
	db := setupTestDB(t)
	if !ftsAvailable(db) {
		t.Skip("FTS5 not available in test environment")
	}
	svc := NewChatService(db, "")

	qq1, _ := svc.Register("alice", "pass123")
	qq2, _ := svc.Register("bob", "pass456")
	qq3, _ := svc.Register("charlie", "pass789")

	svc.HandleMessage(nil, &model.Message{FromQQ: qq1, ToQQ: qq2, Content: "hello bob", MsgType: model.MsgTypeText})
	svc.HandleMessage(nil, &model.Message{FromQQ: qq1, ToQQ: qq3, Content: "hello charlie", MsgType: model.MsgTypeText})

	resp, err := svc.SearchMessages(qq1, "hello", qq2, "", 50)
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	if resp.Total < 1 {
		t.Fatalf("expected at least 1 result, got %d", resp.Total)
	}
	for _, r := range resp.Results {
		if r.ToQQ != qq2 && r.FromQQ != qq2 {
			t.Fatalf("result should involve qq2=%d, got from=%d to=%d", qq2, r.FromQQ, r.ToQQ)
		}
	}
}

func TestSearchMessagesInGroup(t *testing.T) {
	db := setupTestDB(t)
	if !ftsAvailable(db) {
		t.Skip("FTS5 not available in test environment")
	}
	svc := NewChatService(db, "")

	qq1, _ := svc.Register("alice", "pass123")
	groupID, _ := svc.CreateGroup("search group", qq1)

	svc.HandleMessage(nil, &model.Message{FromQQ: qq1, GroupID: groupID, Content: "group hello", MsgType: model.MsgTypeText})
	svc.HandleMessage(nil, &model.Message{FromQQ: qq1, GroupID: groupID, Content: "group world", MsgType: model.MsgTypeText})

	resp, err := svc.SearchMessages(qq1, "group", 0, groupID, 50)
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	if resp.Total < 1 {
		t.Fatalf("expected at least 1 result, got %d", resp.Total)
	}
}

func TestSearchMessagesLimit(t *testing.T) {
	db := setupTestDB(t)
	if !ftsAvailable(db) {
		t.Skip("FTS5 not available in test environment")
	}
	svc := NewChatService(db, "")

	qq1, _ := svc.Register("alice", "pass123")
	qq2, _ := svc.Register("bob", "pass456")

	for i := 0; i < 10; i++ {
		svc.HandleMessage(nil, &model.Message{FromQQ: qq1, ToQQ: qq2, Content: fmt.Sprintf("test message %d", i), MsgType: model.MsgTypeText})
	}

	resp, err := svc.SearchMessages(qq1, "test", 0, "", 3)
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	if resp.Total > 3 {
		t.Fatalf("expected at most 3 results, got %d", resp.Total)
	}
}

func TestSearchMessagesEmpty(t *testing.T) {
	db := setupTestDB(t)
	if !ftsAvailable(db) {
		t.Skip("FTS5 not available in test environment")
	}
	svc := NewChatService(db, "")

	qq1, _ := svc.Register("alice", "pass123")

	resp, err := svc.SearchMessages(qq1, "nonexistent", 0, "", 50)
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	if resp.Total != 0 {
		t.Fatalf("expected 0 results, got %d", resp.Total)
	}
}

func TestEscapeFTS5Keyword(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"hello", `"hello"*`},
		{`say "hi"`, `"say ""hi"""*`},
		{"", `""*`},
	}

	for _, tt := range tests {
		result := escapeFTS5Keyword(tt.input)
		if result != tt.expected {
			t.Fatalf("escapeFTS5Keyword(%q) = %q, expected %q", tt.input, result, tt.expected)
		}
	}
}

func TestParseTimeArg(t *testing.T) {
	tests := []struct {
		input string
		valid bool
	}{
		{"2026-01-15T10:30:00", true},
		{"2026-01-15 10:30:00", true},
		{"2026-01-15", true},
		{"invalid", false},
		{"", false},
	}

	for _, tt := range tests {
		_, err := parseTimeArg(tt.input)
		if tt.valid && err != nil {
			t.Fatalf("parseTimeArg(%q) should succeed, got: %v", tt.input, err)
		}
		if !tt.valid && err == nil {
			t.Fatalf("parseTimeArg(%q) should fail", tt.input)
		}
	}
}

func TestGetSessionsWithGroup(t *testing.T) {
	db := setupTestDB(t)
	svc := NewChatService(db, "")

	qq1, _ := svc.Register("alice", "pass123")
	qq2, _ := svc.Register("bob", "pass456")

	groupID, _ := svc.CreateGroup("session group", qq1)
	svc.JoinGroup(groupID, qq2)

	svc.HandleMessage(nil, &model.Message{FromQQ: qq1, GroupID: groupID, Content: "group msg", MsgType: model.MsgTypeText})

	online := func(qq int64) bool { return false }
	sessions, err := svc.GetSessions(qq1, online)
	if err != nil {
		t.Fatalf("get sessions failed: %v", err)
	}

	foundGroup := false
	for _, s := range sessions {
		if s.Type == "group" && s.GroupID == groupID {
			foundGroup = true
		}
	}
	if !foundGroup {
		t.Fatal("expected group session in session list")
	}
}

func TestGetSessionsEmpty(t *testing.T) {
	db := setupTestDB(t)
	svc := NewChatService(db, "")

	qq1, _ := svc.Register("alice", "pass123")

	online := func(qq int64) bool { return false }
	sessions, err := svc.GetSessions(qq1, online)
	if err != nil {
		t.Fatalf("get sessions failed: %v", err)
	}
	if len(sessions) != 0 {
		t.Fatalf("expected 0 sessions, got %d", len(sessions))
	}
}

func TestValidateToken(t *testing.T) {
	db := setupTestDB(t)
	svc := NewChatService(db, "")

	qq, _ := svc.Register("alice", "password123")
	accessTok, _, _ := svc.Login(qq, "password123")

	valid, err := svc.ValidateToken(qq, accessTok)
	if err != nil {
		t.Fatalf("validate token failed: %v", err)
	}
	if !valid {
		t.Fatal("should be valid")
	}

	valid, err = svc.ValidateToken(qq+1, accessTok)
	if err != nil || valid {
		t.Fatal("should be invalid for wrong qq")
	}

	valid, err = svc.ValidateToken(qq, "garbage")
	if err == nil && valid {
		t.Fatal("should fail for garbage token")
	}
}

func TestGetOfflineMessages(t *testing.T) {
	db := setupTestDB(t)
	svc := NewChatService(db, "")

	qq1, _ := svc.Register("alice", "password123")
	qq2, _ := svc.Register("bob", "password456")

	svc.HandleMessage(nil, &model.Message{MsgType: model.MsgTypeText, FromQQ: qq1, ToQQ: qq2, Content: "msg1"})
	svc.HandleMessage(nil, &model.Message{MsgType: model.MsgTypeText, FromQQ: qq1, ToQQ: qq2, Content: "msg2"})

	msgs, err := svc.GetOfflineMessages(qq2)
	if err != nil {
		t.Fatalf("get offline messages failed: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 offline messages, got %d", len(msgs))
	}
	if msgs[0].Content != "msg1" {
		t.Fatalf("expected msg1, got %s", msgs[0].Content)
	}

	msgs, err = svc.GetOfflineMessages(qq1)
	if err != nil {
		t.Fatalf("get offline messages for sender failed: %v", err)
	}
	if len(msgs) != 0 {
		t.Fatalf("sender should have 0 offline messages, got %d", len(msgs))
	}
}

func TestMarkDelivered(t *testing.T) {
	db := setupTestDB(t)
	svc := NewChatService(db, "")

	qq1, _ := svc.Register("alice", "password123")
	qq2, _ := svc.Register("bob", "password456")

	svc.HandleMessage(nil, &model.Message{MsgType: model.MsgTypeText, FromQQ: qq1, ToQQ: qq2, Content: "hello"})

	var msg model.Message
	db.Where("from_qq = ? AND to_qq = ?", qq1, qq2).First(&msg)
	if msg.Delivered {
		t.Fatal("message should not be delivered yet")
	}

	err := svc.MarkDelivered(msg.ID)
	if err != nil {
		t.Fatalf("mark delivered failed: %v", err)
	}

	db.Where("id = ?", msg.ID).First(&msg)
	if !msg.Delivered {
		t.Fatal("message should be marked as delivered")
	}

	err = svc.MarkDelivered(99999)
	if err != nil {
		t.Fatalf("mark delivered for nonexistent should not error: %v", err)
	}
}

func TestGetHistory(t *testing.T) {
	db := setupTestDB(t)
	svc := NewChatService(db, "")

	qq1, _ := svc.Register("alice", "password123")
	qq2, _ := svc.Register("bob", "password456")
	qq3, _ := svc.Register("charlie", "password789")

	svc.HandleMessage(nil, &model.Message{MsgType: model.MsgTypeText, FromQQ: qq1, ToQQ: qq2, Content: "to bob"})
	svc.HandleMessage(nil, &model.Message{MsgType: model.MsgTypeText, FromQQ: qq2, ToQQ: qq1, Content: "from bob"})
	svc.HandleMessage(nil, &model.Message{MsgType: model.MsgTypeText, FromQQ: qq1, ToQQ: qq3, Content: "to charlie"})

	msgs, err := svc.GetHistory(nil, qq1, 100)
	if err != nil {
		t.Fatalf("get history failed: %v", err)
	}
	if len(msgs) != 3 {
		t.Fatalf("expected 3 messages for qq1, got %d", len(msgs))
	}

	msgs, err = svc.GetHistory(nil, qq2, 100)
	if err != nil {
		t.Fatalf("get history for bob failed: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages for bob, got %d", len(msgs))
	}

	msgs, err = svc.GetHistory(nil, qq1, 2)
	if err != nil {
		t.Fatalf("get history with limit failed: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages with limit=2, got %d", len(msgs))
	}

	msgs, err = svc.GetHistory(nil, qq3, 100)
	if err != nil {
		t.Fatalf("get history for charlie failed: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message for charlie, got %d", len(msgs))
	}
}

func TestRejectFriend(t *testing.T) {
	db := setupTestDB(t)
	svc := NewChatService(db, "")

	qq1, _ := svc.Register("alice", "password123")
	qq2, _ := svc.Register("bob", "password456")

	err := svc.SendFriendRequest(qq1, qq2, "hello")
	if err != nil {
		t.Fatalf("send friend request failed: %v", err)
	}

	err = svc.RejectFriend(qq2, qq1)
	if err != nil {
		t.Fatalf("reject friend failed: %v", err)
	}

	var f model.Friend
	db.Where("qq = ? AND friend_qq = ?", qq1, qq2).First(&f)
	if f.Status != model.FriendStatusRejected {
		t.Fatalf("expected rejected status, got %d", f.Status)
	}

	if svc.IsFriend(qq1, qq2) {
		t.Fatal("should not be friends after rejection")
	}

	err = svc.RejectFriend(qq2, qq1)
	if err != ErrNotFriend {
		t.Fatalf("expected ErrNotFriend for already rejected, got: %v", err)
	}

	err = svc.RejectFriend(qq2, 99999)
	if err != ErrUserNotFound {
		t.Fatalf("expected ErrUserNotFound for nonexistent user, got: %v", err)
	}
}

func TestSearchUsers(t *testing.T) {
	db := setupTestDB(t)
	svc := NewChatService(db, "")

	qq1, _ := svc.Register("searchuser_unique_alpha", "password123")
	qq2, _ := svc.Register("searchuser_unique_beta", "password456")
	_, _ = svc.Register("searchuser_unique_gamma", "password789")

	online := func(qq int64) bool { return qq == qq1 }

	results, err := svc.SearchUsers("searchuser_unique", online)
	if err != nil {
		t.Fatalf("search users failed: %v", err)
	}
	if len(results) < 3 {
		t.Fatalf("expected at least 3 results, got %d", len(results))
	}

	foundOnline := false
	for _, r := range results {
		if r.QQNumber == qq1 && r.Online {
			foundOnline = true
		}
	}
	if !foundOnline {
		t.Fatal("qq1 should be online")
	}

	results, err = svc.SearchUsers(fmt.Sprintf("%d", qq2), online)
	if err != nil {
		t.Fatalf("search by qq number failed: %v", err)
	}
	found := false
	for _, r := range results {
		if r.QQNumber == qq2 {
			found = true
		}
	}
	if !found {
		t.Fatalf("should find user by qq number %d", qq2)
	}

	results, err = svc.SearchUsers("zzz_nonexistent_xyz_999", online)
	if err != nil {
		t.Fatalf("search for nonexistent failed: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected 0 results, got %d", len(results))
	}
}

func TestMoveFriendGroup(t *testing.T) {
	db := setupTestDB(t)
	svc := NewChatService(db, "")

	qq1, _ := svc.Register("alice", "password123")
	qq2, _ := svc.Register("bob", "password456")

	svc.SendFriendRequest(qq1, qq2, "hello")
	svc.AcceptFriend(qq2, qq1)

	err := svc.MoveFriendGroup(qq1, qq2, "我的好友")
	if err != nil {
		t.Fatalf("move to default group failed: %v", err)
	}

	err = svc.MoveFriendGroup(qq1, qq2, "同事")
	if err != ErrGroupNotFound {
		t.Fatalf("expected ErrGroupNotFound for nonexistent group, got: %v", err)
	}

	svc.CreateFriendGroup(qq1, "同事")

	err = svc.MoveFriendGroup(qq1, qq2, "同事")
	if err != nil {
		t.Fatalf("move to existing group failed: %v", err)
	}

	var f model.Friend
	db.Where("qq = ? AND friend_qq = ?", qq1, qq2).First(&f)
	if f.GroupName != "同事" {
		t.Fatalf("expected group '同事', got '%s'", f.GroupName)
	}

	err = svc.MoveFriendGroup(qq1, 99999, "同事")
	if err != ErrUserNotFound {
		t.Fatalf("expected ErrUserNotFound, got: %v", err)
	}

	err = svc.MoveFriendGroup(qq1, qq2+100, "同事")
	if err != ErrUserNotFound {
		t.Fatalf("expected ErrUserNotFound for nonexistent friend, got: %v", err)
	}
}

func TestGetFriendGroups(t *testing.T) {
	db := setupTestDB(t)
	svc := NewChatService(db, "")

	qq1, _ := svc.Register("alice", "password123")

	groups, err := svc.GetFriendGroups(qq1)
	if err != nil {
		t.Fatalf("get friend groups failed: %v", err)
	}
	if len(groups) != 1 || groups[0] != "我的好友" {
		t.Fatalf("expected ['我的好友'], got %v", groups)
	}

	svc.CreateFriendGroup(qq1, "同事")
	svc.CreateFriendGroup(qq1, "家人")

	groups, err = svc.GetFriendGroups(qq1)
	if err != nil {
		t.Fatalf("get friend groups failed: %v", err)
	}
	if len(groups) != 3 {
		t.Fatalf("expected 3 groups, got %d: %v", len(groups), groups)
	}

	qq2, _ := svc.Register("bob", "password456")
	svc.SendFriendRequest(qq1, qq2, "hi")
	svc.AcceptFriend(qq2, qq1)
	db.Model(&model.Friend{}).Where("qq = ? AND friend_qq = ?", qq1, qq2).Update("group_name", "同学")

	groups, err = svc.GetFriendGroups(qq1)
	if err != nil {
		t.Fatalf("get friend groups failed: %v", err)
	}
	foundTongxue := false
	for _, g := range groups {
		if g == "同学" {
			foundTongxue = true
		}
	}
	if !foundTongxue {
		t.Fatalf("expected '同学' from friend records, got %v", groups)
	}
}

func TestSetRemark(t *testing.T) {
	db := setupTestDB(t)
	svc := NewChatService(db, "")

	qq1, _ := svc.Register("alice", "password123")
	qq2, _ := svc.Register("bob", "password456")

	err := svc.SetRemark(qq1, qq2, "小鲍")
	if err != ErrNotFriend {
		t.Fatalf("expected ErrNotFriend, got: %v", err)
	}

	svc.SendFriendRequest(qq1, qq2, "hello")
	svc.AcceptFriend(qq2, qq1)

	err = svc.SetRemark(qq1, qq2, "小鲍")
	if err != nil {
		t.Fatalf("set remark failed: %v", err)
	}

	var f model.Friend
	db.Where("qq = ? AND friend_qq = ?", qq1, qq2).First(&f)
	if f.Remark != "小鲍" {
		t.Fatalf("expected remark '小鲍', got '%s'", f.Remark)
	}

	err = svc.SetRemark(qq1, 99999, "test")
	if err != ErrUserNotFound {
		t.Fatalf("expected ErrUserNotFound, got: %v", err)
	}
}

func TestCreateFriendGroup(t *testing.T) {
	db := setupTestDB(t)
	svc := NewChatService(db, "")

	qq1, _ := svc.Register("alice", "password123")

	err := svc.CreateFriendGroup(qq1, "同事")
	if err != nil {
		t.Fatalf("create friend group failed: %v", err)
	}

	err = svc.CreateFriendGroup(qq1, "同事")
	if err == nil {
		t.Fatal("duplicate group should fail")
	}

	err = svc.CreateFriendGroup(qq1, "")
	if err == nil {
		t.Fatal("empty name should fail")
	}

	err = svc.CreateFriendGroup(qq1, "待处理")
	if err == nil {
		t.Fatal("'待处理' should fail")
	}

	err = svc.CreateFriendGroup(qq1, "我的好友")
	if err == nil {
		t.Fatal("'我的好友' should fail")
	}
}

func TestDeleteFriendGroup(t *testing.T) {
	db := setupTestDB(t)
	svc := NewChatService(db, "")

	qq1, _ := svc.Register("alice", "password123")
	qq2, _ := svc.Register("bob", "password456")

	err := svc.DeleteFriendGroup(qq1, "我的好友")
	if err == nil {
		t.Fatal("cannot delete default group")
	}

	err = svc.DeleteFriendGroup(qq1, "nonexistent")
	if err != ErrGroupNotFound {
		t.Fatalf("expected ErrGroupNotFound, got: %v", err)
	}

	svc.CreateFriendGroup(qq1, "同事")

	err = svc.DeleteFriendGroup(qq1, "同事")
	if err != nil {
		t.Fatalf("delete empty group failed: %v", err)
	}

	svc.CreateFriendGroup(qq1, "家人")
	svc.SendFriendRequest(qq1, qq2, "hi")
	svc.AcceptFriend(qq2, qq1)
	svc.MoveFriendGroup(qq1, qq2, "家人")

	err = svc.DeleteFriendGroup(qq1, "家人")
	if err != ErrGroupNotEmpty {
		t.Fatalf("expected ErrGroupNotEmpty, got: %v", err)
	}
}

func TestBackupDB(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := tmpDir + "/test.db"

	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	db.AutoMigrate(&model.User{}, &model.Message{})
	InitJWT(config.JWTConfig{Secret: "test-secret", AccessTTL: 900, RefreshTTLDays: 7})

	svc := NewChatService(db, dbPath)

	svc.Register("alice", "password123")

	data, filename, err := svc.BackupDB()
	if err != nil {
		t.Fatalf("backup failed: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("backup data should not be empty")
	}
	if filename == "" {
		t.Fatal("backup filename should not be empty")
	}
	if len(filename) < 10 {
		t.Fatalf("unexpected filename: %s", filename)
	}
}

func TestBackupDBInvalidPath(t *testing.T) {
	db := setupTestDB(t)
	svc := NewChatService(db, "/nonexistent/path/db.sqlite")

	_, _, err := svc.BackupDB()
	if err == nil {
		t.Fatal("backup should fail for invalid path")
	}
}

func TestGetHistoryExcludesGroupMessages(t *testing.T) {
	db := setupTestDB(t)
	svc := NewChatService(db, "")

	qq1, _ := svc.Register("alice", "password123")
	qq2, _ := svc.Register("bob", "password456")

	svc.HandleMessage(nil, &model.Message{MsgType: model.MsgTypeText, FromQQ: qq1, ToQQ: qq2, Content: "private msg"})

	groupID, _ := svc.CreateGroup("test", qq1)
	svc.JoinGroup(groupID, qq2)
	svc.HandleMessage(nil, &model.Message{MsgType: model.MsgTypeText, FromQQ: qq1, GroupID: groupID, Content: "group msg"})

	msgs, err := svc.GetHistory(nil, qq1, 100)
	if err != nil {
		t.Fatalf("get history failed: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 private message, got %d", len(msgs))
	}
	if msgs[0].Content != "private msg" {
		t.Fatalf("expected 'private msg', got '%s'", msgs[0].Content)
	}
}

func TestRecallMessageExpired(t *testing.T) {
	db := setupTestDB(t)
	svc := NewChatService(db, "")

	qq1, _ := svc.Register("alice", "password123")
	qq2, _ := svc.Register("bob", "password456")

	svc.HandleMessage(nil, &model.Message{MsgType: model.MsgTypeText, FromQQ: qq1, ToQQ: qq2, Content: "old msg"})

	var msg model.Message
	db.Where("from_qq = ? AND to_qq = ?", qq1, qq2).First(&msg)
	db.Model(&msg).Update("created_at", time.Now().Add(-3*time.Minute))

	err := svc.RecallMessage(qq1, msg.ID)
	if err == nil {
		t.Fatal("should fail for message older than 2 minutes")
	}
}

func TestRecallMessageAlreadyRecalled(t *testing.T) {
	db := setupTestDB(t)
	svc := NewChatService(db, "")

	qq1, _ := svc.Register("alice", "password123")
	qq2, _ := svc.Register("bob", "password456")

	svc.HandleMessage(nil, &model.Message{MsgType: model.MsgTypeText, FromQQ: qq1, ToQQ: qq2, Content: "hello"})

	var msg model.Message
	db.Where("from_qq = ? AND to_qq = ?", qq1, qq2).First(&msg)

	svc.RecallMessage(qq1, msg.ID)

	err := svc.RecallMessage(qq1, msg.ID)
	if err == nil {
		t.Fatal("should fail for already recalled message")
	}
}

func TestRecallMessageNotFound(t *testing.T) {
	db := setupTestDB(t)
	svc := NewChatService(db, "")

	qq1, _ := svc.Register("alice", "password123")

	err := svc.RecallMessage(qq1, 99999)
	if err == nil {
		t.Fatal("should fail for nonexistent message")
	}
}

func TestSendFriendRequestSelf(t *testing.T) {
	db := setupTestDB(t)
	svc := NewChatService(db, "")

	qq1, _ := svc.Register("alice", "password123")

	err := svc.SendFriendRequest(qq1, qq1, "self")
	if err == nil {
		t.Fatal("should fail when adding self")
	}
}

func TestSendFriendRequestNonexistentUser(t *testing.T) {
	db := setupTestDB(t)
	svc := NewChatService(db, "")

	qq1, _ := svc.Register("alice", "password123")

	err := svc.SendFriendRequest(qq1, 99999, "hello")
	if err != ErrUserNotFound {
		t.Fatalf("expected ErrUserNotFound, got: %v", err)
	}
}

func TestDeleteFriendNonexistent(t *testing.T) {
	db := setupTestDB(t)
	svc := NewChatService(db, "")

	qq1, _ := svc.Register("alice", "password123")

	err := svc.DeleteFriend(qq1, 99999)
	if err != ErrUserNotFound {
		t.Fatalf("expected ErrUserNotFound, got: %v", err)
	}
}

func TestBlockUserNotFound(t *testing.T) {
	db := setupTestDB(t)
	svc := NewChatService(db, "")

	qq1, _ := svc.Register("alice", "password123")

	err := svc.BlockUser(qq1, 99999)
	if err != ErrUserNotFound {
		t.Fatalf("expected ErrUserNotFound, got: %v", err)
	}
}

func TestChangePasswordUserNotFound(t *testing.T) {
	db := setupTestDB(t)
	svc := NewChatService(db, "")

	_, _, err := svc.ChangePassword(99999, "old", "new")
	if err != ErrUserNotFound {
		t.Fatalf("expected ErrUserNotFound, got: %v", err)
	}
}

func TestJoinGroupFull(t *testing.T) {
	db := setupTestDB(t)
	svc := NewChatService(db, "")

	qq1, _ := svc.Register("alice", "password123")
	qq2, _ := svc.Register("bob", "password456")

	groupID, _ := svc.CreateGroup("small group", qq1)
	db.Model(&model.Group{}).Where("group_id = ?", groupID).Update("max_members", 1)

	err := svc.JoinGroup(groupID, qq2)
	if err == nil {
		t.Fatal("should fail when group is full")
	}
}

func TestJoinGroupNotFound(t *testing.T) {
	db := setupTestDB(t)
	svc := NewChatService(db, "")

	qq1, _ := svc.Register("alice", "password123")

	err := svc.JoinGroup("nonexistent", qq1)
	if err != ErrGroupNotFound {
		t.Fatalf("expected ErrGroupNotFound, got: %v", err)
	}
}

func TestLeaveGroupNotFound(t *testing.T) {
	db := setupTestDB(t)
	svc := NewChatService(db, "")

	qq1, _ := svc.Register("alice", "password123")

	err := svc.LeaveGroup("nonexistent", qq1)
	if err != ErrGroupNotFound {
		t.Fatalf("expected ErrGroupNotFound, got: %v", err)
	}
}

func TestLeaveGroupNotMember(t *testing.T) {
	db := setupTestDB(t)
	svc := NewChatService(db, "")

	qq1, _ := svc.Register("alice", "password123")
	qq2, _ := svc.Register("bob", "password456")

	groupID, _ := svc.CreateGroup("test", qq1)

	err := svc.LeaveGroup(groupID, qq2)
	if err == nil {
		t.Fatal("non-member should not be able to leave")
	}
}

func TestGetGroupListEmpty(t *testing.T) {
	db := setupTestDB(t)
	svc := NewChatService(db, "")

	qq1, _ := svc.Register("alice", "password123")

	groups, err := svc.GetGroupList(qq1)
	if err != nil {
		t.Fatalf("get group list failed: %v", err)
	}
	if len(groups) != 0 {
		t.Fatalf("expected 0 groups, got %d", len(groups))
	}
}

func TestSearchMessagesLimitCapping(t *testing.T) {
	db := setupTestDB(t)
	if !ftsAvailable(db) {
		t.Skip("FTS5 not available in test environment")
	}
	svc := NewChatService(db, "")

	qq1, _ := svc.Register("alice", "pass123")
	qq2, _ := svc.Register("bob", "pass456")

	for i := 0; i < 5; i++ {
		svc.HandleMessage(nil, &model.Message{FromQQ: qq1, ToQQ: qq2, Content: fmt.Sprintf("test msg %d", i), MsgType: model.MsgTypeText})
	}

	resp, err := svc.SearchMessages(qq1, "test", 0, "", 0)
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	if resp.Total > 50 {
		t.Fatalf("default limit should be 50, got %d results", resp.Total)
	}

	resp, err = svc.SearchMessages(qq1, "test", 0, "", 200)
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	if resp.Total > 100 {
		t.Fatalf("limit should be capped at 100, got %d results", resp.Total)
	}
}

func TestSearchMessagesGroupContext(t *testing.T) {
	db := setupTestDB(t)
	if !ftsAvailable(db) {
		t.Skip("FTS5 not available in test environment")
	}
	svc := NewChatService(db, "")

	qq1, _ := svc.Register("alice", "pass123")
	groupID, _ := svc.CreateGroup("ctx group", qq1)

	svc.HandleMessage(nil, &model.Message{FromQQ: qq1, GroupID: groupID, Content: "before message", MsgType: model.MsgTypeText})
	svc.HandleMessage(nil, &model.Message{FromQQ: qq1, GroupID: groupID, Content: "target keyword here", MsgType: model.MsgTypeText})
	svc.HandleMessage(nil, &model.Message{FromQQ: qq1, GroupID: groupID, Content: "after message", MsgType: model.MsgTypeText})

	time.Sleep(100 * time.Millisecond)

	resp, err := svc.SearchMessages(qq1, "keyword", 0, groupID, 50)
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	if resp.Total < 1 {
		t.Fatal("expected at least 1 result")
	}

	result := resp.Results[0]
	if result.ContextBefore == nil {
		t.Fatal("expected context before for group message")
	}
	if result.ContextBefore.Content != "before message" {
		t.Fatalf("unexpected context before: %s", result.ContextBefore.Content)
	}
	if result.ContextAfter == nil {
		t.Fatal("expected context after for group message")
	}
	if result.ContextAfter.Content != "after message" {
		t.Fatalf("unexpected context after: %s", result.ContextAfter.Content)
	}
}

func TestLoginUserNotFound(t *testing.T) {
	db := setupTestDB(t)
	svc := NewChatService(db, "")

	_, _, err := svc.Login(99999, "password")
	if err != ErrUserNotFound {
		t.Fatalf("expected ErrUserNotFound, got: %v", err)
	}
}

func TestLoginWrongPassword(t *testing.T) {
	db := setupTestDB(t)
	svc := NewChatService(db, "")

	qq, _ := svc.Register("alice", "password123")

	_, _, err := svc.Login(qq, "wrongpassword")
	if err != ErrAuthFailed {
		t.Fatalf("expected ErrAuthFailed, got: %v", err)
	}
}

func TestGetUserByQQNotFound(t *testing.T) {
	db := setupTestDB(t)
	svc := NewChatService(db, "")

	_, err := svc.GetUserByQQ(99999)
	if err == nil {
		t.Fatal("expected error for nonexistent user")
	}
}
