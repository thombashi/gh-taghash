package resolver

import (
	"log"
	"os"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const (
	whereExpired    = "expired_at < ?"
	whereNotExpired = "? <= expired_at"
)

type GitTag struct {
	gorm.Model
	ID        uint
	RepoID    string
	Tag       string
	BaseTag   string
	Hash      string
	ExpiredAt time.Time
}

var gormLogger = logger.New(
	log.New(os.Stdout, "\n", log.LstdFlags), // io writer
	logger.Config{
		IgnoreRecordNotFoundError: true, // Ignore ErrRecordNotFound error for logger
	},
)
