package service

import (
	"fmt"
	"testing"

	"github.com/qqgo/server/internal/model"
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

	return db
}

func TestNonFriendMessageLimit(t *testing.T) {
	db := setupTestDB(t)
	svc := NewChatService(db)

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
	svc := NewChatService(db)

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
	svc := NewChatService(db)

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
	svc := NewChatService(db)

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

	msgs, hasMore, err := svc.GetHistoryWithTarget(qq1, qq2, 0, 30)
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if len(msgs) != 10 {
		t.Fatalf("expected 10 messages, got %d", len(msgs))
	}
	if hasMore {
		t.Fatal("should not have more")
	}

	msgs, hasMore, err = svc.GetHistoryWithTarget(qq1, qq2, 0, 5)
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if len(msgs) != 5 {
		t.Fatalf("expected 5 messages, got %d", len(msgs))
	}
	if !hasMore {
		t.Fatal("should have more")
	}

	msgs, hasMore, err = svc.GetHistoryWithTarget(qq1, qq2, 5, 5)
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if len(msgs) != 5 {
		t.Fatalf("expected 5 messages on page 2, got %d", len(msgs))
	}
	if hasMore {
		t.Fatal("should not have more on page 2")
	}

	msgs, _, err = svc.GetHistoryWithTarget(qq1, qq2, 100, 30)
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if len(msgs) != 0 {
		t.Fatalf("expected 0 messages for out of range offset, got %d", len(msgs))
	}
}

func TestGroupChat(t *testing.T) {
	db := setupTestDB(t)
	svc := NewChatService(db)

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
	svc := NewChatService(db)

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
	svc := NewChatService(db)

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
	svc := NewChatService(db)

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
	svc := NewChatService(db)

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
	svc := NewChatService(db)

	qq, err := svc.Register("alice", "oldpassword")
	if err != nil {
		t.Fatalf("register failed: %v", err)
	}

	newToken, err := svc.ChangePassword(qq, "oldpassword", "newpassword")
	if err != nil {
		t.Fatalf("change password failed: %v", err)
	}
	if newToken == "" {
		t.Fatal("new token should not be empty")
	}

	_, err = svc.Login(qq, "newpassword")
	if err != nil {
		t.Fatalf("login with new password should succeed: %v", err)
	}

	_, err = svc.Login(qq, "oldpassword")
	if err != ErrAuthFailed {
		t.Fatalf("login with old password should fail, got: %v", err)
	}
}

func TestChangePasswordWrongOld(t *testing.T) {
	db := setupTestDB(t)
	svc := NewChatService(db)

	qq, _ := svc.Register("bob", "correctpassword")

	_, err := svc.ChangePassword(qq, "wrongpassword", "newpassword")
	if err != ErrAuthFailed {
		t.Fatalf("should fail with wrong old password, got: %v", err)
	}
}

func TestBlockUser(t *testing.T) {
	db := setupTestDB(t)
	svc := NewChatService(db)

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
	svc := NewChatService(db)

	qq, _ := svc.Register("selfblock", "password123")

	err := svc.BlockUser(qq, qq)
	if err == nil {
		t.Fatal("blocking self should fail")
	}
}

func TestUnblockUser(t *testing.T) {
	db := setupTestDB(t)
	svc := NewChatService(db)

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
	svc := NewChatService(db)

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
	svc := NewChatService(db)

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
	svc := NewChatService(db)

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
	svc := NewChatService(db)

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
	svc := NewChatService(db)

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

	msgs, _, err := svc.GetHistoryWithTarget(qq1, qq2, 0, 30)
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
