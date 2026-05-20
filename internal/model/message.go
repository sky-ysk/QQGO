package model

import "time"

type MessageType int32

const (
	MsgTypeText  MessageType = 1
	MsgTypeImage MessageType = 2
	MsgTypeFile  MessageType = 3
	MsgTypeAudio MessageType = 4
	MsgTypeVideo MessageType = 5

	MsgTypeHeartbeat        MessageType = 100
	MsgTypeLogin            MessageType = 101
	MsgTypeLoginAck         MessageType = 102
	MsgTypeLogout           MessageType = 103
	MsgTypeStatusChange     MessageType = 104
	MsgTypeRegister         MessageType = 105
	MsgTypeRegisterAck      MessageType = 106
	MsgTypeServerAck        MessageType = 107
	MsgTypeDelivered        MessageType = 108
	MsgTypeFriendRequest    MessageType = 300
	MsgTypeFriendAccept     MessageType = 301
	MsgTypeFriendReject     MessageType = 302
	MsgTypeFriendDelete     MessageType = 303
	MsgTypeFriendList       MessageType = 304
	MsgTypeFriendSearch     MessageType = 305
	MsgTypeFriendMoveGroup  MessageType = 306
	MsgTypeFriendRemark     MessageType = 307
	MsgTypeFriendGroups     MessageType = 308
	MsgTypeGroupCreate      MessageType = 200
	MsgTypeGroupJoin        MessageType = 201
	MsgTypeGroupLeave       MessageType = 202
)

const MaxFriends = 500

type Message struct {
	ID        int64       `gorm:"primaryKey;autoIncrement" json:"id"`
	ClientSeq int64       `gorm:"default:0" json:"client_seq,omitempty"`
	MsgType   MessageType `gorm:"not null;index" json:"msg_type"`
	FromUID   string      `gorm:"index;not null" json:"from_uid"`
	ToUID     string      `gorm:"index;not null" json:"to_uid"`
	GroupID   string      `gorm:"index" json:"group_id,omitempty"`
	Content   string      `gorm:"not null" json:"content"`
	Delivered bool        `gorm:"default:false" json:"delivered"`
	CreatedAt time.Time   `gorm:"index" json:"created_at"`
	UpdatedAt time.Time   `json:"-"`
}

type LoginRequest struct {
	UID      string `json:"uid"`
	Password string `json:"password,omitempty"`
	Token    string `json:"token,omitempty"`
	Platform string `json:"platform"`
}

type LoginResponse struct {
	Code     int    `json:"code"`
	Message  string `json:"message"`
	Token    string `json:"token,omitempty"`
	Online   int    `json:"online"`
	QQNumber int64  `json:"qq_number,omitempty"`
}

type RegisterRequest struct {
	UID      string `json:"uid"`
	Password string `json:"password"`
	Nickname string `json:"nickname"`
}

type RegisterResponse struct {
	Code     int    `json:"code"`
	Message  string `json:"message"`
	QQNumber int64  `json:"qq_number,omitempty"`
}

type AckRequest struct {
	MessageID int64 `json:"message_id"`
}

type FriendRequestPayload struct {
	ToQQNumber int64  `json:"to_qq_number"`
	Message    string `json:"message,omitempty"`
}

type FriendListResponse struct {
	Friends []FriendInfo `json:"friends"`
}

type FriendInfo struct {
	QQNumber  int64  `json:"qq_number"`
	UID       string `json:"uid"`
	Nickname  string `json:"nickname"`
	Remark    string `json:"remark,omitempty"`
	GroupName string `json:"group_name"`
	Status    int    `json:"status"`
	Online    bool   `json:"online"`
}

type UserSearchResult struct {
	QQNumber int64  `json:"qq_number"`
	UID      string `json:"uid"`
	Nickname string `json:"nickname"`
	Online   bool   `json:"online"`
}

type FriendGroupListResponse struct {
	Groups []string `json:"groups"`
}
