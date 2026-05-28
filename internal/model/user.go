package model

import "time"

const QQNumberBase = 10000

type User struct {
	ID           uint      `gorm:"primaryKey" json:"-"`
	QQNumber     int64     `gorm:"uniqueIndex;not null" json:"qq_number"`
	PasswordHash string    `gorm:"size:128;not null" json:"-"`
	Token        string    `gorm:"size:128" json:"-"`
	RefreshToken string    `gorm:"size:512" json:"-"`
	Nickname     string    `gorm:"size:64" json:"nickname"`
	Avatar       string    `gorm:"size:256" json:"avatar"`
	Status       int       `gorm:"default:0" json:"status"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type Friend struct {
	ID        uint      `gorm:"primaryKey" json:"-"`
	QQ        int64     `gorm:"uniqueIndex:idx_qq_friend;not null" json:"qq"`
	FriendQQ  int64     `gorm:"uniqueIndex:idx_qq_friend;not null" json:"friend_qq"`
	Remark    string    `gorm:"size:64" json:"remark"`
	GroupName string    `gorm:"size:64;default:我的好友" json:"group_name"`
	Status    int       `gorm:"default:0;index" json:"status"`
	AddedAt   time.Time `gorm:"autoCreateTime" json:"added_at"`
}

const (
	FriendStatusPending  = 0
	FriendStatusAccepted = 1
	FriendStatusRejected = 2
)

type Group struct {
	ID         uint      `gorm:"primaryKey" json:"-"`
	GroupID    string    `gorm:"uniqueIndex;size:64;not null" json:"group_id"`
	Name       string    `gorm:"size:64" json:"name"`
	OwnerQQ    int64     `gorm:"not null" json:"owner_qq"`
	Avatar     string    `gorm:"size:256" json:"avatar"`
	MemberCnt  int       `gorm:"default:0" json:"member_cnt"`
	MaxMembers int       `gorm:"default:200" json:"max_members"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type GroupMember struct {
	ID       uint      `gorm:"primaryKey" json:"-"`
	GroupID  string    `gorm:"index;size:64;not null" json:"group_id"`
	QQ       int64     `gorm:"index;not null" json:"qq"`
	Role     int       `gorm:"default:0" json:"role"`
	JoinedAt time.Time `gorm:"autoCreateTime" json:"joined_at"`
}

type FriendGroup struct {
	ID        uint      `gorm:"primaryKey;autoIncrement" json:"-"`
	QQ        int64     `gorm:"uniqueIndex:idx_qq_group;not null" json:"qq"`
	GroupName string    `gorm:"uniqueIndex:idx_qq_group;size:64;not null" json:"group_name"`
	CreatedAt time.Time `json:"created_at"`
}

type MessageCount struct {
	ID       uint      `gorm:"primaryKey" json:"-"`
	FromQQ   int64     `gorm:"uniqueIndex:idx_from_to;not null" json:"from_qq"`
	ToQQ     int64     `gorm:"uniqueIndex:idx_from_to;not null" json:"to_qq"`
	Count    int       `gorm:"default:0" json:"count"`
	UpdatedAt time.Time `json:"updated_at"`
}

const MaxNonFriendMessages = 1

type Blacklist struct {
	ID         uint      `gorm:"primaryKey" json:"-"`
	QQ         int64     `gorm:"uniqueIndex:idx_qq_blocked;not null" json:"qq"`
	BlockedQQ  int64     `gorm:"uniqueIndex:idx_qq_blocked;not null" json:"blocked_qq"`
	CreatedAt  time.Time `json:"created_at"`
}
