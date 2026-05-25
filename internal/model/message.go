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
	MsgTypeFriendGroups      MessageType = 308
	MsgTypeFriendCreateGroup MessageType = 309
	MsgTypeFriendDeleteGroup MessageType = 310
	MsgTypeCheckUser         MessageType = 311
	MsgTypeHistory           MessageType = 312
	MsgTypeSessionList       MessageType = 313
	MsgTypeGroupCreate       MessageType = 200
	MsgTypeGroupJoin        MessageType = 201
	MsgTypeGroupLeave       MessageType = 202
	MsgTypeGroupList        MessageType = 203
	MsgTypeGroupInfo        MessageType = 204
)

const MaxFriends = 500

type Message struct {
	ID        int64       `gorm:"primaryKey;autoIncrement" json:"id"`
	ClientSeq int64       `gorm:"default:0" json:"client_seq,omitempty"`
	MsgType   MessageType `gorm:"not null;index" json:"msg_type"`
	FromQQ    int64       `gorm:"index;not null" json:"from_qq"`
	ToQQ      int64       `gorm:"index;not null" json:"to_qq"`
	GroupID   string      `gorm:"index" json:"group_id,omitempty"`
	Content   string      `gorm:"not null" json:"content"`
	Delivered bool        `gorm:"default:false" json:"delivered"`
	CreatedAt time.Time   `gorm:"index" json:"created_at"`
	UpdatedAt time.Time   `json:"-"`
}

type LoginRequest struct {
	QQ       int64  `json:"qq"`
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
	Nickname string `json:"nickname,omitempty"`
}

type RegisterRequest struct {
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
	Friends   []FriendInfo `json:"friends"`
	AllGroups []string     `json:"all_groups"`
}

type FriendInfo struct {
	QQNumber  int64  `json:"qq_number"`
	Nickname  string `json:"nickname"`
	Remark    string `json:"remark,omitempty"`
	GroupName string `json:"group_name"`
	Status    int    `json:"status"`
	Online    bool   `json:"online"`
}

type UserSearchResult struct {
	QQNumber int64  `json:"qq_number"`
	Nickname string `json:"nickname"`
	Online   bool   `json:"online"`
}

type FriendGroupListResponse struct {
	Groups []string `json:"groups"`
}

type CheckUserResponse struct {
	Code     int    `json:"code"`
	Message  string `json:"message"`
	QQNumber int64  `json:"qq_number,omitempty"`
	Nickname string `json:"nickname,omitempty"`
	Online   bool   `json:"online"`
}

type HistoryRequest struct {
	TargetQQ int64 `json:"target_qq"`
	Offset   int   `json:"offset"`
	Limit    int   `json:"limit"`
}

type HistoryMessage struct {
	ID        int64     `json:"id"`
	FromQQ    int64     `json:"from_qq"`
	ToQQ      int64     `json:"to_qq"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

type HistoryResponse struct {
	TargetQQ int64            `json:"target_qq"`
	Nickname string           `json:"nickname"`
	Messages []HistoryMessage `json:"messages"`
	Offset   int              `json:"offset"`
	HasMore  bool             `json:"has_more"`
}

type GroupCreateRequest struct {
	Name string `json:"name"`
}

type GroupInfo struct {
	GroupID   string `json:"group_id"`
	Name      string `json:"name"`
	OwnerQQ   int64  `json:"owner_qq"`
	MemberCnt int    `json:"member_cnt"`
}

type GroupListResponse struct {
	Groups []GroupInfo `json:"groups"`
}

type GroupMembersResponse struct {
	GroupID string         `json:"group_id"`
	Members []GroupMemberInfo `json:"members"`
}

type GroupMemberInfo struct {
	QQ       int64  `json:"qq"`
	Nickname string `json:"nickname"`
	Role     int    `json:"role"`
	Online   bool   `json:"online"`
}

type SessionInfo struct {
	Type         string    `json:"type"`
	TargetQQ     int64     `json:"target_qq,omitempty"`
	GroupID      string    `json:"group_id,omitempty"`
	Nickname     string    `json:"nickname"`
	LastMessage  string    `json:"last_message"`
	LastTime     time.Time `json:"last_time"`
	Online       bool      `json:"online,omitempty"`
	UnreadCount  int       `json:"unread_count,omitempty"`
}

type SessionListResponse struct {
	Sessions []SessionInfo `json:"sessions"`
}
