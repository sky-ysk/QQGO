package store

import (
	"fmt"

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
		&model.Blacklist{},
	); err != nil {
		return nil, err
	}

	return db, nil
}

func InitFTS(db *gorm.DB) error {
	createFTS := `CREATE VIRTUAL TABLE IF NOT EXISTS messages_fts USING fts5(
		content,
		tokenize='unicode61'
	)`
	if err := db.Exec(createFTS).Error; err != nil {
		return fmt.Errorf("create fts table: %w", err)
	}

	if err := db.Exec(`CREATE TRIGGER IF NOT EXISTS messages_fts_ai AFTER INSERT ON messages BEGIN
		INSERT INTO messages_fts(rowid, content) VALUES (new.id, new.content);
	END`).Error; err != nil {
		return fmt.Errorf("create insert trigger: %w", err)
	}

	if err := db.Exec(`CREATE TRIGGER IF NOT EXISTS messages_fts_au AFTER UPDATE OF content ON messages BEGIN
		UPDATE messages_fts SET content = new.content WHERE rowid = new.id;
	END`).Error; err != nil {
		return fmt.Errorf("create update trigger: %w", err)
	}

	if err := db.Exec(`CREATE TRIGGER IF NOT EXISTS messages_fts_ad AFTER DELETE ON messages BEGIN
		DELETE FROM messages_fts WHERE rowid = old.id;
	END`).Error; err != nil {
		return fmt.Errorf("create delete trigger: %w", err)
	}

	if err := db.Exec(`INSERT INTO messages_fts(messages_fts) VALUES('rebuild')`).Error; err != nil {
		return fmt.Errorf("rebuild fts index: %w", err)
	}

	return nil
}
