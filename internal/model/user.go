package model

import "time"

const QQNumberBase = 10000

type User struct {
	ID           uint      `gorm:"primaryKey" json:"-"`
	UID          string    `gorm:"uniqueIndex;size:64;not null" json:"uid"`
	QQNumber     int64     `gorm:"uniqueIndex;default:0" json:"qq_number"`
	PasswordHash string    `gorm:"size:128;not null" json:"-"`
	Token        string    `gorm:"size:128" json:"-"`
	Nickname     string    `gorm:"size:64" json:"nickname"`
	Avatar       string    `gorm:"size:256" json:"avatar"`
	Status       int       `gorm:"default:0" json:"status"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type Friend struct {
	ID        uint      `gorm:"primaryKey" json:"-"`
	UID       string    `gorm:"uniqueIndex:idx_uid_friend;size:64;not null" json:"uid"`
	FriendUID string    `gorm:"uniqueIndex:idx_uid_friend;size:64;not null" json:"friend_uid"`
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
	OwnerUID   string    `gorm:"size:64" json:"owner_uid"`
	Avatar     string    `gorm:"size:256" json:"avatar"`
	MemberCnt  int       `gorm:"default:0" json:"member_cnt"`
	MaxMembers int       `gorm:"default:200" json:"max_members"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type GroupMember struct {
	ID       uint      `gorm:"primaryKey" json:"-"`
	GroupID  string    `gorm:"index;size:64;not null" json:"group_id"`
	UID      string    `gorm:"index;size:64;not null" json:"uid"`
	Role     int       `gorm:"default:0" json:"role"`
	JoinedAt time.Time `gorm:"autoCreateTime" json:"joined_at"`
}
