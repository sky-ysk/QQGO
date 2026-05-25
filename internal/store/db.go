package store

import (
	"github.com/qqgo/server/internal/model"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func InitDB(dsn string) (*gorm.DB, error) {
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		return nil, err
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
		return nil, err
	}

	return db, nil
}
