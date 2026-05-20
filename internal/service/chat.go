package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log"
	"strconv"

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
)

type ChatService struct {
	db *gorm.DB
}

func NewChatService(db *gorm.DB) *ChatService {
	return &ChatService{db: db}
}

func (s *ChatService) Register(uid, password, nickname string) (int64, error) {
	var existing model.User
	if err := s.db.Where("uid = ?", uid).First(&existing).Error; err == nil {
		return 0, ErrUserExists
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return 0, err
	}

	token := generateToken()

	user := &model.User{
		UID:          uid,
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

func (s *ChatService) Login(uid, password string) (string, int64, error) {
	var user model.User
	if err := s.db.Where("uid = ?", uid).First(&user).Error; err != nil {
		return "", 0, ErrUserNotFound
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return "", 0, ErrAuthFailed
	}

	token := generateToken()
	s.db.Model(&user).Update("token", token)

	return token, user.QQNumber, nil
}

func (s *ChatService) LoginWithToken(uid, token string) (bool, error) {
	var user model.User
	if err := s.db.Where("uid = ? AND token = ?", uid, token).First(&user).Error; err != nil {
		return false, nil
	}
	return true, nil
}

func (s *ChatService) ValidateToken(uid, token string) (bool, error) {
	return s.LoginWithToken(uid, token)
}

func (s *ChatService) HandleMessage(ctx context.Context, msg *model.Message) error {
	result := s.db.Create(msg)
	if result.Error != nil {
		return result.Error
	}
	log.Printf("[store] saved message id=%d from=%s to=%s", msg.ID, msg.FromUID, msg.ToUID)
	return nil
}

func (s *ChatService) GetOfflineMessages(uid string) ([]*model.Message, error) {
	var msgs []*model.Message
	err := s.db.
		Where("to_uid = ? AND delivered = ?", uid, false).
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

func (s *ChatService) GetHistory(ctx context.Context, uid string, limit int) ([]*model.Message, error) {
	var msgs []*model.Message
	err := s.db.
		Where("(from_uid = ? OR to_uid = ?) AND group_id = ''", uid, uid).
		Order("id desc").
		Limit(limit).
		Find(&msgs).Error
	return msgs, err
}

func (s *ChatService) GetUserByQQNumber(qqNumber int64) (*model.User, error) {
	var user model.User
	err := s.db.Where("qq_number = ?", qqNumber).First(&user).Error
	return &user, err
}

func (s *ChatService) GetUserByUID(uid string) (*model.User, error) {
	var user model.User
	err := s.db.Where("uid = ?", uid).First(&user).Error
	return &user, err
}

func (s *ChatService) SendFriendRequest(fromUID string, toQQNumber int64, message string) error {
	toUser, err := s.GetUserByQQNumber(toQQNumber)
	if err != nil {
		return ErrUserNotFound
	}

	var self model.User
	if err := s.db.Where("uid = ?", fromUID).First(&self).Error; err != nil {
		return ErrUserNotFound
	}

	if self.QQNumber == toQQNumber {
		return errors.New("cannot add yourself")
	}

	var count int64
	s.db.Model(&model.Friend{}).
		Where("uid = ? AND status = ?", fromUID, model.FriendStatusAccepted).
		Count(&count)
	if count >= model.MaxFriends {
		return ErrFriendLimit
	}

	var existing model.Friend
	err = s.db.Where(
		"(uid = ? AND friend_uid = ?) OR (uid = ? AND friend_uid = ?)",
		fromUID, toUser.UID, toUser.UID, fromUID,
	).First(&existing).Error
	if err == nil {
		return ErrAlreadyFriend
	}

	f := &model.Friend{
		UID:       fromUID,
		FriendUID: toUser.UID,
		Status:    model.FriendStatusPending,
	}
	return s.db.Create(f).Error
}

func (s *ChatService) AcceptFriend(uid string, fromQQNumber int64) error {
	fromUser, err := s.GetUserByQQNumber(fromQQNumber)
	if err != nil {
		return ErrUserNotFound
	}

	result := s.db.Model(&model.Friend{}).
		Where("uid = ? AND friend_uid = ? AND status = ?", fromUser.UID, uid, model.FriendStatusPending).
		Update("status", model.FriendStatusAccepted)
	if result.RowsAffected == 0 {
		return ErrNotFriend
	}

	var selfCount int64
	s.db.Model(&model.Friend{}).
		Where("uid = ? AND status = ?", uid, model.FriendStatusAccepted).
		Count(&selfCount)
	if selfCount >= model.MaxFriends {
		s.db.Model(&model.Friend{}).
			Where("uid = ? AND friend_uid = ?", fromUser.UID, uid).
			Update("status", model.FriendStatusPending)
		return ErrFriendLimit
	}

	var targetCount int64
	s.db.Model(&model.Friend{}).
		Where("uid = ? AND status = ?", fromUser.UID, model.FriendStatusAccepted).
		Count(&targetCount)
	if targetCount >= model.MaxFriends {
		s.db.Model(&model.Friend{}).
			Where("uid = ? AND friend_uid = ?", fromUser.UID, uid).
			Update("status", model.FriendStatusPending)
		return ErrFriendLimit
	}

	reverse := &model.Friend{
		UID:       uid,
		FriendUID: fromUser.UID,
		Status:    model.FriendStatusAccepted,
	}
	s.db.Create(reverse)

	return nil
}

func (s *ChatService) RejectFriend(uid string, fromQQNumber int64) error {
	fromUser, err := s.GetUserByQQNumber(fromQQNumber)
	if err != nil {
		return ErrUserNotFound
	}

	result := s.db.Model(&model.Friend{}).
		Where("uid = ? AND friend_uid = ? AND status = ?", fromUser.UID, uid, model.FriendStatusPending).
		Update("status", model.FriendStatusRejected)
	if result.RowsAffected == 0 {
		return ErrNotFriend
	}
	return nil
}

func (s *ChatService) DeleteFriend(uid string, friendQQNumber int64) error {
	friendUser, err := s.GetUserByQQNumber(friendQQNumber)
	if err != nil {
		return ErrUserNotFound
	}

	s.db.Where("uid = ? AND friend_uid = ? AND status = ?", uid, friendUser.UID, model.FriendStatusAccepted).Delete(&model.Friend{})
	s.db.Where("uid = ? AND friend_uid = ? AND status = ?", friendUser.UID, uid, model.FriendStatusAccepted).Delete(&model.Friend{})

	return nil
}

func (s *ChatService) GetFriendList(uid string, onlineFunc func(string) bool) ([]model.FriendInfo, error) {
	var friends []model.Friend
	err := s.db.Where("uid = ? AND status = ?", uid, model.FriendStatusAccepted).Find(&friends).Error
	if err != nil {
		return nil, err
	}

	var pending []model.Friend
	s.db.Where("friend_uid = ? AND status = ?", uid, model.FriendStatusPending).Find(&pending)

	result := make([]model.FriendInfo, 0, len(friends)+len(pending))

	for _, f := range pending {
		reqUser, err := s.GetUserByUID(f.UID)
		if err != nil {
			continue
		}
		result = append(result, model.FriendInfo{
			QQNumber:  reqUser.QQNumber,
			UID:       reqUser.UID,
			Nickname:  reqUser.Nickname,
			Remark:    f.Remark,
			GroupName: "待处理",
			Status:    model.FriendStatusPending,
			Online:    onlineFunc(reqUser.UID),
		})
	}

	for _, f := range friends {
		friendUser, err := s.GetUserByUID(f.FriendUID)
		if err != nil {
			continue
		}
		groupName := f.GroupName
		if groupName == "" {
			groupName = "我的好友"
		}
		result = append(result, model.FriendInfo{
			QQNumber:  friendUser.QQNumber,
			UID:       friendUser.UID,
			Nickname:  friendUser.Nickname,
			Remark:    f.Remark,
			GroupName: groupName,
			Status:    f.Status,
			Online:    onlineFunc(friendUser.UID),
		})
	}

	return result, nil
}

func (s *ChatService) SearchUsers(keyword string, onlineFunc func(string) bool) ([]model.UserSearchResult, error) {
	var users []model.User

	if qqNum, err := strconv.ParseInt(keyword, 10, 64); err == nil {
		s.db.Where("qq_number = ? OR nickname LIKE ? OR uid LIKE ?",
			qqNum, "%"+keyword+"%", "%"+keyword+"%").
			Limit(20).Find(&users)
	} else {
		s.db.Where("nickname LIKE ? OR uid LIKE ?",
			"%"+keyword+"%", "%"+keyword+"%").
			Limit(20).Find(&users)
	}

	results := make([]model.UserSearchResult, 0, len(users))
	for _, u := range users {
		results = append(results, model.UserSearchResult{
			QQNumber: u.QQNumber,
			UID:      u.UID,
			Nickname: u.Nickname,
			Online:   onlineFunc(u.UID),
		})
	}
	return results, nil
}

func (s *ChatService) MoveFriendGroup(uid string, friendQQNumber int64, groupName string) error {
	friendUser, err := s.GetUserByQQNumber(friendQQNumber)
	if err != nil {
		return ErrUserNotFound
	}

	result := s.db.Model(&model.Friend{}).
		Where("uid = ? AND friend_uid = ? AND status = ?", uid, friendUser.UID, model.FriendStatusAccepted).
		Update("group_name", groupName)
	if result.RowsAffected == 0 {
		return ErrNotFriend
	}
	return nil
}

func (s *ChatService) GetFriendGroups(uid string) ([]string, error) {
	var groups []string
	err := s.db.Model(&model.Friend{}).
		Where("uid = ? AND status = ?", uid, model.FriendStatusAccepted).
		Distinct("group_name").
		Pluck("group_name", &groups).Error
	if err != nil {
		return nil, err
	}

	hasDefault := false
	for _, g := range groups {
		if g == "我的好友" {
			hasDefault = true
			break
		}
	}
	if !hasDefault {
		groups = append([]string{"我的好友"}, groups...)
	}

	return groups, nil
}

func (s *ChatService) SetRemark(uid string, friendQQNumber int64, remark string) error {
	friendUser, err := s.GetUserByQQNumber(friendQQNumber)
	if err != nil {
		return ErrUserNotFound
	}

	result := s.db.Model(&model.Friend{}).
		Where("uid = ? AND friend_uid = ? AND status = ?", uid, friendUser.UID, model.FriendStatusAccepted).
		Update("remark", remark)
	if result.RowsAffected == 0 {
		return ErrNotFriend
	}
	return nil
}

func generateToken() string {
	b := make([]byte, 32)
	rand.Read(b)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
