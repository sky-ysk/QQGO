package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log"
	"strconv"
	"strings"

	"github.com/qqgo/server/internal/model"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

var (
	ErrUserExists    = errors.New("user already exists")
	ErrUserNotFound  = errors.New("user not found")
	ErrAuthFailed    = errors.New("auth failed")
	ErrTokenInvalid  = errors.New("invalid token")
	ErrFriendLimit   = errors.New("friend limit reached")
	ErrAlreadyFriend = errors.New("already friend or pending")
	ErrNotFriend     = errors.New("not friend")
	ErrGroupNotFound = errors.New("group not found")
	ErrGroupNotEmpty = errors.New("group is not empty")
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

func (s *ChatService) GetGroupMembers(groupID string) ([]string, error) {
	return nil, errors.New("not implemented")
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

func generateToken() string {
	b := make([]byte, 32)
	rand.Read(b)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
