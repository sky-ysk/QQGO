package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/qqgo/server/internal/model"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

var (
	ErrUserExists           = errors.New("user already exists")
	ErrUserNotFound         = errors.New("user not found")
	ErrAuthFailed           = errors.New("auth failed")
	ErrTokenInvalid         = errors.New("invalid token")
	ErrFriendLimit          = errors.New("friend limit reached")
	ErrAlreadyFriend        = errors.New("already friend or pending")
	ErrNotFriend            = errors.New("not friend")
	ErrGroupNotFound        = errors.New("group not found")
	ErrGroupNotEmpty        = errors.New("group is not empty")
	ErrNonFriendMsgLimit    = errors.New("not friend, only 1 message allowed")
)

type ChatService struct {
	db *gorm.DB
}

func NewChatService(db *gorm.DB) *ChatService {
	return &ChatService{db: db}
}

func (s *ChatService) Register(nickname, password string) (int64, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return 0, err
	}

	token := generateToken()

	user := &model.User{
		PasswordHash: string(hash),
		Token:        token,
		Nickname:     nickname,
	}
	if err := s.db.Create(user).Error; err != nil {
		return 0, err
	}

	qqNumber := int64(model.QQNumberBase) + int64(user.ID)
	s.db.Model(user).Updates(map[string]interface{}{
		"qq_number": qqNumber,
		"token":     token,
	})

	return qqNumber, nil
}

func (s *ChatService) Login(qq int64, password string) (string, error) {
	var user model.User
	if err := s.db.Where("qq_number = ?", qq).First(&user).Error; err != nil {
		return "", ErrUserNotFound
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return "", ErrAuthFailed
	}

	token := generateToken()
	s.db.Model(&user).Update("token", token)

	return token, nil
}

func (s *ChatService) LoginWithToken(qq int64, token string) (bool, error) {
	var user model.User
	if err := s.db.Where("qq_number = ? AND token = ?", qq, token).First(&user).Error; err != nil {
		return false, nil
	}
	return true, nil
}

func (s *ChatService) ValidateToken(qq int64, token string) (bool, error) {
	return s.LoginWithToken(qq, token)
}

func (s *ChatService) HandleMessage(ctx context.Context, msg *model.Message) error {
	result := s.db.Create(msg)
	if result.Error != nil {
		return result.Error
	}
	log.Printf("[store] saved message id=%d from=%d to=%d", msg.ID, msg.FromQQ, msg.ToQQ)
	return nil
}

func (s *ChatService) GetOfflineMessages(qq int64) ([]*model.Message, error) {
	var msgs []*model.Message
	err := s.db.
		Where("to_qq = ? AND delivered = ?", qq, false).
		Order("id asc").
		Find(&msgs).Error
	return msgs, err
}

func (s *ChatService) MarkDelivered(messageID int64) error {
	return s.db.Model(&model.Message{}).Where("id = ?", messageID).Update("delivered", true).Error
}

func (s *ChatService) GetHistory(ctx context.Context, qq int64, limit int) ([]*model.Message, error) {
	var msgs []*model.Message
	err := s.db.
		Where("(from_qq = ? OR to_qq = ?) AND group_id = ''", qq, qq).
		Order("id desc").
		Limit(limit).
		Find(&msgs).Error
	return msgs, err
}

func (s *ChatService) GetUserByQQ(qq int64) (*model.User, error) {
	var user model.User
	err := s.db.Where("qq_number = ?", qq).First(&user).Error
	return &user, err
}

func (s *ChatService) SendFriendRequest(fromQQ int64, toQQ int64, message string) error {
	if _, err := s.GetUserByQQ(toQQ); err != nil {
		return ErrUserNotFound
	}

	var self model.User
	if err := s.db.Where("qq_number = ?", fromQQ).First(&self).Error; err != nil {
		return ErrUserNotFound
	}

	if self.QQNumber == toQQ {
		return errors.New("cannot add yourself")
	}

	var count int64
	s.db.Model(&model.Friend{}).
		Where("qq = ? AND status = ?", fromQQ, model.FriendStatusAccepted).
		Count(&count)
	if count >= model.MaxFriends {
		return ErrFriendLimit
	}

	var existing model.Friend
	err := s.db.Where(
		"(qq = ? AND friend_qq = ?) OR (qq = ? AND friend_qq = ?)",
		fromQQ, toQQ, toQQ, fromQQ,
	).First(&existing).Error
	if err == nil {
		return ErrAlreadyFriend
	}

	f := &model.Friend{
		QQ:       fromQQ,
		FriendQQ: toQQ,
		Status:   model.FriendStatusPending,
	}
	return s.db.Create(f).Error
}

func (s *ChatService) AcceptFriend(qq int64, fromQQ int64) error {
	fromUser, err := s.GetUserByQQ(fromQQ)
	if err != nil {
		return ErrUserNotFound
	}

	result := s.db.Model(&model.Friend{}).
		Where("qq = ? AND friend_qq = ? AND status = ?", fromUser.QQNumber, qq, model.FriendStatusPending).
		Update("status", model.FriendStatusAccepted)
	if result.RowsAffected == 0 {
		return ErrNotFriend
	}

	var selfCount int64
	s.db.Model(&model.Friend{}).
		Where("qq = ? AND status = ?", qq, model.FriendStatusAccepted).
		Count(&selfCount)
	if selfCount >= model.MaxFriends {
		s.db.Model(&model.Friend{}).
			Where("qq = ? AND friend_qq = ?", fromUser.QQNumber, qq).
			Update("status", model.FriendStatusPending)
		return ErrFriendLimit
	}

	var targetCount int64
	s.db.Model(&model.Friend{}).
		Where("qq = ? AND status = ?", fromUser.QQNumber, model.FriendStatusAccepted).
		Count(&targetCount)
	if targetCount >= model.MaxFriends {
		s.db.Model(&model.Friend{}).
			Where("qq = ? AND friend_qq = ?", fromUser.QQNumber, qq).
			Update("status", model.FriendStatusPending)
		return ErrFriendLimit
	}

	reverse := &model.Friend{
		QQ:       qq,
		FriendQQ: fromUser.QQNumber,
		Status:   model.FriendStatusAccepted,
	}
	s.db.Create(reverse)

	s.ClearMessageCounts(qq, fromUser.QQNumber)

	return nil
}

func (s *ChatService) RejectFriend(qq int64, fromQQ int64) error {
	fromUser, err := s.GetUserByQQ(fromQQ)
	if err != nil {
		return ErrUserNotFound
	}

	result := s.db.Model(&model.Friend{}).
		Where("qq = ? AND friend_qq = ? AND status = ?", fromUser.QQNumber, qq, model.FriendStatusPending).
		Update("status", model.FriendStatusRejected)
	if result.RowsAffected == 0 {
		return ErrNotFriend
	}
	return nil
}

func (s *ChatService) DeleteFriend(qq int64, friendQQ int64) error {
	if _, err := s.GetUserByQQ(friendQQ); err != nil {
		return ErrUserNotFound
	}

	s.db.Where("qq = ? AND friend_qq = ? AND status = ?", qq, friendQQ, model.FriendStatusAccepted).Delete(&model.Friend{})
	s.db.Where("qq = ? AND friend_qq = ? AND status = ?", friendQQ, qq, model.FriendStatusAccepted).Delete(&model.Friend{})

	return nil
}

func (s *ChatService) GetFriendList(qq int64, onlineFunc func(int64) bool) ([]model.FriendInfo, error) {
	var friends []model.Friend
	err := s.db.Where("qq = ? AND status = ?", qq, model.FriendStatusAccepted).Find(&friends).Error
	if err != nil {
		return nil, err
	}

	var pending []model.Friend
	s.db.Where("friend_qq = ? AND status = ?", qq, model.FriendStatusPending).Find(&pending)

	result := make([]model.FriendInfo, 0, len(friends)+len(pending))

	for _, f := range pending {
		reqUser, err := s.GetUserByQQ(f.QQ)
		if err != nil {
			continue
		}
		result = append(result, model.FriendInfo{
			QQNumber:  reqUser.QQNumber,
			Nickname:  reqUser.Nickname,
			Remark:    f.Remark,
			GroupName: "待处理",
			Status:    model.FriendStatusPending,
			Online:    onlineFunc(reqUser.QQNumber),
		})
	}

	for _, f := range friends {
		friendUser, err := s.GetUserByQQ(f.FriendQQ)
		if err != nil {
			continue
		}
		groupName := f.GroupName
		if groupName == "" {
			groupName = "我的好友"
		}
		result = append(result, model.FriendInfo{
			QQNumber:  friendUser.QQNumber,
			Nickname:  friendUser.Nickname,
			Remark:    f.Remark,
			GroupName: groupName,
			Status:    f.Status,
			Online:    onlineFunc(friendUser.QQNumber),
		})
	}

	return result, nil
}

func (s *ChatService) SearchUsers(keyword string, onlineFunc func(int64) bool) ([]model.UserSearchResult, error) {
	var users []model.User

	if qqNum, err := strconv.ParseInt(keyword, 10, 64); err == nil {
		s.db.Where("qq_number = ? OR nickname LIKE ?",
			qqNum, "%"+keyword+"%").
			Limit(20).Find(&users)
	} else {
		s.db.Where("nickname LIKE ?",
			"%"+keyword+"%").
			Limit(20).Find(&users)
	}

	results := make([]model.UserSearchResult, 0, len(users))
	for _, u := range users {
		results = append(results, model.UserSearchResult{
			QQNumber: u.QQNumber,
			Nickname: u.Nickname,
			Online:   onlineFunc(u.QQNumber),
		})
	}
	return results, nil
}

func (s *ChatService) MoveFriendGroup(qq int64, friendQQ int64, groupName string) error {
	friendUser, err := s.GetUserByQQ(friendQQ)
	if err != nil {
		return ErrUserNotFound
	}

	if groupName != "我的好友" {
		var count int64
		s.db.Model(&model.FriendGroup{}).Where("qq = ? AND group_name = ?", qq, groupName).Count(&count)
		if count == 0 {
			return ErrGroupNotFound
		}
	}

	result := s.db.Model(&model.Friend{}).
		Where("qq = ? AND friend_qq = ? AND status = ?", qq, friendUser.QQNumber, model.FriendStatusAccepted).
		Update("group_name", groupName)
	if result.RowsAffected == 0 {
		return ErrNotFriend
	}
	return nil
}

func (s *ChatService) GetFriendGroups(qq int64) ([]string, error) {
	var friendGroups []string
	if err := s.db.Model(&model.FriendGroup{}).Where("qq = ?", qq).Pluck("group_name", &friendGroups).Error; err != nil {
		return nil, err
	}

	var friendGroupNames []string
	if err := s.db.Model(&model.Friend{}).
		Where("qq = ? AND status = ?", qq, model.FriendStatusAccepted).
		Distinct("group_name").
		Pluck("group_name", &friendGroupNames).Error; err != nil {
		return nil, err
	}

	seen := make(map[string]bool)
	seen["我的好友"] = true
	result := []string{"我的好友"}

	for _, g := range friendGroups {
		if !seen[g] {
			seen[g] = true
			result = append(result, g)
		}
	}
	for _, g := range friendGroupNames {
		if !seen[g] && g != "" {
			seen[g] = true
			result = append(result, g)
		}
	}

	return result, nil
}

func (s *ChatService) SetRemark(qq int64, friendQQ int64, remark string) error {
	if _, err := s.GetUserByQQ(friendQQ); err != nil {
		return ErrUserNotFound
	}

	result := s.db.Model(&model.Friend{}).
		Where("qq = ? AND friend_qq = ? AND status = ?", qq, friendQQ, model.FriendStatusAccepted).
		Update("remark", remark)
	if result.RowsAffected == 0 {
		return ErrNotFriend
	}
	return nil
}

func (s *ChatService) CreateFriendGroup(qq int64, name string) error {
	if name == "" || name == "待处理" || name == "我的好友" {
		return errors.New("invalid group name")
	}
	fg := &model.FriendGroup{QQ: qq, GroupName: name}
	err := s.db.Create(fg).Error
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") || strings.Contains(err.Error(), "Duplicate") {
			return errors.New("group already exists")
		}
		return err
	}
	return nil
}

func (s *ChatService) DeleteFriendGroup(qq int64, name string) error {
	if name == "我的好友" {
		return errors.New("cannot delete default group")
	}
	var count int64
	if err := s.db.Model(&model.Friend{}).Where("qq = ? AND group_name = ? AND status = ?", qq, name, model.FriendStatusAccepted).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return ErrGroupNotEmpty
	}
	result := s.db.Where("qq = ? AND group_name = ?", qq, name).Delete(&model.FriendGroup{})
	if result.RowsAffected == 0 {
		return ErrGroupNotFound
	}
	return nil
}

func (s *ChatService) IsFriend(qq1 int64, qq2 int64) bool {
	var count int64
	s.db.Model(&model.Friend{}).
		Where("qq = ? AND friend_qq = ? AND status = ?", qq1, qq2, model.FriendStatusAccepted).
		Count(&count)
	return count > 0
}

func (s *ChatService) CheckAndIncrementNonFriendMessage(fromQQ int64, toQQ int64) error {
	if s.IsFriend(fromQQ, toQQ) {
		return nil
	}

	var mc model.MessageCount
	err := s.db.Where("from_qq = ? AND to_qq = ?", fromQQ, toQQ).First(&mc).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			s.db.Create(&model.MessageCount{FromQQ: fromQQ, ToQQ: toQQ, Count: 1})
			return nil
		}
		return err
	}

	if mc.Count >= model.MaxNonFriendMessages {
		return ErrNonFriendMsgLimit
	}

	s.db.Model(&mc).Update("count", mc.Count+1)
	return nil
}

func (s *ChatService) ClearMessageCounts(qq1 int64, qq2 int64) {
	s.db.Where("from_qq = ? AND to_qq = ?", qq1, qq2).Delete(&model.MessageCount{})
	s.db.Where("from_qq = ? AND to_qq = ?", qq2, qq1).Delete(&model.MessageCount{})
}

func (s *ChatService) GetHistoryWithTarget(myQQ int64, targetQQ int64, offset int, limit int) ([]*model.Message, bool, error) {
	var msgs []*model.Message
	query := s.db.Where(
		"((from_qq = ? AND to_qq = ?) OR (from_qq = ? AND to_qq = ?)) AND group_id = ''",
		myQQ, targetQQ, targetQQ, myQQ,
	).Where("msg_type IN ?", []int{1, 2, 3})

	var total int64
	query.Model(&model.Message{}).Count(&total)

	err := query.
		Order("id asc").
		Offset(offset).
		Limit(limit).
		Find(&msgs).Error
	if err != nil {
		return nil, false, err
	}

	hasMore := int64(offset+limit) < total
	return msgs, hasMore, nil
}

func (s *ChatService) CreateGroup(name string, ownerQQ int64) (string, error) {
	groupID := fmt.Sprintf("G%d", time.Now().UnixNano())

	group := &model.Group{
		GroupID:   groupID,
		Name:      name,
		OwnerQQ:   ownerQQ,
		MemberCnt: 1,
	}
	if err := s.db.Create(group).Error; err != nil {
		return "", err
	}

	member := &model.GroupMember{
		GroupID: groupID,
		QQ:      ownerQQ,
		Role:    1,
	}
	s.db.Create(member)

	return groupID, nil
}

func (s *ChatService) JoinGroup(groupID string, qq int64) error {
	var group model.Group
	if err := s.db.Where("group_id = ?", groupID).First(&group).Error; err != nil {
		return ErrGroupNotFound
	}

	var existing model.GroupMember
	err := s.db.Where("group_id = ? AND qq = ?", groupID, qq).First(&existing).Error
	if err == nil {
		return errors.New("already in group")
	}

	if group.MemberCnt >= group.MaxMembers {
		return errors.New("group is full")
	}

	member := &model.GroupMember{
		GroupID: groupID,
		QQ:      qq,
	}
	if err := s.db.Create(member).Error; err != nil {
		return err
	}

	s.db.Model(&model.Group{}).Where("group_id = ?", groupID).Update("member_cnt", group.MemberCnt+1)
	return nil
}

func (s *ChatService) LeaveGroup(groupID string, qq int64) error {
	var group model.Group
	if err := s.db.Where("group_id = ?", groupID).First(&group).Error; err != nil {
		return ErrGroupNotFound
	}

	if group.OwnerQQ == qq {
		return errors.New("owner cannot leave group, transfer ownership first")
	}

	result := s.db.Where("group_id = ? AND qq = ?", groupID, qq).Delete(&model.GroupMember{})
	if result.RowsAffected == 0 {
		return errors.New("not in group")
	}

	s.db.Model(&model.Group{}).Where("group_id = ?", groupID).Update("member_cnt", gorm.Expr("GREATEST(member_cnt - 1, 0)"))
	return nil
}

func (s *ChatService) GetGroupMembers(groupID string) ([]int64, error) {
	var members []model.GroupMember
	if err := s.db.Where("group_id = ?", groupID).Find(&members).Error; err != nil {
		return nil, err
	}

	qqs := make([]int64, 0, len(members))
	for _, m := range members {
		qqs = append(qqs, m.QQ)
	}
	return qqs, nil
}

func (s *ChatService) GetGroupList(qq int64) ([]model.GroupInfo, error) {
	var memberGroups []model.GroupMember
	s.db.Where("qq = ?", qq).Find(&memberGroups)

	groupIDs := make([]string, 0, len(memberGroups))
	for _, m := range memberGroups {
		groupIDs = append(groupIDs, m.GroupID)
	}

	if len(groupIDs) == 0 {
		return []model.GroupInfo{}, nil
	}

	var groups []model.Group
	s.db.Where("group_id IN ?", groupIDs).Find(&groups)

	results := make([]model.GroupInfo, 0, len(groups))
	for _, g := range groups {
		results = append(results, model.GroupInfo{
			GroupID:   g.GroupID,
			Name:      g.Name,
			OwnerQQ:   g.OwnerQQ,
			MemberCnt: g.MemberCnt,
		})
	}
	return results, nil
}

func (s *ChatService) GetGroupInfo(groupID string) (*model.GroupInfo, error) {
	var group model.Group
	if err := s.db.Where("group_id = ?", groupID).First(&group).Error; err != nil {
		return nil, ErrGroupNotFound
	}

	return &model.GroupInfo{
		GroupID:   group.GroupID,
		Name:      group.Name,
		OwnerQQ:   group.OwnerQQ,
		MemberCnt: group.MemberCnt,
	}, nil
}

func (s *ChatService) IsGroupMember(groupID string, qq int64) bool {
	var count int64
	s.db.Model(&model.GroupMember{}).Where("group_id = ? AND qq = ?", groupID, qq).Count(&count)
	return count > 0
}

func (s *ChatService) GetSessions(qq int64, onlineFunc func(int64) bool) ([]model.SessionInfo, error) {
	var sessions []model.SessionInfo
	seen := make(map[string]bool)

	type contactResult struct {
		QQ      int64
		Content string
		Time    time.Time
	}

	var sentContacts []contactResult
	s.db.Raw(`
		SELECT to_qq as qq, content, created_at as time
		FROM messages m1
		WHERE from_qq = ? AND group_id = ''
		AND created_at = (
			SELECT MAX(created_at) FROM messages m2
			WHERE m2.from_qq = ? AND m2.to_qq = m1.to_qq AND m2.group_id = ''
		)
	`, qq, qq).Scan(&sentContacts)

	var receivedContacts []contactResult
	s.db.Raw(`
		SELECT from_qq as qq, content, created_at as time
		FROM messages m1
		WHERE to_qq = ? AND group_id = ''
		AND created_at = (
			SELECT MAX(created_at) FROM messages m2
			WHERE m2.to_qq = ? AND m2.from_qq = m1.from_qq AND m2.group_id = ''
		)
	`, qq, qq).Scan(&receivedContacts)

	contactMap := make(map[int64]contactResult)
	for _, c := range sentContacts {
		contactMap[c.QQ] = c
	}
	for _, c := range receivedContacts {
		if existing, ok := contactMap[c.QQ]; !ok || c.Time.After(existing.Time) {
			contactMap[c.QQ] = c
		}
	}

	for contactQQ, c := range contactMap {
		if contactQQ == qq {
			continue
		}
		key := fmt.Sprintf("p_%d", contactQQ)
		seen[key] = true

		user, err := s.GetUserByQQ(contactQQ)
		nickname := fmt.Sprintf("%d", contactQQ)
		if err == nil && user != nil {
			nickname = user.Nickname
		}

		sessions = append(sessions, model.SessionInfo{
			Type:        "private",
			TargetQQ:    contactQQ,
			Nickname:    nickname,
			LastMessage: c.Content,
			LastTime:    c.Time,
			Online:      onlineFunc(contactQQ),
		})
	}

	var memberGroups []model.GroupMember
	s.db.Where("qq = ?", qq).Find(&memberGroups)

	for _, mg := range memberGroups {
		key := fmt.Sprintf("g_%s", mg.GroupID)
		if seen[key] {
			continue
		}
		seen[key] = true

		var group model.Group
		if err := s.db.Where("group_id = ?", mg.GroupID).First(&group).Error; err != nil {
			continue
		}

		var lastMsg model.Message
		s.db.Where("group_id = ?", mg.GroupID).Order("created_at desc").First(&lastMsg)

		session := model.SessionInfo{
			Type:        "group",
			GroupID:     mg.GroupID,
			Nickname:    group.Name,
			LastMessage: lastMsg.Content,
			LastTime:    lastMsg.CreatedAt,
		}
		sessions = append(sessions, session)
	}

	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].LastTime.After(sessions[j].LastTime)
	})

	return sessions, nil
}

func generateToken() string {
	b := make([]byte, 32)
	rand.Read(b)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
