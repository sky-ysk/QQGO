package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/qqgo/server/internal/model"
	ws "github.com/qqgo/server/pkg/websocket"
)

type mockService struct{}

func (m *mockService) HandleMessage(ctx context.Context, msg *model.Message) error           { return nil }
func (m *mockService) ValidateToken(qq int64, token string) (bool, error)                    { return true, nil }
func (m *mockService) GetOfflineMessages(qq int64) ([]*model.Message, error)                 { return nil, nil }
func (m *mockService) MarkDelivered(messageID int64) error                                   { return nil }
func (m *mockService) Register(nickname, password string) (int64, error)                     { return 0, nil }
func (m *mockService) Login(qq int64, password string) (string, string, error)               { return "access", "refresh", nil }
func (m *mockService) LoginWithToken(qq int64, token string) (bool, error)                   { return true, nil }
func (m *mockService) RefreshToken(qq int64, refreshToken string) (string, error)            { return "new-access", nil }
func (m *mockService) ClearRefreshToken(qq int64) error                                      { return nil }
func (m *mockService) SendFriendRequest(fromQQ, toQQ int64, message string) error            { return nil }
func (m *mockService) AcceptFriend(qq, fromQQ int64) error                                   { return nil }
func (m *mockService) RejectFriend(qq, fromQQ int64) error                                   { return nil }
func (m *mockService) DeleteFriend(qq, friendQQ int64) error                                 { return nil }
func (m *mockService) GetFriendList(qq int64, onlineFunc func(int64) bool) ([]model.FriendInfo, error) {
	return nil, nil
}
func (m *mockService) SearchUsers(keyword string, onlineFunc func(int64) bool) ([]model.UserSearchResult, error) {
	return nil, nil
}
func (m *mockService) MoveFriendGroup(qq, friendQQ int64, groupName string) error            { return nil }
func (m *mockService) GetFriendGroups(qq int64) ([]string, error)                            { return nil, nil }
func (m *mockService) SetRemark(qq, friendQQ int64, remark string) error                     { return nil }
func (m *mockService) CreateFriendGroup(qq int64, name string) error                         { return nil }
func (m *mockService) DeleteFriendGroup(qq int64, name string) error                         { return nil }
func (m *mockService) GetUserByQQ(qq int64) (*model.User, error)                             { return nil, nil }
func (m *mockService) IsFriend(qq1, qq2 int64) bool                                         { return false }
func (m *mockService) CheckAndIncrementNonFriendMessage(fromQQ, toQQ int64) error            { return nil }
func (m *mockService) GetHistoryWithTarget(myQQ, targetQQ int64, offset, limit int) ([]*model.Message, bool, error) {
	return nil, false, nil
}
func (m *mockService) CreateGroup(name string, ownerQQ int64) (string, error)                { return "", nil }
func (m *mockService) JoinGroup(groupID string, qq int64) error                              { return nil }
func (m *mockService) LeaveGroup(groupID string, qq int64) error                             { return nil }
func (m *mockService) GetGroupMembers(groupID string) ([]int64, error)                       { return nil, nil }
func (m *mockService) GetGroupList(qq int64) ([]model.GroupInfo, error)                      { return nil, nil }
func (m *mockService) GetGroupInfo(groupID string) (*model.GroupInfo, error)                 { return nil, nil }
func (m *mockService) IsGroupMember(groupID string, qq int64) bool                           { return false }
func (m *mockService) GetSessions(qq int64, onlineFunc func(int64) bool) ([]model.SessionInfo, error) {
	return nil, nil
}
func (m *mockService) GetGroupHistory(groupID string, offset, limit int) ([]*model.Message, bool, error) {
	return nil, false, nil
}
func (m *mockService) ChangePassword(qq int64, oldPw, newPw string) (string, string, error)  { return "access", "refresh", nil }
func (m *mockService) BlockUser(qq, blockedQQ int64) error                                   { return nil }
func (m *mockService) UnblockUser(qq, blockedQQ int64) error                                 { return nil }
func (m *mockService) IsBlocked(qq, blockedQQ int64) bool                                    { return false }
func (m *mockService) GetBlacklist(qq int64) ([]model.BlockedUserInfo, error)                { return nil, nil }
func (m *mockService) MarkRead(messageID int64) error                                        { return nil }
func (m *mockService) RecallMessage(qq, messageID int64) error                               { return nil }

func TestConnectionLimit(t *testing.T) {
	svc := &mockService{}
	hub := NewHub(svc, nil, 2, nil)

	hub.conns[10001] = &ws.Conn{}
	hub.conns[10002] = &ws.Conn{}

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
