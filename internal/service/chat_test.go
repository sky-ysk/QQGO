package service

import (
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
